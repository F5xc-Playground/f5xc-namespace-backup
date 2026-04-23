package revert

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/F5xc-Playground/f5xc-namespace-backup/internal/client"
	"github.com/F5xc-Playground/f5xc-namespace-backup/internal/diff"
	"github.com/F5xc-Playground/f5xc-namespace-backup/internal/registry"
	"github.com/F5xc-Playground/f5xc-namespace-backup/internal/sanitize"
)

// Options configures a revert operation.
type Options struct {
	BackupDir       string
	TargetNamespace string
	Resources       []registry.Resource
	DryRun          bool
	DeleteExtra     bool // opt-in: delete objects added since backup
}

// Result holds the outcome of a revert operation.
type Result struct {
	Replaced int
	Created  int
	Deleted  int
	Skipped  int
	Failed   int
	Errors   []string
	Warnings []string
}

// Run computes the drift, then pushes backup state back for drifted objects.
func Run(c *client.Client, opts *Options) (*Result, *diff.DriftReport, error) {
	result := &Result{}

	// Run diff first
	report, err := diff.Run(c, &diff.Options{
		BackupDir: opts.BackupDir,
		Namespace: opts.TargetNamespace,
		Resources: opts.Resources,
	})
	if err != nil {
		return result, report, err
	}

	result.Warnings = report.Warnings

	byKind := registry.ByKind(opts.Resources)

	// Unchanged → skip
	result.Skipped = report.Unchanged

	// Modified → Replace (parallel, no tier ordering needed)
	if len(report.Modified) > 0 {
		var mu sync.Mutex
		var wg sync.WaitGroup

		for _, mod := range report.Modified {
			wg.Add(1)
			go func(mod diff.ObjectDiff) {
				defer wg.Done()

				res, ok := byKind[mod.Kind]
				if !ok {
					mu.Lock()
					result.Errors = append(result.Errors, fmt.Sprintf("unknown kind %s for %s", mod.Kind, mod.Name))
					result.Failed++
					mu.Unlock()
					return
				}

				listPath := strings.ReplaceAll(res.ListPath, "{namespace}", opts.TargetNamespace)
				getPath := listPath + "/" + mod.Name

				if opts.DryRun {
					slog.Info("dry run: would replace", "kind", mod.Kind, "name", mod.Name)
					mu.Lock()
					result.Replaced++
					mu.Unlock()
					return
				}

				restored := sanitize.ForRestore(mod.BackupObj, opts.TargetNamespace)
				if err := c.Replace(getPath, restored); err != nil {
					mu.Lock()
					result.Errors = append(result.Errors, fmt.Sprintf("replace %s/%s: %v", mod.Kind, mod.Name, err))
					result.Failed++
					mu.Unlock()
					return
				}

				mu.Lock()
				result.Replaced++
				mu.Unlock()
				slog.Info("replaced", "kind", mod.Kind, "name", mod.Name)
			}(mod)
		}
		wg.Wait()
	}

	// Removed from live → Create from backup (tier-ordered, lowest first)
	if len(report.Removed) > 0 {
		removedByKind := make(map[string][]diff.ObjectRef)
		for _, ref := range report.Removed {
			removedByKind[ref.Kind] = append(removedByKind[ref.Kind], ref)
		}

		var removedResources []registry.Resource
		for kind := range removedByKind {
			if res, ok := byKind[kind]; ok {
				removedResources = append(removedResources, res)
			}
		}
		tiers := registry.Tiers(removedResources)

		for _, tier := range tiers {
			tierResources := registry.FilterByTier(removedResources, tier)

			var mu sync.Mutex
			var wg sync.WaitGroup

			for _, res := range tierResources {
				refs := removedByKind[res.Kind]
				for _, ref := range refs {
					wg.Add(1)
					go func(res registry.Resource, ref diff.ObjectRef) {
						defer wg.Done()

						listPath := strings.ReplaceAll(res.ListPath, "{namespace}", opts.TargetNamespace)

						if opts.DryRun {
							slog.Info("dry run: would create", "kind", ref.Kind, "name", ref.Name)
							mu.Lock()
							result.Created++
							mu.Unlock()
							return
						}

						backupObj := loadBackupObject(opts.BackupDir, ref.Kind, ref.Name)
						if backupObj == nil {
							mu.Lock()
							result.Errors = append(result.Errors, fmt.Sprintf("cannot read backup for %s/%s", ref.Kind, ref.Name))
							result.Failed++
							mu.Unlock()
							return
						}

						restored := sanitize.ForRestore(backupObj, opts.TargetNamespace)
						if err := c.Create(listPath, restored); err != nil {
							mu.Lock()
							result.Errors = append(result.Errors, fmt.Sprintf("create %s/%s: %v", ref.Kind, ref.Name, err))
							result.Failed++
							mu.Unlock()
							return
						}

						mu.Lock()
						result.Created++
						mu.Unlock()
						slog.Info("created", "kind", ref.Kind, "name", ref.Name)
					}(res, ref)
				}
			}
			wg.Wait()
		}
	}

	// Added to live → Delete if --delete-extra, else warn (reverse tier order)
	if len(report.Added) > 0 {
		if opts.DeleteExtra {
			addedByKind := make(map[string][]diff.ObjectRef)
			for _, ref := range report.Added {
				addedByKind[ref.Kind] = append(addedByKind[ref.Kind], ref)
			}

			var addedResources []registry.Resource
			for kind := range addedByKind {
				if res, ok := byKind[kind]; ok {
					addedResources = append(addedResources, res)
				}
			}
			tiers := registry.Tiers(addedResources)

			// Reverse tier order for deletion (highest first)
			sort.Sort(sort.Reverse(sort.IntSlice(tiers)))

			for _, tier := range tiers {
				tierResources := registry.FilterByTier(addedResources, tier)

				var mu sync.Mutex
				var wg sync.WaitGroup

				for _, res := range tierResources {
					refs := addedByKind[res.Kind]
					for _, ref := range refs {
						wg.Add(1)
						go func(res registry.Resource, ref diff.ObjectRef) {
							defer wg.Done()

							listPath := strings.ReplaceAll(res.ListPath, "{namespace}", opts.TargetNamespace)
							deletePath := listPath + "/" + ref.Name

							if opts.DryRun {
								slog.Info("dry run: would delete", "kind", ref.Kind, "name", ref.Name)
								mu.Lock()
								result.Deleted++
								mu.Unlock()
								return
							}

							if err := c.Delete(deletePath); err != nil {
								mu.Lock()
								result.Errors = append(result.Errors, fmt.Sprintf("delete %s/%s: %v", ref.Kind, ref.Name, err))
								result.Failed++
								mu.Unlock()
								return
							}

							mu.Lock()
							result.Deleted++
							mu.Unlock()
							slog.Info("deleted", "kind", ref.Kind, "name", ref.Name)
						}(res, ref)
					}
				}
				wg.Wait()
			}
		} else {
			for _, ref := range report.Added {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("%s/%s exists in live but not in backup (use --delete-extra to remove)", ref.Kind, ref.Name))
			}
		}
	}

	return result, report, nil
}

// loadBackupObject reads a single object from the backup directory.
func loadBackupObject(backupDir, kind, name string) map[string]any {
	path := filepath.Join(backupDir, kind, name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil
	}
	return obj
}

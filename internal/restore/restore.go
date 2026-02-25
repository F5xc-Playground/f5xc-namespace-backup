package restore

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/kevingstewart/xcbackup/internal/client"
	"github.com/kevingstewart/xcbackup/internal/registry"
	"github.com/kevingstewart/xcbackup/internal/sanitize"
)

type Options struct {
	BackupDir       string
	TargetNamespace string
	Resources       []registry.Resource
	DryRun          bool
	OnConflict      string // "skip", "overwrite", "fail"
}

type Result struct {
	Created    int
	Skipped    int
	Updated    int
	Failed     int
	Errors     []string
	Warnings   []string
	AuthFailed bool
}

func Run(c *client.Client, opts *Options) (*Result, error) {
	result := &Result{}

	// Filter to standalone resources that have backup files
	var resources []registry.Resource
	for _, r := range opts.Resources {
		if r.ManagedBy != "" {
			continue
		}
		typeDir := filepath.Join(opts.BackupDir, r.Kind)
		if _, err := os.Stat(typeDir); err == nil {
			resources = append(resources, r)
		}
	}

	// Restore tier by tier
	tiers := registry.Tiers(resources)

	for _, tier := range tiers {
		tierResources := registry.FilterByTier(resources, tier)
		slog.Info("restoring tier", "tier", tier, "resource_types", len(tierResources))

		var mu sync.Mutex
		var wg sync.WaitGroup

		for _, res := range tierResources {
			wg.Add(1)
			go func(res registry.Resource) {
				defer wg.Done()

				typeDir := filepath.Join(opts.BackupDir, res.Kind)
				entries, err := os.ReadDir(typeDir)
				if err != nil {
					mu.Lock()
					result.Errors = append(result.Errors, fmt.Sprintf("read dir %s: %v", res.Kind, err))
					result.Failed++
					mu.Unlock()
					return
				}

				for _, entry := range entries {
					if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
						continue
					}

					data, err := os.ReadFile(filepath.Join(typeDir, entry.Name()))
					if err != nil {
						mu.Lock()
						result.Errors = append(result.Errors, fmt.Sprintf("read %s/%s: %v", res.Kind, entry.Name(), err))
						result.Failed++
						mu.Unlock()
						continue
					}

					var obj map[string]any
					if err := json.Unmarshal(data, &obj); err != nil {
						mu.Lock()
						result.Errors = append(result.Errors, fmt.Sprintf("parse %s/%s: %v", res.Kind, entry.Name(), err))
						result.Failed++
						mu.Unlock()
						continue
					}

					md, _ := obj["metadata"].(map[string]any)
					name, _ := md["name"].(string)

					if opts.DryRun {
						slog.Info("dry run: would create", "kind", res.Kind, "name", name)
						continue
					}

					restored := sanitize.ForRestore(obj, opts.TargetNamespace)
					listPath := strings.ReplaceAll(res.ListPath, "{namespace}", opts.TargetNamespace)
					getPath := listPath + "/" + name

					// Check if object exists
					_, err = c.Get(getPath)
					exists := err == nil

					if exists {
						switch opts.OnConflict {
						case "skip":
							slog.Info("skipping existing", "kind", res.Kind, "name", name)
							mu.Lock()
							result.Skipped++
							mu.Unlock()
							continue
						case "overwrite":
							if err := c.Replace(getPath, restored); err != nil {
								mu.Lock()
								result.Errors = append(result.Errors, fmt.Sprintf("Failed to replace %s/%s: %v", res.Kind, name, err))
								result.Failed++
								mu.Unlock()
								continue
							}
							mu.Lock()
							result.Updated++
							mu.Unlock()
							slog.Info("updated", "kind", res.Kind, "name", name)
							continue
						case "fail":
							mu.Lock()
							result.Errors = append(result.Errors, fmt.Sprintf("%s/%s already exists", res.Kind, name))
							result.Failed++
							mu.Unlock()
							continue
						}
					}

					if err := c.Create(listPath, restored); err != nil {
						var apiErr *client.APIError
						if errors.As(err, &apiErr) {
							if apiErr.StatusCode == 401 {
								mu.Lock()
								result.Errors = append(result.Errors, fmt.Sprintf("Failed to create %s/%s: %v", res.Kind, name, err))
								result.AuthFailed = true
								result.Failed++
								mu.Unlock()
								return
							}
						}
						mu.Lock()
						result.Errors = append(result.Errors, fmt.Sprintf("Failed to create %s/%s: %v", res.Kind, name, err))
						result.Failed++
						mu.Unlock()
						continue
					}

					mu.Lock()
					result.Created++
					mu.Unlock()
					slog.Info("created", "kind", res.Kind, "name", name)
				}
			}(res)
		}

		wg.Wait()

		if result.AuthFailed {
			return result, fmt.Errorf("authentication failed — check your API token or certificate")
		}
	}

	return result, nil
}

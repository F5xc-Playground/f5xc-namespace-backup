package diff

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"github.com/kevingstewart/xcbackup/internal/client"
	"github.com/kevingstewart/xcbackup/internal/registry"
	"github.com/kevingstewart/xcbackup/internal/sanitize"
)

// ObjectRef identifies an object by kind and name.
type ObjectRef struct {
	Kind string
	Name string
}

// ObjectDiff describes a modified object with both versions and a unified diff.
type ObjectDiff struct {
	Kind        string
	Name        string
	BackupObj   map[string]any
	LiveObj     map[string]any
	UnifiedDiff string
}

// DriftReport is the result of comparing a backup against live state.
type DriftReport struct {
	Added     []ObjectRef  // in live, not in backup
	Removed   []ObjectRef  // in backup, not in live
	Modified  []ObjectDiff // in both, spec differs
	Unchanged int
	Errors    []string
	Warnings  []string
}

// Options configures a diff run.
type Options struct {
	BackupDir string
	Namespace string
	Resources []registry.Resource
}

// Run compares backup objects against the live namespace state.
func Run(c *client.Client, opts *Options) (*DriftReport, error) {
	report := &DriftReport{}

	// Filter out view-managed resources
	var resources []registry.Resource
	for _, r := range opts.Resources {
		if r.ManagedBy != "" {
			continue
		}
		resources = append(resources, r)
	}

	// Load backup objects from disk
	backupObjects := make(map[string]map[string]map[string]any) // kind -> name -> obj
	for _, res := range resources {
		typeDir := filepath.Join(opts.BackupDir, res.Kind)
		entries, err := os.ReadDir(typeDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			report.Errors = append(report.Errors, fmt.Sprintf("read dir %s: %v", res.Kind, err))
			continue
		}

		kindMap := make(map[string]map[string]any)
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(typeDir, entry.Name()))
			if err != nil {
				report.Errors = append(report.Errors, fmt.Sprintf("read %s/%s: %v", res.Kind, entry.Name(), err))
				continue
			}
			var obj map[string]any
			if err := json.Unmarshal(data, &obj); err != nil {
				report.Errors = append(report.Errors, fmt.Sprintf("parse %s/%s: %v", res.Kind, entry.Name(), err))
				continue
			}
			md, _ := obj["metadata"].(map[string]any)
			name, _ := md["name"].(string)
			if name == "" {
				name = strings.TrimSuffix(entry.Name(), ".json")
			}
			kindMap[name] = obj
		}
		if len(kindMap) > 0 {
			backupObjects[res.Kind] = kindMap
		}
	}

	// Fetch live objects and compare
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, res := range resources {
		wg.Add(1)
		go func(res registry.Resource) {
			defer wg.Done()

			listPath := strings.ReplaceAll(res.ListPath, "{namespace}", opts.Namespace)
			items, err := c.List(listPath)
			if err != nil {
				var apiErr *client.APIError
				if errors.As(err, &apiErr) {
					if apiErr.StatusCode == 401 {
						mu.Lock()
						report.Errors = append(report.Errors, fmt.Sprintf("Failed to list %s: %v", res.Kind, err))
						mu.Unlock()
						return
					}
					if apiErr.StatusCode == 403 || apiErr.StatusCode == 404 {
						mu.Lock()
						report.Warnings = append(report.Warnings, fmt.Sprintf("skipped %s: not accessible (may require subscription)", res.Kind))
						mu.Unlock()
						return
					}
					if apiErr.StatusCode == 501 {
						mu.Lock()
						report.Warnings = append(report.Warnings, fmt.Sprintf("skipped %s: not implemented", res.Kind))
						mu.Unlock()
						return
					}
				}
				mu.Lock()
				report.Errors = append(report.Errors, fmt.Sprintf("Failed to list %s: %v", res.Kind, err))
				mu.Unlock()
				return
			}

			// Build set of live objects for this kind
			liveObjects := make(map[string]map[string]any)
			for _, item := range items {
				name, _ := item["name"].(string)
				if name == "" {
					if md, ok := item["metadata"].(map[string]any); ok {
						name, _ = md["name"].(string)
					}
				}
				if name == "" {
					continue
				}

				// Filter out objects from other namespaces
				if itemNS, ok := item["namespace"].(string); ok && itemNS != "" && itemNS != opts.Namespace {
					slog.Debug("skipping object from different namespace", "kind", res.Kind, "name", name)
					continue
				}

				getPath := listPath + "/" + name
				obj, err := c.Get(getPath)
				if err != nil {
					mu.Lock()
					report.Errors = append(report.Errors, fmt.Sprintf("get %s/%s: %v", res.Kind, name, err))
					mu.Unlock()
					continue
				}

				liveObjects[name] = sanitize.ForBackup(obj)
			}

			mu.Lock()
			defer mu.Unlock()

			backupKind := backupObjects[res.Kind]

			// Check for objects in live but not in backup (Added)
			for name := range liveObjects {
				if backupKind == nil || backupKind[name] == nil {
					report.Added = append(report.Added, ObjectRef{Kind: res.Kind, Name: name})
				}
			}

			// Check for objects in backup but not in live (Removed), and Modified/Unchanged
			for name, backupObj := range backupKind {
				liveObj, exists := liveObjects[name]
				if !exists {
					report.Removed = append(report.Removed, ObjectRef{Kind: res.Kind, Name: name})
					continue
				}

				if reflect.DeepEqual(backupObj, liveObj) {
					report.Unchanged++
				} else {
					ud := unifiedDiff(
						fmt.Sprintf("backup/%s/%s.json", res.Kind, name),
						fmt.Sprintf("live/%s/%s", res.Kind, name),
						backupObj, liveObj,
					)
					report.Modified = append(report.Modified, ObjectDiff{
						Kind:        res.Kind,
						Name:        name,
						BackupObj:   backupObj,
						LiveObj:     liveObj,
						UnifiedDiff: ud,
					})
				}
			}
		}(res)
	}

	wg.Wait()
	return report, nil
}

// unifiedDiff produces a simple unified diff between two JSON objects.
func unifiedDiff(labelA, labelB string, a, b map[string]any) string {
	aJSON, _ := json.MarshalIndent(a, "", "  ")
	bJSON, _ := json.MarshalIndent(b, "", "  ")

	aLines := strings.Split(string(aJSON), "\n")
	bLines := strings.Split(string(bJSON), "\n")

	return lineDiff(labelA, labelB, aLines, bLines)
}

// lineDiff produces a unified-style diff between two slices of lines.
func lineDiff(labelA, labelB string, a, b []string) string {
	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("--- %s\n", labelA))
	buf.WriteString(fmt.Sprintf("+++ %s\n", labelB))

	// Simple LCS-based diff
	m, n := len(a), len(b)
	// Build LCS table
	lcs := make([][]int, m+1)
	for i := range lcs {
		lcs[i] = make([]int, n+1)
	}
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}

	// Walk the LCS table to produce diff lines
	i, j := 0, 0
	for i < m || j < n {
		if i < m && j < n && a[i] == b[j] {
			buf.WriteString(fmt.Sprintf(" %s\n", a[i]))
			i++
			j++
		} else if j < n && (i >= m || lcs[i][j+1] >= lcs[i+1][j]) {
			buf.WriteString(fmt.Sprintf("+%s\n", b[j]))
			j++
		} else {
			buf.WriteString(fmt.Sprintf("-%s\n", a[i]))
			i++
		}
	}

	return buf.String()
}

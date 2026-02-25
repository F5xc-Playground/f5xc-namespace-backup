package backup

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kevingstewart/xcbackup/internal/client"
	"github.com/kevingstewart/xcbackup/internal/manifest"
	"github.com/kevingstewart/xcbackup/internal/refs"
	"github.com/kevingstewart/xcbackup/internal/registry"
	"github.com/kevingstewart/xcbackup/internal/sanitize"
)

type Options struct {
	Namespace string
	OutputDir string
	Resources []registry.Resource
}

type Result struct {
	ObjectCount     int
	ResourceCounts  map[string]int
	SharedRefs      []refs.SharedRef
	SkippedChildren []string
	Warnings        []string
	Errors          []string
}

func Run(c *client.Client, opts *Options) (*Result, error) {
	result := &Result{
		ResourceCounts: make(map[string]int),
	}

	// Filter out view-managed resources (smart mode)
	var resources []registry.Resource
	for _, r := range opts.Resources {
		if r.ManagedBy != "" {
			slog.Info("skipping view-managed resource", "kind", r.Kind, "managed_by", r.ManagedBy)
			continue
		}
		resources = append(resources, r)
	}

	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, res := range resources {
		wg.Add(1)
		go func(res registry.Resource) {
			defer wg.Done()

			listPath := strings.ReplaceAll(res.ListPath, "{namespace}", opts.Namespace)
			slog.Info("listing resources", "kind", res.Kind, "path", listPath)

			items, err := c.List(listPath)
			if err != nil {
				slog.Warn("failed to list", "kind", res.Kind, "error", err)
				mu.Lock()
				result.Errors = append(result.Errors, fmt.Sprintf("list %s: %v", res.Kind, err))
				mu.Unlock()
				return
			}

			if len(items) == 0 {
				return
			}

			typeDir := filepath.Join(opts.OutputDir, res.Kind)
			if err := os.MkdirAll(typeDir, 0755); err != nil {
				mu.Lock()
				result.Errors = append(result.Errors, fmt.Sprintf("mkdir %s: %v", res.Kind, err))
				mu.Unlock()
				return
			}

			for _, item := range items {
				md, _ := item["metadata"].(map[string]any)
				name, _ := md["name"].(string)
				if name == "" {
					continue
				}

				getPath := listPath + "/" + name
				obj, err := c.Get(getPath)
				if err != nil {
					slog.Warn("failed to get", "kind", res.Kind, "name", name, "error", err)
					mu.Lock()
					result.Errors = append(result.Errors, fmt.Sprintf("get %s/%s: %v", res.Kind, name, err))
					mu.Unlock()
					continue
				}

				sharedRefs := refs.FindSharedRefs(res.Kind, name, obj)
				clean := sanitize.ForBackup(obj)

				data, err := json.MarshalIndent(clean, "", "  ")
				if err != nil {
					mu.Lock()
					result.Errors = append(result.Errors, fmt.Sprintf("marshal %s/%s: %v", res.Kind, name, err))
					mu.Unlock()
					continue
				}

				filePath := filepath.Join(typeDir, name+".json")
				if err := os.WriteFile(filePath, data, 0644); err != nil {
					mu.Lock()
					result.Errors = append(result.Errors, fmt.Sprintf("write %s/%s: %v", res.Kind, name, err))
					mu.Unlock()
					continue
				}

				mu.Lock()
				result.ObjectCount++
				result.ResourceCounts[res.Kind]++
				result.SharedRefs = append(result.SharedRefs, sharedRefs...)
				for _, ref := range sharedRefs {
					result.Warnings = append(result.Warnings,
						fmt.Sprintf("%s/%s references shared/%s (%s)", ref.SourceKind, ref.SourceName, ref.TargetName, ref.FieldPath))
				}
				mu.Unlock()

				slog.Debug("backed up", "kind", res.Kind, "name", name)
			}
		}(res)
	}

	wg.Wait()

	m := &manifest.Manifest{
		Version:             "1",
		ToolVersion:         "0.1.0",
		Tenant:              c.BaseURL(),
		Namespace:           opts.Namespace,
		Timestamp:           time.Now().UTC().Format(time.RFC3339),
		ResourceCounts:      result.ResourceCounts,
		SkippedViewChildren: result.SkippedChildren,
		SharedReferences:    result.SharedRefs,
		Warnings:            result.Warnings,
		Errors:              result.Errors,
	}

	if err := manifest.Write(opts.OutputDir, m); err != nil {
		return result, fmt.Errorf("writing manifest: %w", err)
	}

	return result, nil
}

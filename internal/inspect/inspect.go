package inspect

import (
	"fmt"
	"io"
	"sort"

	"github.com/kevingstewart/xcbackup/internal/manifest"
)

// Run reads a backup directory and prints a summary.
func Run(dir string, w io.Writer) error {
	m, err := manifest.Read(dir)
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "Backup: %s\n", dir)
	fmt.Fprintf(w, "Tenant: %s\n", m.Tenant)
	fmt.Fprintf(w, "Namespace: %s\n", m.Namespace)
	fmt.Fprintf(w, "Timestamp: %s\n", m.Timestamp)
	fmt.Fprintf(w, "Tool Version: %s\n\n", m.ToolVersion)

	kinds := make([]string, 0, len(m.ResourceCounts))
	for k := range m.ResourceCounts {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)

	total := 0
	fmt.Fprintf(w, "Resources:\n")
	for _, k := range kinds {
		count := m.ResourceCounts[k]
		total += count
		fmt.Fprintf(w, "  %-35s %d\n", k, count)
	}
	fmt.Fprintf(w, "  %-35s --\n", "")
	fmt.Fprintf(w, "  %-35s %d\n\n", "Total", total)

	if len(m.SkippedViewChildren) > 0 {
		fmt.Fprintf(w, "Skipped view-managed children:\n")
		for _, s := range m.SkippedViewChildren {
			fmt.Fprintf(w, "  - %s\n", s)
		}
		fmt.Fprintln(w)
	}

	if len(m.Warnings) > 0 {
		fmt.Fprintf(w, "Warnings:\n")
		for _, w2 := range m.Warnings {
			fmt.Fprintf(w, "  ! %s\n", w2)
		}
		fmt.Fprintln(w)
	}

	if len(m.Errors) > 0 {
		fmt.Fprintf(w, "Errors:\n")
		for _, e := range m.Errors {
			fmt.Fprintf(w, "  x %s\n", e)
		}
		fmt.Fprintln(w)
	}

	return nil
}

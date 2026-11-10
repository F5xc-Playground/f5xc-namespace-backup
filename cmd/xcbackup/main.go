package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/F5xc-Playground/f5xc-namespace-backup/internal/backup"
	"github.com/F5xc-Playground/f5xc-namespace-backup/internal/client"
	"github.com/F5xc-Playground/f5xc-namespace-backup/internal/diff"
	"github.com/F5xc-Playground/f5xc-namespace-backup/internal/inspect"
	"github.com/F5xc-Playground/f5xc-namespace-backup/internal/registry"
	"github.com/F5xc-Playground/f5xc-namespace-backup/internal/restore"
	"github.com/F5xc-Playground/f5xc-namespace-backup/internal/revert"
	"github.com/spf13/cobra"
)

var version = "dev"

func main() {
	rootCmd := &cobra.Command{
		Use:     "xcbackup",
		Short:   "Backup and restore F5 XC namespace configurations",
		Version: version,
	}

	// Persistent flags (shared across subcommands)
	rootCmd.PersistentFlags().String("tenant", "", "F5 XC tenant URL (e.g., acme.console.ves.volterra.io)")
	rootCmd.PersistentFlags().String("namespace", "", "Target namespace")
	rootCmd.PersistentFlags().String("token", "", "API token (or set XC_API_TOKEN env var)")
	rootCmd.PersistentFlags().String("cert", "", "Path to mTLS client certificate")
	rootCmd.PersistentFlags().String("key", "", "Path to mTLS client private key")
	rootCmd.PersistentFlags().Int("parallel", 10, "Max concurrent API calls")

	backupCmd := &cobra.Command{
		Use:   "backup",
		Short: "Backup all objects in a namespace",
		RunE:  runBackup,
	}
	backupCmd.Flags().String("output-dir", "", "Output directory (default: backup-{ns}-{timestamp})")
	backupCmd.Flags().StringSlice("types", nil, "Only back up these resource types")
	backupCmd.Flags().StringSlice("exclude-types", nil, "Skip these resource types")
	backupCmd.Flags().Bool("json", false, "Print one JSON result object on stdout instead of text")

	restoreCmd := &cobra.Command{
		Use:   "restore [backup-dir]",
		Short: "Restore objects from a backup",
		Args:  cobra.ExactArgs(1),
		RunE:  runRestore,
	}
	restoreCmd.Flags().Bool("dry-run", false, "Preview without making changes")
	restoreCmd.Flags().String("on-conflict", "skip", "Behavior when object exists: skip, overwrite, fail")
	restoreCmd.Flags().StringSlice("types", nil, "Only restore these resource types")
	restoreCmd.Flags().Bool("json", false, "Print one JSON result object on stdout instead of text")

	diffCmd := &cobra.Command{
		Use:   "diff [backup-dir]",
		Short: "Compare live namespace state against a backup snapshot",
		Args:  cobra.ExactArgs(1),
		RunE:  runDiff,
	}
	diffCmd.Flags().StringSlice("types", nil, "Only diff these resource types")
	diffCmd.Flags().StringSlice("exclude-types", nil, "Skip these resource types")

	revertCmd := &cobra.Command{
		Use:   "revert [backup-dir]",
		Short: "Revert drifted objects to their backup state",
		Args:  cobra.ExactArgs(1),
		RunE:  runRevert,
	}
	revertCmd.Flags().Bool("dry-run", false, "Preview without making changes")
	revertCmd.Flags().Bool("delete-extra", false, "Delete objects added since backup")
	revertCmd.Flags().StringSlice("types", nil, "Only revert these resource types")
	revertCmd.Flags().StringSlice("exclude-types", nil, "Skip these resource types")

	inspectCmd := &cobra.Command{
		Use:   "inspect [backup-dir]",
		Short: "Inspect a backup directory",
		Args:  cobra.ExactArgs(1),
		RunE:  runInspect,
	}

	namespacesCmd := &cobra.Command{
		Use:   "namespaces",
		Short: "List available namespaces",
		RunE:  runNamespaces,
	}

	rootCmd.AddCommand(backupCmd, restoreCmd, diffCmd, revertCmd, inspectCmd, namespacesCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(exitCode(err))
	}
}

// partialError means a command ran to the end but some objects failed.
// It exits with code 2, so callers can tell it from a fatal error (code 1).
type partialError struct {
	failed int
}

func (e *partialError) Error() string {
	return fmt.Sprintf("%d object(s) failed", e.failed)
}

// partial returns a partialError for failed > 0 and stops cobra printing
// usage for it, since the command line was valid.
func partial(cmd *cobra.Command, failed int) error {
	if failed == 0 {
		return nil
	}
	cmd.SilenceUsage = true
	return &partialError{failed: failed}
}

// exitCode maps a command error to the process exit code:
// 0 success, 1 fatal error, 2 completed with failed objects.
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var p *partialError
	if errors.As(err, &p) {
		return 2
	}
	return 1
}

// nonNil keeps empty lists as [] rather than null in JSON output.
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

type backupReport struct {
	OK             bool           `json:"ok"`
	OutputDir      string         `json:"output_dir"`
	ObjectCount    int            `json:"object_count"`
	ResourceCounts map[string]int `json:"resource_counts"`
	Warnings       []string       `json:"warnings"`
	Errors         []string       `json:"errors"`
}

// reportBackup prints the backup result and returns a partialError if any
// type or object could not be read.
func reportBackup(w io.Writer, asJSON bool, outputDir string, r *backup.Result) error {
	failed := len(r.Errors)
	if asJSON {
		counts := r.ResourceCounts
		if counts == nil {
			counts = map[string]int{}
		}
		if err := writeJSON(w, backupReport{
			OK: failed == 0, OutputDir: outputDir, ObjectCount: r.ObjectCount,
			ResourceCounts: counts, Warnings: nonNil(r.Warnings), Errors: nonNil(r.Errors),
		}); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(w, "\nBackup complete: %d objects\n", r.ObjectCount)
		for kind, count := range r.ResourceCounts {
			fmt.Fprintf(w, "  %-30s %d\n", kind, count)
		}
		printWarnings(w, r.Warnings)
		printErrors(w, r.Errors)
	}
	if failed > 0 {
		return &partialError{failed: failed}
	}
	return nil
}

type restoreReport struct {
	OK         bool     `json:"ok"`
	Created    int      `json:"created"`
	Updated    int      `json:"updated"`
	Skipped    int      `json:"skipped"`
	Failed     int      `json:"failed"`
	Retargeted int      `json:"retargeted"`
	Warnings   []string `json:"warnings"`
	Errors     []string `json:"errors"`
}

// reportRestore prints the restore result and returns a partialError if any
// object failed.
func reportRestore(w io.Writer, asJSON bool, r *restore.Result) error {
	if asJSON {
		if err := writeJSON(w, restoreReport{
			OK: r.Failed == 0, Created: r.Created, Updated: r.Updated, Skipped: r.Skipped, Failed: r.Failed,
			Retargeted: r.Retargeted, Warnings: nonNil(r.Warnings), Errors: nonNil(r.Errors),
		}); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(w, "\nRestore complete:\n")
		fmt.Fprintf(w, "  Created:  %d\n", r.Created)
		fmt.Fprintf(w, "  Updated:  %d\n", r.Updated)
		fmt.Fprintf(w, "  Skipped:  %d\n", r.Skipped)
		fmt.Fprintf(w, "  Failed:   %d\n", r.Failed)
		if r.Retargeted > 0 {
			fmt.Fprintf(w, "  Retargeted references: %d\n", r.Retargeted)
		}
		printWarnings(w, r.Warnings)
		printErrors(w, r.Errors)
	}
	if r.Failed > 0 {
		return &partialError{failed: r.Failed}
	}
	return nil
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// buildClient creates an API client from common CLI flags.
func buildClient(cmd *cobra.Command) (*client.Client, error) {
	tenant, _ := cmd.Flags().GetString("tenant")
	token, _ := cmd.Flags().GetString("token")
	certFile, _ := cmd.Flags().GetString("cert")
	keyFile, _ := cmd.Flags().GetString("key")
	parallel, _ := cmd.Flags().GetInt("parallel")

	if tenant == "" {
		return nil, fmt.Errorf("--tenant is required")
	}
	if token == "" {
		token = os.Getenv("XC_API_TOKEN")
	}
	if token == "" && certFile == "" {
		return nil, fmt.Errorf("provide --token (or XC_API_TOKEN) or --cert/--key")
	}

	var opts []client.Option
	if token != "" {
		opts = append(opts, client.WithToken(token))
	}
	if certFile != "" && keyFile != "" {
		opts = append(opts, client.WithCert(certFile, keyFile))
	}
	opts = append(opts, client.WithParallel(parallel))

	return client.New(tenant, opts...), nil
}

// filterResources applies --types and --exclude-types filters to the resource list.
func filterResources(cmd *cobra.Command) []registry.Resource {
	resources := registry.All()
	if types, _ := cmd.Flags().GetStringSlice("types"); len(types) > 0 {
		resources = registry.FilterByKinds(resources, types)
	}
	if excludeTypes, _ := cmd.Flags().GetStringSlice("exclude-types"); len(excludeTypes) > 0 {
		resources = registry.ExcludeKinds(resources, excludeTypes)
	}
	return resources
}

func runBackup(cmd *cobra.Command, args []string) error {
	namespace, _ := cmd.Flags().GetString("namespace")
	outputDir, _ := cmd.Flags().GetString("output-dir")
	asJSON, _ := cmd.Flags().GetBool("json")

	if namespace == "" {
		return fmt.Errorf("--namespace is required")
	}

	c, err := buildClient(cmd)
	if err != nil {
		return err
	}

	resources := filterResources(cmd)

	if outputDir == "" {
		outputDir = fmt.Sprintf("backup-%s-%s", namespace, time.Now().UTC().Format("2006-01-02T15-04-05Z"))
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	if !asJSON {
		fmt.Printf("Backing up namespace %q from %s\n", namespace, c.BaseURL())
		fmt.Printf("Output: %s\n\n", outputDir)
	}

	result, err := backup.Run(c, &backup.Options{
		Namespace: namespace,
		OutputDir: outputDir,
		Resources: resources,
	})
	if err != nil {
		return err
	}

	if err := reportBackup(cmd.OutOrStdout(), asJSON, outputDir, result); err != nil {
		cmd.SilenceUsage = true
		return err
	}
	return nil
}

func runRestore(cmd *cobra.Command, args []string) error {
	namespace, _ := cmd.Flags().GetString("namespace")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	onConflict, _ := cmd.Flags().GetString("on-conflict")
	types, _ := cmd.Flags().GetStringSlice("types")
	asJSON, _ := cmd.Flags().GetBool("json")

	if namespace == "" {
		return fmt.Errorf("--namespace is required")
	}

	c, err := buildClient(cmd)
	if err != nil {
		return err
	}

	resources := registry.All()
	if len(types) > 0 {
		resources = registry.FilterByKinds(resources, types)
	}

	backupDir := args[0]

	if !asJSON {
		if dryRun {
			fmt.Println("DRY RUN -- no changes will be made")
		}
		fmt.Printf("Restoring to namespace %q on %s\n", namespace, c.BaseURL())
		fmt.Printf("From backup: %s\n\n", backupDir)
	}

	result, err := restore.Run(c, &restore.Options{
		BackupDir:       backupDir,
		TargetNamespace: namespace,
		Resources:       resources,
		DryRun:          dryRun,
		OnConflict:      onConflict,
	})
	if err != nil {
		return err
	}

	if err := reportRestore(cmd.OutOrStdout(), asJSON, result); err != nil {
		cmd.SilenceUsage = true
		return err
	}
	return nil
}

func runDiff(cmd *cobra.Command, args []string) error {
	namespace, _ := cmd.Flags().GetString("namespace")

	if namespace == "" {
		return fmt.Errorf("--namespace is required")
	}

	c, err := buildClient(cmd)
	if err != nil {
		return err
	}

	resources := filterResources(cmd)
	backupDir := args[0]

	fmt.Printf("Comparing backup %s against live namespace %q on %s\n\n", backupDir, namespace, c.BaseURL())

	report, err := diff.Run(c, &diff.Options{
		BackupDir: backupDir,
		Namespace: namespace,
		Resources: resources,
	})
	if err != nil {
		return err
	}

	fmt.Printf("Drift report:\n")
	fmt.Printf("  Added (in live, not in backup):    %d\n", len(report.Added))
	fmt.Printf("  Removed (in backup, not in live):  %d\n", len(report.Removed))
	fmt.Printf("  Modified:                          %d\n", len(report.Modified))
	fmt.Printf("  Unchanged:                         %d\n", report.Unchanged)

	if len(report.Added) > 0 {
		fmt.Printf("\nAdded objects:\n")
		added := make([]string, len(report.Added))
		for i, ref := range report.Added {
			added[i] = ref.Kind + "/" + ref.Name
		}
		sort.Strings(added)
		for _, s := range added {
			fmt.Printf("  + %s\n", s)
		}
	}

	if len(report.Removed) > 0 {
		fmt.Printf("\nRemoved objects:\n")
		removed := make([]string, len(report.Removed))
		for i, ref := range report.Removed {
			removed[i] = ref.Kind + "/" + ref.Name
		}
		sort.Strings(removed)
		for _, s := range removed {
			fmt.Printf("  - %s\n", s)
		}
	}

	if len(report.Modified) > 0 {
		fmt.Printf("\nModified objects:\n")
		for _, mod := range report.Modified {
			fmt.Printf("  ~ %s/%s\n", mod.Kind, mod.Name)
			fmt.Println(mod.UnifiedDiff)
		}
	}

	printWarnings(os.Stdout, report.Warnings)
	printErrors(os.Stdout, report.Errors)

	return partial(cmd, len(report.Errors))
}

func runRevert(cmd *cobra.Command, args []string) error {
	namespace, _ := cmd.Flags().GetString("namespace")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	deleteExtra, _ := cmd.Flags().GetBool("delete-extra")

	if namespace == "" {
		return fmt.Errorf("--namespace is required")
	}

	c, err := buildClient(cmd)
	if err != nil {
		return err
	}

	resources := filterResources(cmd)
	backupDir := args[0]

	if dryRun {
		fmt.Println("DRY RUN -- no changes will be made")
	}

	fmt.Printf("Reverting namespace %q on %s to backup %s\n\n", namespace, c.BaseURL(), backupDir)

	result, _, err := revert.Run(c, &revert.Options{
		BackupDir:       backupDir,
		TargetNamespace: namespace,
		Resources:       resources,
		DryRun:          dryRun,
		DeleteExtra:     deleteExtra,
	})
	if err != nil {
		return err
	}

	fmt.Printf("\nRevert complete:\n")
	fmt.Printf("  Replaced: %d\n", result.Replaced)
	fmt.Printf("  Created:  %d\n", result.Created)
	fmt.Printf("  Deleted:  %d\n", result.Deleted)
	fmt.Printf("  Skipped:  %d\n", result.Skipped)
	fmt.Printf("  Failed:   %d\n", result.Failed)

	printWarnings(os.Stdout, result.Warnings)
	printErrors(os.Stdout, result.Errors)

	return partial(cmd, result.Failed)
}

func runInspect(cmd *cobra.Command, args []string) error {
	return inspect.Run(args[0], os.Stdout)
}

func runNamespaces(cmd *cobra.Command, args []string) error {
	c, err := buildClient(cmd)
	if err != nil {
		return err
	}

	items, err := c.List("/api/web/namespaces")
	if err != nil {
		return fmt.Errorf("listing namespaces: %w", err)
	}

	for _, item := range items {
		if name, ok := item["name"].(string); ok {
			fmt.Println(name)
		}
	}

	return nil
}

// printWarnings groups and prints warnings with a count header.
func printWarnings(w io.Writer, warnings []string) {
	if len(warnings) == 0 {
		return
	}

	// Count "skipped" warnings (inaccessible resource types) vs other warnings
	var skippedKinds []string
	var otherWarnings []string
	for _, warning := range warnings {
		if strings.HasPrefix(warning, "skipped ") && strings.HasSuffix(warning, "not accessible (may require subscription)") {
			// Extract the kind name
			kind := strings.TrimPrefix(warning, "skipped ")
			kind = strings.TrimSuffix(kind, ": not accessible (may require subscription)")
			skippedKinds = append(skippedKinds, kind)
		} else {
			otherWarnings = append(otherWarnings, warning)
		}
	}

	if len(skippedKinds) > 0 {
		fmt.Fprintf(w, "\nWarnings (%d resource types not accessible):\n", len(skippedKinds))
		fmt.Fprintf(w, "  Skipped: %s\n", strings.Join(skippedKinds, ", "))
	}

	for _, warning := range otherWarnings {
		fmt.Fprintf(w, "  ! %s\n", warning)
	}
}

// printErrors prints errors with a count header.
func printErrors(w io.Writer, errs []string) {
	if len(errs) == 0 {
		return
	}
	fmt.Fprintf(w, "\nErrors (%d):\n", len(errs))
	for _, e := range errs {
		fmt.Fprintf(w, "  x %s\n", e)
	}
}

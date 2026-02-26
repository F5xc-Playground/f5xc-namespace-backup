package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/kevingstewart/xcbackup/internal/backup"
	"github.com/kevingstewart/xcbackup/internal/client"
	"github.com/kevingstewart/xcbackup/internal/diff"
	"github.com/kevingstewart/xcbackup/internal/inspect"
	"github.com/kevingstewart/xcbackup/internal/registry"
	"github.com/kevingstewart/xcbackup/internal/restore"
	"github.com/kevingstewart/xcbackup/internal/revert"
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

	restoreCmd := &cobra.Command{
		Use:   "restore [backup-dir]",
		Short: "Restore objects from a backup",
		Args:  cobra.ExactArgs(1),
		RunE:  runRestore,
	}
	restoreCmd.Flags().Bool("dry-run", false, "Preview without making changes")
	restoreCmd.Flags().String("on-conflict", "skip", "Behavior when object exists: skip, overwrite, fail")
	restoreCmd.Flags().StringSlice("types", nil, "Only restore these resource types")

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
		os.Exit(1)
	}
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

	fmt.Printf("Backing up namespace %q from %s\n", namespace, c.BaseURL())
	fmt.Printf("Output: %s\n\n", outputDir)

	result, err := backup.Run(c, &backup.Options{
		Namespace: namespace,
		OutputDir: outputDir,
		Resources: resources,
	})
	if err != nil {
		return err
	}

	fmt.Printf("\nBackup complete: %d objects\n", result.ObjectCount)
	for kind, count := range result.ResourceCounts {
		fmt.Printf("  %-30s %d\n", kind, count)
	}
	printWarnings(result.Warnings)
	printErrors(result.Errors)

	return nil
}

func runRestore(cmd *cobra.Command, args []string) error {
	namespace, _ := cmd.Flags().GetString("namespace")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	onConflict, _ := cmd.Flags().GetString("on-conflict")
	types, _ := cmd.Flags().GetStringSlice("types")

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

	if dryRun {
		fmt.Println("DRY RUN -- no changes will be made")
	}

	fmt.Printf("Restoring to namespace %q on %s\n", namespace, c.BaseURL())
	fmt.Printf("From backup: %s\n\n", backupDir)

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

	fmt.Printf("\nRestore complete:\n")
	fmt.Printf("  Created:  %d\n", result.Created)
	fmt.Printf("  Updated:  %d\n", result.Updated)
	fmt.Printf("  Skipped:  %d\n", result.Skipped)
	fmt.Printf("  Failed:   %d\n", result.Failed)

	printWarnings(result.Warnings)
	printErrors(result.Errors)

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

	printWarnings(report.Warnings)
	printErrors(report.Errors)

	return nil
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

	printWarnings(result.Warnings)
	printErrors(result.Errors)

	return nil
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
func printWarnings(warnings []string) {
	if len(warnings) == 0 {
		return
	}

	// Count "skipped" warnings (inaccessible resource types) vs other warnings
	var skippedKinds []string
	var otherWarnings []string
	for _, w := range warnings {
		if strings.HasPrefix(w, "skipped ") && strings.HasSuffix(w, "not accessible (may require subscription)") {
			// Extract the kind name
			kind := strings.TrimPrefix(w, "skipped ")
			kind = strings.TrimSuffix(kind, ": not accessible (may require subscription)")
			skippedKinds = append(skippedKinds, kind)
		} else {
			otherWarnings = append(otherWarnings, w)
		}
	}

	if len(skippedKinds) > 0 {
		fmt.Printf("\nWarnings (%d resource types not accessible):\n", len(skippedKinds))
		fmt.Printf("  Skipped: %s\n", strings.Join(skippedKinds, ", "))
	}

	for _, w := range otherWarnings {
		fmt.Printf("  ! %s\n", w)
	}
}

// printErrors prints errors with a count header.
func printErrors(errs []string) {
	if len(errs) == 0 {
		return
	}
	fmt.Printf("\nErrors (%d):\n", len(errs))
	for _, e := range errs {
		fmt.Printf("  x %s\n", e)
	}
}

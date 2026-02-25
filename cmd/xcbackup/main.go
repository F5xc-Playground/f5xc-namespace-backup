package main

import (
	"fmt"
	"os"
	"time"

	"github.com/kevingstewart/xcbackup/internal/backup"
	"github.com/kevingstewart/xcbackup/internal/client"
	"github.com/kevingstewart/xcbackup/internal/registry"
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

	inspectCmd := &cobra.Command{
		Use:   "inspect [backup-dir]",
		Short: "Inspect a backup directory",
		Args:  cobra.ExactArgs(1),
		RunE:  runInspect,
	}

	rootCmd.AddCommand(backupCmd, restoreCmd, inspectCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runBackup(cmd *cobra.Command, args []string) error {
	tenant, _ := cmd.Flags().GetString("tenant")
	namespace, _ := cmd.Flags().GetString("namespace")
	token, _ := cmd.Flags().GetString("token")
	certFile, _ := cmd.Flags().GetString("cert")
	keyFile, _ := cmd.Flags().GetString("key")
	parallel, _ := cmd.Flags().GetInt("parallel")
	outputDir, _ := cmd.Flags().GetString("output-dir")
	types, _ := cmd.Flags().GetStringSlice("types")
	excludeTypes, _ := cmd.Flags().GetStringSlice("exclude-types")

	if tenant == "" || namespace == "" {
		return fmt.Errorf("--tenant and --namespace are required")
	}
	if token == "" {
		token = os.Getenv("XC_API_TOKEN")
	}
	if token == "" && certFile == "" {
		return fmt.Errorf("provide --token (or XC_API_TOKEN) or --cert/--key")
	}

	var opts []client.Option
	if token != "" {
		opts = append(opts, client.WithToken(token))
	}
	if certFile != "" && keyFile != "" {
		opts = append(opts, client.WithCert(certFile, keyFile))
	}
	opts = append(opts, client.WithParallel(parallel))
	c := client.New(tenant, opts...)

	resources := registry.All()
	if len(types) > 0 {
		resources = registry.FilterByKinds(resources, types)
	}
	if len(excludeTypes) > 0 {
		resources = registry.ExcludeKinds(resources, excludeTypes)
	}

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
	if len(result.Warnings) > 0 {
		fmt.Printf("\nWarnings:\n")
		for _, w := range result.Warnings {
			fmt.Printf("  ⚠ %s\n", w)
		}
	}
	if len(result.Errors) > 0 {
		fmt.Printf("\nErrors:\n")
		for _, e := range result.Errors {
			fmt.Printf("  ✗ %s\n", e)
		}
	}

	return nil
}

func runRestore(cmd *cobra.Command, args []string) error {
	fmt.Println("restore: not yet implemented")
	return nil
}

func runInspect(cmd *cobra.Command, args []string) error {
	fmt.Println("inspect: not yet implemented")
	return nil
}

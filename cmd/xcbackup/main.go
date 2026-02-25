package main

import (
	"fmt"
	"os"

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
	fmt.Println("backup: not yet implemented")
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

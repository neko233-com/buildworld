package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	configFile string
	version    = "dev"
)

var rootCmd = &cobra.Command{
	Use:   "buildworld",
	Short: "buildworld - A modern CI/CD server",
	Long:  `buildworld is a Jenkins alternative with TypeScript DSL, plugin system, and distributed workers.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "Configuration file path (default: per-user BuildWorld config)")
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(stopCmd)
	rootCmd.AddCommand(restartCmd)
	rootCmd.AddCommand(pauseCmd)
	rootCmd.AddCommand(resumeCmd)
	rootCmd.AddCommand(resetRootPasswordCmd)
	rootCmd.AddCommand(enableAutostartCmd)
	rootCmd.AddCommand(disableAutostartCmd)
	rootCmd.AddCommand(versionCmd)
}

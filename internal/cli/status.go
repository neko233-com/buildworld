package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show server status",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Server status: stopped")
		return nil
	},
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the buildworld233 server",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Stopping server...")
		return nil
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("buildworld233 v0.1.0")
	},
}

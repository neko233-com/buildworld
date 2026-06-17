package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the buildworld233 server",
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetInt("port")
		fmt.Printf("Starting buildworld233 on port %d...\n", port)
		// Will implement server start in Task 6
		return nil
	},
}

func init() {
	startCmd.Flags().IntP("port", "p", 6050, "Server port")
	rootCmd.AddCommand(startCmd)
}

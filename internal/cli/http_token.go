package cli

import (
	"fmt"
	"github.com/neko233-com/buildworld/internal/auth"
	"github.com/neko233-com/buildworld/internal/store"
	"github.com/spf13/cobra"
)

var httpTokenCmd = &cobra.Command{
	Use:   "http-token",
	Short: "Ensure the private local HTTP API token and print its file path (never its value)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		_, cfg, err := ensureConfig(configFile)
		if err != nil {
			return err
		}
		database, err := store.New(cfg.Database.Path)
		if err != nil {
			return fmt.Errorf("open BuildWorld database: %w", err)
		}
		defer database.Close()
		path, _, err := auth.EnsureLocalHTTPToken(database, cfg.Database.Path)
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), path)
		return nil
	},
}

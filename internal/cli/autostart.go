package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var enableAutostartCmd = &cobra.Command{
	Use:   "enable-autostart",
	Short: "Enable silent background start when you sign in",
	RunE: func(cmd *cobra.Command, _ []string) error {
		path, _, err := ensureConfig(configFile)
		if err != nil {
			return err
		}
		server, err := serverExecutable()
		if err != nil {
			return err
		}
		if err := enableAutostart(server, path); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "BuildWorld will start silently in the background after sign-in")
		return nil
	},
}

var disableAutostartCmd = &cobra.Command{
	Use:   "disable-autostart",
	Short: "Disable automatic background start",
	RunE: func(cmd *cobra.Command, _ []string) error {
		if err := disableAutostart(); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "BuildWorld autostart disabled")
		return nil
	},
}

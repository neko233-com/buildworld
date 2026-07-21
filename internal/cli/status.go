package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/neko233-com/buildworld/internal/auth"
	"github.com/neko233-com/buildworld/internal/buildinfo"
	"github.com/neko233-com/buildworld/internal/store"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show BuildWorld server status",
	RunE: func(cmd *cobra.Command, _ []string) error {
		_, cfg, err := ensureConfig(configFile)
		if err != nil {
			return err
		}
		if healthy(cfg.Server.Port) {
			fmt.Printf("BuildWorld is running at http://127.0.0.1:%d\n", cfg.Server.Port)
			return nil
		}
		fmt.Fprintln(cmd.OutOrStdout(), "BuildWorld is stopped")
		return nil
	},
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the BuildWorld server",
	RunE: func(cmd *cobra.Command, _ []string) error {
		stopped, err := stopManagedServer()
		if err != nil {
			return err
		}
		if stopped {
			fmt.Fprintln(cmd.OutOrStdout(), "BuildWorld stopped")
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), "BuildWorld is already stopped")
		}
		return nil
	},
}

var pauseCmd = &cobra.Command{
	Use:   "pause",
	Short: "Pause BuildWorld by stopping its background server",
	RunE: func(cmd *cobra.Command, _ []string) error {
		stopped, err := stopManagedServer()
		if err != nil {
			return err
		}
		if !stopped {
			fmt.Fprintln(cmd.OutOrStdout(), "BuildWorld is already paused")
			return nil
		}
		fmt.Fprintln(cmd.OutOrStdout(), "BuildWorld paused; use `buildworld resume` to start it again")
		return nil
	},
}

var resumeCmd = &cobra.Command{
	Use:   "resume",
	Short: "Resume a paused BuildWorld server",
	RunE:  func(cmd *cobra.Command, args []string) error { return startCmd.RunE(cmd, args) },
}

var restartCmd = &cobra.Command{
	Use:   "restart",
	Short: "Restart the BuildWorld server",
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := stopManagedServer(); err != nil {
			return err
		}
		return startCmd.RunE(cmd, args)
	},
}

var resetRootPasswordCmd = &cobra.Command{
	Use:   "reset-root-password",
	Short: "Reset the local root account password",
	RunE: func(cmd *cobra.Command, _ []string) error {
		password, _ := cmd.Flags().GetString("password")
		fromStdin, _ := cmd.Flags().GetBool("password-stdin")
		if password != "" && fromStdin {
			return fmt.Errorf("use either --password or --password-stdin, not both")
		}
		if fromStdin {
			value, err := bufio.NewReader(os.Stdin).ReadString('\n')
			if err != nil && len(value) == 0 {
				return fmt.Errorf("read password from stdin: %w", err)
			}
			password = strings.TrimSpace(value)
		}
		if len(password) < 12 {
			return fmt.Errorf("root password must contain at least 12 characters")
		}
		_, cfg, err := ensureConfig(configFile)
		if err != nil {
			return err
		}
		wasRunning := healthy(cfg.Server.Port)
		if wasRunning {
			if _, err := stopManagedServer(); err != nil {
				return err
			}
		}
		database, err := store.New(cfg.Database.Path)
		if err != nil {
			return fmt.Errorf("open BuildWorld database: %w", err)
		}
		defer database.Close()
		if err := auth.SetupDefaultAdmin(database); err != nil {
			return fmt.Errorf("ensure root account: %w", err)
		}
		root, err := database.GetUserByUsername("root")
		if err != nil {
			return fmt.Errorf("find root account: %w", err)
		}
		hash, err := auth.HashPassword(password)
		if err != nil {
			return err
		}
		if err := database.UpdateUserPassword(root.ID, hash); err != nil {
			return fmt.Errorf("update root password: %w", err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), "Root password reset successfully")
		if wasRunning {
			return startCmd.RunE(cmd, nil)
		}
		return nil
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version",
	Run: func(cmd *cobra.Command, _ []string) {
		fmt.Fprintf(cmd.OutOrStdout(), "buildworld %s\n", buildinfo.Version)
	},
}

func init() {
	resetRootPasswordCmd.Flags().String("password", "", "New root password (prefer --password-stdin to avoid shell history)")
	resetRootPasswordCmd.Flags().Bool("password-stdin", false, "Read the new root password from stdin")
}

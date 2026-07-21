package cli

import (
	"fmt"
	"os/exec"
	"time"

	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the BuildWorld server in the background",
	RunE: func(cmd *cobra.Command, _ []string) error {
		foreground, _ := cmd.Flags().GetBool("foreground")
		path, cfg, err := ensureConfig(configFile)
		if err != nil {
			return err
		}
		if healthy(cfg.Server.Port) {
			fmt.Printf("BuildWorld is already running at http://127.0.0.1:%d\n", cfg.Server.Port)
			return nil
		}
		server, err := serverExecutable()
		if err != nil {
			return err
		}
		if !foreground {
			managed, err := startAutostartService()
			if err != nil {
				return err
			}
			if managed {
				for range 40 {
					if healthy(cfg.Server.Port) {
						fmt.Printf("BuildWorld started at http://127.0.0.1:%d using the registered background service\n", cfg.Server.Port)
						return nil
					}
					time.Sleep(250 * time.Millisecond)
				}
				return fmt.Errorf("BuildWorld background service started, but health check did not become ready; inspect %s", serverLogPath())
			}
		}
		child := exec.Command(server, "-config", path)
		if foreground {
			child.Stdout = cmd.OutOrStdout()
			child.Stderr = cmd.ErrOrStderr()
			return child.Run()
		}
		logFile, err := openServerLog()
		if err != nil {
			return err
		}
		defer logFile.Close()
		child.Stdout = logFile
		child.Stderr = logFile
		prepareBackground(child)
		if err := child.Start(); err != nil {
			return fmt.Errorf("start BuildWorld server: %w", err)
		}
		if err := writePID(child.Process.Pid); err != nil {
			_ = stopProcess(child.Process)
			return err
		}
		for range 40 {
			if healthy(cfg.Server.Port) {
				fmt.Printf("BuildWorld started at http://127.0.0.1:%d (PID %d)\n", cfg.Server.Port, child.Process.Pid)
				return nil
			}
			time.Sleep(250 * time.Millisecond)
		}
		return fmt.Errorf("BuildWorld process started, but health check did not become ready; inspect %s", serverLogPath())
	},
}

func init() {
	startCmd.Flags().Bool("foreground", false, "Run the server in the foreground (for system services)")
}

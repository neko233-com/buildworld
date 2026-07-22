package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/neko233-com/buildworld/internal/jenkins"
	"github.com/neko233-com/buildworld/internal/store"
	"github.com/spf13/cobra"
)

var jenkinsCmd = &cobra.Command{
	Use:   "jenkins",
	Short: "Import jobs from a live Jenkins instance into BuildWorld",
}

var jenkinsImportCmd = &cobra.Command{
	Use:   "import",
	Short: "Import a single Jenkins job (pipeline or freestyle) as a BuildWorld project",
	RunE: func(cmd *cobra.Command, _ []string) error {
		rawURL, _ := cmd.Flags().GetString("url")
		user, _ := cmd.Flags().GetString("user")
		token, _ := cmd.Flags().GetString("token")
		folder, _ := cmd.Flags().GetString("folder")
		job, _ := cmd.Flags().GetString("job")
		name, _ := cmd.Flags().GetString("name")
		groupID, _ := cmd.Flags().GetInt64("group")

		if rawURL == "" || job == "" {
			return fmt.Errorf("--url and --job are required")
		}
		folders := []string{}
		if folder != "" {
			folders = strings.Split(folder, "/")
		}
		jobName := job
		if name != "" {
			jobName = name
		}

		_, cfg, err := ensureConfig(configFile)
		if err != nil {
			return err
		}
		db, err := store.New(cfg.Database.Path)
		if err != nil {
			return fmt.Errorf("open BuildWorld database: %w", err)
		}
		defer db.Close()

		client := jenkins.NewClient(rawURL, user, token)
		xmlData, err := client.FetchConfig(context.Background(), folders, job)
		if err != nil {
			return err
		}
		def, err := jenkins.ParseConfig(xmlData, jobName)
		if err != nil {
			return err
		}

		project, err := db.CreateProject(def.JobName, fmt.Sprintf("Imported from Jenkins %s", rawURL), def.RepoURL, "git", def.DefaultBranch, def.Script, 1, nil, nil)
		if err != nil {
			return fmt.Errorf("create project: %w", err)
		}
		if err := db.SetProjectPipelineSource(project.ID, def.Format, def.SourceMode, def.SCMRepo, def.SCMBranch, def.SCMPath); err != nil {
			return fmt.Errorf("set pipeline source: %w", err)
		}
		if groupID != 0 {
			if err := db.SetProjectGroup(project.ID, &groupID); err != nil {
				return fmt.Errorf("set group: %w", err)
			}
		}

		fmt.Printf("Imported Jenkins job %q as BuildWorld project #%d (format=%s, source=%s)\n", job, project.ID, def.Format, def.SourceMode)
		if def.RepoURL != "" {
			fmt.Printf("  repository: %s @ %s\n", def.RepoURL, def.DefaultBranch)
		}
		for _, warning := range def.Warnings {
			fmt.Printf("  warning: %s\n", warning)
		}
		return nil
	},
}

func init() {
	jenkinsImportCmd.Flags().String("url", "", "Jenkins base URL, e.g. http://192.168.110.42:8080")
	jenkinsImportCmd.Flags().String("user", "", "Jenkins username")
	jenkinsImportCmd.Flags().String("token", "", "Jenkins password or API token")
	jenkinsImportCmd.Flags().String("folder", "", "Folder chain, slash-separated, e.g. \"服务器 Go\"")
	jenkinsImportCmd.Flags().String("job", "", "Jenkins job name")
	jenkinsImportCmd.Flags().String("name", "", "Override the BuildWorld project name (defaults to the job name)")
	jenkinsImportCmd.Flags().Int64("group", 0, "Project group id to assign")
	jenkinsCmd.AddCommand(jenkinsImportCmd)
	rootCmd.AddCommand(jenkinsCmd)
}

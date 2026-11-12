package engine

import (
	"testing"

	"github.com/neko233-com/buildworld/internal/store"
)

func TestPrestartPipelineGuard(t *testing.T) {
	const repo = "https://git.example/game.git"
	project := &store.Project{Enabled: true, PipelineSourceMode: "scm", PipelineFormat: "jenkinsfile", PipelineSCMRepo: repo, PipelineSCMPath: "server_game/.jenkins/Jenkinsfile_ProductionRelease_Prestart"}
	config := &BuildConfig{Parameters: []BuildParameter{{Name: "OPENING_PROBE_SECRET", Type: "password"}}}
	parameters, err := AttachPrestartPipelineGuard(`{"OPENING_PROBE_SECRET":"fixture"}`, repo)
	if err != nil || validateStoredPrestartPipeline(project, config, parameters) != nil {
		t.Fatalf("valid contract rejected: %v", err)
	}
	for name, change := range map[string]func(*store.Project, *BuildConfig){
		"repository": func(p *store.Project, _ *BuildConfig) { p.PipelineSCMRepo = "https://other.example/game.git" },
		"path":       func(p *store.Project, _ *BuildConfig) { p.PipelineSCMPath = "Jenkinsfile" },
		"disabled":   func(p *store.Project, _ *BuildConfig) { p.Enabled = false },
		"unmasked":   func(_ *store.Project, c *BuildConfig) { c.Parameters[0].Type = "string" },
		"missing":    func(_ *store.Project, c *BuildConfig) { c.Parameters = nil },
	} {
		t.Run(name, func(t *testing.T) {
			p := *project
			c := &BuildConfig{Parameters: append([]BuildParameter(nil), config.Parameters...)}
			change(&p, c)
			if validateStoredPrestartPipeline(&p, c, parameters) == nil {
				t.Fatal("guard accepted changed pipeline")
			}
		})
	}
	public := map[string]interface{}{prestartPipelineContextKey: repo}
	PublicBuildParameters(public)
	if len(public) != 0 {
		t.Fatal("internal context exposed as public parameter")
	}
}

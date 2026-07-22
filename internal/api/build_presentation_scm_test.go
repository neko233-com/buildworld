package api

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/neko233-com/buildworld/internal/engine"
	"github.com/neko233-com/buildworld/internal/store"
)

type forbiddenPresentationSCMFetcher struct{}

func (forbiddenPresentationSCMFetcher) FetchSCMPipeline(context.Context, engine.SCMPipelineSource) (string, engine.PipelineFormat, error) {
	panic("build presentation must not fetch an SCM pipeline")
}

func TestPublicBuildPresentationDoesNotFetchSCMPipeline(t *testing.T) {
	previous := engine.DefaultSCMFetcher
	engine.DefaultSCMFetcher = forbiddenPresentationSCMFetcher{}
	t.Cleanup(func() { engine.DefaultSCMFetcher = previous })

	database, err := store.New(filepath.Join(t.TempDir(), "presentation.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	project, err := database.CreateProject("scm-project", "", "", "git", "main", `{}`, 0, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SetProjectPipelineSource(project.ID, "typescript", "scm", "https://example.invalid/repository.git", "main", "buildworld.ts"); err != nil {
		t.Fatal(err)
	}
	build, err := database.CreateBuild(project.ID, 1, "manual", "main", "", `{"password":"top-secret","region":"test"}`, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	result := (&handlers{d: Deps{Store: database}}).publicBuild(build)
	if result.Parameters != `{"password":"********","region":"********"}` {
		t.Fatalf("presented parameters = %s", result.Parameters)
	}
}

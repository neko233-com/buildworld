package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/neko233-com/buildworld/internal/store"
)

// Capture candidates before dispatching recovery work. Fetching an SCM pipeline
// may be slow; it must not delay HTTP readiness or duplicate a recovered build
// that finishes while startup is still resolving another project's pipeline.
func (r *BuildRunner) startStartupBuilds(ctx context.Context) {
	projects, err := r.store.ListProjects()
	if err != nil {
		fmt.Printf("startup builds: list projects: %v\n", err)
		return
	}
	var candidates []*store.Project
	for _, project := range projects {
		if !project.Enabled || !project.BuildOnStartup {
			continue
		}
		eligible, err := r.store.StartupBuildEligible(project.ID)
		if err != nil {
			fmt.Printf("startup eligibility project %d: %v\n", project.ID, err)
		} else if eligible {
			candidates = append(candidates, project)
		}
	}
	r.startupWG.Add(1)
	go func() {
		defer r.startupWG.Done()
		approval := NewApprovalService(r.store)
		for _, project := range candidates {
			if ctx.Err() != nil {
				return
			}
			if err := r.triggerStartupBuild(ctx, project, approval); err != nil {
				fmt.Printf("startup build project %d: %v\n", project.ID, err)
			}
		}
	}()
}

func (r *BuildRunner) triggerStartupBuild(parent context.Context, project *store.Project, approval *ApprovalService) error {
	ctx, cancel := context.WithTimeout(parent, 25*time.Second)
	defer cancel()
	config, err := r.loadBuildConfig(ctx, project)
	if err != nil {
		return fmt.Errorf("load pipeline: %w", err)
	}
	parameters, err := ResolveBuildParameters(config.Parameters, nil)
	if err != nil {
		return fmt.Errorf("default parameters: %w", err)
	}
	encoded, err := json.Marshal(parameters)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	requiresApproval, err := ApprovalRequired(config)
	if err != nil {
		return err
	}
	// Persist the approval hold with the build itself so a concurrent dispatcher
	// cannot start execution while its approval record is being initialized.
	build, err := r.store.CreateStartupBuild(project.ID, project.DefaultBranch, string(encoded), requiresApproval)
	if err != nil || build == nil {
		return err
	}
	if _, err := approval.RequestIfRequired(build.ID, 0, config); err != nil {
		_ = r.store.AppendBuildLog(build.ID, "[buildworld] Startup approval initialization failed.\n")
		_ = r.store.FinishBuild(build.ID, "failed", 0)
		return fmt.Errorf("approval: %w", err)
	}
	if err := r.Enqueue(build.ID); err != nil {
		return fmt.Errorf("enqueue build %d: %w", build.ID, err)
	}
	fmt.Printf("startup build queued: project %d build #%d (id=%d)\n", project.ID, build.Number, build.ID)
	return nil
}

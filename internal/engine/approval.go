package engine

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/neko233-com/buildworld/internal/store"
)

// ApprovalService manages the single-approval strategy lifecycle. The policy
// remains strategy-based so later quorum or external-check implementations can
// be added without changing project configuration.
type ApprovalService struct {
	store *store.Store
}

type PendingApproval struct {
	store.BuildApproval
	ProjectID      int64    `json:"project_id"`
	ProjectName    string   `json:"project_name"`
	BuildNumber    int      `json:"build_number"`
	Branch         string   `json:"branch,omitempty"`
	Trigger        string   `json:"trigger"`
	Prompt         string   `json:"prompt,omitempty"`
	RequiredRoles  []string `json:"required_roles"`
	AllowRequester bool     `json:"allow_requester"`
}

func NewApprovalService(dataStore *store.Store) *ApprovalService {
	return &ApprovalService{store: dataStore}
}

func (a *ApprovalService) loadProjectConfig(project *store.Project) (*BuildConfig, error) {
	config, err := ParsePipelineConfig(project.Config)
	if err != nil {
		return nil, err
	}
	if project.TemplateID == nil {
		return config, nil
	}
	template, err := a.store.GetBuildTemplate(*project.TemplateID)
	if err != nil {
		return nil, err
	}
	templateConfig, err := ParsePipelineConfig(template.Config)
	if err != nil {
		return nil, err
	}
	return MergeBuildConfig(templateConfig, config), nil
}

func (a *ApprovalService) policyForBuild(buildID int64) (ApprovalPolicy, *store.Build, *store.Project, error) {
	build, err := a.store.GetBuild(buildID)
	if err != nil {
		return ApprovalPolicy{}, nil, nil, fmt.Errorf("build not found: %w", err)
	}
	project, err := a.store.GetProject(build.ProjectID)
	if err != nil {
		return ApprovalPolicy{}, nil, nil, fmt.Errorf("project not found: %w", err)
	}
	config, err := a.loadProjectConfig(project)
	if err != nil {
		return ApprovalPolicy{}, nil, nil, fmt.Errorf("invalid build configuration: %w", err)
	}
	policy, err := ResolveApprovalPolicy(config)
	if err != nil {
		return ApprovalPolicy{}, nil, nil, err
	}
	return policy, build, project, nil
}

// RequestIfRequired evaluates the versioned project policy and creates the
// approval record before execution starts.
func (a *ApprovalService) RequestIfRequired(buildID, requesterID int64, config *BuildConfig) (bool, error) {
	policy, err := ResolveApprovalPolicy(config)
	if err != nil {
		return false, err
	}
	if policy.Strategy == ApprovalStrategyNone {
		return false, nil
	}
	if policy.Strategy != ApprovalStrategySingle {
		return false, fmt.Errorf("approval strategy %q is not available", policy.Strategy)
	}
	return true, a.RequestApproval(buildID, requesterID)
}

// RequestApproval marks the build as pending approval. requesterID=0 is used
// for automated triggers and is represented as "automation".
func (a *ApprovalService) RequestApproval(buildID, requesterID int64) error {
	build, err := a.store.GetBuild(buildID)
	if err != nil {
		return fmt.Errorf("build not found: %w", err)
	}
	if existing, err := a.store.GetBuildApprovalByBuild(buildID); err == nil && existing.Status == "pending" {
		return nil
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	requesterName := "automation"
	if requesterID > 0 {
		user, err := a.store.GetUser(requesterID)
		if err != nil {
			return fmt.Errorf("requester not found: %w", err)
		}
		requesterName = user.Username
	}
	if err := a.store.SetBuildApprovalRequired(buildID, true); err != nil {
		return err
	}
	if build.Status == "pending" || build.Status == "" {
		if err := a.store.UpdateBuildStatus(buildID, "pending_approval"); err != nil {
			return err
		}
	}
	if _, err := a.store.CreateBuildApproval(buildID, requesterID, requesterName); err != nil {
		_ = a.store.SetBuildApprovalRequired(buildID, false)
		_ = a.store.UpdateBuildStatus(buildID, "pending")
		return err
	}
	return nil
}

func (a *ApprovalService) authorizeResolution(buildID, resolverID int64) (*store.BuildApproval, *store.User, error) {
	approval, err := a.store.GetBuildApprovalByBuild(buildID)
	if err != nil {
		return nil, nil, fmt.Errorf("approval record not found: %w", err)
	}
	if approval.Status != "pending" {
		return nil, nil, fmt.Errorf("approval already %s", approval.Status)
	}
	user, err := a.store.GetUser(resolverID)
	if err != nil {
		return nil, nil, fmt.Errorf("resolver not found: %w", err)
	}
	policy, _, _, err := a.policyForBuild(buildID)
	if err != nil {
		return nil, nil, err
	}
	if !ApprovalRoleAllowed(policy, user.Role) {
		return nil, nil, fmt.Errorf("role %q is not allowed to resolve this approval", user.Role)
	}
	if !policy.RequesterCanResolve() && approval.UserID > 0 && approval.UserID == resolverID {
		return nil, nil, fmt.Errorf("the requester cannot resolve their own build approval")
	}
	return approval, user, nil
}

func (a *ApprovalService) Approve(buildID, resolverID int64, comment string) error {
	approval, user, err := a.authorizeResolution(buildID, resolverID)
	if err != nil {
		return err
	}
	if err := a.store.UpdateBuildApproval(approval.ID, "approved", comment, user.ID, user.Username); err != nil {
		return err
	}
	if err := a.store.SetBuildApproved(buildID, user.ID); err != nil {
		return err
	}
	if err := a.store.UpdateBuildStatus(buildID, "pending"); err != nil {
		return err
	}
	return a.store.UpdateBuildQueueItemStatusByBuildID(buildID, "queued")
}

func (a *ApprovalService) Reject(buildID, resolverID int64, comment string) error {
	approval, user, err := a.authorizeResolution(buildID, resolverID)
	if err != nil {
		return err
	}
	if err := a.store.UpdateBuildApproval(approval.ID, "rejected", comment, user.ID, user.Username); err != nil {
		return err
	}
	if err := a.store.UpdateBuildStatus(buildID, "rejected"); err != nil {
		return err
	}
	return a.store.UpdateBuildQueueItemStatusByBuildID(buildID, "rejected")
}

func (a *ApprovalService) ListPending() ([]store.BuildApproval, error) {
	items, err := a.store.ListPendingApprovals()
	if err != nil {
		return nil, err
	}
	out := make([]store.BuildApproval, 0, len(items))
	for _, item := range items {
		out = append(out, *item)
	}
	return out, nil
}

func (a *ApprovalService) ListPendingDetails() ([]PendingApproval, error) {
	items, err := a.store.ListPendingApprovals()
	if err != nil {
		return nil, err
	}
	result := make([]PendingApproval, 0, len(items))
	for _, approval := range items {
		policy, build, project, err := a.policyForBuild(approval.BuildID)
		if err != nil {
			return nil, err
		}
		result = append(result, PendingApproval{
			BuildApproval:  *approval,
			ProjectID:      project.ID,
			ProjectName:    project.Name,
			BuildNumber:    build.Number,
			Branch:         build.Branch,
			Trigger:        build.Trigger,
			Prompt:         policy.Prompt,
			RequiredRoles:  append([]string(nil), policy.RequiredRoles...),
			AllowRequester: policy.RequesterCanResolve(),
		})
	}
	return result, nil
}

func ApprovalPolicySummary(policy ApprovalPolicy) string {
	if policy.Strategy == ApprovalStrategyNone {
		return ApprovalStrategyNone
	}
	return fmt.Sprintf("%s:%s", policy.Strategy, strings.Join(policy.RequiredRoles, ","))
}

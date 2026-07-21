package engine

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/neko233-com/buildworld/internal/store"
)

func newApprovalTestStore(t *testing.T) *store.Store {
	t.Helper()
	data, err := store.New(filepath.Join(t.TempDir(), "approval.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = data.Close() })
	return data
}

func boolPointer(value bool) *bool {
	return &value
}

func TestApprovalPolicyParsesSupportedProjectConfigFormats(t *testing.T) {
	tests := map[string]string{
		"yaml": `approval:
  version: 1
  strategy: single
  required_roles:
    - admin
    - developer
  allow_requester: false
  prompt: Release to production
jobs:
  validate:
    steps: [{run: echo approved}]
`,
		"typescript": `import { definePipeline, shell, stage } from "@buildworld/pipeline"
export default definePipeline({
  approval: { version: 1, strategy: "single", required_roles: ["admin", "developer"], allow_requester: false, prompt: "Release to production" },
  stages: [stage("Validate", shell("Echo", "echo approved"))],
})`,
	}

	for name, source := range tests {
		t.Run(name, func(t *testing.T) {
			config, err := ParsePipelineConfig(source)
			if err != nil {
				t.Fatal(err)
			}
			policy, err := ResolveApprovalPolicy(config)
			if err != nil {
				t.Fatal(err)
			}
			if policy.Strategy != ApprovalStrategySingle ||
				policy.RequesterCanResolve() ||
				len(policy.RequiredRoles) != 2 ||
				policy.Prompt != "Release to production" {
				t.Fatalf("policy = %#v", policy)
			}
		})
	}
}

func TestApprovalLifecycleEnforcesRoleAndRequesterPolicy(t *testing.T) {
	data := newApprovalTestStore(t)
	requester, err := data.CreateUser("requester", "requester@example.test", "hash", "admin")
	if err != nil {
		t.Fatal(err)
	}
	admin, err := data.CreateUser("approver", "approver@example.test", "hash", "admin")
	if err != nil {
		t.Fatal(err)
	}
	developer, err := data.CreateUser("developer", "developer@example.test", "hash", "developer")
	if err != nil {
		t.Fatal(err)
	}
	project, err := data.CreateProject(
		"release",
		"requires independent approval",
		"",
		"git",
		"main",
		"approval:\n  version: 1\n  strategy: single\n  required_roles: [admin]\n  allow_requester: false\n  prompt: Ship release?\njobs:\n  approve:\n    steps:\n      - run: echo approved\n",
		requester.ID,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	build, err := data.CreateBuild(project.ID, 1, "manual", "main", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := data.CreateBuildQueueItem(build.ID, project.ID, project.Name, 0, build.Trigger, build.Branch); err != nil {
		t.Fatal(err)
	}
	config, err := ParsePipelineConfig(project.Config)
	if err != nil {
		t.Fatal(err)
	}
	service := NewApprovalService(data)
	required, err := service.RequestIfRequired(build.ID, requester.ID, config)
	if err != nil {
		t.Fatal(err)
	}
	if !required {
		t.Fatal("approval should be required")
	}

	pending, err := service.ListPendingDetails()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 ||
		pending[0].ProjectName != project.Name ||
		pending[0].Prompt != "Ship release?" ||
		pending[0].AllowRequester {
		t.Fatalf("pending approval = %#v", pending)
	}
	if err := service.Approve(build.ID, developer.ID, "developer approve"); err == nil || !strings.Contains(err.Error(), "role") {
		t.Fatalf("developer approval error = %v, want role denial", err)
	}
	if err := service.Approve(build.ID, requester.ID, "self approve"); err == nil || !strings.Contains(err.Error(), "own build") {
		t.Fatalf("requester approval error = %v, want self-approval denial", err)
	}
	if err := service.Approve(build.ID, admin.ID, "reviewed"); err != nil {
		t.Fatal(err)
	}

	approvedBuild, err := data.GetBuild(build.ID)
	if err != nil {
		t.Fatal(err)
	}
	if approvedBuild.Status != "pending" || approvedBuild.ApprovedBy == nil || *approvedBuild.ApprovedBy != admin.ID {
		t.Fatalf("approved build = %#v", approvedBuild)
	}
	approval, err := data.GetBuildApprovalByBuild(build.ID)
	if err != nil {
		t.Fatal(err)
	}
	if approval.Status != "approved" ||
		approval.ResolvedBy == nil ||
		*approval.ResolvedBy != admin.ID ||
		approval.ResolvedByUsername != admin.Username ||
		approval.Comment != "reviewed" {
		t.Fatalf("approval record = %#v", approval)
	}
	queue, err := data.ListBuildQueue("")
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != 1 || queue[0].Status != "queued" {
		t.Fatalf("queue = %#v", queue)
	}
}

func TestApprovalRejectTerminatesBuild(t *testing.T) {
	data := newApprovalTestStore(t)
	admin, err := data.CreateUser("admin", "admin@example.test", "hash", "admin")
	if err != nil {
		t.Fatal(err)
	}
	project, err := data.CreateProject(
		"protected",
		"",
		"",
		"git",
		"main",
		"approval:\n  strategy: single\njobs:\n  approve:\n    steps:\n      - run: echo approved\n",
		admin.ID,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	build, err := data.CreateBuild(project.ID, 1, "manual", "main", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := data.CreateBuildQueueItem(build.ID, project.ID, project.Name, 0, build.Trigger, build.Branch); err != nil {
		t.Fatal(err)
	}
	service := NewApprovalService(data)
	config, err := ParsePipelineConfig(project.Config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RequestIfRequired(build.ID, admin.ID, config); err != nil {
		t.Fatal(err)
	}
	if err := service.Reject(build.ID, admin.ID, "unsafe release"); err != nil {
		t.Fatal(err)
	}
	rejected, err := data.GetBuild(build.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Status != "rejected" {
		t.Fatalf("status = %q, want rejected", rejected.Status)
	}
}

func TestCancellingPendingApprovalRemovesItFromInbox(t *testing.T) {
	data := newApprovalTestStore(t)
	admin, err := data.CreateUser("canceller", "canceller@example.test", "hash", "admin")
	if err != nil {
		t.Fatal(err)
	}
	project, err := data.CreateProject(
		"cancel-protected",
		"",
		"",
		"git",
		"main",
		"approval:\n  strategy: single\njobs:\n  approve:\n    steps:\n      - run: echo approved\n",
		admin.ID,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	build, err := data.CreateBuild(project.ID, 1, "manual", "main", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	service := NewApprovalService(data)
	config, err := ParsePipelineConfig(project.Config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RequestIfRequired(build.ID, admin.ID, config); err != nil {
		t.Fatal(err)
	}
	if err := data.CancelBuild(build.ID); err != nil {
		t.Fatal(err)
	}
	if err := data.CancelPendingBuildApproval(build.ID, admin.ID, admin.Username); err != nil {
		t.Fatal(err)
	}
	pending, err := service.ListPendingDetails()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending approvals = %#v, want empty", pending)
	}
	approval, err := data.GetBuildApprovalByBuild(build.ID)
	if err != nil {
		t.Fatal(err)
	}
	if approval.Status != "cancelled" || approval.ResolvedByUsername != admin.Username {
		t.Fatalf("cancelled approval = %#v", approval)
	}
}

func TestApprovalPolicyRejectsUnknownStrategyAndRole(t *testing.T) {
	if _, err := ResolveApprovalPolicy(&BuildConfig{Approval: &ApprovalPolicy{Strategy: "quorum"}}); err == nil {
		t.Fatal("unknown strategy should fail")
	}
	if _, err := ResolveApprovalPolicy(&BuildConfig{Approval: &ApprovalPolicy{
		Strategy:       ApprovalStrategySingle,
		RequiredRoles:  []string{"viewer"},
		AllowRequester: boolPointer(true),
	}}); err == nil {
		t.Fatal("unknown approval role should fail")
	}
}

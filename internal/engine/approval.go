package engine

import (
	"fmt"

	"github.com/neko233-com/buildworld233/internal/store"
)

// ApprovalService 管理构建审批生命周期
type ApprovalService struct {
	store *store.Store
}

func NewApprovalService(s *store.Store) *ApprovalService {
	return &ApprovalService{store: s}
}

// RequestApproval 把 build 标记为需要审批，并创建一条 pending 审批记录。
func (a *ApprovalService) RequestApproval(buildID, userID int64) error {
	build, err := a.store.GetBuild(buildID)
	if err != nil {
		return fmt.Errorf("build not found: %w", err)
	}
	user, err := a.store.GetUser(userID)
	if err != nil {
		return fmt.Errorf("user not found: %w", err)
	}
	if err := a.store.SetBuildApprovalRequired(buildID, true); err != nil {
		return err
	}
	if build.Status == "pending" || build.Status == "" {
		_ = a.store.UpdateBuildStatus(buildID, "pending_approval")
	}
	_, err = a.store.CreateBuildApproval(buildID, userID, user.Username)
	return err
}

// Approve 通过审批，标记 build.approved_by/approved_at，并把状态改回 pending 以便执行。
func (a *ApprovalService) Approve(buildID, userID int64, comment string) error {
	approval, err := a.store.GetBuildApprovalByBuild(buildID)
	if err != nil {
		return fmt.Errorf("approval record not found: %w", err)
	}
	if approval.Status != "pending" {
		return fmt.Errorf("approval already %s", approval.Status)
	}
	if err := a.store.UpdateBuildApproval(approval.ID, "approved", comment); err != nil {
		return err
	}
	if err := a.store.SetBuildApproved(buildID, userID); err != nil {
		return err
	}
	return a.store.UpdateBuildStatus(buildID, "pending")
}

// Reject 拒绝审批，把 build 状态设为 rejected。
func (a *ApprovalService) Reject(buildID, userID int64, comment string) error {
	approval, err := a.store.GetBuildApprovalByBuild(buildID)
	if err != nil {
		return fmt.Errorf("approval record not found: %w", err)
	}
	if approval.Status != "pending" {
		return fmt.Errorf("approval already %s", approval.Status)
	}
	if err := a.store.UpdateBuildApproval(approval.ID, "rejected", comment); err != nil {
		return err
	}
	_ = userID // 拒绝者不写入 build 字段，仅记入审批记录
	return a.store.UpdateBuildStatus(buildID, "rejected")
}

// ListPending 返回所有 pending 审批。
func (a *ApprovalService) ListPending() ([]store.BuildApproval, error) {
	items, err := a.store.ListPendingApprovals()
	if err != nil {
		return nil, err
	}
	out := make([]store.BuildApproval, 0, len(items))
	for _, x := range items {
		out = append(out, *x)
	}
	return out, nil
}

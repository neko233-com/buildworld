package engine

import (
	"fmt"
	"strings"
)

const (
	ApprovalPolicyVersion  = 1
	ApprovalStrategyNone   = "none"
	ApprovalStrategySingle = "single"
)

// ApprovalPolicy is intentionally versioned and strategy-based so additional
// policies (multiple approvers, quorum, external checks) can be added without
// changing the top-level build configuration shape.
type ApprovalPolicy struct {
	Version        int      `json:"version,omitempty" yaml:"version,omitempty"`
	Strategy       string   `json:"strategy,omitempty" yaml:"strategy,omitempty"`
	RequiredRoles  []string `json:"required_roles,omitempty" yaml:"required_roles,omitempty"`
	AllowRequester *bool    `json:"allow_requester,omitempty" yaml:"allow_requester,omitempty"`
	Prompt         string   `json:"prompt,omitempty" yaml:"prompt,omitempty"`
}

func (p ApprovalPolicy) RequesterCanResolve() bool {
	return p.AllowRequester == nil || *p.AllowRequester
}

func ResolveApprovalPolicy(config *BuildConfig) (ApprovalPolicy, error) {
	policy := ApprovalPolicy{
		Version:       ApprovalPolicyVersion,
		Strategy:      ApprovalStrategyNone,
		RequiredRoles: []string{"admin"},
	}
	if config == nil || config.Approval == nil {
		return policy, nil
	}
	policy = *config.Approval
	if policy.Version == 0 {
		policy.Version = ApprovalPolicyVersion
	}
	if policy.Version > ApprovalPolicyVersion {
		return ApprovalPolicy{}, fmt.Errorf("approval policy version %d is newer than supported version %d", policy.Version, ApprovalPolicyVersion)
	}
	policy.Strategy = strings.ToLower(strings.TrimSpace(policy.Strategy))
	if policy.Strategy == "" {
		policy.Strategy = ApprovalStrategyNone
	}
	switch policy.Strategy {
	case ApprovalStrategyNone, ApprovalStrategySingle:
	default:
		return ApprovalPolicy{}, fmt.Errorf("unsupported approval strategy %q", policy.Strategy)
	}
	if policy.Strategy == ApprovalStrategySingle && len(policy.RequiredRoles) == 0 {
		policy.RequiredRoles = []string{"admin"}
	}
	seen := make(map[string]struct{}, len(policy.RequiredRoles))
	roles := make([]string, 0, len(policy.RequiredRoles))
	for _, role := range policy.RequiredRoles {
		role = strings.ToLower(strings.TrimSpace(role))
		if role != "admin" && role != "developer" {
			return ApprovalPolicy{}, fmt.Errorf("unsupported approval role %q", role)
		}
		if _, exists := seen[role]; exists {
			continue
		}
		seen[role] = struct{}{}
		roles = append(roles, role)
	}
	policy.RequiredRoles = roles
	policy.Prompt = strings.TrimSpace(policy.Prompt)
	return policy, nil
}

func ApprovalRequired(config *BuildConfig) (bool, error) {
	policy, err := ResolveApprovalPolicy(config)
	return policy.Strategy != ApprovalStrategyNone, err
}

func ApprovalRoleAllowed(policy ApprovalPolicy, role string) bool {
	role = strings.ToLower(strings.TrimSpace(role))
	for _, allowed := range policy.RequiredRoles {
		if role == allowed {
			return true
		}
	}
	return false
}

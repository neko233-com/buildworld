package store

import "time"

type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	AvatarURL    string    `json:"avatar_url,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	LastLogin    time.Time `json:"last_login,omitempty"`
}

type SSHKey struct {
	ID          int64     `json:"id"`
	UserID      int64     `json:"user_id"`
	Name        string    `json:"name"`
	PublicKey   string    `json:"public_key"`
	Fingerprint string    `json:"fingerprint"`
	CreatedAt   time.Time `json:"created_at"`
}

type Project struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description,omitempty"`
	RepoURL       string    `json:"repo_url"`
	RepoType      string    `json:"repo_type"`
	DefaultBranch string    `json:"default_branch"`
	VCSRootID     *int64    `json:"vcs_root_id,omitempty"`
	TemplateID    *int64    `json:"template_id,omitempty"`
	GroupID       *int64    `json:"group_id,omitempty"`
	Tags          []string  `json:"tags"`
	Config        string    `json:"config"`
	CreatedBy     int64     `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ProjectSummary is the list-safe project representation. Pipeline config is
// deliberately absent because a single definition may be hundreds of
// kilobytes; callers that edit or run a parameterized build fetch GetProject.
type ProjectSummary struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description,omitempty"`
	RepoURL       string    `json:"repo_url"`
	RepoType      string    `json:"repo_type"`
	DefaultBranch string    `json:"default_branch"`
	VCSRootID     *int64    `json:"vcs_root_id,omitempty"`
	TemplateID    *int64    `json:"template_id,omitempty"`
	GroupID       *int64    `json:"group_id,omitempty"`
	Tags          []string  `json:"tags"`
	CreatedBy     int64     `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Build struct {
	ID               int64          `json:"id"`
	ProjectID        int64          `json:"project_id"`
	Number           int            `json:"number"`
	Status           string         `json:"status"`
	Trigger          string         `json:"trigger"`
	Branch           string         `json:"branch,omitempty"`
	CommitSHA        string         `json:"commit_sha,omitempty"`
	Parameters       string         `json:"parameters,omitempty"`
	WaitDependencyOn *int64         `json:"wait_dependency_on,omitempty"`
	RetriedFrom      *int64         `json:"retried_from,omitempty"`
	Pinned           bool           `json:"pinned"`
	StartedAt        *time.Time     `json:"started_at,omitempty"`
	FinishedAt       *time.Time     `json:"finished_at,omitempty"`
	DurationMs       *int64         `json:"duration_ms,omitempty"`
	Log              string         `json:"log,omitempty"`
	ApprovalRequired bool           `json:"approval_required,omitempty"`
	ApprovedBy       *int64         `json:"approved_by,omitempty"`
	ApprovedAt       *time.Time     `json:"approved_at,omitempty"`
	TimeoutSec       int            `json:"timeout_sec,omitempty"`
	TestResultID     *int64         `json:"test_result_id,omitempty"`
	Approval         *BuildApproval `json:"approval,omitempty"`
}

type EnvVar struct {
	ID          int64  `json:"id"`
	Scope       string `json:"scope"` // global, project
	ProjectID   *int64 `json:"project_id,omitempty"`
	Name        string `json:"name"`
	Value       string `json:"value"`
	IsSecret    bool   `json:"is_secret"`
	Description string `json:"description,omitempty"`
}

type Artifact struct {
	ID          int64     `json:"id"`
	BuildID     int64     `json:"build_id"`
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	Size        int64     `json:"size"`
	SHA256      string    `json:"sha256,omitempty"`
	ContentType string    `json:"content_type,omitempty"`
	Downloads   int       `json:"downloads"`
	CreatedAt   time.Time `json:"created_at"`
}

type Worker struct {
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	Address             string    `json:"address"`
	TokenHash           string    `json:"-"`
	Labels              string    `json:"labels"`
	Pool                string    `json:"pool,omitempty"`
	MaxConcurrentBuilds int       `json:"max_concurrent_builds"`
	ActiveBuilds        int       `json:"active_builds"`
	Status              string    `json:"status"`
	LastHeartbeat       time.Time `json:"last_heartbeat"`
	CreatedAt           time.Time `json:"created_at"`
}

type Plugin struct {
	ID             int64     `json:"id"`
	Name           string    `json:"name"`
	Version        string    `json:"version"`
	Description    string    `json:"description,omitempty"`
	Author         string    `json:"author,omitempty"`
	Enabled        bool      `json:"enabled"`
	Config         string    `json:"config,omitempty"`
	ScriptLang     string    `json:"script_lang"`
	SourceScript   string    `json:"source_script,omitempty"`
	SourceUIScript string    `json:"source_ui_script,omitempty"`
	Path           string    `json:"path,omitempty"`
	Source         string    `json:"source"`
	Steps          string    `json:"steps,omitempty"`
	Triggers       string    `json:"triggers,omitempty"`
	UIExtensions   string    `json:"ui_extensions,omitempty"`
	InstalledAt    time.Time `json:"installed_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type CredentialType string

const (
	CredentialTypeSSHKey CredentialType = "ssh_key"
	CredentialTypeGit    CredentialType = "git"
	CredentialTypeSVN    CredentialType = "svn"
	CredentialTypeHG     CredentialType = "hg"
)

type Credential struct {
	ID          int64          `json:"id"`
	Name        string         `json:"name"`
	Type        CredentialType `json:"type"`
	Host        string         `json:"host"`
	Username    string         `json:"username,omitempty"`
	Password    string         `json:"password,omitempty"`
	PrivateKey  string         `json:"private_key,omitempty"`
	PublicKey   string         `json:"public_key,omitempty"`
	Token       string         `json:"token,omitempty"`
	Description string         `json:"description,omitempty"`
	IsSecret    bool           `json:"is_secret"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type VCSRoot struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	Type         string    `json:"type"`
	URL          string    `json:"url"`
	Branch       string    `json:"branch"`
	CredentialID *int64    `json:"credential_id,omitempty"`
	PollInterval int       `json:"poll_interval"`
	AutoCheckout bool      `json:"auto_checkout"`
	Config       string    `json:"config,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type BuildTemplate struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Config      string    `json:"config"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type NotificationChannelType string

const (
	NotificationChannelWeb      NotificationChannelType = "web"
	NotificationChannelEmail    NotificationChannelType = "email"
	NotificationChannelFeishu   NotificationChannelType = "feishu"
	NotificationChannelWebhook  NotificationChannelType = "webhook"
	NotificationChannelDiscord  NotificationChannelType = "discord"
	NotificationChannelWeCom    NotificationChannelType = "wecom"
	NotificationChannelTelegram NotificationChannelType = "telegram"
)

const DefaultWebNotificationChannelName = "页面内通知"

type NotificationChannel struct {
	ID          int64                   `json:"id"`
	Name        string                  `json:"name"`
	Type        NotificationChannelType `json:"type"`
	Config      string                  `json:"config"`
	Enabled     bool                    `json:"enabled"`
	Conditions  string                  `json:"conditions"`
	Description string                  `json:"description,omitempty"`
	CreatedAt   time.Time               `json:"created_at"`
	UpdatedAt   time.Time               `json:"updated_at"`
}

type NotificationEvent struct {
	ID           int64      `json:"id"`
	ChannelID    int64      `json:"channel_id"`
	BuildID      *int64     `json:"build_id,omitempty"`
	EventType    string     `json:"event_type"`
	Payload      string     `json:"payload"`
	Status       string     `json:"status"`
	ErrorMessage string     `json:"error_message,omitempty"`
	DeliveredAt  *time.Time `json:"delivered_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

type InAppNotificationFeed struct {
	Items      []*NotificationEvent `json:"items"`
	Unread     int                  `json:"unread_count"`
	LastReadID int64                `json:"last_read_id"`
}

// BuildStat 每日构建统计聚合
type BuildStat struct {
	ID           int64  `json:"id"`
	ProjectID    int64  `json:"project_id"`
	Date         string `json:"date"` // YYYY-MM-DD
	TotalBuilds  int    `json:"total_builds"`
	SuccessCount int    `json:"success_count"`
	FailedCount  int    `json:"failed_count"`
	AvgDuration  int64  `json:"avg_duration_ms"`
}

// AuditLog 操作审计日志
type AuditLog struct {
	ID           int64     `json:"id"`
	UserID       int64     `json:"user_id"`
	Username     string    `json:"username"`
	Action       string    `json:"action"` // create/update/delete/trigger/login
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	Detail       string    `json:"detail,omitempty"`
	IP           string    `json:"ip"`
	CreatedAt    time.Time `json:"created_at"`
}

// APIToken 用户级 API 令牌
type APIToken struct {
	ID          int64      `json:"id"`
	UserID      int64      `json:"user_id"`
	Name        string     `json:"name"`
	TokenHash   string     `json:"-"`
	TokenPrefix string     `json:"token_prefix"` // 前 8 位用于展示
	Scopes      string     `json:"scopes"`       // JSON array
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// BuildApproval 构建审批记录
type BuildApproval struct {
	ID                 int64      `json:"id"`
	BuildID            int64      `json:"build_id"`
	UserID             int64      `json:"user_id"`
	Username           string     `json:"username"`
	Status             string     `json:"status"` // pending/approved/rejected
	Comment            string     `json:"comment,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	ResolvedAt         *time.Time `json:"resolved_at,omitempty"`
	ResolvedBy         *int64     `json:"resolved_by,omitempty"`
	ResolvedByUsername string     `json:"resolved_by_username,omitempty"`
	Prompt             string     `json:"prompt,omitempty"`
	RequiredRoles      []string   `json:"required_roles,omitempty"`
	AllowRequester     bool       `json:"allow_requester"`
}

// TestResult 测试结果汇总
type TestResult struct {
	ID        int64     `json:"id"`
	BuildID   int64     `json:"build_id"`
	Total     int       `json:"total"`
	Passed    int       `json:"passed"`
	Failed    int       `json:"failed"`
	Skipped   int       `json:"skipped"`
	Duration  int64     `json:"duration_ms"`
	ReportXML string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

// DeploymentEnv 部署环境
type DeploymentEnv struct {
	ID          int64     `json:"id"`
	ProjectID   int64     `json:"project_id"`
	Name        string    `json:"name"` // dev/staging/production
	Description string    `json:"description,omitempty"`
	Config      string    `json:"config"` // JSON 配置
	LastBuildID *int64    `json:"last_build_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ProjectGroup 项目分组（支持层级）
type ProjectGroup struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	ParentID    *int64    `json:"parent_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// BuildQueueItem 构建队列项
type BuildQueueItem struct {
	ID                    int64      `json:"id"`
	BuildID               int64      `json:"build_id"`
	BuildNumber           int        `json:"build_number"`
	ProjectID             int64      `json:"project_id"`
	ProjectName           string     `json:"project_name"`
	Priority              int        `json:"priority"`
	Status                string     `json:"status"` // queued/running/pending_approval/cancelled
	Trigger               string     `json:"trigger"`
	Branch                string     `json:"branch"`
	QueuedAt              time.Time  `json:"queued_at"`
	StartedAt             *time.Time `json:"started_at,omitempty"`
	QueuePosition         int        `json:"queue_position"`
	WaitReason            string     `json:"wait_reason"`
	WaitingForBuildID     *int64     `json:"waiting_for_build_id,omitempty"`
	WaitingForBuildNumber *int       `json:"waiting_for_build_number,omitempty"`
}

type GitHookEvent string

const (
	GitHookPush         GitHookEvent = "push"
	GitHookTag          GitHookEvent = "tag_push"
	GitHookPullRequest  GitHookEvent = "pull_request"
	GitHookMergeRequest GitHookEvent = "merge_request"
	GitHookRelease      GitHookEvent = "release"
)

type GitHook struct {
	ID          int64        `json:"id"`
	ProjectID   int64        `json:"project_id"`
	Name        string       `json:"name"`
	Event       GitHookEvent `json:"event"`
	Branch      string       `json:"branch"`
	Secret      string       `json:"secret,omitempty"`
	Enabled     bool         `json:"enabled"`
	BuildParams string       `json:"build_params"`
	Description string       `json:"description,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

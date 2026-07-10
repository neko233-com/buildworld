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
	Config        string    `json:"config"`
	CreatedBy     int64     `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Build struct {
	ID               int64      `json:"id"`
	ProjectID        int64      `json:"project_id"`
	Number           int        `json:"number"`
	Status           string     `json:"status"`
	Trigger          string     `json:"trigger"`
	Branch           string     `json:"branch,omitempty"`
	CommitSHA        string     `json:"commit_sha,omitempty"`
	Parameters       string     `json:"parameters,omitempty"`
	WaitDependencyOn *int64     `json:"wait_dependency_on,omitempty"`
	RetriedFrom      *int64     `json:"retried_from,omitempty"`
	Pinned           bool       `json:"pinned"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	FinishedAt       *time.Time `json:"finished_at,omitempty"`
	DurationMs       *int64     `json:"duration_ms,omitempty"`
	Log              string     `json:"log,omitempty"`
}

type EnvVar struct {
	ID          int64  `json:"id"`
	Scope       string `json:"scope"`       // global, project
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
	Status              string    `json:"status"`
	LastHeartbeat       time.Time `json:"last_heartbeat"`
	CreatedAt           time.Time `json:"created_at"`
}

type Plugin struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	Description string    `json:"description,omitempty"`
	Enabled     bool      `json:"enabled"`
	Config      string    `json:"config,omitempty"`
	InstalledAt time.Time `json:"installed_at"`
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
	NotificationChannelEmail    NotificationChannelType = "email"
	NotificationChannelFeishu   NotificationChannelType = "feishu"
	NotificationChannelWebhook  NotificationChannelType = "webhook"
)

type NotificationChannel struct {
	ID          int64                      `json:"id"`
	Name        string                     `json:"name"`
	Type        NotificationChannelType    `json:"type"`
	Config      string                     `json:"config"`
	Enabled     bool                       `json:"enabled"`
	Conditions  string                     `json:"conditions"`
	Description string                     `json:"description,omitempty"`
	CreatedAt   time.Time                  `json:"created_at"`
	UpdatedAt   time.Time                  `json:"updated_at"`
}

type NotificationEvent struct {
	ID              int64     `json:"id"`
	ChannelID       int64     `json:"channel_id"`
	BuildID         *int64    `json:"build_id,omitempty"`
	EventType       string    `json:"event_type"`
	Payload         string    `json:"payload"`
	Status          string    `json:"status"`
	ErrorMessage    string    `json:"error_message,omitempty"`
	DeliveredAt     *time.Time `json:"delivered_at,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

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
	Config        string    `json:"config"`
	CreatedBy     int64     `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Build struct {
	ID         int64      `json:"id"`
	ProjectID  int64      `json:"project_id"`
	Number     int        `json:"number"`
	Status     string     `json:"status"`
	Trigger    string     `json:"trigger"`
	Branch     string     `json:"branch,omitempty"`
	CommitSHA  string     `json:"commit_sha,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	DurationMs *int64     `json:"duration_ms,omitempty"`
	Log        string     `json:"log,omitempty"`
}

type Artifact struct {
	ID        int64     `json:"id"`
	BuildID   int64     `json:"build_id"`
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

type Worker struct {
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	Address             string    `json:"address"`
	TokenHash           string    `json:"-"`
	Labels              string    `json:"labels"`
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

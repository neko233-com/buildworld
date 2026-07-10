package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "github.com/glebarez/go-sqlite"
)

type Store struct {
	db *sql.DB
}

func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(1)

	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) migrate() error {
	for _, m := range migrations {
		if _, err := s.db.Exec(m); err != nil {
			if strings.Contains(err.Error(), "duplicate column name") {
				continue
			}
			return fmt.Errorf("migration: %w", err)
		}
	}
	return nil
}

func (s *Store) Close() error { return s.db.Close() }
func (s *Store) DB() *sql.DB  { return s.db }

// ---------------- Users ----------------

func (s *Store) CreateUser(username, email, passwordHash, role string) (*User, error) {
	res, err := s.db.Exec(
		"INSERT INTO users (username, email, password_hash, role) VALUES (?, ?, ?, ?)",
		username, email, passwordHash, role,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetUser(id)
}

func (s *Store) GetUser(id int64) (*User, error) {
	u := &User{}
	var avatar sql.NullString
	var lastLogin sql.NullTime
	err := s.db.QueryRow(
		"SELECT id, username, email, password_hash, role, avatar_url, created_at, last_login FROM users WHERE id = ?",
		id,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role, &avatar, &u.CreatedAt, &lastLogin)
	if err != nil {
		return nil, err
	}
	if avatar.Valid {
		u.AvatarURL = avatar.String
	}
	if lastLogin.Valid {
		u.LastLogin = lastLogin.Time
	}
	return u, nil
}

func (s *Store) GetUserByUsername(username string) (*User, error) {
	u := &User{}
	var avatar sql.NullString
	var lastLogin sql.NullTime
	err := s.db.QueryRow(
		"SELECT id, username, email, password_hash, role, avatar_url, created_at, last_login FROM users WHERE username = ?",
		username,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role, &avatar, &u.CreatedAt, &lastLogin)
	if err != nil {
		return nil, err
	}
	if avatar.Valid {
		u.AvatarURL = avatar.String
	}
	if lastLogin.Valid {
		u.LastLogin = lastLogin.Time
	}
	return u, nil
}

func (s *Store) ListUsers() ([]*User, error) {
	rows, err := s.db.Query("SELECT id, username, email, password_hash, role, avatar_url, created_at, last_login FROM users ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []*User
	for rows.Next() {
		u := &User{}
		var avatar sql.NullString
		var lastLogin sql.NullTime
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role, &avatar, &u.CreatedAt, &lastLogin); err != nil {
			return nil, err
		}
		if avatar.Valid {
			u.AvatarURL = avatar.String
		}
		if lastLogin.Valid {
			u.LastLogin = lastLogin.Time
		}
		users = append(users, u)
	}
	return users, nil
}

func (s *Store) UpdateUserPassword(id int64, passwordHash string) error {
	_, err := s.db.Exec("UPDATE users SET password_hash = ? WHERE id = ?", passwordHash, id)
	return err
}

func (s *Store) UpdateUserRole(id int64, role string) error {
	_, err := s.db.Exec("UPDATE users SET role = ? WHERE id = ?", role, id)
	return err
}

func (s *Store) UpdateLastLogin(id int64) error {
	_, err := s.db.Exec("UPDATE users SET last_login = ? WHERE id = ?", time.Now(), id)
	return err
}

func (s *Store) DeleteUser(id int64) error {
	_, err := s.db.Exec("DELETE FROM users WHERE id = ?", id)
	return err
}

// ---------------- Projects ----------------

func (s *Store) CreateProject(name, description, repoURL, repoType, defaultBranch, config string, createdBy int64, vcsRootID, templateID *int64) (*Project, error) {
	if defaultBranch == "" {
		defaultBranch = "main"
	}
	res, err := s.db.Exec(
		"INSERT INTO projects (name, description, repo_url, repo_type, default_branch, vcs_root_id, template_id, config, created_by) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		name, description, repoURL, repoType, defaultBranch, vcsRootID, templateID, config, createdBy,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetProject(id)
}

func (s *Store) GetProject(id int64) (*Project, error) {
	p := &Project{}
	var vcsRootID, templateID sql.NullInt64
	err := s.db.QueryRow(
		"SELECT id, name, description, repo_url, repo_type, default_branch, vcs_root_id, template_id, config, created_by, created_at, updated_at FROM projects WHERE id = ?",
		id,
	).Scan(&p.ID, &p.Name, &p.Description, &p.RepoURL, &p.RepoType, &p.DefaultBranch, &vcsRootID, &templateID, &p.Config, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if vcsRootID.Valid {
		v := vcsRootID.Int64
		p.VCSRootID = &v
	}
	if templateID.Valid {
		v := templateID.Int64
		p.TemplateID = &v
	}
	return p, nil
}

func (s *Store) GetProjectByName(name string) (*Project, error) {
	p := &Project{}
	var vcsRootID, templateID sql.NullInt64
	err := s.db.QueryRow(
		"SELECT id, name, description, repo_url, repo_type, default_branch, vcs_root_id, template_id, config, created_by, created_at, updated_at FROM projects WHERE name = ?",
		name,
	).Scan(&p.ID, &p.Name, &p.Description, &p.RepoURL, &p.RepoType, &p.DefaultBranch, &vcsRootID, &templateID, &p.Config, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if vcsRootID.Valid {
		v := vcsRootID.Int64
		p.VCSRootID = &v
	}
	if templateID.Valid {
		v := templateID.Int64
		p.TemplateID = &v
	}
	return p, nil
}

func (s *Store) ListProjects() ([]*Project, error) {
	rows, err := s.db.Query("SELECT id, name, description, repo_url, repo_type, default_branch, vcs_root_id, template_id, config, created_by, created_at, updated_at FROM projects ORDER BY id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var projects []*Project
	for rows.Next() {
		p := &Project{}
		var vcsRootID, templateID sql.NullInt64
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.RepoURL, &p.RepoType, &p.DefaultBranch, &vcsRootID, &templateID, &p.Config, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		if vcsRootID.Valid {
			v := vcsRootID.Int64
			p.VCSRootID = &v
		}
		if templateID.Valid {
			v := templateID.Int64
			p.TemplateID = &v
		}
		projects = append(projects, p)
	}
	return projects, nil
}

func (s *Store) ListProjectsByVCSRoot(vcsRootID int64) ([]*Project, error) {
	rows, err := s.db.Query("SELECT id, name, description, repo_url, repo_type, default_branch, vcs_root_id, template_id, config, created_by, created_at, updated_at FROM projects WHERE vcs_root_id = ?", vcsRootID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var projects []*Project
	for rows.Next() {
		p := &Project{}
		var vrid, tid sql.NullInt64
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.RepoURL, &p.RepoType, &p.DefaultBranch, &vrid, &tid, &p.Config, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		if vrid.Valid {
			v := vrid.Int64
			p.VCSRootID = &v
		}
		if tid.Valid {
			v := tid.Int64
			p.TemplateID = &v
		}
		projects = append(projects, p)
	}
	return projects, nil
}

func (s *Store) UpdateProject(id int64, name, description, repoURL, repoType, defaultBranch, config string, vcsRootID, templateID *int64) error {
	_, err := s.db.Exec(
		"UPDATE projects SET name=?, description=?, repo_url=?, repo_type=?, default_branch=?, vcs_root_id=?, template_id=?, config=?, updated_at=? WHERE id=?",
		name, description, repoURL, repoType, defaultBranch, vcsRootID, templateID, config, time.Now(), id,
	)
	return err
}

func (s *Store) DeleteProject(id int64) error {
	_, err := s.db.Exec("DELETE FROM builds WHERE project_id = ?", id)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("DELETE FROM projects WHERE id = ?", id)
	return err
}

// ---------------- Builds ----------------

func (s *Store) CreateBuild(projectID int64, number int, trigger, branch, commitSHA, parameters string, waitDependencyOn, retriedFrom *int64) (*Build, error) {
	res, err := s.db.Exec(
		"INSERT INTO builds (project_id, number, status, trigger, branch, commit_sha, parameters, wait_dependency_on, retried_from) VALUES (?, ?, 'pending', ?, ?, ?, ?, ?, ?)",
		projectID, number, trigger, branch, commitSHA, parameters, waitDependencyOn, retriedFrom,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetBuild(id)
}

func (s *Store) GetBuild(id int64) (*Build, error) {
	b := &Build{}
	var branch, commit, params, log sql.NullString
	var started, finished sql.NullTime
	var dur sql.NullInt64
	var waitDep, retriedFrom sql.NullInt64
	var pinned sql.NullBool
	err := s.db.QueryRow(
		"SELECT id, project_id, number, status, trigger, branch, commit_sha, parameters, wait_dependency_on, retried_from, pinned, log, started_at, finished_at, duration_ms FROM builds WHERE id = ?",
		id,
	).Scan(&b.ID, &b.ProjectID, &b.Number, &b.Status, &b.Trigger, &branch, &commit, &params, &waitDep, &retriedFrom, &pinned, &log, &started, &finished, &dur)
	if err != nil {
		return nil, err
	}
	b.Branch, b.CommitSHA, b.Parameters, b.Log = branch.String, commit.String, params.String, log.String
	if started.Valid {
		b.StartedAt = &started.Time
	}
	if finished.Valid {
		b.FinishedAt = &finished.Time
	}
	if dur.Valid {
		v := dur.Int64
		b.DurationMs = &v
	}
	if waitDep.Valid {
		v := waitDep.Int64
		b.WaitDependencyOn = &v
	}
	if retriedFrom.Valid {
		v := retriedFrom.Int64
		b.RetriedFrom = &v
	}
	if pinned.Valid {
		b.Pinned = pinned.Bool
	}
	return b, nil
}

func (s *Store) GetBuildByNumber(projectID int64, number int) (*Build, error) {
	b := &Build{}
	var branch, commit, params, log sql.NullString
	var started, finished sql.NullTime
	var dur sql.NullInt64
	var waitDep, retriedFrom sql.NullInt64
	var pinned sql.NullBool
	err := s.db.QueryRow(
		"SELECT id, project_id, number, status, trigger, branch, commit_sha, parameters, wait_dependency_on, retried_from, pinned, log, started_at, finished_at, duration_ms FROM builds WHERE project_id = ? AND number = ?",
		projectID, number,
	).Scan(&b.ID, &b.ProjectID, &b.Number, &b.Status, &b.Trigger, &branch, &commit, &params, &waitDep, &retriedFrom, &pinned, &log, &started, &finished, &dur)
	if err != nil {
		return nil, err
	}
	b.Branch, b.CommitSHA, b.Parameters, b.Log = branch.String, commit.String, params.String, log.String
	if started.Valid {
		b.StartedAt = &started.Time
	}
	if finished.Valid {
		b.FinishedAt = &finished.Time
	}
	if dur.Valid {
		v := dur.Int64
		b.DurationMs = &v
	}
	if waitDep.Valid {
		v := waitDep.Int64
		b.WaitDependencyOn = &v
	}
	if retriedFrom.Valid {
		v := retriedFrom.Int64
		b.RetriedFrom = &v
	}
	if pinned.Valid {
		b.Pinned = pinned.Bool
	}
	return b, nil
}

func (s *Store) ListBuilds(limit int) ([]*Build, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(
		"SELECT id, project_id, number, status, trigger, branch, commit_sha, parameters, wait_dependency_on, retried_from, pinned, log, started_at, finished_at, duration_ms FROM builds ORDER BY id DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBuilds(rows)
}

func (s *Store) ListBuildsByProject(projectID int64) ([]*Build, error) {
	rows, err := s.db.Query(
		"SELECT id, project_id, number, status, trigger, branch, commit_sha, parameters, wait_dependency_on, retried_from, pinned, log, started_at, finished_at, duration_ms FROM builds WHERE project_id = ? ORDER BY number DESC", projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBuilds(rows)
}

func (s *Store) ListPendingBuilds() ([]*Build, error) {
	rows, err := s.db.Query(
		"SELECT id, project_id, number, status, trigger, branch, commit_sha, parameters, wait_dependency_on, retried_from, pinned, log, started_at, finished_at, duration_ms FROM builds WHERE status = 'pending' ORDER BY id ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBuilds(rows)
}

func scanBuilds(rows *sql.Rows) ([]*Build, error) {
	var builds []*Build
	for rows.Next() {
		b := &Build{}
		var branch, commit, params, log sql.NullString
		var started, finished sql.NullTime
		var dur sql.NullInt64
		var waitDep, retriedFrom sql.NullInt64
		var pinned sql.NullBool
		if err := rows.Scan(&b.ID, &b.ProjectID, &b.Number, &b.Status, &b.Trigger, &branch, &commit, &params, &waitDep, &retriedFrom, &pinned, &log, &started, &finished, &dur); err != nil {
			return nil, err
		}
		b.Branch, b.CommitSHA, b.Parameters, b.Log = branch.String, commit.String, params.String, log.String
		if started.Valid {
			b.StartedAt = &started.Time
		}
		if finished.Valid {
			b.FinishedAt = &finished.Time
		}
		if dur.Valid {
			v := dur.Int64
			b.DurationMs = &v
		}
		if waitDep.Valid {
			v := waitDep.Int64
			b.WaitDependencyOn = &v
		}
		if retriedFrom.Valid {
			v := retriedFrom.Int64
			b.RetriedFrom = &v
		}
		if pinned.Valid {
			b.Pinned = pinned.Bool
		}
		builds = append(builds, b)
	}
	return builds, nil
}

func (s *Store) NextBuildNumber(projectID int64) (int, error) {
	var maxNum sql.NullInt64
	err := s.db.QueryRow("SELECT MAX(number) FROM builds WHERE project_id = ?", projectID).Scan(&maxNum)
	if err != nil {
		return 0, err
	}
	if !maxNum.Valid {
		return 1, nil
	}
	return int(maxNum.Int64) + 1, nil
}

func (s *Store) UpdateBuildStatus(id int64, status string) error {
	_, err := s.db.Exec("UPDATE builds SET status = ? WHERE id = ?", status, id)
	return err
}

func (s *Store) StartBuild(id int64) error {
	_, err := s.db.Exec("UPDATE builds SET status='running', started_at=? WHERE id = ?", time.Now(), id)
	return err
}

func (s *Store) FinishBuild(id int64, status string, durationMs int64) error {
	_, err := s.db.Exec("UPDATE builds SET status=?, finished_at=?, duration_ms=? WHERE id = ?", status, time.Now(), durationMs, id)
	return err
}

func (s *Store) AppendBuildLog(id int64, line string) error {
	_, err := s.db.Exec("UPDATE builds SET log = COALESCE(log, '') || ? WHERE id = ?", line, id)
	return err
}

func (s *Store) SetBuildLog(id int64, log string) error {
	_, err := s.db.Exec("UPDATE builds SET log = ? WHERE id = ?", log, id)
	return err
}

func (s *Store) RetryBuild(buildID int64) (*Build, error) {
	old, err := s.GetBuild(buildID)
	if err != nil {
		return nil, err
	}
	num, err := s.NextBuildNumber(old.ProjectID)
	if err != nil {
		return nil, err
	}
	retriedFrom := buildID
	return s.CreateBuild(old.ProjectID, num, "retry", old.Branch, old.CommitSHA, old.Parameters, nil, &retriedFrom)
}

func (s *Store) PinBuild(buildID int64, pinned bool) error {
	_, err := s.db.Exec("UPDATE builds SET pinned = ? WHERE id = ?", pinned, buildID)
	return err
}

func (s *Store) CancelBuild(buildID int64) error {
	_, err := s.db.Exec("UPDATE builds SET status='cancelled', finished_at=? WHERE id = ?", time.Now(), buildID)
	return err
}

func (s *Store) UpdateBuildBranch(buildID int64, branch string) error {
	_, err := s.db.Exec("UPDATE builds SET branch = ? WHERE id = ?", branch, buildID)
	return err
}

func (s *Store) UpdateBuildCommitSHA(buildID int64, sha string) error {
	_, err := s.db.Exec("UPDATE builds SET commit_sha = ? WHERE id = ?", sha, buildID)
	return err
}

// ---------------- Workers ----------------

func (s *Store) CreateWorker(id, name, address, tokenHash string, labels []string, maxBuilds int, pool string) (*Worker, error) {
	labelsJSON, _ := json.Marshal(labels)
	_, err := s.db.Exec(
		"INSERT INTO workers (id, name, address, token_hash, labels, pool, max_concurrent_builds, status) VALUES (?, ?, ?, ?, ?, ?, ?, 'online')",
		id, name, address, tokenHash, string(labelsJSON), pool, maxBuilds,
	)
	if err != nil {
		_, err = s.db.Exec(
			"UPDATE workers SET name=?, address=?, token_hash=?, labels=?, pool=?, max_concurrent_builds=?, status='online' WHERE id=?",
			name, address, tokenHash, string(labelsJSON), pool, maxBuilds, id,
		)
		if err != nil {
			return nil, err
		}
	}
	return s.GetWorker(id)
}

func (s *Store) GetWorker(id string) (*Worker, error) {
	w := &Worker{}
	var labels, pool sql.NullString
	var hb sql.NullTime
	err := s.db.QueryRow(
		"SELECT id, name, address, token_hash, labels, pool, max_concurrent_builds, status, last_heartbeat, created_at FROM workers WHERE id = ?",
		id,
	).Scan(&w.ID, &w.Name, &w.Address, &w.TokenHash, &labels, &pool, &w.MaxConcurrentBuilds, &w.Status, &hb, &w.CreatedAt)
	if err != nil {
		return nil, err
	}
	if labels.Valid {
		w.Labels = labels.String
	}
	if pool.Valid {
		w.Pool = pool.String
	}
	if hb.Valid {
		w.LastHeartbeat = hb.Time
	}
	return w, nil
}

func (s *Store) ListWorkers() ([]*Worker, error) {
	rows, err := s.db.Query("SELECT id, name, address, token_hash, labels, pool, max_concurrent_builds, status, last_heartbeat, created_at FROM workers ORDER BY created_at")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var workers []*Worker
	for rows.Next() {
		w := &Worker{}
		var labels, pool sql.NullString
		var hb sql.NullTime
		if err := rows.Scan(&w.ID, &w.Name, &w.Address, &w.TokenHash, &labels, &pool, &w.MaxConcurrentBuilds, &w.Status, &hb, &w.CreatedAt); err != nil {
			return nil, err
		}
		if labels.Valid {
			w.Labels = labels.String
		}
		if pool.Valid {
			w.Pool = pool.String
		}
		if hb.Valid {
			w.LastHeartbeat = hb.Time
		}
		workers = append(workers, w)
	}
	return workers, nil
}

func (s *Store) ListOnlineWorkers() ([]*Worker, error) {
	rows, err := s.db.Query("SELECT id, name, address, token_hash, labels, pool, max_concurrent_builds, status, last_heartbeat, created_at FROM workers WHERE status = 'online' ORDER BY created_at")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var workers []*Worker
	for rows.Next() {
		w := &Worker{}
		var labels, pool sql.NullString
		var hb sql.NullTime
		if err := rows.Scan(&w.ID, &w.Name, &w.Address, &w.TokenHash, &labels, &pool, &w.MaxConcurrentBuilds, &w.Status, &hb, &w.CreatedAt); err != nil {
			return nil, err
		}
		if labels.Valid {
			w.Labels = labels.String
		}
		if pool.Valid {
			w.Pool = pool.String
		}
		if hb.Valid {
			w.LastHeartbeat = hb.Time
		}
		workers = append(workers, w)
	}
	return workers, nil
}

func (s *Store) UpdateWorkerStatus(id, status string) error {
	_, err := s.db.Exec("UPDATE workers SET status=? WHERE id=?", status, id)
	return err
}

func (s *Store) UpdateWorkerHeartbeat(id string, activeBuilds int) error {
	_, err := s.db.Exec("UPDATE workers SET last_heartbeat=?, status='online' WHERE id=?", time.Now(), id)
	return err
}

func (s *Store) DeleteWorker(id string) error {
	_, err := s.db.Exec("DELETE FROM workers WHERE id=?", id)
	return err
}

func (s *Store) MarkOfflineWorkers() error {
	_, err := s.db.Exec("UPDATE workers SET status='offline' WHERE last_heartbeat IS NOT NULL AND last_heartbeat < ?", time.Now().Add(-30*time.Second))
	return err
}

// ---------------- Plugins ----------------

func (s *Store) CreatePlugin(name, version, description, config string) (*Plugin, error) {
	res, err := s.db.Exec(
		"INSERT INTO plugins (name, version, description, enabled, config) VALUES (?, ?, ?, 1, ?)",
		name, version, description, config,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetPlugin(id)
}

func (s *Store) GetPlugin(id int64) (*Plugin, error) {
	p := &Plugin{}
	var desc, cfg sql.NullString
	err := s.db.QueryRow("SELECT id, name, version, description, enabled, config, installed_at FROM plugins WHERE id = ?", id).
		Scan(&p.ID, &p.Name, &p.Version, &desc, &p.Enabled, &cfg, &p.InstalledAt)
	if err != nil {
		return nil, err
	}
	if desc.Valid {
		p.Description = desc.String
	}
	if cfg.Valid {
		p.Config = cfg.String
	}
	return p, nil
}

func (s *Store) GetPluginByName(name string) (*Plugin, error) {
	p := &Plugin{}
	var desc, cfg sql.NullString
	err := s.db.QueryRow("SELECT id, name, version, description, enabled, config, installed_at FROM plugins WHERE name = ?", name).
		Scan(&p.ID, &p.Name, &p.Version, &desc, &p.Enabled, &cfg, &p.InstalledAt)
	if err != nil {
		return nil, err
	}
	if desc.Valid {
		p.Description = desc.String
	}
	if cfg.Valid {
		p.Config = cfg.String
	}
	return p, nil
}

func (s *Store) ListPlugins() ([]*Plugin, error) {
	rows, err := s.db.Query("SELECT id, name, version, description, enabled, config, installed_at FROM plugins ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var plugins []*Plugin
	for rows.Next() {
		p := &Plugin{}
		var desc, cfg sql.NullString
		if err := rows.Scan(&p.ID, &p.Name, &p.Version, &desc, &p.Enabled, &cfg, &p.InstalledAt); err != nil {
			return nil, err
		}
		if desc.Valid {
			p.Description = desc.String
		}
		if cfg.Valid {
			p.Config = cfg.String
		}
		plugins = append(plugins, p)
	}
	return plugins, nil
}

func (s *Store) UpdatePluginEnabled(id int64, enabled bool) error {
	v := 0
	if enabled {
		v = 1
	}
	_, err := s.db.Exec("UPDATE plugins SET enabled=? WHERE id=?", v, id)
	return err
}

func (s *Store) DeletePlugin(id int64) error {
	_, err := s.db.Exec("DELETE FROM plugins WHERE id=?", id)
	return err
}

// ---------------- Artifacts ----------------

func (s *Store) CreateArtifact(buildID int64, name, path string, size int64, sha256, contentType string) (*Artifact, error) {
	res, err := s.db.Exec("INSERT INTO artifacts (build_id, name, path, size, sha256, content_type, downloads) VALUES (?, ?, ?, ?, ?, ?, 0)", buildID, name, path, size, sha256, contentType)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetArtifact(id)
}

func (s *Store) GetArtifact(id int64) (*Artifact, error) {
	a := &Artifact{}
	var sha256, ct sql.NullString
	var dl sql.NullInt64
	err := s.db.QueryRow("SELECT id, build_id, name, path, size, sha256, content_type, downloads, created_at FROM artifacts WHERE id = ?", id).
		Scan(&a.ID, &a.BuildID, &a.Name, &a.Path, &a.Size, &sha256, &ct, &dl, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	if sha256.Valid {
		a.SHA256 = sha256.String
	}
	if ct.Valid {
		a.ContentType = ct.String
	}
	if dl.Valid {
		a.Downloads = int(dl.Int64)
	}
	return a, nil
}

func (s *Store) ListArtifactsByBuild(buildID int64) ([]*Artifact, error) {
	rows, err := s.db.Query("SELECT id, build_id, name, path, size, sha256, content_type, downloads, created_at FROM artifacts WHERE build_id = ?", buildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var arts []*Artifact
	for rows.Next() {
		a := &Artifact{}
		var sha256, ct sql.NullString
		var dl sql.NullInt64
		if err := rows.Scan(&a.ID, &a.BuildID, &a.Name, &a.Path, &a.Size, &sha256, &ct, &dl, &a.CreatedAt); err != nil {
			return nil, err
		}
		if sha256.Valid {
			a.SHA256 = sha256.String
		}
		if ct.Valid {
			a.ContentType = ct.String
		}
		if dl.Valid {
			a.Downloads = int(dl.Int64)
		}
		arts = append(arts, a)
	}
	return arts, nil
}

func (s *Store) IncArtifactDownloads(id int64) error {
	_, err := s.db.Exec("UPDATE artifacts SET downloads = downloads + 1 WHERE id = ?", id)
	return err
}

func (s *Store) DeleteArtifactsByBuild(buildID int64) error {
	_, err := s.db.Exec("DELETE FROM artifacts WHERE build_id = ?", buildID)
	return err
}

func (s *Store) DeleteArtifact(id int64) error {
	_, err := s.db.Exec("DELETE FROM artifacts WHERE id = ?", id)
	return err
}

// ---------------- Env Vars ----------------

func (s *Store) SetEnvVar(scope string, projectID *int64, name, value string, isSecret bool, description string) error {
	_, err := s.db.Exec(
		`INSERT INTO env_vars (scope, project_id, name, value, is_secret, description) VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(scope, project_id, name) DO UPDATE SET value=excluded.value, is_secret=excluded.is_secret, description=excluded.description`,
		scope, projectID, name, value, isSecret, description)
	return err
}

func (s *Store) ListEnvVars(scope string, projectID *int64) ([]*EnvVar, error) {
	q := "SELECT id, scope, project_id, name, value, is_secret, description FROM env_vars WHERE scope = ?"
	args := []interface{}{scope}
	if projectID != nil {
		q += " AND (project_id IS NULL OR project_id = ?)"
		args = append(args, *projectID)
	} else {
		q += " AND project_id IS NULL"
	}
	q += " ORDER BY name"
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var vars []*EnvVar
	for rows.Next() {
		v := &EnvVar{}
		var pid sql.NullInt64
		var desc sql.NullString
		if err := rows.Scan(&v.ID, &v.Scope, &pid, &v.Name, &v.Value, &v.IsSecret, &desc); err != nil {
			return nil, err
		}
		if pid.Valid {
			pi := pid.Int64
			v.ProjectID = &pi
		}
		if desc.Valid {
			v.Description = desc.String
		}
		vars = append(vars, v)
	}
	return vars, nil
}

func (s *Store) DeleteEnvVar(id int64) error {
	_, err := s.db.Exec("DELETE FROM env_vars WHERE id = ?", id)
	return err
}

// ---------------- Credentials ----------------

func (s *Store) CreateCredential(name string, credType CredentialType, host, username, password, privateKey, publicKey, token, description string, isSecret bool) (*Credential, error) {
	res, err := s.db.Exec(
		"INSERT INTO credentials (name, type, host, username, password, private_key, public_key, token, description, is_secret) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		name, string(credType), host, username, password, privateKey, publicKey, token, description, isSecret,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetCredential(id)
}

func (s *Store) GetCredential(id int64) (*Credential, error) {
	c := &Credential{}
	var username, password, privKey, pubKey, token, desc sql.NullString
	var host sql.NullString
	err := s.db.QueryRow(
		"SELECT id, name, type, host, username, password, private_key, public_key, token, description, is_secret, created_at, updated_at FROM credentials WHERE id = ?",
		id,
	).Scan(&c.ID, &c.Name, &c.Type, &host, &username, &password, &privKey, &pubKey, &token, &desc, &c.IsSecret, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	c.Host = host.String
	c.Username = username.String
	c.Password = password.String
	c.PrivateKey = privKey.String
	c.PublicKey = pubKey.String
	c.Token = token.String
	c.Description = desc.String
	return c, nil
}

func (s *Store) GetCredentialByName(name string) (*Credential, error) {
	c := &Credential{}
	var username, password, privKey, pubKey, token, desc sql.NullString
	var host sql.NullString
	err := s.db.QueryRow(
		"SELECT id, name, type, host, username, password, private_key, public_key, token, description, is_secret, created_at, updated_at FROM credentials WHERE name = ?",
		name,
	).Scan(&c.ID, &c.Name, &c.Type, &host, &username, &password, &privKey, &pubKey, &token, &desc, &c.IsSecret, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	c.Host = host.String
	c.Username = username.String
	c.Password = password.String
	c.PrivateKey = privKey.String
	c.PublicKey = pubKey.String
	c.Token = token.String
	c.Description = desc.String
	return c, nil
}

func (s *Store) FindCredential(host string, credType CredentialType) (*Credential, error) {
	rows, err := s.db.Query(
		"SELECT id, name, type, host, username, password, private_key, public_key, token, description, is_secret, created_at, updated_at FROM credentials WHERE type = ? ORDER BY host DESC",
		string(credType),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var creds []*Credential
	for rows.Next() {
		c := &Credential{}
		var username, password, privKey, pubKey, token, desc sql.NullString
		var h sql.NullString
		if err := rows.Scan(&c.ID, &c.Name, &c.Type, &h, &username, &password, &privKey, &pubKey, &token, &desc, &c.IsSecret, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.Host = h.String
		c.Username = username.String
		c.Password = password.String
		c.PrivateKey = privKey.String
		c.PublicKey = pubKey.String
		c.Token = token.String
		c.Description = desc.String
		creds = append(creds, c)
	}
	var exact, suffix, global *Credential
	for _, c := range creds {
		if c.Host == host {
			exact = c
			break
		}
	}
	if exact != nil {
		return exact, nil
	}
	for _, c := range creds {
		if c.Host != "" && strings.HasSuffix(host, c.Host) {
			suffix = c
			break
		}
	}
	if suffix != nil {
		return suffix, nil
	}
	for _, c := range creds {
		if c.Host == "" {
			global = c
			break
		}
	}
	if global != nil {
		return global, nil
	}
	return nil, sql.ErrNoRows
}

func (s *Store) ListCredentials(credType *CredentialType) ([]*Credential, error) {
	var rows *sql.Rows
	var err error
	if credType != nil {
		rows, err = s.db.Query(
			"SELECT id, name, type, host, username, password, private_key, public_key, token, description, is_secret, created_at, updated_at FROM credentials WHERE type = ? ORDER BY id",
			string(*credType),
		)
	} else {
		rows, err = s.db.Query(
			"SELECT id, name, type, host, username, password, private_key, public_key, token, description, is_secret, created_at, updated_at FROM credentials ORDER BY id",
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var creds []*Credential
	for rows.Next() {
		c := &Credential{}
		var username, password, privKey, pubKey, token, desc sql.NullString
		var host sql.NullString
		if err := rows.Scan(&c.ID, &c.Name, &c.Type, &host, &username, &password, &privKey, &pubKey, &token, &desc, &c.IsSecret, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.Host = host.String
		c.Username = username.String
		c.Password = password.String
		c.PrivateKey = privKey.String
		c.PublicKey = pubKey.String
		c.Token = token.String
		c.Description = desc.String
		creds = append(creds, c)
	}
	return creds, nil
}

func (s *Store) UpdateCredential(id int64, name string, credType CredentialType, host, username, password, privateKey, publicKey, token, description string, isSecret bool) error {
	_, err := s.db.Exec(
		"UPDATE credentials SET name=?, type=?, host=?, username=?, password=?, private_key=?, public_key=?, token=?, description=?, is_secret=?, updated_at=? WHERE id=?",
		name, string(credType), host, username, password, privateKey, publicKey, token, description, isSecret, time.Now(), id,
	)
	return err
}

func (s *Store) DeleteCredential(id int64) error {
	_, err := s.db.Exec("DELETE FROM credentials WHERE id = ?", id)
	return err
}

// ---------------- VCS Roots ----------------

func (s *Store) CreateVCSRoot(name, vcsType, url, branch string, credentialID *int64, pollInterval int, autoCheckout bool, config string) (*VCSRoot, error) {
	if branch == "" {
		branch = "main"
	}
	if pollInterval == 0 {
		pollInterval = 60
	}
	if config == "" {
		config = "{}"
	}
	res, err := s.db.Exec(
		"INSERT INTO vcs_roots (name, type, url, branch, credential_id, poll_interval, auto_checkout, config) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		name, vcsType, url, branch, credentialID, pollInterval, autoCheckout, config,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetVCSRoot(id)
}

func (s *Store) GetVCSRoot(id int64) (*VCSRoot, error) {
	v := &VCSRoot{}
	var credID sql.NullInt64
	var cfg sql.NullString
	err := s.db.QueryRow(
		"SELECT id, name, type, url, branch, credential_id, poll_interval, auto_checkout, config, created_at, updated_at FROM vcs_roots WHERE id = ?",
		id,
	).Scan(&v.ID, &v.Name, &v.Type, &v.URL, &v.Branch, &credID, &v.PollInterval, &v.AutoCheckout, &cfg, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if credID.Valid {
		cid := credID.Int64
		v.CredentialID = &cid
	}
	if cfg.Valid {
		v.Config = cfg.String
	}
	return v, nil
}

func (s *Store) GetVCSRootByName(name string) (*VCSRoot, error) {
	v := &VCSRoot{}
	var credID sql.NullInt64
	var cfg sql.NullString
	err := s.db.QueryRow(
		"SELECT id, name, type, url, branch, credential_id, poll_interval, auto_checkout, config, created_at, updated_at FROM vcs_roots WHERE name = ?",
		name,
	).Scan(&v.ID, &v.Name, &v.Type, &v.URL, &v.Branch, &credID, &v.PollInterval, &v.AutoCheckout, &cfg, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if credID.Valid {
		cid := credID.Int64
		v.CredentialID = &cid
	}
	if cfg.Valid {
		v.Config = cfg.String
	}
	return v, nil
}

func (s *Store) ListVCSRoots() ([]*VCSRoot, error) {
	rows, err := s.db.Query("SELECT id, name, type, url, branch, credential_id, poll_interval, auto_checkout, config, created_at, updated_at FROM vcs_roots ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var roots []*VCSRoot
	for rows.Next() {
		v := &VCSRoot{}
		var credID sql.NullInt64
		var cfg sql.NullString
		if err := rows.Scan(&v.ID, &v.Name, &v.Type, &v.URL, &v.Branch, &credID, &v.PollInterval, &v.AutoCheckout, &cfg, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		if credID.Valid {
			cid := credID.Int64
			v.CredentialID = &cid
		}
		if cfg.Valid {
			v.Config = cfg.String
		}
		roots = append(roots, v)
	}
	return roots, nil
}

func (s *Store) UpdateVCSRoot(id int64, name, vcsType, url, branch string, credentialID *int64, pollInterval int, autoCheckout bool, config string) error {
	_, err := s.db.Exec(
		"UPDATE vcs_roots SET name=?, type=?, url=?, branch=?, credential_id=?, poll_interval=?, auto_checkout=?, config=?, updated_at=? WHERE id=?",
		name, vcsType, url, branch, credentialID, pollInterval, autoCheckout, config, time.Now(), id,
	)
	return err
}

func (s *Store) DeleteVCSRoot(id int64) error {
	_, err := s.db.Exec("DELETE FROM vcs_roots WHERE id = ?", id)
	return err
}

func (s *Store) UpdateVCSRootConfig(id int64, config string) error {
	_, err := s.db.Exec("UPDATE vcs_roots SET config=?, updated_at=? WHERE id=?", config, time.Now(), id)
	return err
}

// ---------------- Build Templates ----------------

func (s *Store) CreateBuildTemplate(name, config, description string) (*BuildTemplate, error) {
	res, err := s.db.Exec(
		"INSERT INTO build_templates (name, config, description) VALUES (?, ?, ?)",
		name, config, description,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetBuildTemplate(id)
}

func (s *Store) GetBuildTemplate(id int64) (*BuildTemplate, error) {
	t := &BuildTemplate{}
	var desc sql.NullString
	err := s.db.QueryRow(
		"SELECT id, name, config, description, created_at FROM build_templates WHERE id = ?",
		id,
	).Scan(&t.ID, &t.Name, &t.Config, &desc, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	if desc.Valid {
		t.Description = desc.String
	}
	return t, nil
}

func (s *Store) GetBuildTemplateByName(name string) (*BuildTemplate, error) {
	t := &BuildTemplate{}
	var desc sql.NullString
	err := s.db.QueryRow(
		"SELECT id, name, config, description, created_at FROM build_templates WHERE name = ?",
		name,
	).Scan(&t.ID, &t.Name, &t.Config, &desc, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	if desc.Valid {
		t.Description = desc.String
	}
	return t, nil
}

func (s *Store) ListBuildTemplates() ([]*BuildTemplate, error) {
	rows, err := s.db.Query("SELECT id, name, config, description, created_at FROM build_templates ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var templates []*BuildTemplate
	for rows.Next() {
		t := &BuildTemplate{}
		var desc sql.NullString
		if err := rows.Scan(&t.ID, &t.Name, &t.Config, &desc, &t.CreatedAt); err != nil {
			return nil, err
		}
		if desc.Valid {
			t.Description = desc.String
		}
		templates = append(templates, t)
	}
	return templates, nil
}

func (s *Store) UpdateBuildTemplate(id int64, name, config, description string) error {
	_, err := s.db.Exec(
		"UPDATE build_templates SET name=?, config=?, description=? WHERE id=?",
		name, config, description, id,
	)
	return err
}

func (s *Store) DeleteBuildTemplate(id int64) error {
	_, err := s.db.Exec("DELETE FROM build_templates WHERE id = ?", id)
	return err
}

// ---------------- Notification Channels ----------------

func (s *Store) CreateNotificationChannel(name string, channelType NotificationChannelType, config, conditions, description string, enabled bool) (*NotificationChannel, error) {
	if config == "" {
		config = "{}"
	}
	if conditions == "" {
		conditions = "{}"
	}
	res, err := s.db.Exec(
		"INSERT INTO notification_channels (name, type, config, conditions, description, enabled) VALUES (?, ?, ?, ?, ?, ?)",
		name, string(channelType), config, conditions, description, enabled,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetNotificationChannel(id)
}

func (s *Store) GetNotificationChannel(id int64) (*NotificationChannel, error) {
	c := &NotificationChannel{}
	var desc, cfg, cond sql.NullString
	err := s.db.QueryRow(
		"SELECT id, name, type, config, conditions, description, enabled, created_at, updated_at FROM notification_channels WHERE id = ?",
		id,
	).Scan(&c.ID, &c.Name, &c.Type, &cfg, &cond, &desc, &c.Enabled, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	c.Config = cfg.String
	c.Conditions = cond.String
	c.Description = desc.String
	return c, nil
}

func (s *Store) ListNotificationChannels() ([]*NotificationChannel, error) {
	rows, err := s.db.Query("SELECT id, name, type, config, conditions, description, enabled, created_at, updated_at FROM notification_channels ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var channels []*NotificationChannel
	for rows.Next() {
		c := &NotificationChannel{}
		var desc, cfg, cond sql.NullString
		if err := rows.Scan(&c.ID, &c.Name, &c.Type, &cfg, &cond, &desc, &c.Enabled, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.Config = cfg.String
		c.Conditions = cond.String
		c.Description = desc.String
		channels = append(channels, c)
	}
	return channels, nil
}

func (s *Store) ListEnabledNotificationChannels() ([]*NotificationChannel, error) {
	rows, err := s.db.Query("SELECT id, name, type, config, conditions, description, enabled, created_at, updated_at FROM notification_channels WHERE enabled = 1 ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var channels []*NotificationChannel
	for rows.Next() {
		c := &NotificationChannel{}
		var desc, cfg, cond sql.NullString
		if err := rows.Scan(&c.ID, &c.Name, &c.Type, &cfg, &cond, &desc, &c.Enabled, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.Config = cfg.String
		c.Conditions = cond.String
		c.Description = desc.String
		channels = append(channels, c)
	}
	return channels, nil
}

func (s *Store) UpdateNotificationChannel(id int64, name string, channelType NotificationChannelType, config, conditions, description string, enabled bool) error {
	_, err := s.db.Exec(
		"UPDATE notification_channels SET name=?, type=?, config=?, conditions=?, description=?, enabled=?, updated_at=? WHERE id=?",
		name, string(channelType), config, conditions, description, enabled, time.Now(), id,
	)
	return err
}

func (s *Store) DeleteNotificationChannel(id int64) error {
	_, err := s.db.Exec("DELETE FROM notification_channels WHERE id = ?", id)
	return err
}

// ---------------- Notification Events ----------------

func (s *Store) CreateNotificationEvent(channelID int64, buildID *int64, eventType, payload string) (*NotificationEvent, error) {
	res, err := s.db.Exec(
		"INSERT INTO notification_events (channel_id, build_id, event_type, payload, status) VALUES (?, ?, ?, ?, 'pending')",
		channelID, buildID, eventType, payload,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetNotificationEvent(id)
}

func (s *Store) GetNotificationEvent(id int64) (*NotificationEvent, error) {
	e := &NotificationEvent{}
	var buildID sql.NullInt64
	var payload, errMsg sql.NullString
	var delivered sql.NullTime
	err := s.db.QueryRow(
		"SELECT id, channel_id, build_id, event_type, payload, status, error_message, delivered_at, created_at FROM notification_events WHERE id = ?",
		id,
	).Scan(&e.ID, &e.ChannelID, &buildID, &e.EventType, &payload, &e.Status, &errMsg, &delivered, &e.CreatedAt)
	if err != nil {
		return nil, err
	}
	if buildID.Valid {
		bid := buildID.Int64
		e.BuildID = &bid
	}
	e.Payload = payload.String
	e.ErrorMessage = errMsg.String
	if delivered.Valid {
		e.DeliveredAt = &delivered.Time
	}
	return e, nil
}

func (s *Store) ListNotificationEvents(channelID int64, limit int) ([]*NotificationEvent, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.Query(
		"SELECT id, channel_id, build_id, event_type, payload, status, error_message, delivered_at, created_at FROM notification_events WHERE channel_id = ? ORDER BY id DESC LIMIT ?",
		channelID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNotificationEvents(rows)
}

func scanNotificationEvents(rows *sql.Rows) ([]*NotificationEvent, error) {
	var events []*NotificationEvent
	for rows.Next() {
		e := &NotificationEvent{}
		var buildID sql.NullInt64
		var payload, errMsg sql.NullString
		var delivered sql.NullTime
		if err := rows.Scan(&e.ID, &e.ChannelID, &buildID, &e.EventType, &payload, &e.Status, &errMsg, &delivered, &e.CreatedAt); err != nil {
			return nil, err
		}
		if buildID.Valid {
			bid := buildID.Int64
			e.BuildID = &bid
		}
		e.Payload = payload.String
		e.ErrorMessage = errMsg.String
		if delivered.Valid {
			e.DeliveredAt = &delivered.Time
		}
		events = append(events, e)
	}
	return events, nil
}

func (s *Store) UpdateNotificationEventStatus(id int64, status string, errorMessage *string, deliveredAt *time.Time) error {
	q := "UPDATE notification_events SET status=?"
	args := []interface{}{status}
	if errorMessage != nil {
		q += ", error_message=?"
		args = append(args, *errorMessage)
	}
	if deliveredAt != nil {
		q += ", delivered_at=?"
		args = append(args, *deliveredAt)
	}
	q += " WHERE id=?"
	args = append(args, id)
	_, err := s.db.Exec(q, args...)
	return err
}

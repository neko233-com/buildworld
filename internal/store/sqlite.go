package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
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
			msg := err.Error()
			if strings.Contains(msg, "duplicate column name") ||
				strings.Contains(msg, "already exists") {
				continue
			}
			return fmt.Errorf("migration [%s]: %w", m[:min(60, len(m))], err)
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
	var started, finished, approvedAt sql.NullTime
	var dur sql.NullInt64
	var waitDep, retriedFrom, approvedBy, testResultID sql.NullInt64
	var pinned, approvalRequired sql.NullBool
	err := s.db.QueryRow(
		"SELECT id, project_id, number, status, trigger, branch, commit_sha, parameters, wait_dependency_on, retried_from, pinned, log, started_at, finished_at, duration_ms, approval_required, approved_by, approved_at, timeout_sec, test_result_id FROM builds WHERE id = ?",
		id,
	).Scan(&b.ID, &b.ProjectID, &b.Number, &b.Status, &b.Trigger, &branch, &commit, &params, &waitDep, &retriedFrom, &pinned, &log, &started, &finished, &dur, &approvalRequired, &approvedBy, &approvedAt, &b.TimeoutSec, &testResultID)
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
	if approvalRequired.Valid {
		b.ApprovalRequired = approvalRequired.Bool
	}
	if approvedBy.Valid {
		v := approvedBy.Int64
		b.ApprovedBy = &v
	}
	if approvedAt.Valid {
		b.ApprovedAt = &approvedAt.Time
	}
	if testResultID.Valid {
		v := testResultID.Int64
		b.TestResultID = &v
	}
	return b, nil
}

func (s *Store) GetBuildByNumber(projectID int64, number int) (*Build, error) {
	b := &Build{}
	var branch, commit, params, log sql.NullString
	var started, finished, approvedAt sql.NullTime
	var dur sql.NullInt64
	var waitDep, retriedFrom, approvedBy, testResultID sql.NullInt64
	var pinned, approvalRequired sql.NullBool
	err := s.db.QueryRow(
		"SELECT id, project_id, number, status, trigger, branch, commit_sha, parameters, wait_dependency_on, retried_from, pinned, log, started_at, finished_at, duration_ms, approval_required, approved_by, approved_at, timeout_sec, test_result_id FROM builds WHERE project_id = ? AND number = ?",
		projectID, number,
	).Scan(&b.ID, &b.ProjectID, &b.Number, &b.Status, &b.Trigger, &branch, &commit, &params, &waitDep, &retriedFrom, &pinned, &log, &started, &finished, &dur, &approvalRequired, &approvedBy, &approvedAt, &b.TimeoutSec, &testResultID)
	if err != nil {
		return nil, err
	}
	scanBuildExtras(b, branch, commit, params, log, started, finished, dur, waitDep, retriedFrom, pinned, approvalRequired, approvedBy, approvedAt, testResultID)
	return b, nil
}

func (s *Store) ListBuilds(limit int) ([]*Build, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.Query(
		"SELECT id, project_id, number, status, trigger, branch, commit_sha, parameters, wait_dependency_on, retried_from, pinned, log, started_at, finished_at, duration_ms, approval_required, approved_by, approved_at, timeout_sec, test_result_id FROM builds ORDER BY id DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBuilds(rows)
}

func (s *Store) ListBuildsByProject(projectID int64) ([]*Build, error) {
	rows, err := s.db.Query(
		"SELECT id, project_id, number, status, trigger, branch, commit_sha, parameters, wait_dependency_on, retried_from, pinned, log, started_at, finished_at, duration_ms, approval_required, approved_by, approved_at, timeout_sec, test_result_id FROM builds WHERE project_id = ? ORDER BY number DESC", projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBuilds(rows)
}

func (s *Store) ListPendingBuilds() ([]*Build, error) {
	rows, err := s.db.Query(
		"SELECT id, project_id, number, status, trigger, branch, commit_sha, parameters, wait_dependency_on, retried_from, pinned, log, started_at, finished_at, duration_ms, approval_required, approved_by, approved_at, timeout_sec, test_result_id FROM builds WHERE status = 'pending' ORDER BY id ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBuilds(rows)
}

// scanBuildExtras 把可空列从 sql.Null* 拷贝到 Build。集中实现避免重复。
func scanBuildExtras(b *Build, branch, commit, params, log sql.NullString,
	started, finished sql.NullTime, dur sql.NullInt64,
	waitDep, retriedFrom sql.NullInt64, pinned, approvalRequired sql.NullBool,
	approvedBy sql.NullInt64, approvedAt sql.NullTime, testResultID sql.NullInt64) {
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
	if approvalRequired.Valid {
		b.ApprovalRequired = approvalRequired.Bool
	}
	if approvedBy.Valid {
		v := approvedBy.Int64
		b.ApprovedBy = &v
	}
	if approvedAt.Valid {
		b.ApprovedAt = &approvedAt.Time
	}
	if testResultID.Valid {
		v := testResultID.Int64
		b.TestResultID = &v
	}
}

func scanBuilds(rows *sql.Rows) ([]*Build, error) {
	var builds []*Build
	for rows.Next() {
		b := &Build{}
		var branch, commit, params, log sql.NullString
		var started, finished, approvedAt sql.NullTime
		var dur sql.NullInt64
		var waitDep, retriedFrom, approvedBy, testResultID sql.NullInt64
		var pinned, approvalRequired sql.NullBool
		if err := rows.Scan(&b.ID, &b.ProjectID, &b.Number, &b.Status, &b.Trigger, &branch, &commit, &params, &waitDep, &retriedFrom, &pinned, &log, &started, &finished, &dur, &approvalRequired, &approvedBy, &approvedAt, &b.TimeoutSec, &testResultID); err != nil {
			return nil, err
		}
		scanBuildExtras(b, branch, commit, params, log, started, finished, dur, waitDep, retriedFrom, pinned, approvalRequired, approvedBy, approvedAt, testResultID)
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

func (s *Store) CreatePlugin(name, version, description, author, config, scriptLang, sourceScript, sourceUIScript, source string) (*Plugin, error) {
	if source == "" {
		source = "builtin"
	}
	if scriptLang == "" {
		scriptLang = "js"
	}
	res, err := s.db.Exec(
		"INSERT INTO plugins (name, version, description, author, enabled, config, script_lang, source_script, source_ui_script, source, steps, triggers, ui_extensions) VALUES (?, ?, ?, ?, 1, ?, ?, ?, ?, ?, '[]', '[]', '[]')",
		name, version, description, author, config, scriptLang, sourceScript, sourceUIScript, source,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetPlugin(id)
}

type scannable interface {
	Scan(dest ...interface{}) error
}

func scanPlugin(p *Plugin, row scannable) error {
	var desc, author, cfg, scriptLang, sourceScript, sourceUIScript, path, steps, triggers, uiExt sql.NullString
	var updatedAt sql.NullTime
	err := row.Scan(&p.ID, &p.Name, &p.Version, &desc, &author, &p.Enabled, &cfg, &scriptLang, &sourceScript, &sourceUIScript, &path, &p.Source, &steps, &triggers, &uiExt, &p.InstalledAt, &updatedAt)
	if err != nil {
		return err
	}
	if desc.Valid {
		p.Description = desc.String
	}
	if author.Valid {
		p.Author = author.String
	}
	if cfg.Valid {
		p.Config = cfg.String
	}
	if scriptLang.Valid {
		p.ScriptLang = scriptLang.String
	}
	if sourceScript.Valid {
		p.SourceScript = sourceScript.String
	}
	if sourceUIScript.Valid {
		p.SourceUIScript = sourceUIScript.String
	}
	if path.Valid {
		p.Path = path.String
	}
	if steps.Valid {
		p.Steps = steps.String
	}
	if triggers.Valid {
		p.Triggers = triggers.String
	}
	if uiExt.Valid {
		p.UIExtensions = uiExt.String
	}
	if updatedAt.Valid {
		p.UpdatedAt = updatedAt.Time
	}
	if p.Source == "" {
		p.Source = "builtin"
	}
	if p.ScriptLang == "" {
		p.ScriptLang = "js"
	}
	if p.Steps == "" {
		p.Steps = "[]"
	}
	if p.Triggers == "" {
		p.Triggers = "[]"
	}
	if p.UIExtensions == "" {
		p.UIExtensions = "[]"
	}
	return nil
}

func (s *Store) GetPlugin(id int64) (*Plugin, error) {
	p := &Plugin{}
	err := scanPlugin(p, s.db.QueryRow(
		"SELECT id, name, version, description, author, enabled, config, script_lang, source_script, source_ui_script, path, source, steps, triggers, ui_extensions, installed_at, updated_at FROM plugins WHERE id = ?", id))
	return p, err
}

func (s *Store) GetPluginByName(name string) (*Plugin, error) {
	p := &Plugin{}
	err := scanPlugin(p, s.db.QueryRow(
		"SELECT id, name, version, description, author, enabled, config, script_lang, source_script, source_ui_script, path, source, steps, triggers, ui_extensions, installed_at, updated_at FROM plugins WHERE name = ?", name))
	return p, err
}

func (s *Store) ListPlugins() ([]*Plugin, error) {
	rows, err := s.db.Query("SELECT id, name, version, description, author, enabled, config, script_lang, source_script, source_ui_script, path, source, steps, triggers, ui_extensions, installed_at, updated_at FROM plugins ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var plugins []*Plugin
	for rows.Next() {
		p := &Plugin{}
		if err := scanPlugin(p, rows); err != nil {
			return nil, err
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
	_, err := s.db.Exec("UPDATE plugins SET enabled=?, updated_at=? WHERE id=?", v, time.Now(), id)
	return err
}

func (s *Store) UpdatePluginStatus(name string, enabled bool, steps, triggers, uiExtensions string) error {
	v := 0
	if enabled {
		v = 1
	}
	if steps == "" {
		steps = "[]"
	}
	if triggers == "" {
		triggers = "[]"
	}
	if uiExtensions == "" {
		uiExtensions = "[]"
	}
	_, err := s.db.Exec("UPDATE plugins SET enabled=?, steps=?, triggers=?, ui_extensions=?, updated_at=? WHERE name=?", v, steps, triggers, uiExtensions, time.Now(), name)
	return err
}

func (s *Store) DeletePluginByName(name string) error {
	_, err := s.db.Exec("DELETE FROM plugins WHERE name=?", name)
	return err
}

func (s *Store) DeletePlugin(id int64) error {
	_, err := s.db.Exec("DELETE FROM plugins WHERE id=?", id)
	return err
}

func (s *Store) UpdatePluginSource(id int64, scriptLang, sourceScript, sourceUIScript, script, uiScript string) error {
	if scriptLang == "" {
		scriptLang = "js"
	}
	_, err := s.db.Exec(
		"UPDATE plugins SET script_lang=?, source_script=?, source_ui_script=?, updated_at=? WHERE id=?",
		scriptLang, sourceScript, sourceUIScript, time.Now(), id,
	)
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

// ---------------- Build 扩展字段更新 ----------------

func (s *Store) SetBuildApprovalRequired(buildID int64, required bool) error {
	_, err := s.db.Exec("UPDATE builds SET approval_required=? WHERE id=?", required, buildID)
	return err
}

func (s *Store) SetBuildApproved(buildID, userID int64) error {
	_, err := s.db.Exec("UPDATE builds SET approved_by=?, approved_at=? WHERE id=?", userID, time.Now(), buildID)
	return err
}

func (s *Store) SetBuildTestResult(buildID, testResultID int64) error {
	_, err := s.db.Exec("UPDATE builds SET test_result_id=? WHERE id=?", testResultID, buildID)
	return err
}

func (s *Store) SetBuildTimeout(buildID int64, timeoutSec int) error {
	_, err := s.db.Exec("UPDATE builds SET timeout_sec=? WHERE id=?", timeoutSec, buildID)
	return err
}

// ---------------- Build Stats ----------------

func (s *Store) CreateBuildStat(projectID int64, date string, total, success, failed int, avgDuration int64) (*BuildStat, error) {
	_, err := s.db.Exec(
		`INSERT INTO build_stats (project_id, date, total_builds, success_count, failed_count, avg_duration_ms)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(project_id, date) DO UPDATE SET
		   total_builds=excluded.total_builds,
		   success_count=excluded.success_count,
		   failed_count=excluded.failed_count,
		   avg_duration_ms=excluded.avg_duration_ms`,
		projectID, date, total, success, failed, avgDuration,
	)
	if err != nil {
		return nil, err
	}
	var bs BuildStat
	err = s.db.QueryRow(
		"SELECT id, project_id, date, total_builds, success_count, failed_count, avg_duration_ms FROM build_stats WHERE project_id=? AND date=?",
		projectID, date,
	).Scan(&bs.ID, &bs.ProjectID, &bs.Date, &bs.TotalBuilds, &bs.SuccessCount, &bs.FailedCount, &bs.AvgDuration)
	if err != nil {
		return nil, err
	}
	return &bs, nil
}

func (s *Store) GetProjectBuildStats(projectID int64, days int) ([]*BuildStat, error) {
	if days <= 0 {
		days = 30
	}
	rows, err := s.db.Query(
		`SELECT id, project_id, date, total_builds, success_count, failed_count, avg_duration_ms
		 FROM build_stats
		 WHERE project_id=? AND date >= date('now', ?)
		 ORDER BY date DESC`,
		projectID, "-"+strconv.Itoa(days)+" days",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var stats []*BuildStat
	for rows.Next() {
		bs := &BuildStat{}
		if err := rows.Scan(&bs.ID, &bs.ProjectID, &bs.Date, &bs.TotalBuilds, &bs.SuccessCount, &bs.FailedCount, &bs.AvgDuration); err != nil {
			return nil, err
		}
		stats = append(stats, bs)
	}
	return stats, nil
}

func (s *Store) GetDashboardStats() ([]*BuildStat, error) {
	rows, err := s.db.Query(
		`SELECT date, SUM(total_builds) AS total, SUM(success_count) AS success, SUM(failed_count) AS failed, CAST(AVG(avg_duration_ms) AS INTEGER) AS avg
		 FROM build_stats
		 WHERE date >= date('now', '-30 days')
		 GROUP BY date
		 ORDER BY date DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var stats []*BuildStat
	for rows.Next() {
		bs := &BuildStat{}
		if err := rows.Scan(&bs.ID, &bs.ProjectID, &bs.Date, &bs.TotalBuilds, &bs.SuccessCount, &bs.FailedCount, &bs.AvgDuration); err != nil {
			return nil, err
		}
		stats = append(stats, bs)
	}
	return stats, nil
}

// ---------------- Audit Logs ----------------

func (s *Store) CreateAuditLog(userID int64, username, action, resourceType, resourceID, detail, ip string) error {
	_, err := s.db.Exec(
		"INSERT INTO audit_logs (user_id, username, action, resource_type, resource_id, detail, ip) VALUES (?, ?, ?, ?, ?, ?, ?)",
		userID, username, action, resourceType, resourceID, detail, ip,
	)
	return err
}

func (s *Store) ListAuditLogs(limit, offset int) ([]*AuditLog, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.db.Query(
		"SELECT id, user_id, username, action, resource_type, resource_id, detail, ip, created_at FROM audit_logs ORDER BY id DESC LIMIT ? OFFSET ?",
		limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var logs []*AuditLog
	for rows.Next() {
		a := &AuditLog{}
		var detail, ip sql.NullString
		if err := rows.Scan(&a.ID, &a.UserID, &a.Username, &a.Action, &a.ResourceType, &a.ResourceID, &detail, &ip, &a.CreatedAt); err != nil {
			return nil, err
		}
		a.Detail, a.IP = detail.String, ip.String
		logs = append(logs, a)
	}
	return logs, nil
}

// ---------------- API Tokens ----------------

func (s *Store) CreateAPIToken(userID int64, name, tokenHash, tokenPrefix, scopes string, expiresAt *time.Time) (*APIToken, error) {
	res, err := s.db.Exec(
		"INSERT INTO api_tokens (user_id, name, token_hash, token_prefix, scopes, expires_at) VALUES (?, ?, ?, ?, ?, ?)",
		userID, name, tokenHash, tokenPrefix, scopes, expiresAt,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetAPIToken(id)
}

func (s *Store) GetAPIToken(id int64) (*APIToken, error) {
	t := &APIToken{}
	var expires, lastUsed sql.NullTime
	err := s.db.QueryRow(
		"SELECT id, user_id, name, token_hash, token_prefix, scopes, expires_at, last_used_at, created_at FROM api_tokens WHERE id = ?",
		id,
	).Scan(&t.ID, &t.UserID, &t.Name, &t.TokenHash, &t.TokenPrefix, &t.Scopes, &expires, &lastUsed, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	if expires.Valid {
		t.ExpiresAt = &expires.Time
	}
	if lastUsed.Valid {
		t.LastUsedAt = &lastUsed.Time
	}
	return t, nil
}

func (s *Store) GetAPITokenByHash(hash string) (*APIToken, error) {
	t := &APIToken{}
	var expires, lastUsed sql.NullTime
	err := s.db.QueryRow(
		"SELECT id, user_id, name, token_hash, token_prefix, scopes, expires_at, last_used_at, created_at FROM api_tokens WHERE token_hash = ?",
		hash,
	).Scan(&t.ID, &t.UserID, &t.Name, &t.TokenHash, &t.TokenPrefix, &t.Scopes, &expires, &lastUsed, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	if expires.Valid {
		t.ExpiresAt = &expires.Time
	}
	if lastUsed.Valid {
		t.LastUsedAt = &lastUsed.Time
	}
	return t, nil
}

func (s *Store) ListAPITokens(userID int64) ([]*APIToken, error) {
	rows, err := s.db.Query(
		"SELECT id, user_id, name, token_hash, token_prefix, scopes, expires_at, last_used_at, created_at FROM api_tokens WHERE user_id = ? ORDER BY id DESC",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tokens []*APIToken
	for rows.Next() {
		t := &APIToken{}
		var expires, lastUsed sql.NullTime
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.TokenHash, &t.TokenPrefix, &t.Scopes, &expires, &lastUsed, &t.CreatedAt); err != nil {
			return nil, err
		}
		if expires.Valid {
			t.ExpiresAt = &expires.Time
		}
		if lastUsed.Valid {
			t.LastUsedAt = &lastUsed.Time
		}
		tokens = append(tokens, t)
	}
	return tokens, nil
}

func (s *Store) DeleteAPIToken(id int64) error {
	_, err := s.db.Exec("DELETE FROM api_tokens WHERE id = ?", id)
	return err
}

func (s *Store) UpdateAPITokenLastUsed(id int64) error {
	_, err := s.db.Exec("UPDATE api_tokens SET last_used_at=? WHERE id=?", time.Now(), id)
	return err
}

// ---------------- Build Approvals ----------------

func (s *Store) CreateBuildApproval(buildID, userID int64, username string) (*BuildApproval, error) {
	res, err := s.db.Exec(
		"INSERT INTO build_approvals (build_id, user_id, username, status) VALUES (?, ?, ?, 'pending')",
		buildID, userID, username,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetBuildApproval(id)
}

func (s *Store) GetBuildApproval(id int64) (*BuildApproval, error) {
	a := &BuildApproval{}
	var comment sql.NullString
	var resolved sql.NullTime
	err := s.db.QueryRow(
		"SELECT id, build_id, user_id, username, status, comment, created_at, resolved_at FROM build_approvals WHERE id = ?",
		id,
	).Scan(&a.ID, &a.BuildID, &a.UserID, &a.Username, &a.Status, &comment, &a.CreatedAt, &resolved)
	if err != nil {
		return nil, err
	}
	a.Comment = comment.String
	if resolved.Valid {
		a.ResolvedAt = &resolved.Time
	}
	return a, nil
}

func (s *Store) GetBuildApprovalByBuild(buildID int64) (*BuildApproval, error) {
	a := &BuildApproval{}
	var comment sql.NullString
	var resolved sql.NullTime
	err := s.db.QueryRow(
		"SELECT id, build_id, user_id, username, status, comment, created_at, resolved_at FROM build_approvals WHERE build_id = ? ORDER BY id DESC LIMIT 1",
		buildID,
	).Scan(&a.ID, &a.BuildID, &a.UserID, &a.Username, &a.Status, &comment, &a.CreatedAt, &resolved)
	if err != nil {
		return nil, err
	}
	a.Comment = comment.String
	if resolved.Valid {
		a.ResolvedAt = &resolved.Time
	}
	return a, nil
}

func (s *Store) ListPendingApprovals() ([]*BuildApproval, error) {
	rows, err := s.db.Query(
		"SELECT id, build_id, user_id, username, status, comment, created_at, resolved_at FROM build_approvals WHERE status = 'pending' ORDER BY id DESC",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBuildApprovals(rows)
}

func (s *Store) UpdateBuildApproval(id int64, status, comment string) error {
	_, err := s.db.Exec(
		"UPDATE build_approvals SET status=?, comment=?, resolved_at=? WHERE id=?",
		status, comment, time.Now(), id,
	)
	return err
}

func scanBuildApprovals(rows *sql.Rows) ([]*BuildApproval, error) {
	var approvals []*BuildApproval
	for rows.Next() {
		a := &BuildApproval{}
		var comment sql.NullString
		var resolved sql.NullTime
		if err := rows.Scan(&a.ID, &a.BuildID, &a.UserID, &a.Username, &a.Status, &comment, &a.CreatedAt, &resolved); err != nil {
			return nil, err
		}
		a.Comment = comment.String
		if resolved.Valid {
			a.ResolvedAt = &resolved.Time
		}
		approvals = append(approvals, a)
	}
	return approvals, nil
}

// ---------------- Test Results ----------------

func (s *Store) CreateTestResult(buildID int64, total, passed, failed, skipped int, duration int64, reportXML string) (*TestResult, error) {
	res, err := s.db.Exec(
		"INSERT INTO test_results (build_id, total, passed, failed, skipped, duration_ms, report_xml) VALUES (?, ?, ?, ?, ?, ?, ?)",
		buildID, total, passed, failed, skipped, duration, reportXML,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetTestResult(id)
}

func (s *Store) GetTestResult(id int64) (*TestResult, error) {
	t := &TestResult{}
	err := s.db.QueryRow(
		"SELECT id, build_id, total, passed, failed, skipped, duration_ms, report_xml, created_at FROM test_results WHERE id = ?",
		id,
	).Scan(&t.ID, &t.BuildID, &t.Total, &t.Passed, &t.Failed, &t.Skipped, &t.Duration, &t.ReportXML, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (s *Store) ListBuildTestResults(buildID int64) ([]*TestResult, error) {
	rows, err := s.db.Query(
		"SELECT id, build_id, total, passed, failed, skipped, duration_ms, report_xml, created_at FROM test_results WHERE build_id = ? ORDER BY id DESC",
		buildID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []*TestResult
	for rows.Next() {
		t := &TestResult{}
		if err := rows.Scan(&t.ID, &t.BuildID, &t.Total, &t.Passed, &t.Failed, &t.Skipped, &t.Duration, &t.ReportXML, &t.CreatedAt); err != nil {
			return nil, err
		}
		results = append(results, t)
	}
	return results, nil
}

// ---------------- Deployment Envs ----------------

func (s *Store) CreateDeploymentEnv(projectID int64, name, description, config string) (*DeploymentEnv, error) {
	if config == "" {
		config = "{}"
	}
	res, err := s.db.Exec(
		"INSERT INTO deployment_envs (project_id, name, description, config) VALUES (?, ?, ?, ?)",
		projectID, name, description, config,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetDeploymentEnv(id)
}

func (s *Store) GetDeploymentEnv(id int64) (*DeploymentEnv, error) {
	d := &DeploymentEnv{}
	var desc sql.NullString
	var lastBuild sql.NullInt64
	err := s.db.QueryRow(
		"SELECT id, project_id, name, description, config, last_build_id, created_at, updated_at FROM deployment_envs WHERE id = ?",
		id,
	).Scan(&d.ID, &d.ProjectID, &d.Name, &desc, &d.Config, &lastBuild, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	d.Description = desc.String
	if lastBuild.Valid {
		v := lastBuild.Int64
		d.LastBuildID = &v
	}
	return d, nil
}

func (s *Store) ListDeploymentEnvs(projectID int64) ([]*DeploymentEnv, error) {
	rows, err := s.db.Query(
		"SELECT id, project_id, name, description, config, last_build_id, created_at, updated_at FROM deployment_envs WHERE project_id = ? ORDER BY id",
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var envs []*DeploymentEnv
	for rows.Next() {
		d := &DeploymentEnv{}
		var desc sql.NullString
		var lastBuild sql.NullInt64
		if err := rows.Scan(&d.ID, &d.ProjectID, &d.Name, &desc, &d.Config, &lastBuild, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		d.Description = desc.String
		if lastBuild.Valid {
			v := lastBuild.Int64
			d.LastBuildID = &v
		}
		envs = append(envs, d)
	}
	return envs, nil
}

func (s *Store) UpdateDeploymentEnv(id int64, name, description, config string) error {
	_, err := s.db.Exec(
		"UPDATE deployment_envs SET name=?, description=?, config=?, updated_at=? WHERE id=?",
		name, description, config, time.Now(), id,
	)
	return err
}

func (s *Store) UpdateDeploymentLastBuild(envID, buildID int64) error {
	_, err := s.db.Exec("UPDATE deployment_envs SET last_build_id=?, updated_at=? WHERE id=?", buildID, time.Now(), envID)
	return err
}

func (s *Store) DeleteDeploymentEnv(id int64) error {
	_, err := s.db.Exec("DELETE FROM deployment_envs WHERE id = ?", id)
	return err
}

// ---------------- Project Groups ----------------

func (s *Store) CreateProjectGroup(name, description string, parentID *int64) (*ProjectGroup, error) {
	res, err := s.db.Exec(
		"INSERT INTO project_groups (name, description, parent_id) VALUES (?, ?, ?)",
		name, description, parentID,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetProjectGroup(id)
}

func (s *Store) GetProjectGroup(id int64) (*ProjectGroup, error) {
	g := &ProjectGroup{}
	var desc sql.NullString
	var parentID sql.NullInt64
	err := s.db.QueryRow(
		"SELECT id, name, description, parent_id, created_at FROM project_groups WHERE id = ?",
		id,
	).Scan(&g.ID, &g.Name, &desc, &parentID, &g.CreatedAt)
	if err != nil {
		return nil, err
	}
	g.Description = desc.String
	if parentID.Valid {
		v := parentID.Int64
		g.ParentID = &v
	}
	return g, nil
}

func (s *Store) ListProjectGroups() ([]*ProjectGroup, error) {
	rows, err := s.db.Query("SELECT id, name, description, parent_id, created_at FROM project_groups ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []*ProjectGroup
	for rows.Next() {
		g := &ProjectGroup{}
		var desc sql.NullString
		var parentID sql.NullInt64
		if err := rows.Scan(&g.ID, &g.Name, &desc, &parentID, &g.CreatedAt); err != nil {
			return nil, err
		}
		g.Description = desc.String
		if parentID.Valid {
			v := parentID.Int64
			g.ParentID = &v
		}
		groups = append(groups, g)
	}
	return groups, nil
}

func (s *Store) DeleteProjectGroup(id int64) error {
	_, err := s.db.Exec("DELETE FROM project_groups WHERE id = ?", id)
	return err
}

// ---------------- Build Queue Items ----------------

func (s *Store) CreateBuildQueueItem(buildID, projectID int64, projectName string, priority int, trigger, branch string) (*BuildQueueItem, error) {
	res, err := s.db.Exec(
		"INSERT INTO build_queue_items (build_id, project_id, project_name, priority, status, trigger, branch) VALUES (?, ?, ?, ?, 'queued', ?, ?)",
		buildID, projectID, projectName, priority, trigger, branch,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	row := s.db.QueryRow(
		"SELECT id, build_id, project_id, project_name, priority, status, trigger, branch, queued_at, started_at FROM build_queue_items WHERE id = ?",
		id,
	)
	item := &BuildQueueItem{}
	var started sql.NullTime
	if err := row.Scan(&item.ID, &item.BuildID, &item.ProjectID, &item.ProjectName, &item.Priority, &item.Status, &item.Trigger, &item.Branch, &item.QueuedAt, &started); err != nil {
		return nil, err
	}
	if started.Valid {
		item.StartedAt = &started.Time
	}
	return item, nil
}

func (s *Store) ListBuildQueue(status string) ([]*BuildQueueItem, error) {
	q := "SELECT id, build_id, project_id, project_name, priority, status, trigger, branch, queued_at, started_at FROM build_queue_items"
	var args []interface{}
	if status != "" {
		q += " WHERE status = ?"
		args = append(args, status)
	}
	q += " ORDER BY priority DESC, queued_at ASC"
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []*BuildQueueItem
	for rows.Next() {
		item := &BuildQueueItem{}
		var started sql.NullTime
		if err := rows.Scan(&item.ID, &item.BuildID, &item.ProjectID, &item.ProjectName, &item.Priority, &item.Status, &item.Trigger, &item.Branch, &item.QueuedAt, &started); err != nil {
			return nil, err
		}
		if started.Valid {
			item.StartedAt = &started.Time
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Store) UpdateBuildQueueItemStatus(id int64, status string) error {
	var startedAt interface{}
	if status == "running" {
		startedAt = time.Now()
	}
	_, err := s.db.Exec(
		"UPDATE build_queue_items SET status=?, started_at=COALESCE(?, started_at) WHERE id=?",
		status, startedAt, id,
	)
	return err
}

func (s *Store) UpdateBuildQueueItemPriority(id, priority int64) error {
	_, err := s.db.Exec("UPDATE build_queue_items SET priority=? WHERE id=?", priority, id)
	return err
}

// ---------------- Git Hooks ----------------

func (s *Store) CreateGitHook(projectID int64, name string, event GitHookEvent, branch, secret string, enabled bool, buildParams, description string) (*GitHook, error) {
	if buildParams == "" {
		buildParams = "{}"
	}
	res, err := s.db.Exec(
		"INSERT INTO git_hooks (project_id, name, event, branch, secret, enabled, build_params, description) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		projectID, name, string(event), branch, secret, enabled, buildParams, description,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetGitHook(id)
}

func (s *Store) GetGitHook(id int64) (*GitHook, error) {
	h := &GitHook{}
	var branch, secret, desc sql.NullString
	err := s.db.QueryRow(
		"SELECT id, project_id, name, event, branch, secret, enabled, build_params, description, created_at, updated_at FROM git_hooks WHERE id = ?",
		id,
	).Scan(&h.ID, &h.ProjectID, &h.Name, &h.Event, &branch, &secret, &h.Enabled, &h.BuildParams, &desc, &h.CreatedAt, &h.UpdatedAt)
	if err != nil {
		return nil, err
	}
	h.Branch = branch.String
	h.Secret = secret.String
	h.Description = desc.String
	return h, nil
}

func (s *Store) ListGitHooks(projectID int64) ([]*GitHook, error) {
	rows, err := s.db.Query(
		"SELECT id, project_id, name, event, branch, secret, enabled, build_params, description, created_at, updated_at FROM git_hooks WHERE project_id = ? ORDER BY id",
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hooks []*GitHook
	for rows.Next() {
		h := &GitHook{}
		var branch, secret, desc sql.NullString
		if err := rows.Scan(&h.ID, &h.ProjectID, &h.Name, &h.Event, &branch, &secret, &h.Enabled, &h.BuildParams, &desc, &h.CreatedAt, &h.UpdatedAt); err != nil {
			return nil, err
		}
		h.Branch = branch.String
		h.Secret = secret.String
		h.Description = desc.String
		hooks = append(hooks, h)
	}
	return hooks, nil
}

func (s *Store) UpdateGitHook(id int64, name string, event GitHookEvent, branch, secret string, enabled bool, buildParams, description string) error {
	if buildParams == "" {
		buildParams = "{}"
	}
	_, err := s.db.Exec(
		"UPDATE git_hooks SET name=?, event=?, branch=?, secret=?, enabled=?, build_params=?, description=?, updated_at=? WHERE id=?",
		name, string(event), branch, secret, enabled, buildParams, description, time.Now(), id,
	)
	return err
}

func (s *Store) DeleteGitHook(id int64) error {
	_, err := s.db.Exec("DELETE FROM git_hooks WHERE id = ?", id)
	return err
}

func (s *Store) GetGitHookByProjectAndEvent(projectID int64, event string) ([]*GitHook, error) {
	rows, err := s.db.Query(
		"SELECT id, project_id, name, event, branch, secret, enabled, build_params, description, created_at, updated_at FROM git_hooks WHERE project_id = ? AND event = ? AND enabled = 1",
		projectID, event,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hooks []*GitHook
	for rows.Next() {
		h := &GitHook{}
		var branch, secret, desc sql.NullString
		if err := rows.Scan(&h.ID, &h.ProjectID, &h.Name, &h.Event, &branch, &secret, &h.Enabled, &h.BuildParams, &desc, &h.CreatedAt, &h.UpdatedAt); err != nil {
			return nil, err
		}
		h.Branch = branch.String
		h.Secret = secret.String
		h.Description = desc.String
		hooks = append(hooks, h)
	}
	return hooks, nil
}

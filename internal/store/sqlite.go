package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	_ "github.com/glebarez/go-sqlite"
)

type Store struct {
	db *sql.DB
}

var ErrBuildNotCancellable = errors.New("build is not active")
var ErrRequiredNotificationChannel = errors.New("required notification channel must remain enabled")
var ErrProjectGroupNameExists = errors.New("project group name already exists")
var ErrInvalidProjectGroupColor = errors.New("invalid project group color")
var ErrBuildTemplateInUse = errors.New("build template is in use")

const ProjectGroupColorNeutral = "neutral"

// IsValidProjectGroupColor limits group backgrounds to the muted palette
// supported by the Web client. Keeping tokens stable also makes API payloads
// and exported data independent of a particular CSS color value.
func IsValidProjectGroupColor(color string) bool {
	switch color {
	case ProjectGroupColorNeutral, "blue", "cyan", "mint", "green", "yellow", "orange", "pink", "purple":
		return true
	default:
		return false
	}
}

const (
	// BuildLogRetentionCharacters bounds the durable SQLite blob. UTF-8 makes
	// the byte ceiling at most four times this value, while character-based
	// SQLite substr avoids corrupting multi-byte log text.
	BuildLogRetentionCharacters = 1_000_000
	BuildLogAppendMaxBytes      = 256 * 1024
	BuildLogTruncationMarker    = "[buildworld] Earlier persisted log output was truncated; only the newest output is retained. Live WebSocket output was not truncated.\n"
	BuildLogOversizedMarker     = "[buildworld] One oversized log entry was truncated before persistence. "
)

const appendBuildLogExpression = `CASE
	WHEN length(COALESCE(log, '')) + length(?) <= ? THEN COALESCE(log, '') || ?
	ELSE ? || substr(COALESCE(log, ''), -(? - length(?))) || ?
END`

func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(1)
	// The store intentionally uses one SQLite connection, so WAL cannot add
	// read/write concurrency here. Rollback journaling avoids the persistent
	// WAL/SHM files and shared-memory locks that can strand background servers
	// after a forced exit, especially on Windows ReFS development volumes.
	if _, err := db.Exec("PRAGMA journal_mode=DELETE"); err != nil {
		db.Close()
		return nil, fmt.Errorf("configure database journal: %w", err)
	}
	// The database contains password hashes, repository credentials, API tokens,
	// notification secrets, and secret environment variables. Keep an existing
	// or newly created database private even when the process umask is permissive.
	if dbPath != ":memory:" && !strings.HasPrefix(dbPath, "file::memory:") {
		if err := os.Chmod(dbPath, 0o600); err != nil {
			db.Close()
			return nil, fmt.Errorf("secure database permissions: %w", err)
		}
	}

	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	if err := store.EnsureDefaultWebNotificationChannel(); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) migrate() error {
	if err := runSchemaMigrations(s.db, schemaMigrations); err != nil {
		return err
	}
	return s.normalizeEmptyRepositoryTypes()
}

// normalizeEmptyRepositoryTypes repairs only historical empty values. It
// leaves every valid Git URL, branch, config, and association untouched.
func (s *Store) normalizeEmptyRepositoryTypes() error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin repository type normalization: %w", err)
	}
	statements := []string{
		`UPDATE projects SET repo_type = 'git' WHERE repo_type IS NULL OR trim(repo_type) = ''`,
		`UPDATE vcs_roots SET type = 'git' WHERE type IS NULL OR trim(type) = ''`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement); err != nil {
			tx.Rollback()
			return fmt.Errorf("normalize empty repository type: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit repository type normalization: %w", err)
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
		"SELECT id, username, email, password_hash, role, session_version, avatar_url, created_at, last_login FROM users WHERE id = ?",
		id,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role, &u.SessionVersion, &avatar, &u.CreatedAt, &lastLogin)
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
		"SELECT id, username, email, password_hash, role, session_version, avatar_url, created_at, last_login FROM users WHERE username = ?",
		username,
	).Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role, &u.SessionVersion, &avatar, &u.CreatedAt, &lastLogin)
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
	rows, err := s.db.Query("SELECT id, username, email, password_hash, role, session_version, avatar_url, created_at, last_login FROM users ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []*User
	for rows.Next() {
		u := &User{}
		var avatar sql.NullString
		var lastLogin sql.NullTime
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Role, &u.SessionVersion, &avatar, &u.CreatedAt, &lastLogin); err != nil {
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
	result, err := s.db.Exec("UPDATE users SET password_hash = ?, session_version = session_version + 1 WHERE id = ?", passwordHash, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) UpdateUserRole(id int64, role string) error {
	_, err := s.db.Exec("UPDATE users SET role = ? WHERE id = ?", role, id)
	return err
}

// UpdateUserProfile updates the non-credential profile fields of a user. It is
// used by the portability import path so a migrated user's identity (email,
// avatar) stays consistent without disturbing the password hash.
func (s *Store) UpdateUserProfile(id int64, email, avatarURL string) error {
	_, err := s.db.Exec("UPDATE users SET email = ?, avatar_url = ? WHERE id = ?", email, avatarURL, id)
	return err
}

func (s *Store) UpdateLastLogin(id int64) error {
	_, err := s.db.Exec("UPDATE users SET last_login = ? WHERE id = ?", time.Now(), id)
	return err
}

func (s *Store) DeleteUser(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Revoke credentials explicitly. This remains safe even on SQLite
	// installations where foreign-key enforcement was disabled historically.
	if _, err := tx.Exec("DELETE FROM api_tokens WHERE user_id = ?", id); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM users WHERE id = ?", id); err != nil {
		return err
	}
	return tx.Commit()
}

// ---------------- Projects ----------------

func (s *Store) CreateProject(name, description, repoURL, repoType, defaultBranch, config string, createdBy int64, vcsRootID, templateID *int64, tagSets ...[]string) (*Project, error) {
	var err error
	repoType, err = NormalizeRepositoryType(repoType)
	if err != nil {
		return nil, err
	}
	if defaultBranch == "" {
		defaultBranch = "main"
	}
	tags := []string{}
	if len(tagSets) > 0 {
		tags = normalizeProjectTags(tagSets[0])
	}
	tagsJSON, err := json.Marshal(tags)
	if err != nil {
		return nil, err
	}
	res, err := s.db.Exec(
		"INSERT INTO projects (name, description, repo_url, repo_type, default_branch, vcs_root_id, template_id, tags, config, created_by) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		name, description, repoURL, repoType, defaultBranch, vcsRootID, templateID, string(tagsJSON), config, createdBy,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetProject(id)
}

func (s *Store) GetProject(id int64) (*Project, error) {
	p := &Project{}
	var vcsRootID, templateID, groupID sql.NullInt64
	var tagsJSON string
	err := s.db.QueryRow(
		"SELECT id, name, description, repo_url, repo_type, default_branch, vcs_root_id, template_id, group_id, tags, config, pipeline_format, pipeline_source_mode, pipeline_scm_repo, pipeline_scm_branch, pipeline_scm_path, created_by, created_at, updated_at, favorite, quick_access, enabled FROM projects WHERE id = ?",
		id,
	).Scan(&p.ID, &p.Name, &p.Description, &p.RepoURL, &p.RepoType, &p.DefaultBranch, &vcsRootID, &templateID, &groupID, &tagsJSON, &p.Config, &p.PipelineFormat, &p.PipelineSourceMode, &p.PipelineSCMRepo, &p.PipelineSCMBranch, &p.PipelineSCMPath, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt, &p.Favorite, &p.QuickAccess, &p.Enabled)
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
	if groupID.Valid {
		v := groupID.Int64
		p.GroupID = &v
	}
	p.Tags = parseProjectTags(tagsJSON)
	return p, nil
}

func (s *Store) GetProjectByName(name string) (*Project, error) {
	p := &Project{}
	var vcsRootID, templateID, groupID sql.NullInt64
	var tagsJSON string
	err := s.db.QueryRow(
		"SELECT id, name, description, repo_url, repo_type, default_branch, vcs_root_id, template_id, group_id, tags, config, pipeline_format, pipeline_source_mode, pipeline_scm_repo, pipeline_scm_branch, pipeline_scm_path, created_by, created_at, updated_at, favorite, quick_access, enabled FROM projects WHERE name = ?",
		name,
	).Scan(&p.ID, &p.Name, &p.Description, &p.RepoURL, &p.RepoType, &p.DefaultBranch, &vcsRootID, &templateID, &groupID, &tagsJSON, &p.Config, &p.PipelineFormat, &p.PipelineSourceMode, &p.PipelineSCMRepo, &p.PipelineSCMBranch, &p.PipelineSCMPath, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt, &p.Favorite, &p.QuickAccess, &p.Enabled)
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
	if groupID.Valid {
		v := groupID.Int64
		p.GroupID = &v
	}
	p.Tags = parseProjectTags(tagsJSON)
	return p, nil
}

func (s *Store) ListProjects() ([]*Project, error) {
	rows, err := s.db.Query(`SELECT p.id, p.name, p.description, p.repo_url, p.repo_type, p.default_branch,
		p.vcs_root_id, p.template_id, p.group_id, p.tags, p.config, p.pipeline_format,
		p.pipeline_source_mode, p.pipeline_scm_repo, p.pipeline_scm_branch, p.pipeline_scm_path,
		p.created_by, p.created_at, p.updated_at, p.favorite, p.quick_access, p.enabled
		FROM projects p
		LEFT JOIN project_display_order project_order ON project_order.project_id=p.id
		ORDER BY project_order.position IS NULL, project_order.position, p.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var projects []*Project
	for rows.Next() {
		p := &Project{}
		var vcsRootID, templateID, groupID sql.NullInt64
		var tagsJSON string
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.RepoURL, &p.RepoType, &p.DefaultBranch, &vcsRootID, &templateID, &groupID, &tagsJSON, &p.Config, &p.PipelineFormat, &p.PipelineSourceMode, &p.PipelineSCMRepo, &p.PipelineSCMBranch, &p.PipelineSCMPath, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt, &p.Favorite, &p.QuickAccess, &p.Enabled); err != nil {
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
		if groupID.Valid {
			v := groupID.Int64
			p.GroupID = &v
		}
		p.Tags = parseProjectTags(tagsJSON)
		projects = append(projects, p)
	}
	return projects, nil
}

func (s *Store) ListProjectSummaries() ([]*ProjectSummary, error) {
	rows, err := s.db.Query(`
		SELECT id, name, description, repo_url, repo_type, default_branch,
			vcs_root_id, template_id, group_id, tags, pipeline_format, pipeline_source_mode,
			created_by, created_at, updated_at, favorite, quick_access, enabled
		FROM projects
		LEFT JOIN project_display_order project_order ON project_order.project_id=projects.id
		ORDER BY project_order.position IS NULL, project_order.position, projects.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var projects []*ProjectSummary
	for rows.Next() {
		project := &ProjectSummary{}
		var vcsRootID, templateID, groupID sql.NullInt64
		var tagsJSON string
		if err := rows.Scan(
			&project.ID, &project.Name, &project.Description, &project.RepoURL,
			&project.RepoType, &project.DefaultBranch, &vcsRootID, &templateID,
			&groupID, &tagsJSON, &project.PipelineFormat, &project.PipelineSourceMode,
			&project.CreatedBy, &project.CreatedAt, &project.UpdatedAt,
			&project.Favorite, &project.QuickAccess, &project.Enabled,
		); err != nil {
			return nil, err
		}
		if vcsRootID.Valid {
			value := vcsRootID.Int64
			project.VCSRootID = &value
		}
		if templateID.Valid {
			value := templateID.Int64
			project.TemplateID = &value
		}
		if groupID.Valid {
			value := groupID.Int64
			project.GroupID = &value
		}
		project.Tags = parseProjectTags(tagsJSON)
		projects = append(projects, project)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return projects, nil
}

// ListQuickAccessProjects returns projects flagged for quick access, ordered so
// favorites appear first. It powers the dashboard sidebar quick-access section.
func (s *Store) ListQuickAccessProjects() ([]*ProjectSummary, error) {
	rows, err := s.db.Query(`
		SELECT id, name, description, repo_url, repo_type, default_branch,
			vcs_root_id, template_id, group_id, tags, pipeline_format, pipeline_source_mode,
			created_by, created_at, updated_at, favorite, quick_access, enabled
		FROM projects
		LEFT JOIN project_display_order project_order ON project_order.project_id=projects.id
		WHERE quick_access = 1
		ORDER BY project_order.position IS NULL, project_order.position, projects.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var projects []*ProjectSummary
	for rows.Next() {
		project := &ProjectSummary{}
		var vcsRootID, templateID, groupID sql.NullInt64
		var tagsJSON string
		if err := rows.Scan(
			&project.ID, &project.Name, &project.Description, &project.RepoURL,
			&project.RepoType, &project.DefaultBranch, &vcsRootID, &templateID,
			&groupID, &tagsJSON, &project.PipelineFormat, &project.PipelineSourceMode,
			&project.CreatedBy, &project.CreatedAt, &project.UpdatedAt,
			&project.Favorite, &project.QuickAccess, &project.Enabled,
		); err != nil {
			return nil, err
		}
		if vcsRootID.Valid {
			value := vcsRootID.Int64
			project.VCSRootID = &value
		}
		if templateID.Valid {
			value := templateID.Int64
			project.TemplateID = &value
		}
		if groupID.Valid {
			value := groupID.Int64
			project.GroupID = &value
		}
		project.Tags = parseProjectTags(tagsJSON)
		projects = append(projects, project)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return projects, nil
}

// SetProjectFlags updates the favorite and quick_access booleans for a project.
func (s *Store) SetProjectFlags(id int64, favorite, quickAccess bool) error {
	_, err := s.db.Exec(
		"UPDATE projects SET favorite=?, quick_access=?, updated_at=? WHERE id=?",
		favorite, quickAccess, time.Now(), id,
	)
	return err
}

// ReorderProjects replaces global dashboard order. Requiring every current
// project exactly once prevents filtered views or stale clients from silently
// dropping projects from the ordering set.
func (s *Store) ReorderProjects(orderedIDs []int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.Query("SELECT id FROM projects")
	if err != nil {
		return err
	}
	current := make(map[int64]struct{})
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		current[id] = struct{}{}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(orderedIDs) != len(current) {
		return fmt.Errorf("project order must contain all %d projects", len(current))
	}
	seen := make(map[int64]struct{}, len(orderedIDs))
	for _, id := range orderedIDs {
		if _, exists := current[id]; !exists {
			return fmt.Errorf("project %d not found", id)
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("project %d appears more than once", id)
		}
		seen[id] = struct{}{}
	}
	if _, err := tx.Exec("DELETE FROM project_display_order"); err != nil {
		return err
	}
	for position, id := range orderedIDs {
		if _, err := tx.Exec(
			"INSERT INTO project_display_order (project_id, position) VALUES (?, ?)",
			id, position,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SetProjectEnabled(id int64, enabled bool) error {
	result, err := s.db.Exec(
		"UPDATE projects SET enabled=?, updated_at=? WHERE id=?",
		enabled, time.Now(), id,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// SetProjectPipelineSource persists the pipeline authoring configuration for a
// project: the source syntax (typescript / yaml / jenkinsfile) and, when the
// pipeline definition is sourced from a repository, the SCM coordinates. It is
// written separately from CreateProject/UpdateProject so the positional
// signatures of those methods stay stable for the many test call sites.
func (s *Store) SetProjectPipelineSource(id int64, format, sourceMode, scmRepo, scmBranch, scmPath string) error {
	if format == "" {
		format = "yaml"
	}
	if sourceMode == "" {
		sourceMode = "inline"
	}
	_, err := s.db.Exec(
		"UPDATE projects SET pipeline_format=?, pipeline_source_mode=?, pipeline_scm_repo=?, pipeline_scm_branch=?, pipeline_scm_path=?, updated_at=? WHERE id=?",
		format, sourceMode, scmRepo, scmBranch, scmPath, time.Now(), id,
	)
	return err
}

func (s *Store) ListProjectsByVCSRoot(vcsRootID int64) ([]*Project, error) {
	rows, err := s.db.Query("SELECT id, name, description, repo_url, repo_type, default_branch, vcs_root_id, template_id, group_id, tags, config, pipeline_format, pipeline_source_mode, pipeline_scm_repo, pipeline_scm_branch, pipeline_scm_path, created_by, created_at, updated_at, favorite, quick_access, enabled FROM projects WHERE vcs_root_id = ?", vcsRootID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var projects []*Project
	for rows.Next() {
		p := &Project{}
		var vrid, tid, gid sql.NullInt64
		var tagsJSON string
		if err := rows.Scan(&p.ID, &p.Name, &p.Description, &p.RepoURL, &p.RepoType, &p.DefaultBranch, &vrid, &tid, &gid, &tagsJSON, &p.Config, &p.PipelineFormat, &p.PipelineSourceMode, &p.PipelineSCMRepo, &p.PipelineSCMBranch, &p.PipelineSCMPath, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt, &p.Favorite, &p.QuickAccess, &p.Enabled); err != nil {
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
		if gid.Valid {
			v := gid.Int64
			p.GroupID = &v
		}
		p.Tags = parseProjectTags(tagsJSON)
		projects = append(projects, p)
	}
	return projects, nil
}

func (s *Store) UpdateProject(id int64, name, description, repoURL, repoType, defaultBranch, config string, vcsRootID, templateID *int64, tagSets ...[]string) error {
	var err error
	repoType, err = NormalizeRepositoryType(repoType)
	if err != nil {
		return err
	}
	tags := []string{}
	if len(tagSets) > 0 {
		tags = normalizeProjectTags(tagSets[0])
	}
	tagsJSON, err := json.Marshal(tags)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		"UPDATE projects SET name=?, description=?, repo_url=?, repo_type=?, default_branch=?, vcs_root_id=?, template_id=?, tags=?, config=?, updated_at=? WHERE id=?",
		name, description, repoURL, repoType, defaultBranch, vcsRootID, templateID, string(tagsJSON), config, time.Now(), id,
	)
	return err
}

func (s *Store) SetProjectGroup(projectID int64, groupID *int64) error {
	if groupID != nil {
		if *groupID <= 0 {
			return fmt.Errorf("invalid project group id")
		}
		if _, err := s.GetProjectGroup(*groupID); err != nil {
			return fmt.Errorf("project group not found: %w", err)
		}
	}
	result, err := s.db.Exec("UPDATE projects SET group_id=?, updated_at=? WHERE id=?", groupID, time.Now(), projectID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func normalizeProjectTags(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		key := strings.ToLower(tag)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, tag)
	}
	return result
}

func parseProjectTags(source string) []string {
	var tags []string
	if err := json.Unmarshal([]byte(source), &tags); err != nil {
		return []string{}
	}
	return normalizeProjectTags(tags)
}

func (s *Store) DeleteProject(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin project deletion: %w", err)
	}
	defer tx.Rollback()
	if err := deleteProjectTx(tx, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit project deletion: %w", err)
	}
	return nil
}

func deleteProjectTx(tx *sql.Tx, id int64) error {
	exec := func(label, query string, args ...interface{}) error {
		if _, err := tx.Exec(query, args...); err != nil {
			return fmt.Errorf("delete project %s: %w", label, err)
		}
		return nil
	}

	// Build dependencies can cross project boundaries. Preserve those builds
	// while removing references to history that belongs to the deleted project.
	if err := exec("build dependency references", `UPDATE builds SET wait_dependency_on=NULL
		WHERE wait_dependency_on IN (SELECT id FROM builds WHERE project_id=?)`, id); err != nil {
		return err
	}
	if err := exec("build retry references", `UPDATE builds SET retried_from=NULL
		WHERE retried_from IN (SELECT id FROM builds WHERE project_id=?)`, id); err != nil {
		return err
	}
	if err := exec("test result references", `UPDATE builds SET test_result_id=NULL
		WHERE test_result_id IN (
			SELECT result.id FROM test_results result
			JOIN builds build ON build.id=result.build_id
			WHERE build.project_id=?
		)`, id); err != nil {
		return err
	}

	// Delete build-owned records before builds. Explicit ordering works on both
	// foreign-key-enforced databases and installations created before FK checks
	// were consistently enabled.
	buildChildren := []struct {
		label string
		query string
		args  []interface{}
	}{
		{"notification events", "DELETE FROM notification_events WHERE build_id IN (SELECT id FROM builds WHERE project_id=?)", []interface{}{id}},
		{"artifacts", "DELETE FROM artifacts WHERE build_id IN (SELECT id FROM builds WHERE project_id=?)", []interface{}{id}},
		{"queue items", "DELETE FROM build_queue_items WHERE project_id=? OR build_id IN (SELECT id FROM builds WHERE project_id=?)", []interface{}{id, id}},
		{"test results", "DELETE FROM test_results WHERE build_id IN (SELECT id FROM builds WHERE project_id=?)", []interface{}{id}},
		{"build approvals", "DELETE FROM build_approvals WHERE build_id IN (SELECT id FROM builds WHERE project_id=?)", []interface{}{id}},
	}
	for _, child := range buildChildren {
		if err := exec(child.label, child.query, child.args...); err != nil {
			return err
		}
	}
	if err := exec("build statistics", "DELETE FROM build_stats WHERE project_id=?", id); err != nil {
		return err
	}
	if err := exec("environment variables", "DELETE FROM env_vars WHERE project_id=?", id); err != nil {
		return err
	}
	if err := exec("display order", "DELETE FROM project_display_order WHERE project_id=?", id); err != nil {
		return err
	}
	if err := exec("builds", "DELETE FROM builds WHERE project_id=?", id); err != nil {
		return err
	}
	// VCS roots, build templates, groups, users, channels, and credentials are
	// shared resources. Deleting only the project row preserves them.
	if err := exec("row", "DELETE FROM projects WHERE id=?", id); err != nil {
		return err
	}
	return nil
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

// PruneCompletedBuilds keeps the newest completed builds for a project. Pinned,
// pending, and running builds are always retained so the LRU policy cannot remove
// work that operators still need.
func (s *Store) PruneCompletedBuilds(projectID int64, retain int) error {
	if retain < 0 {
		return nil
	}
	rows, err := s.db.Query(`SELECT id FROM builds
		WHERE project_id = ? AND status IN ('success', 'failed', 'cancelled') AND pinned = 0
		ORDER BY COALESCE(finished_at, started_at) DESC, id DESC`, projectID)
	if err != nil {
		return err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if len(ids) <= retain {
		return rows.Err()
	}
	for _, id := range ids[retain:] {
		if _, err := s.db.Exec("DELETE FROM notification_events WHERE build_id = ?", id); err != nil {
			return err
		}
		if _, err := s.db.Exec("DELETE FROM artifacts WHERE build_id = ?", id); err != nil {
			return err
		}
		if _, err := s.db.Exec("DELETE FROM build_queue_items WHERE build_id = ?", id); err != nil {
			return err
		}
		if _, err := s.db.Exec("DELETE FROM test_results WHERE build_id = ?", id); err != nil {
			return err
		}
		if _, err := s.db.Exec("DELETE FROM build_approvals WHERE build_id = ?", id); err != nil {
			return err
		}
		if _, err := s.db.Exec("DELETE FROM builds WHERE id = ?", id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListPendingBuilds() ([]*Build, error) {
	rows, err := s.db.Query(
		`SELECT b.id, b.project_id, b.number, b.status, b.trigger, b.branch, b.commit_sha, b.parameters, b.wait_dependency_on, b.retried_from, b.pinned, b.log, b.started_at, b.finished_at, b.duration_ms, b.approval_required, b.approved_by, b.approved_at, b.timeout_sec, b.test_result_id
		 FROM builds b
		 LEFT JOIN build_queue_items q ON q.build_id = b.id
		 WHERE b.status = 'pending'
		 ORDER BY COALESCE(q.priority, 0) DESC, b.id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBuilds(rows)
}

// RecoverInterruptedBuilds requeues work that was executing when the server
// process exited. A normal user cancellation is terminal and is never touched.
// It is transactional so the build and its visible queue item cannot disagree
// after a restart.
func (s *Store) RecoverInterruptedBuilds() (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	rows, err := tx.Query("SELECT id FROM builds WHERE status = 'running'")
	if err != nil {
		return 0, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}

	for _, id := range ids {
		args := boundedBuildLogArgs("\n[buildworld] Requeued after interrupted server restart.\n")
		args = append(args, id)
		if _, err := tx.Exec("UPDATE builds SET status='pending', started_at=NULL, finished_at=NULL, duration_ms=NULL, log="+appendBuildLogExpression+" WHERE id=?", args...); err != nil {
			return 0, err
		}
		if _, err := tx.Exec("UPDATE build_queue_items SET status='queued', started_at=NULL WHERE build_id=?", id); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(ids), nil
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
	args := boundedBuildLogArgs(line)
	args = append(args, id)
	_, err := s.db.Exec("UPDATE builds SET log = "+appendBuildLogExpression+" WHERE id = ?", args...)
	return err
}

func boundedBuildLogArgs(line string) []any {
	line = boundBuildLogEntry(line)
	markerCharacters := utf8.RuneCountInString(BuildLogTruncationMarker)
	retainedCharacters := BuildLogRetentionCharacters - markerCharacters
	return []any{
		line,
		BuildLogRetentionCharacters,
		line,
		BuildLogTruncationMarker,
		retainedCharacters,
		line,
		line,
	}
}

func boundBuildLogEntry(line string) string {
	if !utf8.ValidString(line) {
		line = strings.ToValidUTF8(line, "\uFFFD")
	}
	if len(line) <= BuildLogAppendMaxBytes {
		return line
	}
	start := len(line) - BuildLogAppendMaxBytes
	for start < len(line) && !utf8.RuneStart(line[start]) {
		start++
	}
	return BuildLogTruncationMarker + BuildLogOversizedMarker + line[start:]
}

func IsBuildLogTruncated(log string) bool {
	return strings.HasPrefix(log, BuildLogTruncationMarker) || strings.Contains(log, BuildLogOversizedMarker)
}

func (s *Store) SetBuildLog(id int64, log string) error {
	log = retainCompleteBuildLog(log)
	_, err := s.db.Exec("UPDATE builds SET log = ? WHERE id = ?", log, id)
	return err
}

func retainCompleteBuildLog(log string) string {
	if !utf8.ValidString(log) {
		log = strings.ToValidUTF8(log, "\uFFFD")
	}
	characters := utf8.RuneCountInString(log)
	if characters <= BuildLogRetentionCharacters {
		return log
	}
	keep := BuildLogRetentionCharacters - utf8.RuneCountInString(BuildLogTruncationMarker)
	drop := characters - keep
	start := len(log)
	for index := range log {
		if drop == 0 {
			start = index
			break
		}
		drop--
	}
	return BuildLogTruncationMarker + log[start:]
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
	finishedAt := time.Now()
	result, err := s.db.Exec(`
		UPDATE builds
		SET status='cancelled',
		    finished_at=?,
		    duration_ms=CASE
		        WHEN started_at IS NULL THEN 0
		        ELSE MAX(0, CAST((julianday(?) - julianday(started_at)) * 86400000 AS INTEGER))
		    END
		WHERE id = ? AND status IN ('pending', 'running', 'pending_approval')`,
		finishedAt, finishedAt, buildID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected > 0 {
		return nil
	}
	var status string
	if err := s.db.QueryRow("SELECT status FROM builds WHERE id = ?", buildID).Scan(&status); err != nil {
		return err
	}
	return ErrBuildNotCancellable
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
		"SELECT id, name, address, token_hash, labels, pool, max_concurrent_builds, active_builds, status, last_heartbeat, created_at FROM workers WHERE id = ?",
		id,
	).Scan(&w.ID, &w.Name, &w.Address, &w.TokenHash, &labels, &pool, &w.MaxConcurrentBuilds, &w.ActiveBuilds, &w.Status, &hb, &w.CreatedAt)
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
	rows, err := s.db.Query("SELECT id, name, address, token_hash, labels, pool, max_concurrent_builds, active_builds, status, last_heartbeat, created_at FROM workers ORDER BY created_at")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var workers []*Worker
	for rows.Next() {
		w := &Worker{}
		var labels, pool sql.NullString
		var hb sql.NullTime
		if err := rows.Scan(&w.ID, &w.Name, &w.Address, &w.TokenHash, &labels, &pool, &w.MaxConcurrentBuilds, &w.ActiveBuilds, &w.Status, &hb, &w.CreatedAt); err != nil {
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
	rows, err := s.db.Query("SELECT id, name, address, token_hash, labels, pool, max_concurrent_builds, active_builds, status, last_heartbeat, created_at FROM workers WHERE status = 'online' ORDER BY active_builds ASC, created_at")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var workers []*Worker
	for rows.Next() {
		w := &Worker{}
		var labels, pool sql.NullString
		var hb sql.NullTime
		if err := rows.Scan(&w.ID, &w.Name, &w.Address, &w.TokenHash, &labels, &pool, &w.MaxConcurrentBuilds, &w.ActiveBuilds, &w.Status, &hb, &w.CreatedAt); err != nil {
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

// TryAcquireWorker reserves one execution slot using a single conditional SQL
// update. Concurrent scheduler goroutines cannot overbook a worker this way.
func (s *Store) TryAcquireWorker(id string) (bool, error) {
	result, err := s.db.Exec("UPDATE workers SET active_builds = active_builds + 1 WHERE id = ? AND status = 'online' AND active_builds < max_concurrent_builds", id)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

// ReleaseWorker frees a server-side scheduling lease. The guarded expression
// makes cleanup idempotent for failed or cancelled remote dispatches.
func (s *Store) ReleaseWorker(id string) error {
	_, err := s.db.Exec("UPDATE workers SET active_builds = CASE WHEN active_builds > 0 THEN active_builds - 1 ELSE 0 END WHERE id = ?", id)
	return err
}

func (s *Store) DeleteWorker(id string) error {
	_, err := s.db.Exec("DELETE FROM workers WHERE id=?", id)
	return err
}

func (s *Store) MarkOfflineWorkers() error {
	_, err := s.db.Exec("UPDATE workers SET status='offline', active_builds=0 WHERE last_heartbeat IS NOT NULL AND last_heartbeat < ?", time.Now().Add(-30*time.Second))
	return err
}

// ---------------- Plugins ----------------

func (s *Store) CreatePlugin(name, version, description, author, config, path, source string) (*Plugin, error) {
	res, err := s.db.Exec(
		"INSERT INTO plugins (name, version, description, author, enabled, config, path, source) VALUES (?, ?, ?, ?, 1, ?, ?, ?)",
		name, version, description, author, config, path, source,
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
	var desc, author, cfg, path, source sql.NullString
	var updatedAt sql.NullTime
	err := row.Scan(&p.ID, &p.Name, &p.Version, &desc, &author, &p.Enabled, &cfg, &path, &source, &p.InstalledAt, &updatedAt)
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
	if path.Valid {
		p.Path = path.String
	}
	if source.Valid {
		p.Source = source.String
	}
	if updatedAt.Valid {
		p.UpdatedAt = updatedAt.Time
	}
	return nil
}

func (s *Store) GetPlugin(id int64) (*Plugin, error) {
	p := &Plugin{}
	err := scanPlugin(p, s.db.QueryRow(
		"SELECT id, name, version, description, author, enabled, config, path, source, installed_at, updated_at FROM plugins WHERE id = ?", id))
	return p, err
}

func (s *Store) GetPluginByName(name string) (*Plugin, error) {
	p := &Plugin{}
	err := scanPlugin(p, s.db.QueryRow(
		"SELECT id, name, version, description, author, enabled, config, path, source, installed_at, updated_at FROM plugins WHERE name = ?", name))
	return p, err
}

func (s *Store) ListPlugins() ([]*Plugin, error) {
	rows, err := s.db.Query("SELECT id, name, version, description, author, enabled, config, path, source, installed_at, updated_at FROM plugins ORDER BY id")
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

func (s *Store) UpdatePluginMetadata(id int64, version, description, author, path, source string) error {
	_, err := s.db.Exec(
		"UPDATE plugins SET version=?, description=?, author=?, path=?, source=?, updated_at=? WHERE id=?",
		version, description, author, path, source, time.Now(), id,
	)
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
	normalizedType, err := NormalizeCredentialType(string(credType))
	if err != nil {
		return nil, err
	}
	res, err := s.db.Exec(
		"INSERT INTO credentials (name, type, host, username, password, private_key, public_key, token, description, is_secret) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		name, string(normalizedType), host, username, password, privateKey, publicKey, token, description, isSecret,
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
	normalizedType, err := NormalizeCredentialType(string(credType))
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		"UPDATE credentials SET name=?, type=?, host=?, username=?, password=?, private_key=?, public_key=?, token=?, description=?, is_secret=?, updated_at=? WHERE id=?",
		name, string(normalizedType), host, username, password, privateKey, publicKey, token, description, isSecret, time.Now(), id,
	)
	return err
}

func (s *Store) DeleteCredential(id int64) error {
	_, err := s.db.Exec("DELETE FROM credentials WHERE id = ?", id)
	return err
}

// ---------------- VCS repository templates (Git only) ----------------

func (s *Store) CreateVCSRoot(name, vcsType, url, branch string, credentialID *int64, pollInterval int, autoCheckout bool, config string) (*VCSRoot, error) {
	var err error
	vcsType, err = NormalizeRepositoryType(vcsType)
	if err != nil {
		return nil, err
	}
	if branch == "" {
		branch = "main"
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
	var err error
	vcsType, err = NormalizeRepositoryType(vcsType)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
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
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var references int
	if err := tx.QueryRow("SELECT COUNT(*) FROM projects WHERE template_id = ?", id).Scan(&references); err != nil {
		return err
	}
	if references > 0 {
		return fmt.Errorf("%w: %d project(s)", ErrBuildTemplateInUse, references)
	}
	if _, err := tx.Exec("DELETE FROM build_templates WHERE id = ?", id); err != nil {
		return err
	}
	return tx.Commit()
}

// ---------------- Notification Channels ----------------

func (s *Store) EnsureDefaultWebNotificationChannel() error {
	_, err := s.db.Exec(
		`INSERT INTO notification_channels (name, type, config, conditions, description, enabled)
		 VALUES (?, ?, '{}', '{}', ?, 1)
		 ON CONFLICT(name) DO UPDATE SET
		   type = excluded.type,
		   config = '{}',
		   enabled = 1,
		   updated_at = CURRENT_TIMESTAMP`,
		DefaultWebNotificationChannelName,
		string(NotificationChannelWeb),
		"系统标配的页面内实时构建通知；与其他启用渠道并行投递。",
	)
	return err
}

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

func (s *Store) GetNotificationChannelByName(name string) (*NotificationChannel, error) {
	c := &NotificationChannel{}
	var desc, cfg, cond sql.NullString
	err := s.db.QueryRow(
		"SELECT id, name, type, config, conditions, description, enabled, created_at, updated_at FROM notification_channels WHERE name = ?",
		name,
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
	existing, err := s.GetNotificationChannel(id)
	if err != nil {
		return err
	}
	if existing.Name == DefaultWebNotificationChannelName && existing.Type == NotificationChannelWeb {
		if !enabled || name != DefaultWebNotificationChannelName || channelType != NotificationChannelWeb {
			return ErrRequiredNotificationChannel
		}
		name = DefaultWebNotificationChannelName
		channelType = NotificationChannelWeb
		config = "{}"
		enabled = true
	}
	_, err = s.db.Exec(
		"UPDATE notification_channels SET name=?, type=?, config=?, conditions=?, description=?, enabled=?, updated_at=? WHERE id=?",
		name, string(channelType), config, conditions, description, enabled, time.Now(), id,
	)
	return err
}

func (s *Store) DeleteNotificationChannel(id int64) error {
	existing, err := s.GetNotificationChannel(id)
	if err != nil {
		return err
	}
	if existing.Name == DefaultWebNotificationChannelName && existing.Type == NotificationChannelWeb {
		return ErrRequiredNotificationChannel
	}
	_, err = s.db.Exec("DELETE FROM notification_channels WHERE id = ?", id)
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

func (s *Store) GetInAppNotificationFeed(userID int64, limit int) (*InAppNotificationFeed, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	lastReadID := int64(0)
	err := s.db.QueryRow("SELECT last_event_id FROM web_notification_reads WHERE user_id = ?", userID).Scan(&lastReadID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	rows, err := s.db.Query(
		`SELECT ne.id, ne.channel_id, ne.build_id, ne.event_type, ne.payload, ne.status,
		        ne.error_message, ne.delivered_at, ne.created_at
		   FROM notification_events ne
		   JOIN notification_channels nc ON nc.id = ne.channel_id
		  WHERE nc.type = ? AND ne.status = 'delivered'
		  ORDER BY ne.id DESC
		  LIMIT ?`,
		string(NotificationChannelWeb), limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items, err := scanNotificationEvents(rows)
	if err != nil {
		return nil, err
	}

	unread := 0
	if err := s.db.QueryRow(
		`SELECT COUNT(*)
		   FROM notification_events ne
		   JOIN notification_channels nc ON nc.id = ne.channel_id
		  WHERE nc.type = ? AND ne.status = 'delivered' AND ne.id > ?`,
		string(NotificationChannelWeb), lastReadID,
	).Scan(&unread); err != nil {
		return nil, err
	}
	return &InAppNotificationFeed{Items: items, Unread: unread, LastReadID: lastReadID}, nil
}

func (s *Store) MarkInAppNotificationsRead(userID, lastEventID int64) error {
	if userID <= 0 || lastEventID < 0 {
		return errors.New("invalid notification read cursor")
	}
	var exists int
	if lastEventID > 0 {
		if err := s.db.QueryRow(
			`SELECT COUNT(*)
			   FROM notification_events ne
			   JOIN notification_channels nc ON nc.id = ne.channel_id
			  WHERE ne.id = ? AND nc.type = ?`,
			lastEventID, string(NotificationChannelWeb),
		).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return sql.ErrNoRows
		}
	}
	_, err := s.db.Exec(
		`INSERT INTO web_notification_reads (user_id, last_event_id, updated_at)
		 VALUES (?, ?, ?)
		 ON CONFLICT(user_id) DO UPDATE SET
		   last_event_id = CASE
		     WHEN excluded.last_event_id > web_notification_reads.last_event_id THEN excluded.last_event_id
		     ELSE web_notification_reads.last_event_id
		   END,
		   updated_at = excluded.updated_at`,
		userID, lastEventID, time.Now(),
	)
	return err
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
		`SELECT
		   date(COALESCE(finished_at, started_at), 'localtime') AS build_date,
		   COUNT(*) AS total,
		   COALESCE(SUM(CASE WHEN status='success' THEN 1 ELSE 0 END), 0) AS success,
		   COALESCE(SUM(CASE WHEN status='failed' THEN 1 ELSE 0 END), 0) AS failed,
		   CAST(COALESCE(AVG(COALESCE(duration_ms, 0)), 0) AS INTEGER) AS avg_duration
		 FROM builds
		 WHERE project_id=?
		   AND status IN ('success', 'failed')
		   AND COALESCE(finished_at, started_at) IS NOT NULL
		   AND date(COALESCE(finished_at, started_at), 'localtime') >= date('now', 'localtime', ?)
		 GROUP BY build_date
		 ORDER BY build_date DESC`,
		projectID, "-"+strconv.Itoa(days)+" days",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var stats []*BuildStat
	for rows.Next() {
		bs := &BuildStat{ProjectID: projectID}
		if err := rows.Scan(&bs.Date, &bs.TotalBuilds, &bs.SuccessCount, &bs.FailedCount, &bs.AvgDuration); err != nil {
			return nil, err
		}
		stats = append(stats, bs)
	}
	return stats, rows.Err()
}

func (s *Store) GetDashboardStats() ([]*BuildStat, error) {
	rows, err := s.db.Query(
		`SELECT
		   date(COALESCE(finished_at, started_at), 'localtime') AS build_date,
		   COUNT(*) AS total,
		   COALESCE(SUM(CASE WHEN status='success' THEN 1 ELSE 0 END), 0) AS success,
		   COALESCE(SUM(CASE WHEN status='failed' THEN 1 ELSE 0 END), 0) AS failed,
		   CAST(COALESCE(AVG(COALESCE(duration_ms, 0)), 0) AS INTEGER) AS avg_duration
		 FROM builds
		 WHERE status IN ('success', 'failed')
		   AND COALESCE(finished_at, started_at) IS NOT NULL
		   AND date(COALESCE(finished_at, started_at), 'localtime') >= date('now', 'localtime', '-30 days')
		 GROUP BY build_date
		 ORDER BY build_date DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var stats []*BuildStat
	for rows.Next() {
		bs := &BuildStat{}
		if err := rows.Scan(&bs.Date, &bs.TotalBuilds, &bs.SuccessCount, &bs.FailedCount, &bs.AvgDuration); err != nil {
			return nil, err
		}
		stats = append(stats, bs)
	}
	return stats, rows.Err()
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

// DeleteAPITokenForUser deletes only tokens owned by the authenticated user.
// The boolean result distinguishes an absent/foreign token from a successful
// delete without exposing which of those cases occurred.
func (s *Store) DeleteAPITokenForUser(id, userID int64) (bool, error) {
	result, err := s.db.Exec("DELETE FROM api_tokens WHERE id = ? AND user_id = ?", id, userID)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected == 1, err
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
	var resolvedBy sql.NullInt64
	var resolvedByUsername sql.NullString
	err := s.db.QueryRow(
		"SELECT id, build_id, user_id, username, status, comment, created_at, resolved_at, resolved_by, resolved_by_username FROM build_approvals WHERE id = ?",
		id,
	).Scan(&a.ID, &a.BuildID, &a.UserID, &a.Username, &a.Status, &comment, &a.CreatedAt, &resolved, &resolvedBy, &resolvedByUsername)
	if err != nil {
		return nil, err
	}
	a.Comment = comment.String
	if resolved.Valid {
		a.ResolvedAt = &resolved.Time
	}
	if resolvedBy.Valid {
		value := resolvedBy.Int64
		a.ResolvedBy = &value
	}
	a.ResolvedByUsername = resolvedByUsername.String
	return a, nil
}

func (s *Store) GetBuildApprovalByBuild(buildID int64) (*BuildApproval, error) {
	a := &BuildApproval{}
	var comment sql.NullString
	var resolved sql.NullTime
	var resolvedBy sql.NullInt64
	var resolvedByUsername sql.NullString
	err := s.db.QueryRow(
		"SELECT id, build_id, user_id, username, status, comment, created_at, resolved_at, resolved_by, resolved_by_username FROM build_approvals WHERE build_id = ? ORDER BY id DESC LIMIT 1",
		buildID,
	).Scan(&a.ID, &a.BuildID, &a.UserID, &a.Username, &a.Status, &comment, &a.CreatedAt, &resolved, &resolvedBy, &resolvedByUsername)
	if err != nil {
		return nil, err
	}
	a.Comment = comment.String
	if resolved.Valid {
		a.ResolvedAt = &resolved.Time
	}
	if resolvedBy.Valid {
		value := resolvedBy.Int64
		a.ResolvedBy = &value
	}
	a.ResolvedByUsername = resolvedByUsername.String
	return a, nil
}

func (s *Store) ListPendingApprovals() ([]*BuildApproval, error) {
	rows, err := s.db.Query(
		"SELECT id, build_id, user_id, username, status, comment, created_at, resolved_at, resolved_by, resolved_by_username FROM build_approvals WHERE status = 'pending' ORDER BY id DESC",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBuildApprovals(rows)
}

func (s *Store) UpdateBuildApproval(id int64, status, comment string, resolvedBy int64, resolvedByUsername string) error {
	_, err := s.db.Exec(
		"UPDATE build_approvals SET status=?, comment=?, resolved_at=?, resolved_by=?, resolved_by_username=? WHERE id=?",
		status, strings.TrimSpace(comment), time.Now(), resolvedBy, resolvedByUsername, id,
	)
	return err
}

func (s *Store) CancelPendingBuildApproval(buildID, resolvedBy int64, resolvedByUsername string) error {
	_, err := s.db.Exec(
		`UPDATE build_approvals
		 SET status='cancelled', comment='build cancelled', resolved_at=?, resolved_by=?, resolved_by_username=?
		 WHERE build_id=? AND status='pending'`,
		time.Now(), resolvedBy, resolvedByUsername, buildID,
	)
	return err
}

func scanBuildApprovals(rows *sql.Rows) ([]*BuildApproval, error) {
	var approvals []*BuildApproval
	for rows.Next() {
		a := &BuildApproval{}
		var comment sql.NullString
		var resolved sql.NullTime
		var resolvedBy sql.NullInt64
		var resolvedByUsername sql.NullString
		if err := rows.Scan(&a.ID, &a.BuildID, &a.UserID, &a.Username, &a.Status, &comment, &a.CreatedAt, &resolved, &resolvedBy, &resolvedByUsername); err != nil {
			return nil, err
		}
		a.Comment = comment.String
		if resolved.Valid {
			a.ResolvedAt = &resolved.Time
		}
		if resolvedBy.Valid {
			value := resolvedBy.Int64
			a.ResolvedBy = &value
		}
		a.ResolvedByUsername = resolvedByUsername.String
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

// ---------------- Project Groups ----------------

func (s *Store) CreateProjectGroup(name, description string) (*ProjectGroup, error) {
	return s.CreateProjectGroupWithColor(name, description, ProjectGroupColorNeutral)
}

func (s *Store) CreateProjectGroupWithColor(name, description, color string) (*ProjectGroup, error) {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if name == "" {
		return nil, fmt.Errorf("project group name is required")
	}
	if !IsValidProjectGroupColor(color) {
		return nil, ErrInvalidProjectGroupColor
	}
	if _, err := s.GetProjectGroupByName(name); err == nil {
		return nil, ErrProjectGroupNameExists
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	res, err := s.db.Exec(
		"INSERT INTO project_groups (name, description, color, updated_at) VALUES (?, ?, ?, ?)",
		name, description, color, time.Now(),
	)
	if err != nil {
		if isProjectGroupNameConstraint(err) {
			return nil, ErrProjectGroupNameExists
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetProjectGroup(id)
}

func (s *Store) GetProjectGroup(id int64) (*ProjectGroup, error) {
	g := &ProjectGroup{}
	var desc sql.NullString
	var updatedAt sql.NullTime
	err := s.db.QueryRow(
		"SELECT id, name, description, color, created_at, updated_at FROM project_groups WHERE id = ?",
		id,
	).Scan(&g.ID, &g.Name, &desc, &g.Color, &g.CreatedAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	g.UpdatedAt = g.CreatedAt
	if updatedAt.Valid {
		g.UpdatedAt = updatedAt.Time
	}
	g.Description = desc.String
	return g, nil
}

func (s *Store) GetProjectGroupByName(name string) (*ProjectGroup, error) {
	var id int64
	if err := s.db.QueryRow("SELECT id FROM project_groups WHERE name = ?", strings.TrimSpace(name)).Scan(&id); err != nil {
		return nil, err
	}
	return s.GetProjectGroup(id)
}

func (s *Store) ListProjectGroups() ([]*ProjectGroup, error) {
	rows, err := s.db.Query("SELECT id, name, description, color, created_at, updated_at FROM project_groups ORDER BY name COLLATE NOCASE, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []*ProjectGroup
	for rows.Next() {
		g := &ProjectGroup{}
		var desc sql.NullString
		var updatedAt sql.NullTime
		if err := rows.Scan(&g.ID, &g.Name, &desc, &g.Color, &g.CreatedAt, &updatedAt); err != nil {
			return nil, err
		}
		g.UpdatedAt = g.CreatedAt
		if updatedAt.Valid {
			g.UpdatedAt = updatedAt.Time
		}
		g.Description = desc.String
		groups = append(groups, g)
	}
	return groups, nil
}

func (s *Store) UpdateProjectGroup(id int64, name, description string) error {
	existing, err := s.GetProjectGroup(id)
	if err != nil {
		return err
	}
	return s.UpdateProjectGroupWithColor(id, name, description, existing.Color)
}

func (s *Store) UpdateProjectGroupWithColor(id int64, name, description, color string) error {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if name == "" {
		return fmt.Errorf("project group name is required")
	}
	if !IsValidProjectGroupColor(color) {
		return ErrInvalidProjectGroupColor
	}
	if existing, err := s.GetProjectGroupByName(name); err == nil && existing.ID != id {
		return ErrProjectGroupNameExists
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	result, err := s.db.Exec(
		"UPDATE project_groups SET name=?, description=?, color=?, updated_at=? WHERE id=?",
		name, description, color, time.Now(), id,
	)
	if err != nil {
		if isProjectGroupNameConstraint(err) {
			return ErrProjectGroupNameExists
		}
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func isProjectGroupNameConstraint(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint failed") && strings.Contains(message, "project_groups.name")
}

func (s *Store) DeleteProjectGroup(id int64) error {
	return s.DeleteProjectGroupWithProjects(id, false)
}

// DeleteProjectGroupWithProjects removes a project group. By default its
// projects are deliberately retained and become ungrouped. Removing projects
// is opt-in and happens in the same transaction as removing the group.
func (s *Store) DeleteProjectGroupWithProjects(id int64, deleteProjects bool) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var existingID int64
	if err := tx.QueryRow("SELECT id FROM project_groups WHERE id = ?", id).Scan(&existingID); err != nil {
		return err
	}
	if deleteProjects {
		rows, err := tx.Query("SELECT id FROM projects WHERE group_id=? ORDER BY id", id)
		if err != nil {
			return err
		}
		var projectIDs []int64
		for rows.Next() {
			var projectID int64
			if err := rows.Scan(&projectID); err != nil {
				rows.Close()
				return err
			}
			projectIDs = append(projectIDs, projectID)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for _, projectID := range projectIDs {
			if err := deleteProjectTx(tx, projectID); err != nil {
				return err
			}
		}
	} else if _, err := tx.Exec("UPDATE projects SET group_id=NULL, updated_at=? WHERE group_id=?", time.Now(), id); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM project_groups WHERE id = ?", id); err != nil {
		return err
	}
	return tx.Commit()
}

// ---------------- Build Queue Items ----------------

func (s *Store) CreateBuildQueueItem(buildID, projectID int64, projectName string, priority int, trigger, branch string) (*BuildQueueItem, error) {
	_, err := s.db.Exec(
		"INSERT OR IGNORE INTO build_queue_items (build_id, project_id, project_name, priority, status, trigger, branch) VALUES (?, ?, ?, ?, 'queued', ?, ?)",
		buildID, projectID, projectName, priority, trigger, branch,
	)
	if err != nil {
		return nil, err
	}
	row := s.db.QueryRow(
		"SELECT id, build_id, project_id, project_name, priority, status, trigger, branch, queued_at, started_at FROM build_queue_items WHERE build_id = ?",
		buildID,
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
	q := `SELECT q.id, q.build_id, q.project_id, q.project_name, q.priority,
		q.status, q.trigger, q.branch, q.queued_at, q.started_at,
		b.number, b.status, b.wait_dependency_on, dependency.number
		FROM build_queue_items q
		JOIN builds b ON b.id = q.build_id
		LEFT JOIN builds dependency ON dependency.id = b.wait_dependency_on`
	var args []interface{}
	if status != "" {
		q += " WHERE q.status = ?"
		args = append(args, status)
	} else {
		q += " WHERE q.status IN ('queued', 'running', 'pending_approval')"
	}
	q += " ORDER BY q.priority DESC, q.queued_at ASC"
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []*BuildQueueItem
	queuePosition := 0
	for rows.Next() {
		item := &BuildQueueItem{}
		var started sql.NullTime
		var buildStatus string
		var dependency sql.NullInt64
		var dependencyNumber sql.NullInt64
		if err := rows.Scan(
			&item.ID, &item.BuildID, &item.ProjectID, &item.ProjectName,
			&item.Priority, &item.Status, &item.Trigger, &item.Branch,
			&item.QueuedAt, &started, &item.BuildNumber, &buildStatus, &dependency, &dependencyNumber,
		); err != nil {
			return nil, err
		}
		if started.Valid {
			item.StartedAt = &started.Time
		}
		switch {
		case item.Status == "running" || buildStatus == "running":
			item.Status = "running"
			item.WaitReason = "running"
		case item.Status == "pending_approval" || buildStatus == "pending_approval":
			item.Status = "pending_approval"
			item.WaitReason = "approval"
		case dependency.Valid:
			value := dependency.Int64
			item.WaitingForBuildID = &value
			if dependencyNumber.Valid {
				number := int(dependencyNumber.Int64)
				item.WaitingForBuildNumber = &number
			}
			item.WaitReason = "dependency"
		default:
			item.WaitReason = "dispatch"
		}
		if item.Status != "running" {
			queuePosition++
			item.QueuePosition = queuePosition
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

func (s *Store) UpdateBuildQueueItemStatusByBuildID(buildID int64, status string) error {
	var startedAt interface{}
	if status == "running" {
		startedAt = time.Now()
	}
	_, err := s.db.Exec(
		"UPDATE build_queue_items SET status=?, started_at=COALESCE(?, started_at) WHERE build_id=?",
		status, startedAt, buildID,
	)
	return err
}

func (s *Store) UpdateBuildQueueItemPriority(id, priority int64) error {
	_, err := s.db.Exec("UPDATE build_queue_items SET priority=? WHERE id=?", priority, id)
	return err
}

package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/neko233-com/buildworld/internal/auth"
	"github.com/neko233-com/buildworld/internal/buildinfo"
	"github.com/neko233-com/buildworld/internal/engine"
	"github.com/neko233-com/buildworld/internal/migration"
	"github.com/neko233-com/buildworld/internal/plugin"
	"github.com/neko233-com/buildworld/internal/portability"
	"github.com/neko233-com/buildworld/internal/store"
	"github.com/neko233-com/buildworld/internal/webhook"
)

// handlers holds all dependencies needed by route handlers.
type handlers struct {
	d           Deps
	portability *portability.Registry
	migrations  *migration.Registry
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func writeCodedErr(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"error": message, "code": code})
}

func parseIDInt64(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

func userIDOf(r *http.Request) int64 { return auth.UserIDFromContext(r.Context()) }

// ---------------------------------------------------------------------------
// health & version
// ---------------------------------------------------------------------------

func (h *handlers) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handlers) version(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"version": buildinfo.Version})
}

// ---------------------------------------------------------------------------
// auth
// ---------------------------------------------------------------------------

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResp struct {
	Token     string      `json:"token"`
	User      *store.User `json:"user"`
	ExpiresIn int64       `json:"expires_in"`
}

func (h *handlers) login(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Username == "" || req.Password == "" {
		writeErr(w, http.StatusBadRequest, "username and password required")
		return
	}

	user, err := h.d.Store.GetUserByUsername(req.Username)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if !auth.CheckPassword(user.PasswordHash, req.Password) {
		writeErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	_ = h.d.Store.UpdateLastLogin(user.ID)

	const sessionDuration = 30 * 24 * time.Hour
	token, err := h.d.JWT.Generate(user.ID, user.Role, user.SessionVersion, sessionDuration)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: "bw_session", Value: token, Path: "/", MaxAge: int(sessionDuration.Seconds()),
		HttpOnly: true, Secure: requestUsesHTTPS(r), SameSite: http.SameSiteLaxMode,
	})
	// Login is intentionally audited directly because this public endpoint has
	// not yet passed through the JWT middleware that h.audit normally reads.
	_ = h.d.Store.CreateAuditLog(user.ID, user.Username, "login", "session", "", "native JWT session issued", clientIP(r))
	user.PasswordHash = ""
	writeJSON(w, http.StatusOK, loginResp{Token: token, User: user, ExpiresIn: int64(sessionDuration.Seconds())})
}

func requestUsesHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	forwarded := strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0]
	return strings.EqualFold(strings.TrimSpace(forwarded), "https")
}

type registerReq struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *handlers) register(w http.ResponseWriter, r *http.Request) {
	var req registerReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Username == "" || req.Password == "" {
		writeErr(w, http.StatusBadRequest, "username and password required")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to hash password")
		return
	}
	user, err := h.d.Store.CreateUser(req.Username, req.Email, hash, "developer")
	if err != nil {
		writeErr(w, http.StatusConflict, "username already exists")
		return
	}
	user.PasswordHash = ""
	writeJSON(w, http.StatusCreated, user)
}

func (h *handlers) me(w http.ResponseWriter, r *http.Request) {
	uid := userIDOf(r)
	user, err := h.d.Store.GetUser(uid)
	if err != nil {
		writeErr(w, http.StatusNotFound, "user not found")
		return
	}
	user.PasswordHash = ""
	writeJSON(w, http.StatusOK, user)
}

// ---------------------------------------------------------------------------
// projects
// ---------------------------------------------------------------------------

type createProjectReq struct {
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	RepoURL            string   `json:"repo_url"`
	RepoType           string   `json:"repo_type"`
	DefaultBranch      string   `json:"default_branch"`
	VCSRootID          *int64   `json:"vcs_root_id"`
	TemplateID         *int64   `json:"template_id"`
	GroupID            *int64   `json:"group_id"`
	Tags               []string `json:"tags"`
	Config             string   `json:"config"`
	PipelineFormat     string   `json:"pipeline_format"`
	PipelineSourceMode string   `json:"pipeline_source_mode"`
	PipelineSCMRepo    string   `json:"pipeline_scm_repo"`
	PipelineSCMBranch  string   `json:"pipeline_scm_branch"`
	PipelineSCMPath    string   `json:"pipeline_scm_path"`
	Enabled            *bool    `json:"enabled"`
}

func (h *handlers) listProjects(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("quick_access") == "1" {
		projects, err := h.d.Store.ListQuickAccessProjects()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.Header().Set("X-Buildworld-View-Version", "1")
		writeJSON(w, http.StatusOK, projects)
		return
	}
	projects, err := h.d.Store.ListProjectSummaries()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("X-Buildworld-View-Version", "1")
	writeJSON(w, http.StatusOK, projects)
}

type setProjectFlagsReq struct {
	Favorite    bool `json:"favorite"`
	QuickAccess bool `json:"quick_access"`
}

// setProjectFlags toggles the favorite / quick-access state of a project without
// requiring a full pipeline-config resubmission.
func (h *handlers) setProjectFlags(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req setProjectFlagsReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.d.Store.SetProjectFlags(id, req.Favorite, req.QuickAccess); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	p, _ := h.d.Store.GetProject(id)
	writeJSON(w, http.StatusOK, p)
}

func (h *handlers) createProject(w http.ResponseWriter, r *http.Request) {
	var req createProjectReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	repoType, err := store.NormalizeRepositoryType(req.RepoType)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	req.RepoType = repoType
	if err := validateProjectPipelineSource(req.PipelineSourceMode, req.PipelineFormat, req.Config); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if req.GroupID != nil {
		if _, err := h.d.Store.GetProjectGroup(*req.GroupID); err != nil {
			writeErr(w, http.StatusBadRequest, "project group not found")
			return
		}
	}
	if err := h.validateProjectBuildTemplate(req.TemplateID); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	p, err := h.d.Store.CreateProject(req.Name, req.Description, req.RepoURL, req.RepoType,
		req.DefaultBranch, req.Config, userIDOf(r), req.VCSRootID, req.TemplateID, req.Tags)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.d.Store.SetProjectPipelineSource(p.ID, req.PipelineFormat, req.PipelineSourceMode,
		req.PipelineSCMRepo, req.PipelineSCMBranch, req.PipelineSCMPath); err != nil {
		writeErr(w, http.StatusInternalServerError, "persist pipeline source: "+err.Error())
		return
	}
	if err := h.d.Store.SetProjectGroup(p.ID, req.GroupID); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Enabled != nil {
		if err := h.d.Store.SetProjectEnabled(p.ID, *req.Enabled); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	p, _ = h.d.Store.GetProject(p.ID)
	writeJSON(w, http.StatusCreated, p)
}

func (h *handlers) getProject(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	p, err := h.d.Store.GetProject(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *handlers) updateProject(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req createProjectReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	repoType, err := store.NormalizeRepositoryType(req.RepoType)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	req.RepoType = repoType
	if err := validateProjectPipelineSource(req.PipelineSourceMode, req.PipelineFormat, req.Config); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if req.GroupID != nil {
		if _, err := h.d.Store.GetProjectGroup(*req.GroupID); err != nil {
			writeErr(w, http.StatusBadRequest, "project group not found")
			return
		}
	}
	if err := h.validateProjectBuildTemplate(req.TemplateID); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.d.Store.UpdateProject(id, req.Name, req.Description, req.RepoURL, req.RepoType,
		req.DefaultBranch, req.Config, req.VCSRootID, req.TemplateID, req.Tags); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.d.Store.SetProjectPipelineSource(id, req.PipelineFormat, req.PipelineSourceMode,
		req.PipelineSCMRepo, req.PipelineSCMBranch, req.PipelineSCMPath); err != nil {
		writeErr(w, http.StatusInternalServerError, "persist pipeline source: "+err.Error())
		return
	}
	if err := h.d.Store.SetProjectGroup(id, req.GroupID); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Enabled != nil {
		if err := h.d.Store.SetProjectEnabled(id, *req.Enabled); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	p, err := h.d.Store.GetProject(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

type validatePipelineReq struct {
	Source string `json:"source"`
}

func writePipelineValidation(w http.ResponseWriter, source string) {
	config, err := engine.ParsePipelineConfig(source)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeBuildConfigValidation(w, config, string(engine.DetectPipelineFormat(source)))
}

func writeBuildConfigValidation(w http.ResponseWriter, config *engine.BuildConfig, format string) {
	stepCount := 0
	for _, stage := range config.Stages {
		stepCount += len(stage.Steps)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"valid":              true,
		"format":             format,
		"stages":             len(config.Stages),
		"steps":              stepCount,
		"parameters":         config.Parameters,
		"allow_long_running": config.AllowLongRunning,
	})
}

// validatePipelineSource parses an unsaved editor draft without persisting it
// or queuing a build. Monaco uses the response for authoritative DSL feedback.
func (h *handlers) validatePipelineSource(w http.ResponseWriter, r *http.Request) {
	var req validatePipelineReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	writePipelineValidation(w, req.Source)
}

// validateProjectConfig parses the persisted source without queuing a build.
// Migration tooling uses it to prove that an imported definition is runnable
// while avoiding side effects on an existing service.
func (h *handlers) validateProjectConfig(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	project, err := h.d.Store.GetProject(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "project not found")
		return
	}
	config, err := h.loadProjectBuildConfig(r.Context(), project)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeBuildConfigValidation(w, config, string(engine.NormalizeFormat(project.PipelineFormat)))
}

func (h *handlers) deleteProject(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.d.Store.DeleteProject(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type reorderProjectsReq struct {
	OrderedIDs []int64 `json:"ordered_ids"`
}

func (h *handlers) reorderProjects(w http.ResponseWriter, r *http.Request) {
	var req reorderProjectsReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.d.Store.ReorderProjects(req.OrderedIDs); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ---------------------------------------------------------------------------
// builds
// ---------------------------------------------------------------------------

type triggerBuildReq struct {
	Branch     string                 `json:"branch"`
	Parameters map[string]interface{} `json:"parameters"`
}

func decodeTriggerBuildRequest(r *http.Request) (triggerBuildReq, error) {
	var req triggerBuildReq
	if r.Body == nil {
		return req, nil
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		return req, fmt.Errorf("invalid request body")
	}
	return req, nil
}

// validateProjectPipelineSource validates the pipeline definition supplied at
// project create/update time. Inline definitions must be non-empty and parse
// for the declared format; SCM-sourced definitions only need a repository and
// file path because the actual content is fetched at build time.
func validateProjectPipelineSource(sourceMode, format, config string) error {
	if strings.EqualFold(strings.TrimSpace(sourceMode), "scm") {
		return nil
	}
	if strings.TrimSpace(config) == "" {
		return fmt.Errorf("pipeline config is required")
	}
	if _, err := engine.ParsePipelineConfigWithFormat(engine.NormalizeFormat(format), config, ""); err != nil {
		return fmt.Errorf("invalid pipeline config: %w", err)
	}
	return nil
}

func (h *handlers) validateProjectBuildTemplate(templateID *int64) error {
	if templateID == nil {
		return nil
	}
	if *templateID <= 0 {
		return fmt.Errorf("build template not found")
	}
	if _, err := h.d.Store.GetBuildTemplate(*templateID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("build template not found")
		}
		return fmt.Errorf("load build template: %w", err)
	}
	return nil
}

func (h *handlers) loadProjectBuildConfig(ctx context.Context, project *store.Project) (*engine.BuildConfig, error) {
	loadCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	config, err := engine.ResolveProjectBuildConfig(loadCtx, project)
	if err != nil {
		return nil, fmt.Errorf("invalid project build configuration: %w", err)
	}
	if project.TemplateID == nil {
		return config, nil
	}

	template, err := h.d.Store.GetBuildTemplate(*project.TemplateID)
	if err != nil {
		return nil, fmt.Errorf("project build template is unavailable: %w", err)
	}
	templateConfig, err := engine.ParsePipelineConfig(template.Config)
	if err != nil {
		return nil, fmt.Errorf("invalid build template configuration: %w", err)
	}
	return engine.MergeBuildConfig(templateConfig, config), nil
}

// loadProjectBuildConfigForPresentation never performs network I/O. Build
// list/detail serialization must remain available when an SCM host or local
// credential helper is unavailable. Without the fetched parameter schema,
// every SCM parameter is conservatively redacted during presentation.
func (h *handlers) loadProjectBuildConfigForPresentation(project *store.Project) *engine.BuildConfig {
	if !strings.EqualFold(strings.TrimSpace(project.PipelineSourceMode), "scm") {
		config, _ := h.loadProjectBuildConfig(context.Background(), project)
		return config
	}
	if project.TemplateID == nil {
		return nil
	}
	template, err := h.d.Store.GetBuildTemplate(*project.TemplateID)
	if err != nil {
		return nil
	}
	config, _ := engine.ParsePipelineConfig(template.Config)
	return config
}

func (h *handlers) resolveProjectBuildParameters(project *store.Project, supplied map[string]interface{}) (string, error) {
	config, err := h.loadProjectBuildConfig(context.Background(), project)
	if err != nil {
		return "", err
	}
	resolved, err := engine.ResolveBuildParameters(config.Parameters, supplied)
	if err != nil {
		return "", err
	}
	if len(resolved) == 0 {
		return "", nil
	}
	encoded, err := json.Marshal(resolved)
	if err != nil {
		return "", fmt.Errorf("encode build parameters: %w", err)
	}
	return string(encoded), nil
}

func (h *handlers) requestBuildApproval(project *store.Project, build *store.Build, requesterID int64) error {
	config, err := h.loadProjectBuildConfig(context.Background(), project)
	if err != nil {
		return err
	}
	required, err := engine.ApprovalRequired(config)
	if err != nil || !required {
		return err
	}
	if h.d.Approval == nil {
		return fmt.Errorf("approval service is unavailable")
	}
	if _, err := h.d.Approval.RequestIfRequired(build.ID, requesterID, config); err != nil {
		_ = h.d.Store.FinishBuild(build.ID, "failed", 0)
		return err
	}
	return nil
}

func secretBuildParameterNames(config *engine.BuildConfig) map[string]struct{} {
	names := make(map[string]struct{})
	if config == nil {
		return names
	}
	for _, parameter := range config.Parameters {
		if parameter.IsSecret || strings.EqualFold(strings.TrimSpace(parameter.Type), "password") {
			names[parameter.Name] = struct{}{}
		}
	}
	return names
}

func looksLikeSecretParameter(name string) bool {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if normalized == "key" {
		return true
	}
	for _, marker := range []string{"password", "passwd", "secret", "token", "private_key", "api_key", "access_key", "_key", "-key"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func redactBuildParameters(build *store.Build, secretNames map[string]struct{}, redactAll bool) *store.Build {
	if build == nil {
		return nil
	}
	redacted := *build
	if strings.TrimSpace(build.Parameters) == "" {
		return &redacted
	}

	var parameters map[string]interface{}
	if err := json.Unmarshal([]byte(build.Parameters), &parameters); err != nil {
		redacted.Parameters = ""
		return &redacted
	}
	for name := range parameters {
		if _, secret := secretNames[name]; redactAll || secret || looksLikeSecretParameter(name) {
			parameters[name] = "********"
		}
	}
	encoded, err := json.Marshal(parameters)
	if err != nil {
		redacted.Parameters = ""
		return &redacted
	}
	redacted.Parameters = string(encoded)
	return &redacted
}

func (h *handlers) publicBuild(build *store.Build) *store.Build {
	if build == nil {
		return nil
	}
	project, err := h.d.Store.GetProject(build.ProjectID)
	if err != nil {
		return redactBuildParameters(build, nil, false)
	}
	config := h.loadProjectBuildConfigForPresentation(project)
	redactAll := strings.EqualFold(strings.TrimSpace(project.PipelineSourceMode), "scm")
	result := redactBuildParameters(build, secretBuildParameterNames(config), redactAll)
	if approval, approvalErr := h.d.Store.GetBuildApprovalByBuild(build.ID); approvalErr == nil {
		if policy, policyErr := engine.ResolveApprovalPolicy(config); policyErr == nil {
			approval.Prompt = policy.Prompt
			approval.RequiredRoles = append([]string(nil), policy.RequiredRoles...)
			approval.AllowRequester = policy.RequesterCanResolve()
		}
		result.Approval = approval
	}
	return result
}

func (h *handlers) publicBuilds(builds []*store.Build) []*store.Build {
	result := make([]*store.Build, 0, len(builds))
	secretNamesByProject := make(map[int64]map[string]struct{})
	redactAllByProject := make(map[int64]bool)
	for _, build := range builds {
		names, cached := secretNamesByProject[build.ProjectID]
		if !cached {
			project, projectErr := h.d.Store.GetProject(build.ProjectID)
			if projectErr == nil {
				config := h.loadProjectBuildConfigForPresentation(project)
				names = secretBuildParameterNames(config)
				redactAllByProject[build.ProjectID] = strings.EqualFold(strings.TrimSpace(project.PipelineSourceMode), "scm")
			}
			secretNamesByProject[build.ProjectID] = names
		}
		result = append(result, redactBuildParameters(build, names, redactAllByProject[build.ProjectID]))
	}
	return result
}

func (h *handlers) listBuilds(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	builds, err := h.d.Store.ListBuilds(limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, h.publicBuilds(builds))
}

func (h *handlers) searchBuilds(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	search := store.BuildSearch{
		Query:   query.Get("q"),
		Trigger: query.Get("trigger"),
		Branch:  query.Get("branch"),
		Limit:   25,
	}
	if raw := query.Get("project_id"); raw != "" {
		projectID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || projectID <= 0 {
			writeErr(w, http.StatusBadRequest, "invalid project_id")
			return
		}
		search.ProjectID = &projectID
	}
	if raw := query.Get("status"); raw != "" {
		allowed := map[string]bool{
			"pending": true, "pending_approval": true, "running": true,
			"success": true, "failed": true, "cancelled": true, "rejected": true,
		}
		for _, status := range strings.Split(raw, ",") {
			status = strings.TrimSpace(status)
			if !allowed[status] {
				writeErr(w, http.StatusBadRequest, "invalid status")
				return
			}
			search.Statuses = append(search.Statuses, status)
		}
	}
	if raw := query.Get("pinned"); raw != "" {
		pinned, err := strconv.ParseBool(raw)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid pinned")
			return
		}
		search.Pinned = &pinned
	}
	if raw := query.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit <= 0 {
			writeErr(w, http.StatusBadRequest, "invalid limit")
			return
		}
		search.Limit = min(limit, 100)
	}
	if raw := query.Get("offset"); raw != "" {
		offset, err := strconv.Atoi(raw)
		if err != nil || offset < 0 {
			writeErr(w, http.StatusBadRequest, "invalid offset")
			return
		}
		search.Offset = offset
	}
	result, err := h.d.Store.SearchBuilds(search)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	result.Items = h.publicBuilds(result.Items)
	writeJSON(w, http.StatusOK, result)
}

func (h *handlers) listProjectBuilds(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	builds, err := h.d.Store.ListBuildsByProject(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, h.publicBuilds(builds))
}

func (h *handlers) triggerBuild(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	project, err := h.d.Store.GetProject(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "project not found")
		return
	}
	if !project.Enabled {
		writeCodedErr(w, http.StatusConflict, "project_disabled", "project is disabled")
		return
	}

	req, err := decodeTriggerBuildRequest(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	branch := strings.TrimSpace(req.Branch)
	if branch == "" {
		branch = project.DefaultBranch
	}
	paramsJSON, err := h.resolveProjectBuildParameters(project, req.Parameters)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	num, err := h.d.Store.NextBuildNumber(project.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	build, err := h.d.Store.CreateBuild(project.ID, num, "manual", branch, "", paramsJSON, nil, nil)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.requestBuildApproval(project, build, userIDOf(r)); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	if h.d.Runner != nil {
		if err := h.d.Runner.Enqueue(build.ID); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	writeJSON(w, http.StatusCreated, h.publicBuild(build))
}

func (h *handlers) getBuild(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	build, err := h.d.Store.GetBuild(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "build not found")
		return
	}
	writeJSON(w, http.StatusOK, h.publicBuild(build))
}

func (h *handlers) getBuildLogs(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	build, err := h.d.Store.GetBuild(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "build not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"log":                  build.Log,
		"truncated":            store.IsBuildLogTruncated(build.Log),
		"retention_characters": store.BuildLogRetentionCharacters,
	})
}

func (h *handlers) getBuildTimeline(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	build, err := h.d.Store.GetBuild(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "build not found")
		return
	}
	writeJSON(w, http.StatusOK, engine.ParseBuildTimeline(build.Status, build.Log))
}

func (h *handlers) getBuildProblems(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	build, err := h.d.Store.GetBuild(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "build not found")
		return
	}
	writeJSON(w, http.StatusOK, engine.AnalyzeBuildProblems(build.Status, build.Log))
}

func (h *handlers) getBuildChain(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	chain, err := engine.NewBuildChainService(h.d.Store, nil).Resolve(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "build not found")
		return
	}
	writeJSON(w, http.StatusOK, chain)
}

func (h *handlers) stopBuild(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.d.Store.CancelBuild(id); err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			writeErr(w, http.StatusNotFound, "build not found")
		case errors.Is(err, store.ErrBuildNotCancellable):
			writeErr(w, http.StatusConflict, "build is already finished")
		default:
			writeErr(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	if err := h.d.Store.UpdateBuildQueueItemStatusByBuildID(id, "cancelled"); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	resolverID := userIDOf(r)
	resolverName := ""
	if resolverID > 0 {
		if user, userErr := h.d.Store.GetUser(resolverID); userErr == nil {
			resolverName = user.Username
		}
	}
	if err := h.d.Store.CancelPendingBuildApproval(id, resolverID, resolverName); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.d.Runner != nil {
		h.d.Runner.Stop(id)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

// ---------------------------------------------------------------------------
// agents / workers
// ---------------------------------------------------------------------------

type registerAgentReq struct {
	Name      string   `json:"name"`
	Address   string   `json:"address"`
	Labels    []string `json:"labels"`
	MaxBuilds int      `json:"max_builds"`
	Pool      string   `json:"pool"`
}

func (h *handlers) listAgents(w http.ResponseWriter, _ *http.Request) {
	// Mark stale workers as offline before listing.
	_ = h.d.Store.MarkOfflineWorkers()
	workers, err := h.d.Store.ListWorkers()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, workers)
}

func (h *handlers) registerAgent(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/auto-register") && (h.d.Cfg.Workers.EnrollmentToken == "" || r.Header.Get("X-Buildworld-Enroll-Token") != h.d.Cfg.Workers.EnrollmentToken) {
		writeErr(w, http.StatusUnauthorized, "invalid worker enrollment token")
		return
	}
	var req registerAgentReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.MaxBuilds == 0 {
		req.MaxBuilds = 4
	}
	agentID := uuid.New().String()
	token := uuid.New().String()
	tokenHash, _ := auth.HashPassword(token)

	w_, err := h.d.Store.CreateWorker(agentID, req.Name, req.Address, tokenHash, req.Labels, req.MaxBuilds, req.Pool)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Return the plaintext token once — only stored as hash.
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"agent": w_,
		"token": token,
	})
}

func (h *handlers) workerHeartbeat(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	worker, err := h.d.Store.GetWorker(id)
	if err != nil || !auth.CheckPassword(worker.TokenHash, r.Header.Get("X-Buildworld-Worker-Token")) {
		writeErr(w, http.StatusUnauthorized, "invalid worker token")
		return
	}
	if err := h.d.Store.UpdateWorkerHeartbeat(id, 0); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handlers) deleteAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.d.Store.DeleteWorker(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handlers) generateAgentToken(w http.ResponseWriter, _ *http.Request) {
	if h.d.Store == nil || h.d.Cfg == nil {
		writeErr(w, http.StatusServiceUnavailable, "agent enrollment is unavailable")
		return
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		writeErr(w, http.StatusInternalServerError, "unable to generate secure enrollment token")
		return
	}
	token := "bw_enroll_" + hex.EncodeToString(random)
	if err := h.d.Store.SetEnvVar("system", nil, agentEnrollmentTokenSetting, token, true, "Remote agent enrollment and dispatch credential"); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.d.Cfg.Workers.EnrollmentToken = token
	if h.d.Runner != nil {
		h.d.Runner.SetWorkerDispatchToken(token)
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

// ---------------------------------------------------------------------------
// plugins
// ---------------------------------------------------------------------------

type pluginWithStatus struct {
	*store.Plugin
	Loaded       bool                 `json:"loaded"`
	Steps        []string             `json:"steps"`
	Hooks        []string             `json:"hooks"`
	UIExtensions []plugin.UIExtension `json:"ui_extensions"`
}

type installGitHubPluginReq struct {
	URL string `json:"url"`
}

func (h *handlers) installGitHubPlugin(w http.ResponseWriter, r *http.Request) {
	if h.d.Loader == nil {
		writeErr(w, http.StatusServiceUnavailable, "plugin loader unavailable")
		return
	}
	var req installGitHubPluginReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
		writeErr(w, http.StatusBadRequest, "GitHub URL is required")
		return
	}
	manifest, err := h.d.Loader.InstallGitHub(r.Context(), req.URL)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	enabled := true
	loaded := h.d.Loader.GetInstalled(manifest.Name)
	if loaded == nil {
		writeErr(w, http.StatusInternalServerError, "installed plugin is not loaded")
		return
	}
	stored, storeErr := h.d.Store.GetPluginByName(manifest.Name)
	switch {
	case storeErr == nil:
		enabled = stored.Enabled
		storeErr = h.d.Store.UpdatePluginMetadata(stored.ID, manifest.Version, manifest.Description, manifest.Author, loaded.Path, manifest.Source)
	case errors.Is(storeErr, sql.ErrNoRows):
		stored, storeErr = h.d.Store.CreatePlugin(manifest.Name, manifest.Version, manifest.Description, manifest.Author, "", loaded.Path, manifest.Source)
	default:
		// Preserve the original store error below.
	}
	if storeErr != nil {
		writeErr(w, http.StatusInternalServerError, "persist plugin metadata: "+storeErr.Error())
		return
	}
	h.d.Loader.SetEnabled(manifest.Name, enabled)
	h.audit(r, "install", "plugin", manifest.Name, "GitHub binary plugin: "+req.URL)
	writeJSON(w, http.StatusCreated, manifest)
}

func (h *handlers) listPlugins(w http.ResponseWriter, _ *http.Request) {
	loadedPlugins := make(map[string]*plugin.Plugin)
	if h.d.Loader != nil {
		for _, loaded := range h.d.Loader.ListAll() {
			loadedPlugins[loaded.Name] = loaded
		}
	}

	dbPlugins, err := h.d.Store.ListPlugins()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	result := make([]pluginWithStatus, 0, len(dbPlugins))
	for _, stored := range dbPlugins {
		status := plugin.PluginStatus{Steps: []string{}, Hooks: []string{}, UIExtensions: []plugin.UIExtension{}}
		steps := []string{}
		hooks := []string{}
		extensions := []plugin.UIExtension{}
		if loaded := loadedPlugins[stored.Name]; loaded != nil {
			status = loaded.GetStatus()
			steps = status.Steps
			hooks = status.Hooks
			extensions = status.UIExtensions
		}
		result = append(result, pluginWithStatus{Plugin: stored, Loaded: status.Loaded, Steps: steps, Hooks: hooks, UIExtensions: extensions})
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *handlers) resolvePlugin(idOrName string) (*store.Plugin, error) {
	if id, err := strconv.ParseInt(idOrName, 10, 64); err == nil {
		return h.d.Store.GetPlugin(id)
	}
	return h.d.Store.GetPluginByName(idOrName)
}

func (h *handlers) deletePlugin(w http.ResponseWriter, r *http.Request) {
	stored, err := h.resolvePlugin(chi.URLParam(r, "name"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "plugin not found")
		return
	}
	if h.d.Loader == nil {
		writeErr(w, http.StatusServiceUnavailable, "plugin loader unavailable")
		return
	}
	if err := h.d.Loader.DeletePlugin(stored.Name); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.d.Store.DeletePlugin(stored.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(r, "delete", "plugin", stored.Name, "removed Go binary plugin")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handlers) togglePlugin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	stored, err := h.resolvePlugin(chi.URLParam(r, "name"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "plugin not found")
		return
	}
	if h.d.Loader == nil {
		writeErr(w, http.StatusServiceUnavailable, "plugin loader unavailable")
		return
	}
	if req.Enabled && h.d.Loader.GetInstalled(stored.Name) == nil {
		if err := h.d.Loader.Load(stored.Name); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	h.d.Loader.SetEnabled(stored.Name, req.Enabled)
	if err := h.d.Store.UpdatePluginEnabled(stored.ID, req.Enabled); err != nil {
		h.d.Loader.SetEnabled(stored.Name, stored.Enabled)
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(r, "update", "plugin", stored.Name, fmt.Sprintf("enabled=%t", req.Enabled))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handlers) reloadPlugin(w http.ResponseWriter, r *http.Request) {
	stored, err := h.resolvePlugin(chi.URLParam(r, "name"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "plugin not found")
		return
	}
	if h.d.Loader == nil {
		writeErr(w, http.StatusServiceUnavailable, "plugin loader unavailable")
		return
	}
	if err := h.d.Loader.Reload(stored.Name); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	h.d.Loader.SetEnabled(stored.Name, stored.Enabled)
	loaded := h.d.Loader.GetInstalled(stored.Name)
	if loaded == nil {
		writeErr(w, http.StatusInternalServerError, "reloaded plugin is unavailable")
		return
	}
	if err := h.d.Store.UpdatePluginMetadata(stored.ID, loaded.Version, loaded.Description, loaded.Author, loaded.Path, loaded.InstallSource()); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(r, "reload", "plugin", stored.Name, "reloaded Go binary plugin")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ---------------------------------------------------------------------------
// users
// ---------------------------------------------------------------------------

type createUserReq struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

func (h *handlers) listUsers(w http.ResponseWriter, _ *http.Request) {
	users, err := h.d.Store.ListUsers()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, users)
}

func (h *handlers) createUser(w http.ResponseWriter, r *http.Request) {
	var req createUserReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Username == "" || req.Password == "" {
		writeErr(w, http.StatusBadRequest, "username and password required")
		return
	}
	if req.Role == "" {
		req.Role = "developer"
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to hash password")
		return
	}
	user, err := h.d.Store.CreateUser(req.Username, req.Email, hash, req.Role)
	if err != nil {
		writeErr(w, http.StatusConflict, "username already exists")
		return
	}
	user.PasswordHash = ""
	writeJSON(w, http.StatusCreated, user)
}

func (h *handlers) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if id == userIDOf(r) {
		writeErr(w, http.StatusBadRequest, "cannot delete yourself")
		return
	}
	if err := h.d.Store.DeleteUser(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handlers) updateUserRole(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Role == "" {
		writeErr(w, http.StatusBadRequest, "role is required")
		return
	}
	if err := h.d.Store.UpdateUserRole(id, req.Role); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handlers) updateUserPassword(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Password == "" {
		writeErr(w, http.StatusBadRequest, "password is required")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to hash password")
		return
	}
	if err := h.d.Store.UpdateUserPassword(id, hash); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ---------------------------------------------------------------------------
// env vars
// ---------------------------------------------------------------------------

type setEnvVarReq struct {
	Scope       string `json:"scope"` // global | project
	ProjectID   *int64 `json:"project_id"`
	Name        string `json:"name"`
	Value       string `json:"value"`
	IsSecret    bool   `json:"is_secret"`
	Description string `json:"description"`
}

func (h *handlers) listEnvVars(w http.ResponseWriter, r *http.Request) {
	scope := r.URL.Query().Get("scope")
	if scope == "" {
		scope = "global"
	}
	var pid *int64
	if s := r.URL.Query().Get("project_id"); s != "" {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			pid = &v
		}
	}
	vars, err := h.d.Store.ListEnvVars(scope, pid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Mask secret values.
	for _, v := range vars {
		if v.IsSecret {
			v.Value = "********"
		}
	}
	writeJSON(w, http.StatusOK, vars)
}

func (h *handlers) setEnvVar(w http.ResponseWriter, r *http.Request) {
	var req setEnvVarReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Scope == "" {
		req.Scope = "global"
	}
	if err := h.d.Store.SetEnvVar(req.Scope, req.ProjectID, req.Name, req.Value, req.IsSecret, req.Description); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handlers) deleteEnvVar(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.d.Store.DeleteEnvVar(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ---------------------------------------------------------------------------
// webhooks
// ---------------------------------------------------------------------------

// triggerBuildByWebhook creates a build for a project matching the repo URL.
func (h *handlers) triggerBuildByWebhook(payload webhook.WebhookPayload) error {
	projects, err := h.d.Store.ListProjects()
	if err != nil {
		return err
	}
	for _, p := range projects {
		if !p.Enabled {
			continue
		}
		if p.RepoURL == "" {
			continue
		}
		if p.RepoURL == payload.Repository.CloneURL || p.RepoURL == payload.Repository.FullName {
			branch, commit, _ := webhook.ParsePushEvent(payload)
			num, _ := h.d.Store.NextBuildNumber(p.ID)
			build, err := h.d.Store.CreateBuild(p.ID, num, "webhook", branch, commit, "", nil, nil)
			if err != nil {
				return err
			}
			if err := h.requestBuildApproval(p, build, 0); err != nil {
				return err
			}
			if h.d.Runner != nil {
				if err := h.d.Runner.Enqueue(build.ID); err != nil {
					return err
				}
			}
			return nil
		}
	}
	return nil
}

func (h *handlers) githubWebhook(w http.ResponseWriter, r *http.Request) {
	wh := webhook.NewWebhookHandler(h.webhookSecret())
	wh.OnPush(h.triggerBuildByWebhook)
	wh.HandleGitHubWebhook(w, r)
}

func (h *handlers) gitlabWebhook(w http.ResponseWriter, r *http.Request) {
	wh := webhook.NewWebhookHandler(h.webhookSecret())
	wh.OnPush(h.triggerBuildByWebhook)
	wh.HandleGitLabWebhook(w, r)
}

func (h *handlers) giteaWebhook(w http.ResponseWriter, r *http.Request) {
	wh := webhook.NewWebhookHandler(h.webhookSecret())
	wh.OnPush(h.triggerBuildByWebhook)
	wh.HandleGiteaWebhook(w, r)
}

func (h *handlers) webhookSecret() string {
	if h.d.Cfg == nil {
		return ""
	}
	return h.d.Cfg.Automation.GitHubWebhookSecret
}

// ---------------------------------------------------------------------------
// credentials
// ---------------------------------------------------------------------------

type createCredentialReq struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Host        string `json:"host"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	PrivateKey  string `json:"private_key"`
	PublicKey   string `json:"public_key"`
	Token       string `json:"token"`
	Description string `json:"description"`
	IsSecret    bool   `json:"is_secret"`
}

func maskCredentialSecrets(c *store.Credential) {
	if c.Password != "" {
		c.Password = "********"
	}
	if c.PrivateKey != "" {
		c.PrivateKey = "********"
	}
	if c.Token != "" {
		c.Token = "********"
	}
}

func (h *handlers) listCredentials(w http.ResponseWriter, r *http.Request) {
	var credType *store.CredentialType
	if t := r.URL.Query().Get("type"); t != "" {
		ct, err := store.NormalizeCredentialType(t)
		if err != nil {
			writeErr(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		credType = &ct
	}
	creds, err := h.d.Store.ListCredentials(credType)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	reveal := r.URL.Query().Get("reveal") == "true"
	if !reveal {
		for _, c := range creds {
			maskCredentialSecrets(c)
		}
	}
	writeJSON(w, http.StatusOK, creds)
}

func (h *handlers) createCredential(w http.ResponseWriter, r *http.Request) {
	var req createCredentialReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	ct, err := store.NormalizeCredentialType(req.Type)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	c, err := h.d.Store.CreateCredential(req.Name, ct, req.Host, req.Username, req.Password, req.PrivateKey, req.PublicKey, req.Token, req.Description, req.IsSecret)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	maskCredentialSecrets(c)
	writeJSON(w, http.StatusCreated, c)
}

func (h *handlers) getCredential(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	c, err := h.d.Store.GetCredential(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "credential not found")
		return
	}
	reveal := r.URL.Query().Get("reveal") == "true"
	if !reveal {
		maskCredentialSecrets(c)
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *handlers) updateCredential(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req createCredentialReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	ct, err := store.NormalizeCredentialType(req.Type)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	existing, err := h.d.Store.GetCredential(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "credential not found")
		return
	}
	if req.Password == "********" {
		req.Password = existing.Password
	}
	if req.PrivateKey == "********" {
		req.PrivateKey = existing.PrivateKey
	}
	if req.Token == "********" {
		req.Token = existing.Token
	}
	if err := h.d.Store.UpdateCredential(id, req.Name, ct, req.Host, req.Username, req.Password, req.PrivateKey, req.PublicKey, req.Token, req.Description, req.IsSecret); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	c, err := h.d.Store.GetCredential(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	maskCredentialSecrets(c)
	writeJSON(w, http.StatusOK, c)
}

func (h *handlers) deleteCredential(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.d.Store.DeleteCredential(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handlers) lookupCredential(w http.ResponseWriter, r *http.Request) {
	host := r.URL.Query().Get("host")
	credType := r.URL.Query().Get("type")
	if host == "" {
		writeErr(w, http.StatusBadRequest, "host is required")
		return
	}
	if credType == "" {
		credType = "git"
	}
	ct, err := store.NormalizeCredentialType(credType)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	c, err := h.d.Store.FindCredential(host, ct)
	if err != nil {
		writeErr(w, http.StatusNotFound, "credential not found")
		return
	}
	reveal := r.URL.Query().Get("reveal") == "true"
	if !reveal {
		maskCredentialSecrets(c)
	}
	writeJSON(w, http.StatusOK, c)
}

// ---------------------------------------------------------------------------
// VCS repository templates (Git only)
// ---------------------------------------------------------------------------

type createVCSRootReq struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	URL          string `json:"url"`
	Branch       string `json:"branch"`
	CredentialID *int64 `json:"credential_id"`
	PollInterval *int   `json:"poll_interval"`
	AutoCheckout bool   `json:"auto_checkout"`
	Config       string `json:"config"`
}

func (h *handlers) listVCSRoots(w http.ResponseWriter, _ *http.Request) {
	roots, err := h.d.Store.ListVCSRoots()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, roots)
}

func (h *handlers) createVCSRoot(w http.ResponseWriter, r *http.Request) {
	var req createVCSRootReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	vcsType, err := store.NormalizeRepositoryType(req.Type)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	req.Type = vcsType
	pollInterval := 60
	if req.PollInterval != nil {
		pollInterval = max(0, *req.PollInterval)
	}
	v, err := h.d.Store.CreateVCSRoot(req.Name, req.Type, req.URL, req.Branch, req.CredentialID, pollInterval, req.AutoCheckout, req.Config)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (h *handlers) getVCSRoot(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	v, err := h.d.Store.GetVCSRoot(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "vcs root not found")
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *handlers) updateVCSRoot(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req createVCSRootReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	vcsType, err := store.NormalizeRepositoryType(req.Type)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	req.Type = vcsType
	pollInterval := 60
	if existing, getErr := h.d.Store.GetVCSRoot(id); getErr == nil {
		pollInterval = existing.PollInterval
	}
	if req.PollInterval != nil {
		pollInterval = max(0, *req.PollInterval)
	}
	if err := h.d.Store.UpdateVCSRoot(id, req.Name, req.Type, req.URL, req.Branch, req.CredentialID, pollInterval, req.AutoCheckout, req.Config); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	v, err := h.d.Store.GetVCSRoot(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (h *handlers) deleteVCSRoot(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.d.Store.DeleteVCSRoot(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ---------------------------------------------------------------------------
// Build Templates
// ---------------------------------------------------------------------------

type createTemplateReq struct {
	Name        string `json:"name"`
	Config      string `json:"config"`
	Description string `json:"description"`
}

func (h *handlers) listTemplates(w http.ResponseWriter, _ *http.Request) {
	templates, err := h.d.Store.ListBuildTemplates()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, templates)
}

func (h *handlers) createTemplate(w http.ResponseWriter, r *http.Request) {
	var req createTemplateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	if _, err := engine.ParsePipelineConfig(req.Config); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "invalid pipeline configuration: "+err.Error())
		return
	}
	t, err := h.d.Store.CreateBuildTemplate(req.Name, req.Config, req.Description)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (h *handlers) getTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	t, err := h.d.Store.GetBuildTemplate(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "template not found")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (h *handlers) updateTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req createTemplateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if _, err := engine.ParsePipelineConfig(req.Config); err != nil {
		writeErr(w, http.StatusUnprocessableEntity, "invalid pipeline configuration: "+err.Error())
		return
	}
	if err := h.d.Store.UpdateBuildTemplate(id, req.Name, req.Config, req.Description); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	t, err := h.d.Store.GetBuildTemplate(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (h *handlers) deleteTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.d.Store.DeleteBuildTemplate(id); err != nil {
		if errors.Is(err, store.ErrBuildTemplateInUse) {
			writeCodedErr(w, http.StatusConflict, "build_template_in_use", err.Error())
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ---------------------------------------------------------------------------
// Build control: retry / pin
// ---------------------------------------------------------------------------

func (h *handlers) retryBuild(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	newBuild, err := h.d.Store.RetryBuild(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	project, err := h.d.Store.GetProject(newBuild.ProjectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.requestBuildApproval(project, newBuild, userIDOf(r)); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.d.Runner != nil {
		if err := h.d.Runner.Enqueue(newBuild.ID); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	writeJSON(w, http.StatusCreated, h.publicBuild(newBuild))
}

func (h *handlers) pinBuild(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		Pinned bool `json:"pinned"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.d.Store.PinBuild(id, req.Pinned); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ---------------------------------------------------------------------------
// Artifacts
// ---------------------------------------------------------------------------

func (h *handlers) listBuildArtifacts(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	arts, err := h.d.Store.ListArtifactsByBuild(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, arts)
}

func (h *handlers) downloadArtifact(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if h.d.Artifacts == nil {
		writeErr(w, http.StatusServiceUnavailable, "artifact manager not available")
		return
	}
	reader, a, err := h.d.Artifacts.Open(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "artifact not found")
		return
	}
	defer reader.Close()
	if a.ContentType != "" {
		w.Header().Set("Content-Type", a.ContentType)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", a.Name))
	w.Header().Set("Content-Length", strconv.FormatInt(a.Size, 10))
	io.Copy(w, reader)
}

func (h *handlers) uploadArtifact(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if h.d.Artifacts == nil {
		writeErr(w, http.StatusServiceUnavailable, "artifact manager not available")
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "file field required")
		return
	}
	defer file.Close()
	name := header.Filename
	a, err := h.d.Artifacts.Save(id, name, file)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

// ---------------------------------------------------------------------------
// Notification Channels
// ---------------------------------------------------------------------------

type createNotificationChannelReq struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Config      string `json:"config"`
	Conditions  string `json:"conditions"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

func validNotificationChannelType(value store.NotificationChannelType) bool {
	switch value {
	case store.NotificationChannelWeb,
		store.NotificationChannelEmail,
		store.NotificationChannelFeishu,
		store.NotificationChannelWebhook,
		store.NotificationChannelDiscord,
		store.NotificationChannelWeCom,
		store.NotificationChannelTelegram:
		return true
	default:
		return false
	}
}

func (h *handlers) listInAppNotifications(w http.ResponseWriter, r *http.Request) {
	limit := 30
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil {
			limit = value
		}
	}
	feed, err := h.d.Store.GetInAppNotificationFeed(userIDOf(r), limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, feed)
}

func (h *handlers) markInAppNotificationsRead(w http.ResponseWriter, r *http.Request) {
	var req struct {
		LastEventID int64 `json:"last_event_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.LastEventID < 0 {
		writeErr(w, http.StatusBadRequest, "valid last_event_id is required")
		return
	}
	if err := h.d.Store.MarkInAppNotificationsRead(userIDOf(r), req.LastEventID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "web notification not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ok", "last_event_id": req.LastEventID})
}

func (h *handlers) listNotificationChannels(w http.ResponseWriter, _ *http.Request) {
	channels, err := h.d.Store.ListNotificationChannels()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, channels)
}

func (h *handlers) createNotificationChannel(w http.ResponseWriter, r *http.Request) {
	var req createNotificationChannelReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Type == "" {
		req.Type = "webhook"
	}
	ct := store.NotificationChannelType(req.Type)
	if !validNotificationChannelType(ct) {
		writeErr(w, http.StatusBadRequest, "unsupported notification channel type")
		return
	}
	if ct == store.NotificationChannelWeb {
		writeErr(w, http.StatusConflict, "the built-in web notification channel already exists")
		return
	}
	c, err := h.d.Store.CreateNotificationChannel(req.Name, ct, req.Config, req.Conditions, req.Description, req.Enabled)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (h *handlers) getNotificationChannel(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	c, err := h.d.Store.GetNotificationChannel(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "channel not found")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *handlers) updateNotificationChannel(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req createNotificationChannelReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ct := store.NotificationChannelType(req.Type)
	if !validNotificationChannelType(ct) {
		writeErr(w, http.StatusBadRequest, "unsupported notification channel type")
		return
	}
	if err := h.d.Store.UpdateNotificationChannel(id, req.Name, ct, req.Config, req.Conditions, req.Description, req.Enabled); err != nil {
		if errors.Is(err, store.ErrRequiredNotificationChannel) {
			writeCodedErr(w, http.StatusConflict, "required_notification_channel", "the built-in web notification channel must remain enabled")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	c, err := h.d.Store.GetNotificationChannel(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *handlers) deleteNotificationChannel(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.d.Store.DeleteNotificationChannel(id); err != nil {
		if errors.Is(err, store.ErrRequiredNotificationChannel) {
			writeCodedErr(w, http.StatusConflict, "required_notification_channel", "the built-in web notification channel is required")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handlers) listNotificationEvents(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	events, err := h.d.Store.ListNotificationEvents(id, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, events)
}

// ---------------------------------------------------------------------------
// Statistics
// ---------------------------------------------------------------------------

func (h *handlers) getDashboardStats(w http.ResponseWriter, _ *http.Request) {
	if h.d.Statistics == nil {
		writeErr(w, http.StatusServiceUnavailable, "statistics service not available")
		return
	}
	stats, err := h.d.Statistics.GetDashboardStats()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (h *handlers) getProjectStats(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if v, err := strconv.Atoi(d); err == nil && v > 0 {
			days = v
		}
	}
	if h.d.Statistics == nil {
		// 退化为直接查 store
		stats, err := h.d.Store.GetProjectBuildStats(id, days)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, stats)
		return
	}
	stats, err := h.d.Statistics.GetProjectStats(id, days)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// ---------------------------------------------------------------------------
// Audit Logs
// ---------------------------------------------------------------------------

func (h *handlers) listAuditLogs(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}
	logs, err := h.d.Store.ListAuditLogs(limit, offset)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, logs)
}

// audit 是个轻量封装：记录审计日志，失败不影响主流程
func (h *handlers) audit(r *http.Request, action, resourceType, resourceID, detail string) {
	if h.d.Store == nil {
		return
	}
	uid := userIDOf(r)
	var username string
	if uid > 0 {
		if u, err := h.d.Store.GetUser(uid); err == nil {
			username = u.Username
		}
	}
	ip := clientIP(r)
	_ = h.d.Store.CreateAuditLog(uid, username, action, resourceType, resourceID, detail, ip)
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.Index(xff, ","); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return xff
	}
	if r.RemoteAddr == "" {
		return ""
	}
	if i := strings.LastIndex(r.RemoteAddr, ":"); i > 0 {
		return r.RemoteAddr[:i]
	}
	return r.RemoteAddr
}

// ---------------------------------------------------------------------------
// API Tokens
// ---------------------------------------------------------------------------

type createAPITokenReq struct {
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at"`
}

func normalizeAPITokenScopes(requested []string) ([]string, error) {
	normalized := make([]string, 0, len(requested))
	seen := make(map[string]struct{}, len(requested))
	for _, scope := range requested {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			return nil, fmt.Errorf("scope cannot be empty")
		}
		if len(scope) > 128 {
			return nil, fmt.Errorf("scope is too long")
		}
		if _, exists := seen[scope]; exists {
			continue
		}
		seen[scope] = struct{}{}
		normalized = append(normalized, scope)
	}
	return normalized, nil
}

func hasAPITokenScope(scopes []string, required string) bool {
	for _, scope := range scopes {
		if scope == required {
			return true
		}
	}
	return false
}

func canTriggerBuildRole(role string) bool {
	return role == "admin" || role == "developer"
}

func (h *handlers) listAPITokens(w http.ResponseWriter, r *http.Request) {
	uid := userIDOf(r)
	tokens, err := h.d.Store.ListAPITokens(uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tokens)
}

func (h *handlers) createAPIToken(w http.ResponseWriter, r *http.Request) {
	var req createAPITokenReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	scopes, err := normalizeAPITokenScopes(req.Scopes)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.ExpiresAt != nil && !req.ExpiresAt.After(time.Now()) {
		writeErr(w, http.StatusBadRequest, "expires_at must be in the future")
		return
	}
	owner, err := h.d.Store.GetUser(userIDOf(r))
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "authenticated user no longer exists")
		return
	}
	if hasAPITokenScope(scopes, auth.ScopeBuildTrigger) && !canTriggerBuildRole(owner.Role) {
		writeErr(w, http.StatusForbidden, "current role cannot grant build:trigger")
		return
	}
	if hasAPITokenScope(scopes, auth.ScopeSystemUpdate) && owner.Role != "admin" {
		writeErr(w, http.StatusForbidden, "only administrators can grant system:update")
		return
	}
	// 生成 bw_<32hex> 明文 token，仅返回一次
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	plain := "bw_" + hex.EncodeToString(raw)
	hash := sha256.Sum256([]byte(plain))
	tokenHash := hex.EncodeToString(hash[:])
	prefix := plain[:11] // "bw_" + 前 8 hex
	scopesJSONBytes, err := json.Marshal(scopes)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to encode scopes")
		return
	}
	t, err := h.d.Store.CreateAPIToken(owner.ID, req.Name, tokenHash, prefix, string(scopesJSONBytes), req.ExpiresAt)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(r, "create", "api_token", fmt.Sprintf("%d", t.ID), req.Name)
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"token":  plain,
		"detail": t,
	})
}

func (h *handlers) deleteAPIToken(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	deleted, err := h.d.Store.DeleteAPITokenForUser(id, userIDOf(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !deleted {
		writeErr(w, http.StatusNotFound, "api token not found")
		return
	}
	h.audit(r, "delete", "api_token", fmt.Sprintf("%d", id), "")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ---------------------------------------------------------------------------
// Build Approvals
// ---------------------------------------------------------------------------

func (h *handlers) listPendingApprovals(w http.ResponseWriter, _ *http.Request) {
	if h.d.Approval == nil {
		writeErr(w, http.StatusServiceUnavailable, "approval service not available")
		return
	}
	items, err := h.d.Approval.ListPendingDetails()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *handlers) approveBuild(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if h.d.Approval == nil {
		writeErr(w, http.StatusServiceUnavailable, "approval service not available")
		return
	}
	var req struct {
		Comment string `json:"comment"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if err := h.d.Approval.Approve(id, userIDOf(r), req.Comment); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	h.audit(r, "approve", "build", fmt.Sprintf("%d", id), req.Comment)
	if h.d.Runner != nil {
		if err := h.d.Runner.Enqueue(id); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "approved"})
}

func (h *handlers) rejectBuild(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if h.d.Approval == nil {
		writeErr(w, http.StatusServiceUnavailable, "approval service not available")
		return
	}
	var req struct {
		Comment string `json:"comment"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if err := h.d.Approval.Reject(id, userIDOf(r), req.Comment); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	h.audit(r, "reject", "build", fmt.Sprintf("%d", id), req.Comment)
	writeJSON(w, http.StatusOK, map[string]string{"status": "rejected"})
}

// ---------------------------------------------------------------------------
// Build Logs: download & search
// ---------------------------------------------------------------------------

func (h *handlers) downloadBuildLogs(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "txt"
	}
	mgr := engine.NewBuildLogManager(h.d.Store)
	data, filename, truncated := mgr.DownloadLogsWithRetention(id, format)
	if filename == "" {
		writeErr(w, http.StatusNotFound, "build not found")
		return
	}
	ct := "text/plain; charset=utf-8"
	if format == "json" {
		ct = "application/json"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("X-BuildWorld-Log-Truncated", strconv.FormatBool(truncated))
	w.Header().Set("X-BuildWorld-Log-Retention-Characters", strconv.Itoa(store.BuildLogRetentionCharacters))
	w.Write(data)
}

func (h *handlers) searchBuildLogs(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	q := r.URL.Query().Get("q")
	mgr := engine.NewBuildLogManager(h.d.Store)
	entries, err := mgr.SearchLogs(id, q)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

// ---------------------------------------------------------------------------
// Build Badge (公开 SVG)
// ---------------------------------------------------------------------------

func (h *handlers) buildBadge(w http.ResponseWriter, r *http.Request) {
	projectName := chi.URLParam(r, "projectName")
	if projectName == "" {
		http.NotFound(w, r)
		return
	}
	p, err := h.d.Store.GetProjectByName(projectName)
	if err != nil {
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Write(engine.GenerateBadge("unknown", "build"))
		return
	}
	builds, err := h.d.Store.ListBuildsByProject(p.ID)
	if err != nil || len(builds) == 0 {
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Write(engine.GenerateBadge("pending", p.Name))
		return
	}
	status := builds[0].Status
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(engine.GenerateBadge(status, p.Name))
}

// ---------------------------------------------------------------------------
// Test Results
// ---------------------------------------------------------------------------

type buildTestResultsResponse struct {
	Summary *store.TestResult       `json:"summary"`
	Cases   []engine.TestCaseResult `json:"cases"`
	Results []*store.TestResult     `json:"results"`
}

func (h *handlers) getBuildTestResults(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	results, err := h.d.Store.ListBuildTestResults(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	response := buildTestResultsResponse{
		Cases:   make([]engine.TestCaseResult, 0),
		Results: results,
	}
	if response.Results == nil {
		response.Results = make([]*store.TestResult, 0)
	}
	if len(results) > 0 {
		response.Summary = results[0]
		parsed, parseErr := engine.ParseJUnitReport([]byte(results[0].ReportXML))
		if parseErr != nil {
			writeErr(w, http.StatusInternalServerError, "stored test report is invalid")
			return
		}
		response.Cases = parsed.Cases
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *handlers) uploadTestResults(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 50<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "failed to read body")
		return
	}
	if len(body) == 0 {
		writeErr(w, http.StatusBadRequest, "empty body")
		return
	}
	parser := engine.NewTestReportParser()
	result, err := parser.ParseAndSave(id, body, h.d.Store)
	if err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("parse failed: %v", err))
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

// ---------------------------------------------------------------------------
// Project Groups
// ---------------------------------------------------------------------------

type createProjectGroupReq struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Color       *string `json:"color"`
}

func (h *handlers) listProjectGroups(w http.ResponseWriter, _ *http.Request) {
	groups, err := h.d.Store.ListProjectGroups()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, groups)
}

func (h *handlers) createProjectGroup(w http.ResponseWriter, r *http.Request) {
	var req createProjectGroupReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	color := store.ProjectGroupColorNeutral
	if req.Color != nil {
		if !store.IsValidProjectGroupColor(*req.Color) {
			writeErr(w, http.StatusBadRequest, store.ErrInvalidProjectGroupColor.Error())
			return
		}
		color = *req.Color
	}
	g, err := h.d.Store.CreateProjectGroupWithColor(req.Name, req.Description, color)
	if err != nil {
		if errors.Is(err, store.ErrProjectGroupNameExists) {
			writeCodedErr(w, http.StatusConflict, "project_group_name_exists", err.Error())
			return
		}
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	h.audit(r, "create", "project_group", fmt.Sprintf("%d", g.ID), req.Name)
	writeJSON(w, http.StatusCreated, g)
}

func (h *handlers) updateProjectGroup(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req createProjectGroupReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	existing, err := h.d.Store.GetProjectGroup(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "project group not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	color := existing.Color
	if req.Color != nil {
		if !store.IsValidProjectGroupColor(*req.Color) {
			writeErr(w, http.StatusBadRequest, store.ErrInvalidProjectGroupColor.Error())
			return
		}
		color = *req.Color
	}
	if err := h.d.Store.UpdateProjectGroupWithColor(id, req.Name, req.Description, color); err != nil {
		if errors.Is(err, store.ErrProjectGroupNameExists) {
			writeCodedErr(w, http.StatusConflict, "project_group_name_exists", err.Error())
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "project group not found")
			return
		}
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	group, err := h.d.Store.GetProjectGroup(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(r, "update", "project_group", fmt.Sprintf("%d", id), req.Name)
	writeJSON(w, http.StatusOK, group)
}

func (h *handlers) deleteProjectGroup(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.d.Store.DeleteProjectGroup(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "project group not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(r, "delete", "project_group", fmt.Sprintf("%d", id), "")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ---------------------------------------------------------------------------
// Build Queue
// ---------------------------------------------------------------------------

func (h *handlers) listBuildQueue(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	items, err := h.d.Store.ListBuildQueue(status)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *handlers) reorderBuildQueue(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		Operation string `json:"operation"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if strings.TrimSpace(req.Operation) == "" {
		writeErr(w, http.StatusBadRequest, "operation is required")
		return
	}
	result, err := engine.NewBuildQueueReorderService(h.d.Store, nil).Apply(id, req.Operation)
	if err != nil {
		switch {
		case errors.Is(err, engine.ErrUnknownQueueOperation):
			writeErr(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, store.ErrBuildQueueItemNotMovable):
			writeErr(w, http.StatusConflict, "build queue item is no longer movable")
		default:
			writeErr(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	if h.d.Runner != nil {
		h.d.Runner.WakeQueue()
	}
	h.audit(r, "reorder", "build_queue", fmt.Sprintf("%d", id), req.Operation)
	writeJSON(w, http.StatusOK, result)
}

// ---------------------------------------------------------------------------
// HTTP Trigger (公开，使用 API Token 认证)
// ---------------------------------------------------------------------------

func (h *handlers) httpTriggerBuild(w http.ResponseWriter, r *http.Request) {
	projectName := chi.URLParam(r, "projectName")
	if projectName == "" {
		writeErr(w, http.StatusBadRequest, "projectName is required")
		return
	}
	// API token validation is intentionally repeated here because this endpoint
	// is public to JWT middleware and supports only scoped API-token auth.
	authorization := strings.Fields(r.Header.Get("Authorization"))
	if len(authorization) != 2 || !strings.EqualFold(authorization[0], "Bearer") {
		writeErr(w, http.StatusUnauthorized, "valid api token required")
		return
	}
	principal, ok := auth.ResolveAPIToken(h.d.Store, authorization[1])
	if !ok {
		writeErr(w, http.StatusUnauthorized, "invalid api token")
		return
	}
	if !canTriggerBuildRole(principal.Role) {
		writeErr(w, http.StatusForbidden, "current role cannot trigger builds")
		return
	}
	if _, ok := principal.Scopes[auth.ScopeBuildTrigger]; !ok {
		writeErr(w, http.StatusForbidden, "build:trigger scope required")
		return
	}
	_ = h.d.Store.UpdateAPITokenLastUsed(principal.TokenID)

	project, err := h.d.Store.GetProjectByName(projectName)
	if err != nil {
		writeErr(w, http.StatusNotFound, "project not found")
		return
	}
	if !project.Enabled {
		writeCodedErr(w, http.StatusConflict, "project_disabled", "project is disabled")
		return
	}

	req, err := decodeTriggerBuildRequest(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	branch := strings.TrimSpace(req.Branch)
	if branch == "" {
		branch = project.DefaultBranch
	}
	paramsJSON, err := h.resolveProjectBuildParameters(project, req.Parameters)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	num, err := h.d.Store.NextBuildNumber(project.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	build, err := h.d.Store.CreateBuild(project.ID, num, "api", branch, "", paramsJSON, nil, nil)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.requestBuildApproval(project, build, principal.UserID); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	// 记录触发者
	_ = h.d.Store.CreateAuditLog(principal.UserID, "", "trigger", "project", projectName, fmt.Sprintf("build #%d via api token %q", build.Number, principal.Name), clientIP(r))
	if h.d.Runner != nil {
		if err := h.d.Runner.Enqueue(build.ID); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	writeJSON(w, http.StatusCreated, h.publicBuild(build))
}

// ---------------------------------------------------------------------------
// Global Settings (用 env_vars scope=system 存储)
// ---------------------------------------------------------------------------

func (h *handlers) getGlobalSettings(w http.ResponseWriter, _ *http.Request) {
	vars, err := h.d.Store.ListEnvVars("system", nil)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	settings := defaultGlobalSettings(h.d.Cfg)
	for _, v := range vars {
		if v.Name == "port" {
			continue
		}
		if v.IsSecret {
			settings[v.Name+"_configured"] = strconv.FormatBool(v.Value != "")
			continue
		}
		settings[v.Name] = v.Value
	}
	writeJSON(w, http.StatusOK, settings)
}

func (h *handlers) updateGlobalSettings(w http.ResponseWriter, r *http.Request) {
	var settings map[string]string
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	for name, value := range settings {
		if err := validateGlobalSetting(name, value); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := h.d.Store.SetEnvVar("system", nil, name, value, false, ""); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if h.d.Runner != nil {
		if err := h.d.Runner.ReloadExecutionPolicy(); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	h.audit(r, "update", "settings", "", fmt.Sprintf("%d keys", len(settings)))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ---------------------------------------------------------------------------
// Server Metrics
// ---------------------------------------------------------------------------

func (h *handlers) serverMetrics(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	builds, _ := h.d.Store.ListBuilds(500)
	projects, _ := h.d.Store.ListProjects()
	workers, _ := h.d.Store.ListWorkers()

	var runningBuilds int
	for _, b := range builds {
		if b.Status == "running" {
			runningBuilds++
		}
	}
	var onlineWorkers int
	for _, w_ := range workers {
		if w_.Status == "online" {
			onlineWorkers++
		}
	}

	cpuLimitPercent := 100
	backgroundMode := false
	if h.d.Runner != nil {
		policy := h.d.Runner.ExecutionPolicySnapshot()
		cpuLimitPercent = policy.CPUPercent
		backgroundMode = policy.BackgroundMode
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"go_version":        runtime.Version(),
		"goroutines":        runtime.NumGoroutine(),
		"cpu_count":         runtime.NumCPU(),
		"gomaxprocs":        runtime.GOMAXPROCS(0),
		"cpu_limit_percent": cpuLimitPercent,
		"background_mode":   backgroundMode,
		"mem_alloc_bytes":   m.Alloc,
		"mem_sys_bytes":     m.Sys,
		"heap_objects":      m.HeapObjects,
		"projects":          len(projects),
		"builds_total":      len(builds),
		"running_builds":    runningBuilds,
		"online_workers":    onlineWorkers,
		"workers_total":     len(workers),
		"uptime_seconds":    int64(time.Since(startTime).Seconds()),
	})
}

// startTime is the process start used for the metrics uptime.
var startTime = time.Now()

// ---------------------------------------------------------------------------
// Big Screen
// ---------------------------------------------------------------------------

func (h *handlers) getBigScreenData(w http.ResponseWriter, _ *http.Request) {
	if h.d.BigScreen == nil {
		writeErr(w, http.StatusServiceUnavailable, "bigscreen service not available")
		return
	}
	data, err := h.d.BigScreen.GetBigScreenData()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, data)
}

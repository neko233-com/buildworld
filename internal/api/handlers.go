package api

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/neko233-com/buildworld/internal/auth"
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
	writeJSON(w, http.StatusOK, map[string]string{"version": "0.1.0"})
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
	token, err := h.d.JWT.Generate(user.ID, user.Role, sessionDuration)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: "bw_session", Value: token, Path: "/", MaxAge: int(sessionDuration.Seconds()),
		HttpOnly: true, Secure: h.d.Cfg.Server.TLS, SameSite: http.SameSiteLaxMode,
	})
	// Login is intentionally audited directly because this public endpoint has
	// not yet passed through the JWT middleware that h.audit normally reads.
	_ = h.d.Store.CreateAuditLog(user.ID, user.Username, "login", "session", "", "native JWT session issued", clientIP(r))
	user.PasswordHash = ""
	writeJSON(w, http.StatusOK, loginResp{Token: token, User: user, ExpiresIn: int64(sessionDuration.Seconds())})
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
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	RepoURL       string   `json:"repo_url"`
	RepoType      string   `json:"repo_type"`
	DefaultBranch string   `json:"default_branch"`
	VCSRootID     *int64   `json:"vcs_root_id"`
	TemplateID    *int64   `json:"template_id"`
	GroupID       *int64   `json:"group_id"`
	Tags          []string `json:"tags"`
	Config        string   `json:"config"`
}

func (h *handlers) listProjects(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("view") == "summary" {
		projects, err := h.d.Store.ListProjectSummaries()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.Header().Set("X-Buildworld-View-Version", "1")
		writeJSON(w, http.StatusOK, projects)
		return
	}
	projects, err := h.d.Store.ListProjects()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, projects)
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
	if req.RepoType == "" {
		req.RepoType = "git"
	}
	if req.GroupID != nil {
		if _, err := h.d.Store.GetProjectGroup(*req.GroupID); err != nil {
			writeErr(w, http.StatusBadRequest, "project group not found")
			return
		}
	}
	p, err := h.d.Store.CreateProject(req.Name, req.Description, req.RepoURL, req.RepoType,
		req.DefaultBranch, req.Config, userIDOf(r), req.VCSRootID, req.TemplateID, req.Tags)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.d.Store.SetProjectGroup(p.ID, req.GroupID); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
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
	if req.GroupID != nil {
		if _, err := h.d.Store.GetProjectGroup(*req.GroupID); err != nil {
			writeErr(w, http.StatusBadRequest, "project group not found")
			return
		}
	}
	if err := h.d.Store.UpdateProject(id, req.Name, req.Description, req.RepoURL, req.RepoType,
		req.DefaultBranch, req.Config, req.VCSRootID, req.TemplateID, req.Tags); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.d.Store.SetProjectGroup(id, req.GroupID); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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
	config, err := engine.ParsePipelineConfig(project.Config)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	stepCount := 0
	for _, stage := range config.Stages {
		stepCount += len(stage.Steps)
	}
	format := "yaml"
	trimmed := strings.TrimSpace(project.Config)
	if engine.IsTypeScriptPipeline(trimmed) {
		format = "typescript"
	} else if strings.HasPrefix(trimmed, "{") {
		format = "json"
	} else if strings.HasPrefix(trimmed, "#") {
		format = "markdown"
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"valid":              true,
		"format":             format,
		"stages":             len(config.Stages),
		"steps":              stepCount,
		"allow_long_running": config.AllowLongRunning,
	})
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

func (h *handlers) loadProjectBuildConfig(project *store.Project) (*engine.BuildConfig, error) {
	config, err := engine.ParsePipelineConfig(project.Config)
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

func (h *handlers) resolveProjectBuildParameters(project *store.Project, supplied map[string]interface{}) (string, error) {
	config, err := h.loadProjectBuildConfig(project)
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
	config, err := h.loadProjectBuildConfig(project)
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

func redactBuildParameters(build *store.Build, secretNames map[string]struct{}) *store.Build {
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
		if _, secret := secretNames[name]; secret || looksLikeSecretParameter(name) {
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
		return redactBuildParameters(build, nil)
	}
	config, _ := h.loadProjectBuildConfig(project)
	result := redactBuildParameters(build, secretBuildParameterNames(config))
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
	for _, build := range builds {
		names, cached := secretNamesByProject[build.ProjectID]
		if !cached {
			project, projectErr := h.d.Store.GetProject(build.ProjectID)
			if projectErr == nil {
				config, _ := h.loadProjectBuildConfig(project)
				names = secretBuildParameterNames(config)
			}
			secretNamesByProject[build.ProjectID] = names
		}
		result = append(result, redactBuildParameters(build, names))
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
	writeJSON(w, http.StatusOK, map[string]string{"log": build.Log})
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
	Status plugin.PluginStatus `json:"status"`
}

type installPluginReq struct {
	Name           string `json:"name"`
	Version        string `json:"version"`
	Description    string `json:"description"`
	Author         string `json:"author"`
	Config         string `json:"config"`
	Script         string `json:"script"`
	UIScript       string `json:"ui_script"`
	ScriptLang     string `json:"script_lang"`
	SourceScript   string `json:"source_script"`
	SourceUIScript string `json:"source_ui_script"`
	UILang         string `json:"ui_lang"`
	Source         string `json:"source"`
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
	stored, storeErr := h.d.Store.GetPluginByName(manifest.Name)
	switch {
	case storeErr == nil:
		enabled = stored.Enabled
		storeErr = h.d.Store.UpdatePluginMetadata(stored.ID, manifest.Version, manifest.Description, "", manifest.Source)
	case errors.Is(storeErr, sql.ErrNoRows):
		stored, storeErr = h.d.Store.CreatePlugin(manifest.Name, manifest.Version, manifest.Description, "", "", "go", "", "", manifest.Source)
	default:
		// Preserve the original store error below.
	}
	if storeErr != nil {
		writeErr(w, http.StatusInternalServerError, "persist plugin metadata: "+storeErr.Error())
		return
	}
	stepsJSON, _ := json.Marshal(manifest.Steps)
	_ = h.d.Store.UpdatePluginStatus(manifest.Name, enabled, string(stepsJSON), "[]", "[]")
	h.audit(r, "install", "plugin", manifest.Name, "GitHub binary plugin: "+req.URL)
	writeJSON(w, http.StatusCreated, manifest)
}

func (h *handlers) listPlugins(w http.ResponseWriter, _ *http.Request) {
	dbPlugins, err := h.d.Store.ListPlugins()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	pluginMap := make(map[string]*store.Plugin)
	for _, p := range dbPlugins {
		pluginMap[p.Name] = p
	}

	if h.d.Loader != nil {
		for _, name := range getBuiltinPluginNames(h.d.Loader) {
			p := h.d.Loader.Get(name)
			if p == nil {
				continue
			}
			source := p.InstallSource()
			if dbP, exists := pluginMap[name]; exists {
				if source != "builtin" && dbP.Source != source {
					_ = h.d.Store.UpdatePluginMetadata(dbP.ID, p.Version, p.Description, p.Author, source)
					dbP.Version, dbP.Description, dbP.Author, dbP.Source = p.Version, p.Description, p.Author, source
				}
				continue
			}
			uiExtJSON, _ := json.Marshal(p.UIExtensions())
			dbP := &store.Plugin{
				Name:         p.Name,
				Version:      p.Version,
				Description:  p.Description,
				Author:       p.Author,
				Enabled:      true,
				Source:       source,
				Steps:        "[]",
				Triggers:     "[]",
				UIExtensions: string(uiExtJSON),
			}
			pluginMap[name] = dbP
			created, _ := h.d.Store.CreatePlugin(p.Name, p.Version, p.Description, p.Author, "", "js", "", "", source)
			if created != nil {
				dbP.ID = created.ID
			}
		}
	}

	var result []pluginWithStatus
	for _, p := range pluginMap {
		status := plugin.PluginStatus{Loaded: false, Steps: []string{}, Triggers: []string{}}
		if h.d.Loader != nil {
			if rp := h.d.Loader.Get(p.Name); rp != nil {
				status = rp.GetStatus()
				stepsJSON, _ := json.Marshal(status.Steps)
				triggersJSON, _ := json.Marshal(status.Triggers)
				uiExtJSON, _ := json.Marshal(rp.UIExtensions())
				p.Steps = string(stepsJSON)
				p.Triggers = string(triggersJSON)
				p.UIExtensions = string(uiExtJSON)
			}
		}
		result = append(result, pluginWithStatus{Plugin: p, Status: status})
	}

	writeJSON(w, http.StatusOK, result)
}

func getBuiltinPluginNames(l *plugin.Loader) []string {
	var names []string
	for _, p := range l.ListAll() {
		names = append(names, p.Name)
	}
	return names
}

func (h *handlers) installPlugin(w http.ResponseWriter, r *http.Request) {
	var req installPluginReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Version == "" {
		req.Version = "1.0.0"
	}
	if req.Source == "" {
		req.Source = "upload"
	}
	if req.ScriptLang == "" {
		req.ScriptLang = "js"
	}

	if h.d.Loader != nil && req.Script != "" {
		if err := h.d.Loader.InstallPlugin(req.Name, req.Version, req.Description, req.Author, req.Script, req.UIScript, req.ScriptLang, req.SourceScript, req.SourceUIScript, req.UILang); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	p, err := h.d.Store.CreatePlugin(req.Name, req.Version, req.Description, req.Author, req.Config, req.ScriptLang, req.SourceScript, req.SourceUIScript, req.Source)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	if h.d.Loader != nil {
		if err := h.d.Loader.Load(req.Name); err != nil {
			log.Printf("Warning: failed to load installed plugin %s: %v", req.Name, err)
		} else if rp := h.d.Loader.Get(req.Name); rp != nil {
			stepsJSON, _ := json.Marshal(rp.StepTypes())
			triggersJSON, _ := json.Marshal(nil)
			uiExtJSON, _ := json.Marshal(rp.UIExtensions())
			_ = h.d.Store.UpdatePluginStatus(req.Name, true, string(stepsJSON), string(triggersJSON), string(uiExtJSON))
		}
	}

	writeJSON(w, http.StatusCreated, p)
}

func (h *handlers) deletePlugin(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		id, err := parseIDInt64(r)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid name or id")
			return
		}
		p, err := h.d.Store.GetPlugin(id)
		if err != nil {
			writeErr(w, http.StatusNotFound, "plugin not found")
			return
		}
		name = p.Name
	}

	p, err := h.d.Store.GetPluginByName(name)
	if err != nil {
		writeErr(w, http.StatusNotFound, "plugin not found")
		return
	}
	if p.Source == "builtin" {
		writeErr(w, http.StatusBadRequest, "cannot delete builtin plugin")
		return
	}

	if h.d.Loader != nil {
		_ = h.d.Loader.DeletePlugin(name)
	}
	if err := h.d.Store.DeletePluginByName(name); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handlers) togglePlugin(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		id, err := parseIDInt64(r)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid name or id")
			return
		}
		p, err := h.d.Store.GetPlugin(id)
		if err != nil {
			writeErr(w, http.StatusNotFound, "plugin not found")
			return
		}
		name = p.Name
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	p, err := h.d.Store.GetPluginByName(name)
	if err != nil {
		writeErr(w, http.StatusNotFound, "plugin not found")
		return
	}
	if err := h.d.Store.UpdatePluginEnabled(p.ID, req.Enabled); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	if h.d.Loader != nil {
		h.d.Loader.SetEnabled(name, req.Enabled)
		if req.Enabled {
			if err := h.d.Loader.Load(name); err != nil {
				log.Printf("Warning: failed to enable plugin %s: %v", name, err)
			}
		} else {
			h.d.Loader.Unload(name)
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handlers) reloadPlugin(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	if h.d.Loader == nil {
		writeErr(w, http.StatusInternalServerError, "loader not configured")
		return
	}
	if err := h.d.Loader.Reload(name); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rp := h.d.Loader.Get(name); rp != nil {
		stepsJSON, _ := json.Marshal(rp.StepTypes())
		triggersJSON, _ := json.Marshal(nil)
		uiExtJSON, _ := json.Marshal(rp.UIExtensions())
		_ = h.d.Store.UpdatePluginStatus(name, true, string(stepsJSON), string(triggersJSON), string(uiExtJSON))
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handlers) getPluginUI(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	if h.d.Loader == nil {
		http.NotFound(w, r)
		return
	}
	script, ok := h.d.Loader.GetPluginUI(name)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(script))
}

func (h *handlers) listUIExtensions(w http.ResponseWriter, _ *http.Request) {
	if h.d.Loader == nil {
		writeJSON(w, http.StatusOK, map[string][]plugin.UIExtension{})
		return
	}
	exts := h.d.Loader.GetAllUIExtensions()
	writeJSON(w, http.StatusOK, exts)
}

func (h *handlers) getPluginSource(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}

	p, err := h.d.Store.GetPluginByName(name)
	if err != nil {
		writeErr(w, http.StatusNotFound, "plugin not found")
		return
	}

	if p.Source == "builtin" {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"source_script":    "",
			"source_ui_script": "",
			"script_lang":      "js",
			"ui_lang":          "js",
			"builtin":          true,
		})
		return
	}

	scriptLang := p.ScriptLang
	sourceScript := p.SourceScript
	sourceUIScript := p.SourceUIScript
	uiLang := ""

	if h.d.Loader != nil {
		if s, l, ok := h.d.Loader.GetSourceScript(name); ok && sourceScript == "" {
			sourceScript = s
			scriptLang = l
		}
		if s, l, ok := h.d.Loader.GetSourceUIScript(name); ok && sourceUIScript == "" {
			sourceUIScript = s
			uiLang = l
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"source_script":    sourceScript,
		"source_ui_script": sourceUIScript,
		"script_lang":      scriptLang,
		"ui_lang":          uiLang,
		"builtin":          false,
	})
}

func (h *handlers) updatePluginSource(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}

	p, err := h.d.Store.GetPluginByName(name)
	if err != nil {
		writeErr(w, http.StatusNotFound, "plugin not found")
		return
	}

	if p.Source == "builtin" {
		writeErr(w, http.StatusBadRequest, "cannot edit builtin plugin")
		return
	}

	var req installPluginReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.ScriptLang == "" {
		req.ScriptLang = "js"
	}

	if h.d.Loader != nil {
		if err := h.d.Loader.WritePluginFiles(name, req.Script, req.UIScript, req.ScriptLang, req.SourceScript, req.SourceUIScript, req.UILang); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	if err := h.d.Store.UpdatePluginSource(p.ID, req.ScriptLang, req.SourceScript, req.SourceUIScript, req.Script, req.UIScript); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	if h.d.Loader != nil {
		if err := h.d.Loader.Reload(name); err != nil {
			log.Printf("Warning: failed to reload plugin %s: %v", name, err)
		} else if rp := h.d.Loader.Get(name); rp != nil {
			stepsJSON, _ := json.Marshal(rp.StepTypes())
			triggersJSON, _ := json.Marshal(nil)
			uiExtJSON, _ := json.Marshal(rp.UIExtensions())
			_ = h.d.Store.UpdatePluginStatus(name, true, string(stepsJSON), string(triggersJSON), string(uiExtJSON))
		}
	}

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
	if h.shouldRestartDevelopmentServer(payload.HeadCommit.Message) {
		if err := h.restartDevelopmentServer(); err != nil {
			return err
		}
	}
	projects, err := h.d.Store.ListProjects()
	if err != nil {
		return err
	}
	for _, p := range projects {
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

func (h *handlers) shouldRestartDevelopmentServer(message string) bool {
	marker := h.d.Cfg.Automation.CommitRestartMarker
	if marker == "" {
		marker = "[buildworld:restart]"
	}
	return strings.Contains(strings.ToLower(message), strings.ToLower(marker))
}

func (h *handlers) restartDevelopmentServer() error {
	command := strings.TrimSpace(h.d.Cfg.Automation.DevRestartCommand)
	if command == "" {
		return nil
	}
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", command)
	} else {
		cmd = exec.Command("sh", "-c", command)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Start()
}

func (h *handlers) githubWebhook(w http.ResponseWriter, r *http.Request) {
	secret := h.d.Cfg.Automation.GitHubWebhookSecret
	if secret == "" {
		secret = h.d.Cfg.Auth.JWTSecret
	}
	wh := webhook.NewWebhookHandler(secret)
	wh.OnPush(h.triggerBuildByWebhook)
	wh.HandleGitHubWebhook(w, r)
}

func (h *handlers) gitlabWebhook(w http.ResponseWriter, r *http.Request) {
	wh := webhook.NewWebhookHandler(h.d.Cfg.Automation.GitHubWebhookSecret)
	wh.OnPush(h.triggerBuildByWebhook)
	wh.HandleGitLabWebhook(w, r)
}

// giteaWebhook uses the same GitHub-compatible signature scheme (X-Gitea-Signature).
func (h *handlers) giteaWebhook(w http.ResponseWriter, r *http.Request) {
	wh := webhook.NewWebhookHandler(h.d.Cfg.Automation.GitHubWebhookSecret)
	wh.OnPush(h.triggerBuildByWebhook)
	wh.HandleGitHubWebhook(w, r)
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
		ct := store.CredentialType(t)
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
	if req.Type == "" {
		req.Type = "git"
	}
	ct := store.CredentialType(req.Type)
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
	if req.Type == "" {
		req.Type = "git"
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
	ct := store.CredentialType(req.Type)
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
	ct := store.CredentialType(credType)
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
// VCS Roots
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
	if req.Type == "" {
		req.Type = "git"
	}
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
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
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
	scopesJSON := "[]"
	if len(req.Scopes) > 0 {
		if b, err := json.Marshal(req.Scopes); err == nil {
			scopesJSON = string(b)
		}
	}
	t, err := h.d.Store.CreateAPIToken(userIDOf(r), req.Name, tokenHash, prefix, scopesJSON, req.ExpiresAt)
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
	data, filename := mgr.DownloadLogs(id, format)
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
	writeJSON(w, http.StatusOK, results)
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
// Deployment Environments
// ---------------------------------------------------------------------------

type createDeploymentEnvReq struct {
	ProjectID   int64  `json:"project_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Config      string `json:"config"`
}

func (h *handlers) listDeploymentEnvs(w http.ResponseWriter, r *http.Request) {
	projectIDStr := r.URL.Query().Get("project_id")
	if projectIDStr == "" {
		writeErr(w, http.StatusBadRequest, "project_id is required")
		return
	}
	pid, err := strconv.ParseInt(projectIDStr, 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid project_id")
		return
	}
	envs, err := h.d.Store.ListDeploymentEnvs(pid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, envs)
}

func (h *handlers) createDeploymentEnv(w http.ResponseWriter, r *http.Request) {
	var req createDeploymentEnvReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.ProjectID == 0 {
		writeErr(w, http.StatusBadRequest, "name and project_id are required")
		return
	}
	d, err := h.d.Store.CreateDeploymentEnv(req.ProjectID, req.Name, req.Description, req.Config)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(r, "create", "deployment_env", fmt.Sprintf("%d", d.ID), req.Name)
	writeJSON(w, http.StatusCreated, d)
}

func (h *handlers) updateDeploymentEnv(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req createDeploymentEnvReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	if _, err := h.d.Store.GetDeploymentEnv(id); err != nil {
		writeErr(w, http.StatusNotFound, "environment not found")
		return
	}
	if err := h.d.Store.UpdateDeploymentEnv(id, req.Name, req.Description, req.Config); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	updated, err := h.d.Store.GetDeploymentEnv(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(r, "update", "deployment_env", fmt.Sprintf("%d", id), req.Name)
	writeJSON(w, http.StatusOK, updated)
}

func (h *handlers) deleteDeploymentEnv(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.d.Store.DeleteDeploymentEnv(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(r, "delete", "deployment_env", fmt.Sprintf("%d", id), "")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handlers) deployBuild(w http.ResponseWriter, r *http.Request) {
	envID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid env id")
		return
	}
	buildID, err := strconv.ParseInt(chi.URLParam(r, "buildId"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid build id")
		return
	}
	env, err := h.d.Store.GetDeploymentEnv(envID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "env not found")
		return
	}
	build, err := h.d.Store.GetBuild(buildID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "build not found")
		return
	}
	if build.ProjectID != env.ProjectID {
		writeErr(w, http.StatusBadRequest, "build does not belong to env's project")
		return
	}
	if err := h.d.Store.UpdateDeploymentLastBuild(envID, buildID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(r, "deploy", "deployment_env", fmt.Sprintf("%d", envID), fmt.Sprintf("build %d", buildID))
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":   "deployed",
		"env_id":   envID,
		"build_id": buildID,
		"env_name": env.Name,
	})
}

// ---------------------------------------------------------------------------
// Project Groups
// ---------------------------------------------------------------------------

type createProjectGroupReq struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ParentID    *int64 `json:"parent_id"`
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
	g, err := h.d.Store.CreateProjectGroup(req.Name, req.Description, req.ParentID)
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
	if err := h.d.Store.UpdateProjectGroup(id, req.Name, req.Description, req.ParentID); err != nil {
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
		Priority  *int   `json:"priority,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Operation != "" {
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
		return
	}

	// Version 0 compatibility: older clients can still assign a raw priority.
	if req.Priority == nil {
		writeErr(w, http.StatusBadRequest, "operation is required")
		return
	}
	if err := h.d.Store.UpdateBuildQueueItemPriority(id, int64(*req.Priority)); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.d.Runner != nil {
		h.d.Runner.WakeQueue()
	}
	items, err := h.d.Store.ListBuildQueue("")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, &engine.BuildQueueReorderResult{
		Version:   engine.BuildQueueReorderVersion,
		Status:    "ok",
		Operation: "set_priority",
		Items:     items,
	})
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
	// API Token 校验
	token := r.Header.Get("Authorization")
	token = strings.TrimPrefix(token, "Bearer ")
	if !strings.HasPrefix(token, "bw_") {
		writeErr(w, http.StatusUnauthorized, "valid api token required")
		return
	}
	hash := sha256.Sum256([]byte(token))
	hashStr := hex.EncodeToString(hash[:])
	apiToken, err := h.d.Store.GetAPITokenByHash(hashStr)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid api token")
		return
	}
	if apiToken.ExpiresAt != nil && apiToken.ExpiresAt.Before(time.Now()) {
		writeErr(w, http.StatusUnauthorized, "token expired")
		return
	}
	_ = h.d.Store.UpdateAPITokenLastUsed(apiToken.ID)

	project, err := h.d.Store.GetProjectByName(projectName)
	if err != nil {
		writeErr(w, http.StatusNotFound, "project not found")
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
	if err := h.requestBuildApproval(project, build, apiToken.UserID); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	// 记录触发者
	_ = h.d.Store.CreateAuditLog(apiToken.UserID, "", "trigger", "project", projectName, fmt.Sprintf("build #%d via api token %q", build.Number, apiToken.Name), clientIP(r))
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

	var diskFree int64 = -1
	if stat, err := os.Stat("."); err == nil {
		// 仅做 best-effort，跨平台时失败忽略
		_ = stat
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
		"disk_free_bytes":   diskFree,
		"projects":          len(projects),
		"builds_total":      len(builds),
		"running_builds":    runningBuilds,
		"online_workers":    onlineWorkers,
		"workers_total":     len(workers),
		"uptime_seconds":    int64(time.Since(startTime).Seconds()),
	})
}

// startTime 用于 metrics 的 uptime 计算
var startTime = time.Now()

// ---------------------------------------------------------------------------
// Git Hooks
// ---------------------------------------------------------------------------

type createGitHookReq struct {
	Name        string `json:"name"`
	Event       string `json:"event"`
	Branch      string `json:"branch"`
	Secret      string `json:"secret"`
	Enabled     *bool  `json:"enabled"`
	BuildParams string `json:"build_params"`
	Description string `json:"description"`
}

func (h *handlers) listGitHooks(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid project id")
		return
	}
	hooks, err := h.d.Store.ListGitHooks(projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, hooks)
}

func (h *handlers) createGitHook(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid project id")
		return
	}
	var req createGitHookReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Event == "" {
		req.Event = "push"
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	hook, err := h.d.Store.CreateGitHook(projectID, req.Name, store.GitHookEvent(req.Event), req.Branch, req.Secret, enabled, req.BuildParams, req.Description)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(r, "create", "git_hook", fmt.Sprintf("%d", hook.ID), req.Name)
	writeJSON(w, http.StatusCreated, hook)
}

func (h *handlers) getGitHook(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	hook, err := h.d.Store.GetGitHook(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "hook not found")
		return
	}
	writeJSON(w, http.StatusOK, hook)
}

func (h *handlers) updateGitHook(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	existing, err := h.d.Store.GetGitHook(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "hook not found")
		return
	}
	var req createGitHookReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		req.Name = existing.Name
	}
	if req.Event == "" {
		req.Event = string(existing.Event)
	}
	enabled := existing.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if err := h.d.Store.UpdateGitHook(id, req.Name, store.GitHookEvent(req.Event), req.Branch, req.Secret, enabled, req.BuildParams, req.Description); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	hook, err := h.d.Store.GetGitHook(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(r, "update", "git_hook", fmt.Sprintf("%d", id), req.Name)
	writeJSON(w, http.StatusOK, hook)
}

func (h *handlers) deleteGitHook(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.d.Store.DeleteGitHook(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.audit(r, "delete", "git_hook", fmt.Sprintf("%d", id), "")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

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

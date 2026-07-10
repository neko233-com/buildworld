package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/neko233-com/buildworld233/internal/auth"
	"github.com/neko233-com/buildworld233/internal/store"
	"github.com/neko233-com/buildworld233/internal/webhook"
)

// handlers holds all dependencies needed by route handlers.
type handlers struct {
	d Deps
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
	writeJSON(w, http.StatusOK, map[string]string{"version": "1.0.0"})
}

// ---------------------------------------------------------------------------
// auth
// ---------------------------------------------------------------------------

type loginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResp struct {
	Token string       `json:"token"`
	User  *store.User  `json:"user"`
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

	token, err := h.d.JWT.Generate(user.ID, user.Role, 24*time.Hour)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	user.PasswordHash = ""
	writeJSON(w, http.StatusOK, loginResp{Token: token, User: user})
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
	Name          string `json:"name"`
	Description   string `json:"description"`
	RepoURL       string `json:"repo_url"`
	RepoType      string `json:"repo_type"`
	DefaultBranch string `json:"default_branch"`
	VCSRootID     *int64 `json:"vcs_root_id"`
	TemplateID    *int64 `json:"template_id"`
	Config        string `json:"config"`
}

func (h *handlers) listProjects(w http.ResponseWriter, _ *http.Request) {
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
	p, err := h.d.Store.CreateProject(req.Name, req.Description, req.RepoURL, req.RepoType,
		req.DefaultBranch, req.Config, userIDOf(r), req.VCSRootID, req.TemplateID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
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
	if err := h.d.Store.UpdateProject(id, req.Name, req.Description, req.RepoURL, req.RepoType,
		req.DefaultBranch, req.Config, req.VCSRootID, req.TemplateID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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
	writeJSON(w, http.StatusOK, builds)
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
	writeJSON(w, http.StatusOK, builds)
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

	var req triggerBuildReq
	_ = json.NewDecoder(r.Body).Decode(&req)

	branch := req.Branch
	if branch == "" {
		branch = project.DefaultBranch
	}
	paramsJSON := ""
	if len(req.Parameters) > 0 {
		if b, err := json.Marshal(req.Parameters); err == nil {
			paramsJSON = string(b)
		}
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

	// Kick off async build execution.
	if h.d.Runner != nil {
		h.d.Runner.Run(build.ID)
	}
	writeJSON(w, http.StatusCreated, build)
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
	writeJSON(w, http.StatusOK, build)
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

func (h *handlers) stopBuild(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	_ = h.d.Store.CancelBuild(id)
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

func (h *handlers) deleteAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.d.Store.DeleteWorker(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handlers) generateAgentToken(w http.ResponseWriter, _ *http.Request) {
	token := uuid.New().String()
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

// ---------------------------------------------------------------------------
// plugins
// ---------------------------------------------------------------------------

type installPluginReq struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Config      string `json:"config"`
}

func (h *handlers) listPlugins(w http.ResponseWriter, _ *http.Request) {
	// DB-installed plugins.
	plugins, err := h.d.Store.ListPlugins()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Merge with runtime-loaded plugins from the loader.
	if h.d.Loader != nil {
		for _, p := range h.d.Loader.List() {
			plugins = append(plugins, &store.Plugin{
				Name:        p.Name,
				Version:     p.Version,
				Description: p.Description,
				Enabled:     true,
			})
		}
	}
	writeJSON(w, http.StatusOK, plugins)
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
	p, err := h.d.Store.CreatePlugin(req.Name, req.Version, req.Description, req.Config)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Attempt to load from disk if loader is configured.
	if h.d.Loader != nil {
		_ = h.d.Loader.Load(req.Name)
	}
	writeJSON(w, http.StatusCreated, p)
}

func (h *handlers) deletePlugin(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.d.Store.DeletePlugin(id); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handlers) togglePlugin(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.d.Store.UpdatePluginEnabled(id, req.Enabled); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
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
	Scope       string `json:"scope"`       // global | project
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
			if h.d.Runner != nil {
				h.d.Runner.Run(build.ID)
			}
			return nil
		}
	}
	return nil
}

func (h *handlers) githubWebhook(w http.ResponseWriter, r *http.Request) {
	wh := webhook.NewWebhookHandler(h.d.Cfg.Auth.JWTSecret)
	wh.OnPush(h.triggerBuildByWebhook)
	wh.HandleGitHubWebhook(w, r)
}

func (h *handlers) gitlabWebhook(w http.ResponseWriter, r *http.Request) {
	wh := webhook.NewWebhookHandler(h.d.Cfg.Auth.JWTSecret)
	wh.OnPush(h.triggerBuildByWebhook)
	wh.HandleGitLabWebhook(w, r)
}

// giteaWebhook uses the same GitHub-compatible signature scheme (X-Gitea-Signature).
func (h *handlers) giteaWebhook(w http.ResponseWriter, r *http.Request) {
	wh := webhook.NewWebhookHandler(h.d.Cfg.Auth.JWTSecret)
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
	c.Password = "********"
	c.PrivateKey = "********"
	c.Token = "********"
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
	PollInterval int    `json:"poll_interval"`
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
	v, err := h.d.Store.CreateVCSRoot(req.Name, req.Type, req.URL, req.Branch, req.CredentialID, req.PollInterval, req.AutoCheckout, req.Config)
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
	if err := h.d.Store.UpdateVCSRoot(id, req.Name, req.Type, req.URL, req.Branch, req.CredentialID, req.PollInterval, req.AutoCheckout, req.Config); err != nil {
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
	if h.d.Runner != nil {
		h.d.Runner.Run(newBuild.ID)
	}
	writeJSON(w, http.StatusCreated, newBuild)
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
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	Config      string                 `json:"config"`
	Conditions  string                 `json:"conditions"`
	Description string                 `json:"description"`
	Enabled     bool                   `json:"enabled"`
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
	if err := h.d.Store.UpdateNotificationChannel(id, req.Name, ct, req.Config, req.Conditions, req.Description, req.Enabled); err != nil {
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


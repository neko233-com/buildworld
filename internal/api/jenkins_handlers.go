package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/neko233-com/buildworld/internal/jenkins"
)

type importJenkinsJobReq struct {
	URL     string `json:"url"`
	User    string `json:"user"`
	Token   string `json:"token"`
	Folder  string `json:"folder"`
	Job     string `json:"job"`
	Name    string `json:"name"`
	GroupID *int64 `json:"group_id"`
}

// importJenkinsJob imports a job from a live Jenkins instance. It fetches the
// job config.xml, normalizes it into BuildWorld's project model, and persists
// it exactly like createProject does.
func (h *handlers) importJenkinsJob(w http.ResponseWriter, r *http.Request) {
	var req importJenkinsJobReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.URL == "" || req.Job == "" {
		writeErr(w, http.StatusBadRequest, "url and job are required")
		return
	}
	folders := []string{}
	if req.Folder != "" {
		folders = strings.Split(req.Folder, "/")
	}

	client := jenkins.NewClient(req.URL, req.User, req.Token)
	xmlData, err := client.FetchConfig(r.Context(), folders, req.Job)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	def, err := jenkins.ParseConfig(xmlData, req.Job)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	jobName := req.Job
	if req.Name != "" {
		jobName = req.Name
	}
	project, err := h.d.Store.CreateProject(jobName, fmt.Sprintf("Imported from Jenkins %s", req.URL), def.RepoURL, "git", def.DefaultBranch, def.Script, userIDOf(r), nil, nil)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.d.Store.SetProjectPipelineSource(project.ID, def.Format, def.SourceMode, def.SCMRepo, def.SCMBranch, def.SCMPath); err != nil {
		writeErr(w, http.StatusInternalServerError, "persist pipeline source: "+err.Error())
		return
	}
	if req.GroupID != nil {
		if err := h.d.Store.SetProjectGroup(project.ID, req.GroupID); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	project, _ = h.d.Store.GetProject(project.ID)
	writeJSON(w, http.StatusCreated, project)
}

// registerJenkinsRoutes wires the live-Jenkins import endpoint. It is called
// from the router setup so the dependency on the jenkins package stays local.
func registerJenkinsRoutes(r chi.Router, h *handlers, editors func(http.Handler) http.Handler) {
	r.With(editors).Post("/migration/jenkins", h.importJenkinsJob)
}

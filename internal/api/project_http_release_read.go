package api

import (
	"crypto/subtle"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/neko233-com/buildworld/internal/store"
)

// Project trigger holders may observe their project's release completion, but
// cannot read logs, parameters, configuration, secrets or another project's builds.
type projectHTTPReleaseStatus struct {
	ID             int64                       `json:"id"`
	ProjectID      int64                       `json:"project_id"`
	Number         int                         `json:"number"`
	Status         string                      `json:"status"`
	StartedAt      *time.Time                  `json:"started_at,omitempty"`
	FinishedAt     *time.Time                  `json:"finished_at,omitempty"`
	DurationMs     *int64                      `json:"duration_ms,omitempty"`
	ReleaseResults []*store.BuildReleaseResult `json:"release_results,omitempty"`
}

func (h *handlers) projectHTTPReleaseReader(w http.ResponseWriter, r *http.Request) (*store.Project, bool) {
	w.Header().Set("Cache-Control", "no-store")
	id, err := parseIDInt64(r)
	if err != nil || id <= 0 {
		writeErr(w, http.StatusNotFound, "trigger not found")
		return nil, false
	}
	project, err := h.d.Store.GetProject(id)
	if err != nil || !project.HTTPTriggerEnabled || project.HTTPTriggerToken == "" ||
		subtle.ConstantTimeCompare([]byte(project.HTTPTriggerToken), []byte(chi.URLParam(r, "token"))) != 1 {
		writeErr(w, http.StatusNotFound, "trigger not found")
		return nil, false
	}
	if !project.Enabled {
		writeCodedErr(w, http.StatusConflict, "project_disabled", "project is disabled")
		return nil, false
	}
	return project, true
}

func (h *handlers) projectHTTPReleaseCapabilities(w http.ResponseWriter, r *http.Request) {
	project, ok := h.projectHTTPReleaseReader(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, struct {
		SchemaVersion int   `json:"schemaVersion"`
		ProjectID     int64 `json:"projectId"`
	}{SchemaVersion: 1, ProjectID: project.ID})
}

func (h *handlers) projectHTTPReleaseBuildStatus(w http.ResponseWriter, r *http.Request) {
	project, ok := h.projectHTTPReleaseReader(w, r)
	if !ok {
		return
	}
	buildID, err := strconv.ParseInt(chi.URLParam(r, "buildID"), 10, 64)
	if err != nil || buildID <= 0 {
		writeErr(w, http.StatusNotFound, "build not found")
		return
	}
	build, err := h.d.Store.GetProjectBuildReleaseStatus(project.ID, buildID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "build not found")
		return
	}
	if err := h.d.Store.AttachReleaseResults([]*store.Build{build}); err != nil {
		writeErr(w, http.StatusInternalServerError, "release status unavailable")
		return
	}
	writeJSON(w, http.StatusOK, projectHTTPReleaseStatus{
		ID: build.ID, ProjectID: build.ProjectID, Number: build.Number, Status: build.Status,
		StartedAt: build.StartedAt, FinishedAt: build.FinishedAt, DurationMs: build.DurationMs,
		ReleaseResults: build.ReleaseResults,
	})
}

func (h *handlers) projectHTTPReleaseRecords(w http.ResponseWriter, r *http.Request) {
	project, ok := h.projectHTTPReleaseReader(w, r)
	if !ok {
		return
	}
	records, err := h.d.Store.ListReleaseRecords(project.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "release records unavailable")
		return
	}
	writeJSON(w, http.StatusOK, records)
}

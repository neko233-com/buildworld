package api

import (
	"net/http"
	"strconv"
)

func (h *handlers) listRecentBuilds(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	builds, err := h.d.Store.ListRecentBuilds(limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, builds)
}

func (h *handlers) listProjectBuildOverviews(w http.ResponseWriter, _ *http.Request) {
	overviews, err := h.d.Store.ListProjectBuildOverviews()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, overviews)
}

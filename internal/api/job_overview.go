package api

import "net/http"

func (h *handlers) listProjectBuildOverviews(w http.ResponseWriter, _ *http.Request) {
	overviews, err := h.d.Store.ListProjectBuildOverviews()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, overviews)
}

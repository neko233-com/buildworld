package api

import (
	"net/http"
)

func (h *handlers) listProjectChanges(w http.ResponseWriter, r *http.Request) {
	projectID, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if _, err := h.d.Store.GetProject(projectID); err != nil {
		writeErr(w, http.StatusNotFound, "project not found")
		return
	}
	changes, err := h.d.Store.ListProjectChanges(projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, changes)
}

package api

import "net/http"

func (h *handlers) listReleaseTargets(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if _, err := h.d.Store.GetProject(id); err != nil {
		writeErr(w, http.StatusNotFound, "project not found")
		return
	}
	targets, err := h.d.Store.ListReleaseTargetOverviews(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, targets)
}

func (h *handlers) listReleaseRecords(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid id")
		return
	}
	if _, err := h.d.Store.GetProject(id); err != nil {
		writeErr(w, http.StatusNotFound, "project not found")
		return
	}
	records, err := h.d.Store.ListReleaseRecords(id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "release records unavailable")
		return
	}
	writeJSON(w, http.StatusOK, records)
}

package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/neko233-com/buildworld/internal/migration"
)

func (h *handlers) migratePipeline(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	var request migration.Request
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid migration request")
		return
	}
	if strings.TrimSpace(request.Source) == "" {
		writeErr(w, http.StatusBadRequest, "pipeline source is required")
		return
	}
	result, err := h.migrations.Convert(chi.URLParam(r, "format"), request)
	if err != nil {
		writeErr(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

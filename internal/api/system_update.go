package api

import (
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"strings"

	"github.com/neko233-com/buildworld/internal/buildinfo"
	"github.com/neko233-com/buildworld/internal/systemupdate"
)

func (h *handlers) getSystemUpdate(w http.ResponseWriter, _ *http.Request) {
	response := map[string]interface{}{
		"current_version":     buildinfo.Version,
		"platform":            runtime.GOOS + "/" + runtime.GOARCH,
		"supported":           h.d.Updater != nil,
		"auto_update_enabled": h.automaticUpdatesEnabled(),
		"max_bundle_bytes":    systemupdate.MaxBundleBytes,
	}
	if h.d.Updater == nil {
		response["operation"] = systemupdate.Status{
			Status:  "unsupported",
			Message: "trusted system update helper is unavailable",
		}
	} else {
		response["operation"] = h.d.Updater.Status()
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *handlers) applySystemUpdate(w http.ResponseWriter, r *http.Request) {
	if h.d.Updater == nil {
		writeErr(w, http.StatusNotImplemented, "system update is unavailable on this installation")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, systemupdate.MaxBundleBytes+(2<<20))
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid or oversized multipart update request")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	mode := strings.TrimSpace(r.FormValue("mode"))
	if mode == "" {
		mode = "manual"
	}
	if mode != "manual" && mode != "automatic" {
		writeErr(w, http.StatusBadRequest, "mode must be manual or automatic")
		return
	}
	automatic := mode == "automatic"
	if automatic && !h.automaticUpdatesEnabled() {
		writeCodedErr(w, http.StatusConflict, "automatic_updates_disabled", "automatic updates are disabled")
		return
	}
	bundle, _, err := r.FormFile("bundle")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bundle file is required")
		return
	}
	defer bundle.Close()

	status, err := h.d.Updater.Start(r.Context(), systemupdate.Request{
		Version:   r.FormValue("version"),
		SHA256:    r.FormValue("sha256"),
		Bundle:    bundle,
		Automatic: automatic,
	})
	if err != nil {
		switch {
		case errors.Is(err, systemupdate.ErrUpdateInProgress):
			writeCodedErr(w, http.StatusConflict, "update_in_progress", err.Error())
		case strings.Contains(err.Error(), "checksum"), strings.Contains(err.Error(), "bundle"), strings.Contains(err.Error(), "version"):
			writeErr(w, http.StatusBadRequest, err.Error())
		default:
			writeErr(w, http.StatusInternalServerError, "failed to stage system update")
		}
		return
	}
	h.audit(r, "update", "system", status.OperationID, fmt.Sprintf("mode=%s version=%s", mode, status.Version))
	writeJSON(w, http.StatusAccepted, status)
}

func (h *handlers) automaticUpdatesEnabled() bool {
	if h == nil {
		return false
	}
	enabled := h.d.Cfg != nil && h.d.Cfg.Updates.AutoUpdateEnabled
	if h.d.Store == nil {
		return enabled
	}
	values, err := h.d.Store.ListEnvVars("system", nil)
	if err != nil {
		return enabled
	}
	for _, value := range values {
		if value.Name == "auto_update_enabled" {
			parsed, parseErr := strconv.ParseBool(value.Value)
			return parseErr == nil && parsed
		}
	}
	return enabled
}

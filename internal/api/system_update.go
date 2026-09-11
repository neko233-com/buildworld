package api

import (
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strings"

	"github.com/neko233-com/buildworld/internal/buildinfo"
	"github.com/neko233-com/buildworld/internal/systemupdate"
)

func (h *handlers) getSystemUpdate(w http.ResponseWriter, _ *http.Request) {
	response := map[string]interface{}{
		"current_version":        buildinfo.Version,
		"platform":               runtime.GOOS + "/" + runtime.GOARCH,
		"supported":              h.d.Updater != nil,
		"manual_only":            true,
		"administrator_required": true,
		"max_bundle_bytes":       systemupdate.MaxBundleBytes,
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

func (h *handlers) checkSystemUpdate(w http.ResponseWriter, r *http.Request) {
	if h.d.UpdateCatalog == nil {
		writeErr(w, http.StatusNotImplemented, "system update checks are unavailable on this installation")
		return
	}
	release, err := h.d.UpdateCatalog.Check(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, fmt.Sprintf("failed to check official updates: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"current_version":        buildinfo.Version,
		"latest_version":         release.Version,
		"update_available":       systemupdate.IsNewerVersion(buildinfo.Version, release.Version),
		"platform":               runtime.GOOS + "/" + runtime.GOARCH,
		"asset_name":             release.AssetName,
		"asset_size":             release.AssetSize,
		"release_url":            release.ReleaseURL,
		"published_at":           release.PublishedAt,
		"manual_only":            true,
		"administrator_required": true,
	})
}

func (h *handlers) applyLatestSystemUpdate(w http.ResponseWriter, r *http.Request) {
	if h.d.Updater == nil {
		writeErr(w, http.StatusNotImplemented, "system update is unavailable on this installation")
		return
	}
	if h.d.UpdateCatalog == nil {
		writeErr(w, http.StatusNotImplemented, "system update checks are unavailable on this installation")
		return
	}
	release, err := h.d.UpdateCatalog.Check(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, fmt.Sprintf("failed to check official updates: %v", err))
		return
	}
	if !systemupdate.IsNewerVersion(buildinfo.Version, release.Version) {
		writeCodedErr(w, http.StatusConflict, "no_update_available", "no newer stable update is available")
		return
	}
	bundle, err := h.d.UpdateCatalog.Download(r.Context(), release)
	if err != nil {
		writeErr(w, http.StatusBadGateway, fmt.Sprintf("failed to download official update: %v", err))
		return
	}
	if bundle.Reader == nil || bundle.Version != release.Version || bundle.SHA256 == "" {
		if bundle.Reader != nil {
			_ = bundle.Reader.Close()
		}
		writeErr(w, http.StatusBadGateway, "official update bundle metadata is invalid")
		return
	}
	defer bundle.Reader.Close()
	status, err := h.d.Updater.Start(r.Context(), systemupdate.Request{
		Version: bundle.Version,
		SHA256:  bundle.SHA256,
		Bundle:  bundle.Reader,
	})
	if err != nil {
		writeUpdateStartError(w, err)
		return
	}
	h.audit(r, "update", "system", status.OperationID, fmt.Sprintf("mode=manual source=official version=%s", status.Version))
	writeJSON(w, http.StatusAccepted, status)
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
	if mode != "manual" {
		writeCodedErr(w, http.StatusForbidden, "automatic_updates_disabled", "automatic updates are disabled; use a manual administrator action")
		return
	}
	bundle, _, err := r.FormFile("bundle")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bundle file is required")
		return
	}
	defer bundle.Close()

	status, err := h.d.Updater.Start(r.Context(), systemupdate.Request{
		Version: r.FormValue("version"),
		SHA256:  r.FormValue("sha256"),
		Bundle:  bundle,
	})
	if err != nil {
		writeUpdateStartError(w, err)
		return
	}
	h.audit(r, "update", "system", status.OperationID, fmt.Sprintf("mode=manual version=%s", status.Version))
	writeJSON(w, http.StatusAccepted, status)
}

func writeUpdateStartError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, systemupdate.ErrUpdateInProgress):
		writeCodedErr(w, http.StatusConflict, "update_in_progress", err.Error())
	case strings.Contains(err.Error(), "checksum"), strings.Contains(err.Error(), "bundle"), strings.Contains(err.Error(), "version"):
		writeErr(w, http.StatusBadRequest, err.Error())
	default:
		writeErr(w, http.StatusInternalServerError, "failed to stage system update")
	}
}

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/neko233-com/buildworld/internal/portability"
)

const maxPortabilityBundleBytes = 32 << 20

func (h *handlers) portabilityCapabilities(w http.ResponseWriter, _ *http.Request) {
	if h.portability == nil {
		writeErr(w, http.StatusServiceUnavailable, "portability service unavailable")
		return
	}
	writeJSON(w, http.StatusOK, h.portability.Capabilities())
}

func (h *handlers) exportPortabilityBundle(w http.ResponseWriter, r *http.Request) {
	if h.portability == nil {
		writeErr(w, http.StatusServiceUnavailable, "portability service unavailable")
		return
	}
	var options portability.ExportOptions
	if err := decodeBoundedJSON(w, r, &options, 1<<20); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	bundle, err := h.portability.Export(r.Context(), options)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	filename := "buildworld-config-" + time.Now().UTC().Format("20060102-150405") + ".json"
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	h.audit(r, "export", "configuration_bundle", filename, strings.Join(options.Sections, ","))
	w.WriteHeader(http.StatusOK)
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(bundle)
}

func (h *handlers) inspectPortabilityBundle(w http.ResponseWriter, r *http.Request) {
	if h.portability == nil {
		writeErr(w, http.StatusServiceUnavailable, "portability service unavailable")
		return
	}
	var bundle portability.Bundle
	if err := decodeBoundedJSON(w, r, &bundle, maxPortabilityBundleBytes); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	inspection, err := h.portability.Inspect(&bundle)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, inspection)
}

func (h *handlers) importPortabilityBundle(w http.ResponseWriter, r *http.Request) {
	if h.portability == nil {
		writeErr(w, http.StatusServiceUnavailable, "portability service unavailable")
		return
	}
	var bundle portability.Bundle
	if err := decodeBoundedJSON(w, r, &bundle, maxPortabilityBundleBytes); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	options := portability.ImportOptions{Mode: r.URL.Query().Get("mode")}
	result, err := h.portability.Import(r.Context(), &bundle, options)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	h.audit(r, "import", "configuration_bundle", bundle.ExportedAt.Format(time.RFC3339), options.Mode)
	writeJSON(w, http.StatusOK, result)
}

func decodeBoundedJSON(w http.ResponseWriter, r *http.Request, target any, limit int64) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid JSON payload: %w", err)
	}
	return nil
}

package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/neko233-com/buildworld/internal/auth"
	"github.com/neko233-com/buildworld/internal/store"
	"github.com/neko233-com/buildworld/internal/workercontrol"
	"github.com/neko233-com/buildworld/internal/workerupdate"
)

func (h *handlers) workerUpdateRoot() string {
	return filepath.Join(h.d.Cfg.Storage.Artifacts, "worker-updates")
}

func (h *handlers) uploadWorkerUpdate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	worker, err := h.d.Store.GetWorker(id)
	if err != nil {
		writeErr(w, 404, "worker not found")
		return
	}
	raw, _, observed, err := h.d.Store.WorkerRuntime(id)
	var sample workercontrol.Runtime
	at, _ := time.Parse(time.RFC3339Nano, observed)
	if err != nil || json.Unmarshal(raw, &sample) != nil || sample.Version == "" || sample.OS == "" || worker.Status != "online" || time.Since(at) > 45*time.Second {
		writeErr(w, 409, "worker must report a current management-capable runtime before update")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, workerupdate.MaxBundleBytes+(2<<20))
	if err = r.ParseMultipartForm(4 << 20); err != nil {
		writeErr(w, 400, "invalid or oversized worker update upload")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	if r.FormValue("mode") != "manual" {
		writeErr(w, 400, "worker updates require mode=manual")
		return
	}
	version, checksum := r.FormValue("version"), r.FormValue("sha256")
	if !workerupdate.VersionPattern.MatchString(version) || !workerupdate.DigestPattern.MatchString(checksum) {
		writeErr(w, 400, "valid version and sha256 are required")
		return
	}
	input, _, err := r.FormFile("bundle")
	if err != nil {
		writeErr(w, 400, "official release bundle is required")
		return
	}
	defer input.Close()
	root := h.workerUpdateRoot()
	if err = os.MkdirAll(root, 0700); err != nil {
		writeErr(w, 500, "cannot create worker update storage")
		return
	}
	temporary, err := os.MkdirTemp(root, ".upload-")
	if err != nil {
		writeErr(w, 500, "cannot stage worker update")
		return
	}
	defer os.RemoveAll(temporary)
	archive := filepath.Join(temporary, "bundle")
	if err = workerupdate.StageArchive(input, archive, checksum); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	platform := sample.OS + "/" + sample.Arch
	if _, err = workerupdate.Extract(archive, filepath.Join(temporary, "validate"), version, platform); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	stored := filepath.Join(root, checksum+".bundle")
	if _, err = os.Stat(stored); os.IsNotExist(err) {
		if err = os.Rename(archive, stored); err != nil {
			writeErr(w, 500, "cannot preserve verified worker bundle")
			return
		}
	} else if err != nil {
		writeErr(w, 500, "cannot inspect worker bundle store")
		return
	}
	op, err := h.d.Store.QueueWorkerOperation(id, workercontrol.Operation{RequestID: r.FormValue("request_id"), Kind: "update", Version: version, SHA256: checksum, Platform: platform})
	if err != nil {
		status := 400
		if errors.Is(err, store.ErrWorkerOperationConflict) {
			status = 409
		}
		writeErr(w, status, err.Error())
		return
	}
	h.audit(r, "worker-update", "worker", id, "mode=manual operation="+op.ID+" version="+op.Version+" sha256="+checksum)
	writeJSON(w, 202, op)
}

func (h *handlers) downloadWorkerBundle(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	worker, err := h.d.Store.GetWorker(id)
	if err != nil || !auth.CheckPassword(worker.TokenHash, r.Header.Get("X-Buildworld-Worker-Token")) {
		writeErr(w, 401, "invalid worker token")
		return
	}
	op, err := h.d.Store.GetWorkerOperation(id, chi.URLParam(r, "operation"))
	if err != nil || op.Kind != "update" || !workerupdate.DigestPattern.MatchString(op.SHA256) || (op.Status != "pending" && op.Status != "running") {
		writeErr(w, 404, "active update bundle not found")
		return
	}
	file, err := os.Open(filepath.Join(h.workerUpdateRoot(), op.SHA256+".bundle"))
	if err != nil {
		writeErr(w, 404, "verified update bundle not found")
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		writeErr(w, 500, "cannot inspect update bundle")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, "worker.bundle", info.ModTime(), file)
}

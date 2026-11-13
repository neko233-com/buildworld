package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/neko233-com/buildworld/internal/auth"
	"github.com/neko233-com/buildworld/internal/store"
	"github.com/neko233-com/buildworld/internal/workercontrol"
)

func (h *handlers) workerRuntime(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	worker, err := h.d.Store.GetWorker(id)
	if err != nil {
		writeErr(w, 404, "worker not found")
		return
	}
	raw, draining, observed, err := h.d.Store.WorkerRuntime(id)
	if err != nil {
		writeErr(w, 500, "cannot read worker runtime")
		return
	}
	ops, err := h.d.Store.ListWorkerOperations(id)
	if err != nil {
		writeErr(w, 500, "cannot read worker operations")
		return
	}
	writeJSON(w, 200, map[string]any{"worker": worker, "runtime": raw, "draining": draining, "observed_at": observed, "operations": ops})
}

func (h *handlers) queueWorkerOperation(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var input workercontrol.Operation
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF || input.Kind == "update" {
		writeErr(w, 400, "invalid operation; updates require a verified bundle upload")
		return
	}
	if auth.IsAPITokenRequest(r.Context()) && !auth.HasAPITokenScope(r.Context(), auth.ScopeSystemUpdate) {
		writeErr(w, 403, "worker administration requires system:update token scope")
		return
	}
	if input.ID != "" || input.WorkerID != "" || input.Status != "" || input.Result != nil || input.Message != "" || input.CreatedAt != "" || input.UpdatedAt != "" {
		writeErr(w, 400, "operation receipt fields are read-only")
		return
	}
	op, err := h.d.Store.QueueWorkerOperation(chi.URLParam(r, "id"), input)
	if err != nil {
		status := 400
		if errors.Is(err, store.ErrWorkerOperationConflict) {
			status = 409
		}
		writeErr(w, status, err.Error())
		return
	}
	h.audit(r, "worker-operation", "worker", op.WorkerID, "kind="+op.Kind+" operation="+op.ID)
	writeJSON(w, 202, op)
}

func (h *handlers) workerHeartbeatReport(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	worker, err := h.d.Store.GetWorker(id)
	if err != nil || !auth.CheckPassword(worker.TokenHash, r.Header.Get("X-Buildworld-Worker-Token")) {
		writeErr(w, 401, "invalid worker token")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, workercontrol.MaxReportBytes+8192)
	var report workercontrol.Heartbeat
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&report); err != nil && !errors.Is(err, io.EOF) {
		writeErr(w, 400, "invalid heartbeat report")
		return
	}
	if err == nil && decoder.Decode(new(any)) != io.EOF {
		writeErr(w, 400, "invalid heartbeat report")
		return
	}
	op, err := h.d.Store.ReportWorkerRuntime(id, report)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err = h.d.Store.UpdateWorkerHeartbeat(id, 0); err != nil {
		writeErr(w, 500, "cannot record heartbeat")
		return
	}
	writeJSON(w, 200, workercontrol.Reply{Status: "ok", Operation: op})
}

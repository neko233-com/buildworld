package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/neko233-com/buildworld/internal/store"
)

// lookupIdempotentBuild 只查询原请求回执；未找到时不创建或重新提交构建。
func (h *handlers) lookupIdempotentBuild(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDInt64(r)
	if err != nil || userIDOf(r) <= 0 {
		writeErr(w, http.StatusBadRequest, "invalid build identity")
		return
	}
	req, err := decodeTriggerBuildRequest(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	key, fingerprint, err := buildIdempotencyIdentity(id, r.Header.Get("Idempotency-Key"), req)
	if err != nil || key == "" {
		writeErr(w, http.StatusBadRequest, "invalid idempotency key")
		return
	}
	build, err := h.d.Store.FindIdempotentBuild(key, fingerprint, userIDOf(r))
	if errors.Is(err, store.ErrBuildIdempotencyConflict) {
		writeErr(w, http.StatusConflict, "idempotency request conflicts")
		return
	}
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, "build receipt unavailable")
		return
	}
	if build == nil {
		writeErr(w, http.StatusNotFound, "build receipt not found")
		return
	}
	if build.ProjectID != id {
		writeErr(w, http.StatusConflict, "build project conflicts")
		return
	}
	w.Header().Set("Idempotency-Lookup-Supported", "v1")
	writeJSON(w, http.StatusOK, h.publicBuild(build))
}

// buildIdempotencyIdentity binds a caller-provided key to one project and the
// original JSON request. Neither the key nor raw parameters enter audit logs.
func buildIdempotencyIdentity(projectID int64, key string, req triggerBuildReq) (string, string, error) {
	if key == "" {
		return "", "", nil
	}
	if key != strings.TrimSpace(key) || len(key) < 16 || len(key) > 128 {
		return "", "", errors.New("invalid idempotency key")
	}
	for _, ch := range key {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-' || ch == '_' || ch == '.') {
			return "", "", errors.New("invalid idempotency key")
		}
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return "", "", errors.New("invalid build request")
	}
	keyHash := sha256.Sum256([]byte(fmt.Sprintf("%d:%s", projectID, key)))
	requestHash := sha256.Sum256(payload)
	return hex.EncodeToString(keyHash[:]), hex.EncodeToString(requestHash[:]), nil
}

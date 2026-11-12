package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

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

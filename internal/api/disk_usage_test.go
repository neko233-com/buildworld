package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neko233-com/buildworld/internal/config"
)

func TestSystemStorageReportsBuiltinBuildCacheVolume(t *testing.T) {
	handler := &handlers{d: Deps{Cfg: &config.Config{
		Storage: config.StorageConfig{BuildTemp: t.TempDir()},
	}}}
	response := httptest.NewRecorder()

	handler.systemStorage(response, httptest.NewRequest(http.MethodGet, "/api/system/storage", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Executor    string  `json:"executor"`
		Volume      string  `json:"volume"`
		TotalBytes  uint64  `json:"total_bytes"`
		UsedBytes   uint64  `json:"used_bytes"`
		FreeBytes   uint64  `json:"free_bytes"`
		UsedPercent float64 `json:"used_percent"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Executor != "builtin" || payload.Volume == "" {
		t.Fatalf("storage identity = %q %q, want builtin volume", payload.Executor, payload.Volume)
	}
	if payload.TotalBytes == 0 || payload.UsedBytes+payload.FreeBytes != payload.TotalBytes {
		t.Fatalf("invalid byte totals: %+v", payload)
	}
	if payload.UsedPercent < 0 || payload.UsedPercent > 100 {
		t.Fatalf("used_percent = %f, want 0..100", payload.UsedPercent)
	}
}

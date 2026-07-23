package api

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/neko233-com/buildworld/internal/config"
	"github.com/neko233-com/buildworld/internal/store"
	"github.com/neko233-com/buildworld/internal/systemupdate"
)

type fakeUpdateService struct {
	request systemupdate.Request
	status  systemupdate.Status
}

func (f *fakeUpdateService) Status() systemupdate.Status { return f.status }

func (f *fakeUpdateService) Start(_ context.Context, request systemupdate.Request) (systemupdate.Status, error) {
	f.request = request
	return systemupdate.Status{Status: "accepted", OperationID: "operation-1", Version: request.Version}, nil
}

func TestSystemUpdateDefaultsAutomaticModeOffAndAllowsManualBundle(t *testing.T) {
	database, err := store.New(filepath.Join(t.TempDir(), "updates.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	updater := &fakeUpdateService{status: systemupdate.Status{Status: "idle", Version: "1.0.0"}}
	handler := &handlers{d: Deps{
		Cfg:     &config.Config{},
		Store:   database,
		Updater: updater,
	}}

	statusResponse := httptest.NewRecorder()
	handler.getSystemUpdate(statusResponse, httptest.NewRequest(http.MethodGet, "/api/system/update/", nil))
	var status map[string]interface{}
	if err := json.NewDecoder(statusResponse.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if status["auto_update_enabled"] != false || status["supported"] != true {
		t.Fatalf("system update status = %#v", status)
	}

	automaticBody, automaticType := updateMultipart(t, "automatic")
	automaticResponse := httptest.NewRecorder()
	automaticRequest := httptest.NewRequest(http.MethodPost, "/api/system/update/", automaticBody)
	automaticRequest.Header.Set("Content-Type", automaticType)
	handler.applySystemUpdate(automaticResponse, automaticRequest)
	if automaticResponse.Code != http.StatusConflict || !bytes.Contains(automaticResponse.Body.Bytes(), []byte("automatic_updates_disabled")) {
		t.Fatalf("automatic response = %d %s", automaticResponse.Code, automaticResponse.Body.String())
	}

	manualBody, manualType := updateMultipart(t, "manual")
	manualResponse := httptest.NewRecorder()
	manualRequest := httptest.NewRequest(http.MethodPost, "/api/system/update/", manualBody)
	manualRequest.Header.Set("Content-Type", manualType)
	handler.applySystemUpdate(manualResponse, manualRequest)
	if manualResponse.Code != http.StatusAccepted {
		t.Fatalf("manual response = %d %s", manualResponse.Code, manualResponse.Body.String())
	}
	if updater.request.Automatic || updater.request.Version != "1.0.1" || updater.request.SHA256 == "" {
		t.Fatalf("update request = %#v", updater.request)
	}
}

func updateMultipart(t *testing.T, mode string) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range map[string]string{
		"mode": mode, "version": "1.0.1", "sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	} {
		if err := writer.WriteField(name, value); err != nil {
			t.Fatal(err)
		}
	}
	part, err := writer.CreateFormFile("bundle", "buildworld.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("bundle")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return &body, writer.FormDataContentType()
}

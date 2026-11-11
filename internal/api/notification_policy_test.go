package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/neko233-com/buildworld/internal/auth"
	"github.com/neko233-com/buildworld/internal/config"
	"github.com/neko233-com/buildworld/internal/engine"
	"github.com/neko233-com/buildworld/internal/store"
)

func TestAdminBuildNotificationPolicy(t *testing.T) {
	database, err := store.New(filepath.Join(t.TempDir(), "buildworld.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	admin, err := database.CreateUser("policy-admin", "admin@example.test", "unused", "admin")
	if err != nil {
		t.Fatal(err)
	}
	developer, err := database.CreateUser("policy-dev", "dev@example.test", "unused", "developer")
	if err != nil {
		t.Fatal(err)
	}
	project, err := database.CreateProject("policy", "", "", "git", "main", `pipeline { agent any stages { stage('test') { steps { echo 'test' } } } }`, admin.ID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	jwt := auth.NewJWT("notification-policy-test")
	adminToken, err := jwt.Generate(admin.ID, admin.Role, admin.SessionVersion, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	devToken, err := jwt.Generate(developer.ID, developer.Role, developer.SessionVersion, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(Deps{Cfg: &config.Config{}, Store: database, JWT: jwt})
	path := fmt.Sprintf("/api/projects/%d/builds", project.ID)
	body := `{"notification_mode":"in_app_only","notification_reason":"authorized_release"}`
	for _, tc := range []struct {
		token, body string
		status      int
	}{
		{devToken, body, http.StatusForbidden},
		{adminToken, `{"notification_mode":"in_app_only"}`, http.StatusBadRequest},
		{adminToken, `{"notification_mode":"unknown","notification_reason":"testing"}`, http.StatusBadRequest},
		{adminToken, body, http.StatusCreated},
	} {
		response := apiTokenRequest(router, http.MethodPost, path, tc.token, tc.body)
		if response.Code != tc.status {
			t.Fatalf("status=%d wanted=%d body=%s", response.Code, tc.status, response.Body.String())
		}
		if tc.status == http.StatusCreated {
			var public store.Build
			if err := json.Unmarshal(response.Body.Bytes(), &public); err != nil {
				t.Fatal(err)
			}
			if public.NotificationMode != "in_app_only" || strings.Contains(public.Parameters, "notificationMode") {
				t.Fatalf("public build=%+v", public)
			}
			stored, err := database.GetBuild(public.ID)
			if err != nil || !engine.BuildNotificationsInAppOnly(stored.Parameters) {
				t.Fatalf("policy not persisted: %v", err)
			}
			retried, err := engine.AttachJenkinsRetryContext(stored.Parameters, "manual", "policy-admin")
			if err != nil || !engine.BuildNotificationsInAppOnly(retried) {
				t.Fatalf("retry lost policy: %v", err)
			}
		}
	}
	logs, err := database.ListAuditLogs(20, 0)
	if err != nil {
		t.Fatal(err)
	}
	matched := false
	for _, log := range logs {
		if log.Action == "set_notification_policy" && strings.Contains(log.Detail, "authorized_release") {
			matched = true
		}
	}
	if !matched {
		t.Fatal("policy audit missing")
	}
	rawToken := "bw_99999999999999999999999999999999"
	persistAPIToken(t, database, admin.ID, rawToken, `["build:trigger"]`)
	response := apiTokenRequest(router, http.MethodPost, "/api/trigger/"+project.Name, rawToken, body)
	if response.Code != http.StatusForbidden {
		t.Fatalf("HTTP trigger policy status=%d body=%s", response.Code, response.Body.String())
	}
}

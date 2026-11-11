package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/neko233-com/buildworld/internal/store"
)

func TestInAppOnlyPolicySurvivesRunnerMasking(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	database, err := store.New(filepath.Join(t.TempDir(), "runner.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	config, _ := json.Marshal(map[string]string{"url": server.URL})
	_, err = database.CreateNotificationChannel("external", store.NotificationChannelWebhook, string(config), "{}", "", true)
	if err != nil {
		t.Fatal(err)
	}
	// This secret matches part of the reserved metadata key, so masking the
	// serialized parameters before reading the policy would lose the decision.
	if err := database.SetEnvVar("global", nil, "POLICY_SECRET", "buildworld", true, ""); err != nil {
		t.Fatal(err)
	}
	pipeline := "jobs:\n  test:\n    steps:\n      - type: notify\n"
	project, err := database.CreateProject("runner-policy", "", "", "git", "main", pipeline, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	params, err := AttachInAppOnlyNotifications(`{}`)
	if err != nil {
		t.Fatal(err)
	}
	build, err := database.CreateBuild(project.ID, 1, "manual", "main", "", params, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	runner := NewBuildRunner(database, nil, filepath.Join(t.TempDir(), "workspaces"), nil)
	defer runner.Shutdown(context.Background())
	runner.SetNotificationService(NewNotificationService(database))
	runner.Run(build.ID)
	waitForBuildStatus(t, database, build.ID, 8*time.Second, "success")
	waitForBuildNotificationEvents(t, database, build.ID, 3)
	if requests.Load() != 0 {
		t.Fatalf("runner sent %d external notifications", requests.Load())
	}
}

func TestInAppOnlyNotificationPolicy(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	database, err := store.New(filepath.Join(t.TempDir(), "notifications.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	config, _ := json.Marshal(map[string]string{"url": server.URL})
	_, err = database.CreateNotificationChannel("external", store.NotificationChannelWebhook, string(config), "{}", "", true)
	if err != nil {
		t.Fatal(err)
	}
	project, err := database.CreateProject("policy", "", "", "git", "main", testEmptyPipelineSource, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	params, err := AttachInAppOnlyNotifications(`{"large":9007199254740993}`)
	if err != nil {
		t.Fatal(err)
	}
	if !BuildNotificationsInAppOnly(params) {
		t.Fatal("policy missing")
	}
	if _, err = AttachInAppOnlyNotifications(params); err == nil {
		t.Fatal("duplicate policy accepted")
	}
	var fields map[string]interface{}
	if err := json.Unmarshal([]byte(params), &fields); err != nil {
		t.Fatal(err)
	}
	PublicBuildParameters(fields)
	if len(fields) != 1 {
		t.Fatalf("metadata exposed: %v", fields)
	}
	if !BuildNotificationsInAppOnly("{invalid") || BuildNotificationsInAppOnly(`{}`) {
		t.Fatal("fail-closed policy mismatch")
	}
	build, err := database.CreateBuild(project.ID, 1, "manual", "main", "", params, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build.Status = "success"
	service := NewNotificationService(database)
	if err := service.SendBuildEvent(build, project, "build.completed"); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 0 {
		t.Fatal("quiet build sent external notification")
	}
	channels, err := database.ListNotificationChannels()
	if err != nil {
		t.Fatal(err)
	}
	for _, channel := range channels {
		events, err := database.ListNotificationEvents(channel.ID, 10)
		if err != nil {
			t.Fatal(err)
		}
		if channel.Type == store.NotificationChannelWeb && (len(events) != 1 || events[0].Status != "delivered") {
			t.Fatalf("web events: %+v", events)
		}
		if channel.Type == store.NotificationChannelWebhook && len(events) != 0 {
			t.Fatalf("external events: %+v", events)
		}
	}
	// A masked or broken copy must not determine delivery; the original policy does.
	masked := *build
	masked.Parameters = `{}`
	if err := service.sendBuildEvent(&masked, project, "pipeline.notification", BuildNotificationsInAppOnly(build.Parameters)); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 0 {
		t.Fatal("masking changed delivery policy")
	}
	build.Parameters = `{}`
	if err := service.SendBuildEvent(build, project, "build.completed"); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 {
		t.Fatalf("normal delivery count = %d", requests.Load())
	}
}

func TestNotificationServiceDeliversAllMatchingChannels(t *testing.T) {
	dataStore, err := store.New(filepath.Join(t.TempDir(), "notifications.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dataStore.Close() })

	channels, err := dataStore.ListNotificationChannels()
	if err != nil || len(channels) != 1 {
		t.Fatalf("default channels = %#v, err=%v", channels, err)
	}
	secondary, err := dataStore.CreateNotificationChannel(
		"secondary web",
		store.NotificationChannelWeb,
		"{}",
		"{}",
		"parallel delivery test",
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	project, err := dataStore.CreateProject("parallel-project", "", "", "git", "main", testEmptyPipelineSource, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build, err := dataStore.CreateBuild(project.ID, 1, "manual", "main", "", "{}", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build.Status = "running"

	service := NewNotificationService(dataStore)
	if err := service.SendBuildEvent(build, project, "build.started"); err != nil {
		t.Fatal(err)
	}

	for _, channelID := range []int64{channels[0].ID, secondary.ID} {
		events, listErr := dataStore.ListNotificationEvents(channelID, 10)
		if listErr != nil || len(events) != 1 {
			t.Fatalf("channel %d events = %#v, err=%v", channelID, events, listErr)
		}
		if events[0].Status != "delivered" || events[0].EventType != "build.started" {
			t.Fatalf("channel %d event = %#v", channelID, events[0])
		}
	}
}

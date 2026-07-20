package engine

import (
	"path/filepath"
	"testing"

	"github.com/neko233-com/buildworld/internal/store"
)

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
	project, err := dataStore.CreateProject("parallel-project", "", "", "git", "main", "{}", 0, nil, nil)
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

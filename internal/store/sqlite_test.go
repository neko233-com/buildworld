package store

import (
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func newTestStore(t *testing.T) (*Store, func()) {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	name := tmpFile.Name()
	tmpFile.Close()

	store, err := New(name)
	if err != nil {
		os.Remove(name)
		t.Fatalf("New() error = %v", err)
	}
	return store, func() {
		store.Close()
		os.Remove(name)
	}
}

func TestSQLiteStore(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	store, err := New(tmpFile.Name())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer store.Close()

	user, err := store.CreateUser("testuser", "test@example.com", "hashedpassword", "admin")
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	if user.Username != "testuser" {
		t.Errorf("Username = %s, want testuser", user.Username)
	}

	fetched, err := store.GetUserByUsername("testuser")
	if err != nil {
		t.Fatalf("GetUserByUsername() error = %v", err)
	}

	if fetched.ID != user.ID {
		t.Errorf("ID = %d, want %d", fetched.ID, user.ID)
	}
}

func TestSQLiteStoreUsesSingleConnectionRollbackJournal(t *testing.T) {
	data, cleanup := newTestStore(t)
	defer cleanup()

	stats := data.db.Stats()
	if stats.MaxOpenConnections != 1 {
		t.Fatalf("MaxOpenConnections = %d, want 1", stats.MaxOpenConnections)
	}
	var journalMode string
	if err := data.db.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(journalMode, "delete") {
		t.Fatalf("journal_mode = %q, want delete", journalMode)
	}
}

func TestSearchBuildsFiltersPaginatesAndOmitsLogs(t *testing.T) {
	data, cleanup := newTestStore(t)
	defer cleanup()

	alpha, err := data.CreateProject("alpha-service", "", "", "git", "main", `{}`, 0, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	beta, err := data.CreateProject("beta-release", "", "", "git", "release", `{}`, 0, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	first, err := data.CreateBuild(alpha.ID, 1, "manual", "main", "abc123", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := data.CreateBuild(beta.ID, 1, "webhook", "release", "def456", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	third, err := data.CreateBuild(beta.ID, 2, "manual", "release", "fed987", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := data.UpdateBuildStatus(first.ID, "success"); err != nil {
		t.Fatal(err)
	}
	if err := data.UpdateBuildStatus(second.ID, "failed"); err != nil {
		t.Fatal(err)
	}
	if err := data.UpdateBuildStatus(third.ID, "success"); err != nil {
		t.Fatal(err)
	}
	if err := data.SetBuildLog(second.ID, strings.Repeat("large log\n", 100)); err != nil {
		t.Fatal(err)
	}
	if err := data.PinBuild(second.ID, true); err != nil {
		t.Fatal(err)
	}

	failed := "failed"
	pinned := true
	result, err := data.SearchBuilds(BuildSearch{
		Query: "beta", ProjectID: &beta.ID, Statuses: []string{failed},
		Trigger: "webhook", Branch: "release", Pinned: &pinned, Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Version != BuildSearchResultVersion || result.Total != 1 || len(result.Items) != 1 {
		t.Fatalf("filtered search = %#v", result)
	}
	if result.Items[0].ID != second.ID || result.Items[0].Log != "" {
		t.Fatalf("lightweight item = %#v", result.Items[0])
	}

	page, err := data.SearchBuilds(BuildSearch{ProjectID: &beta.ID, Limit: 1, Offset: 1})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Items) != 1 || page.Items[0].ID != second.ID {
		t.Fatalf("second page = %#v", page)
	}
}

func TestListProjectSummariesOmitsLargePipelineDefinitions(t *testing.T) {
	data, cleanup := newTestStore(t)
	defer cleanup()

	largeConfig := `{"stages":[],"metadata":{"padding":"` + strings.Repeat("x", 512*1024) + `"}}`
	project, err := data.CreateProject(
		"large-config-project",
		"loaded only when needed",
		"https://example.invalid/large.git",
		"git",
		"main",
		largeConfig,
		0,
		nil,
		nil,
		[]string{"qa", "large"},
	)
	if err != nil {
		t.Fatal(err)
	}

	summaries, err := data.ListProjectSummaries()
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 {
		t.Fatalf("summaries = %d, want 1", len(summaries))
	}
	summary := summaries[0]
	if summary.ID != project.ID || summary.Name != project.Name || summary.Description != project.Description {
		t.Fatalf("summary = %#v, project = %#v", summary, project)
	}
	if len(summary.Tags) != 2 || summary.Tags[0] != "qa" || summary.Tags[1] != "large" {
		t.Fatalf("summary tags = %#v", summary.Tags)
	}

	full, err := data.ListProjects()
	if err != nil {
		t.Fatal(err)
	}
	if len(full) != 1 || full[0].Config != largeConfig {
		t.Fatalf("full project definition was not preserved")
	}
}

func TestDefaultWebNotificationChannelAndReadCursor(t *testing.T) {
	dataStore, cleanup := newTestStore(t)
	defer cleanup()

	channels, err := dataStore.ListNotificationChannels()
	if err != nil {
		t.Fatal(err)
	}
	if len(channels) != 1 || channels[0].Name != DefaultWebNotificationChannelName || channels[0].Type != NotificationChannelWeb || !channels[0].Enabled || channels[0].Conditions != "{}" {
		t.Fatalf("default channels = %#v", channels)
	}
	if err := dataStore.EnsureDefaultWebNotificationChannel(); err != nil {
		t.Fatal(err)
	}
	channels, err = dataStore.ListNotificationChannels()
	if err != nil || len(channels) != 1 {
		t.Fatalf("idempotent default channels = %#v, err=%v", channels, err)
	}
	if err := dataStore.DeleteNotificationChannel(channels[0].ID); !errors.Is(err, ErrRequiredNotificationChannel) {
		t.Fatalf("DeleteNotificationChannel(default) error = %v", err)
	}
	if err := dataStore.UpdateNotificationChannel(channels[0].ID, channels[0].Name, channels[0].Type, channels[0].Config, channels[0].Conditions, channels[0].Description, false); !errors.Is(err, ErrRequiredNotificationChannel) {
		t.Fatalf("UpdateNotificationChannel(default disabled) error = %v", err)
	}
	stillEnabled, err := dataStore.GetNotificationChannel(channels[0].ID)
	if err != nil || !stillEnabled.Enabled {
		t.Fatalf("required channel after rejected update = %#v, err=%v", stillEnabled, err)
	}
	if _, err := dataStore.db.Exec("UPDATE notification_channels SET enabled = 0 WHERE id = ?", channels[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := dataStore.EnsureDefaultWebNotificationChannel(); err != nil {
		t.Fatal(err)
	}
	repaired, err := dataStore.GetNotificationChannel(channels[0].ID)
	if err != nil || !repaired.Enabled {
		t.Fatalf("required channel was not repaired = %#v, err=%v", repaired, err)
	}

	user, err := dataStore.CreateUser("notification-reader", "notification-reader@example.test", "unused", "viewer")
	if err != nil {
		t.Fatal(err)
	}
	project, err := dataStore.CreateProject("notification-project", "", "", "git", "main", "{}", user.ID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build, err := dataStore.CreateBuild(project.ID, 1, "manual", "main", "", "{}", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	event, err := dataStore.CreateNotificationEvent(channels[0].ID, &build.ID, "build.started", `{"status":"running"}`)
	if err != nil {
		t.Fatal(err)
	}
	deliveredAt := time.Now()
	if err := dataStore.UpdateNotificationEventStatus(event.ID, "delivered", nil, &deliveredAt); err != nil {
		t.Fatal(err)
	}

	feed, err := dataStore.GetInAppNotificationFeed(user.ID, 30)
	if err != nil || feed.Unread != 1 || len(feed.Items) != 1 || feed.Items[0].ID != event.ID {
		t.Fatalf("initial feed = %#v, err=%v", feed, err)
	}
	if err := dataStore.MarkInAppNotificationsRead(user.ID, event.ID); err != nil {
		t.Fatal(err)
	}
	feed, err = dataStore.GetInAppNotificationFeed(user.ID, 30)
	if err != nil || feed.Unread != 0 || feed.LastReadID != event.ID {
		t.Fatalf("read feed = %#v, err=%v", feed, err)
	}
}

func TestGetDashboardStatsUsesLiveBuildsAndIgnoresStaleMaterializedRows(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	first, err := store.CreateProject("first", "", "https://example.com/first.git", "git", "main", "{}", 0, nil, nil)
	if err != nil {
		t.Fatalf("CreateProject(first) error = %v", err)
	}
	second, err := store.CreateProject("second", "", "https://example.com/second.git", "git", "main", "{}", 0, nil, nil)
	if err != nil {
		t.Fatalf("CreateProject(second) error = %v", err)
	}
	firstBuild, err := store.CreateBuild(first.ID, 1, "manual", "main", "", "{}", nil, nil)
	if err != nil {
		t.Fatalf("CreateBuild(first) error = %v", err)
	}
	if err := store.StartBuild(firstBuild.ID); err != nil {
		t.Fatalf("StartBuild(first) error = %v", err)
	}
	if err := store.FinishBuild(firstBuild.ID, "success", 1200); err != nil {
		t.Fatalf("FinishBuild(first) error = %v", err)
	}
	secondBuild, err := store.CreateBuild(second.ID, 1, "manual", "main", "", "{}", nil, nil)
	if err != nil {
		t.Fatalf("CreateBuild(second) error = %v", err)
	}
	if err := store.StartBuild(secondBuild.ID); err != nil {
		t.Fatalf("StartBuild(second) error = %v", err)
	}
	if err := store.FinishBuild(secondBuild.ID, "failed", 1800); err != nil {
		t.Fatalf("FinishBuild(second) error = %v", err)
	}

	today := time.Now().Format("2006-01-02")
	if _, err := store.CreateBuildStat(first.ID, today, 40, 30, 10, 9000); err != nil {
		t.Fatalf("CreateBuildStat(stale first) error = %v", err)
	}
	if _, err := store.CreateBuildStat(second.ID, today, 60, 40, 20, 11000); err != nil {
		t.Fatalf("CreateBuildStat(stale second) error = %v", err)
	}

	stats, err := store.GetDashboardStats()
	if err != nil {
		t.Fatalf("GetDashboardStats() error = %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("len(stats) = %d, want 1", len(stats))
	}
	got := stats[0]
	if got.Date != today || got.TotalBuilds != 2 || got.SuccessCount != 1 || got.FailedCount != 1 || got.AvgDuration != 1500 {
		t.Fatalf("GetDashboardStats() = %+v, want statistics rebuilt from live builds", got)
	}
}

func TestCreateVCSRootAllowsDisabledPolling(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	root, err := store.CreateVCSRoot("manual", "git", "https://example.com/manual.git", "main", nil, 0, true, "{}")
	if err != nil {
		t.Fatalf("CreateVCSRoot() error = %v", err)
	}
	if root.PollInterval != 0 {
		t.Fatalf("PollInterval = %d, want 0 (disabled)", root.PollInterval)
	}
}

func TestDeleteAPITokenForUserEnforcesOwnership(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	owner, err := store.CreateUser("token-owner", "owner@example.test", "hash", "developer")
	if err != nil {
		t.Fatalf("CreateUser(owner) error = %v", err)
	}
	other, err := store.CreateUser("token-other", "other@example.test", "hash", "developer")
	if err != nil {
		t.Fatalf("CreateUser(other) error = %v", err)
	}
	token, err := store.CreateAPIToken(owner.ID, "automation", "hash", "bw_12345678", `["read"]`, nil)
	if err != nil {
		t.Fatalf("CreateAPIToken() error = %v", err)
	}

	deleted, err := store.DeleteAPITokenForUser(token.ID, other.ID)
	if err != nil {
		t.Fatalf("DeleteAPITokenForUser(other) error = %v", err)
	}
	if deleted {
		t.Fatal("foreign user unexpectedly deleted the token")
	}
	if _, err := store.GetAPIToken(token.ID); err != nil {
		t.Fatalf("token was removed by foreign user: %v", err)
	}

	deleted, err = store.DeleteAPITokenForUser(token.ID, owner.ID)
	if err != nil {
		t.Fatalf("DeleteAPITokenForUser(owner) error = %v", err)
	}
	if !deleted {
		t.Fatal("owner delete returned false")
	}
}

func TestCreateMultipleUsers(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	users := []struct {
		username string
		email    string
		role     string
	}{
		{"alice", "alice@example.com", "admin"},
		{"bob", "bob@example.com", "developer"},
		{"charlie", "charlie@example.com", "viewer"},
	}

	var created []*User
	for _, u := range users {
		user, err := store.CreateUser(u.username, u.email, "hash-"+u.username, u.role)
		if err != nil {
			t.Fatalf("CreateUser(%q) error = %v", u.username, err)
		}
		created = append(created, user)
	}

	if len(created) != 3 {
		t.Fatalf("created %d users, want 3", len(created))
	}

	for i, u := range users {
		if created[i].Username != u.username {
			t.Errorf("user[%d].Username = %q, want %q", i, created[i].Username, u.username)
		}
		if created[i].Role != u.role {
			t.Errorf("user[%d].Role = %q, want %q", i, created[i].Role, u.role)
		}
	}

	for i, u := range users {
		fetched, err := store.GetUserByUsername(u.username)
		if err != nil {
			t.Fatalf("GetUserByUsername(%q) error = %v", u.username, err)
		}
		if fetched.ID != created[i].ID {
			t.Errorf("user[%d] ID mismatch: got %d, want %d", i, fetched.ID, created[i].ID)
		}
	}
}

func TestWorkerCapacityReservations(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()
	if _, err := store.CreateWorker("capacity-worker", "capacity", "127.0.0.1:6051", "token", []string{"linux"}, 1, ""); err != nil {
		t.Fatal(err)
	}
	acquired, err := store.TryAcquireWorker("capacity-worker")
	if err != nil || !acquired {
		t.Fatalf("first acquire = %v, %v", acquired, err)
	}
	acquired, err = store.TryAcquireWorker("capacity-worker")
	if err != nil || acquired {
		t.Fatalf("second acquire = %v, %v", acquired, err)
	}
	worker, err := store.GetWorker("capacity-worker")
	if err != nil || worker.ActiveBuilds != 1 {
		t.Fatalf("worker after acquire = %#v, %v", worker, err)
	}
	if err := store.ReleaseWorker("capacity-worker"); err != nil {
		t.Fatal(err)
	}
	worker, err = store.GetWorker("capacity-worker")
	if err != nil || worker.ActiveBuilds != 0 {
		t.Fatalf("worker after release = %#v, %v", worker, err)
	}
}

func TestUserNotFound(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	_, err := store.GetUser(9999)
	if err == nil {
		t.Fatal("GetUser(9999) expected error, got nil")
	}

	_, err = store.GetUserByUsername("nonexistent")
	if err == nil {
		t.Fatal("GetUserByUsername(\"nonexistent\") expected error, got nil")
	}
}

func TestDuplicateUsername(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	_, err := store.CreateUser("dupuser", "first@example.com", "hash1", "admin")
	if err != nil {
		t.Fatalf("first CreateUser() error = %v", err)
	}

	_, err = store.CreateUser("dupuser", "second@example.com", "hash2", "viewer")
	if err == nil {
		t.Fatal("CreateUser() with duplicate username expected error, got nil")
	}
}

func TestProjectTagsPersistAndNormalize(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	project, err := store.CreateProject("tagged", "", "", "git", "main", `{}`, 0, nil, nil, []string{" Web ", "web", "Production", ""})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(project.Tags), 2; got != want || project.Tags[0] != "Web" || project.Tags[1] != "Production" {
		t.Fatalf("created tags = %#v, want [Web Production]", project.Tags)
	}
	if err := store.UpdateProject(project.ID, project.Name, "", "", "git", "main", `{}`, nil, nil, []string{"Desktop", "desktop"}); err != nil {
		t.Fatal(err)
	}
	fetched, err := store.GetProject(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(fetched.Tags), 1; got != want || fetched.Tags[0] != "Desktop" {
		t.Fatalf("updated tags = %#v, want [Desktop]", fetched.Tags)
	}
}

func TestBuildQueueTracksActiveBuildsAndPriority(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	project, err := store.CreateProject("queue-demo", "", "", "git", "main", `{}`, 0, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.CreateBuild(project.ID, 1, "manual", "main", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateBuild(project.ID, 2, "retry", "release", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateBuildQueueItem(first.ID, project.ID, project.Name, 1, first.Trigger, first.Branch); err != nil {
		t.Fatal(err)
	}
	secondQueue, err := store.CreateBuildQueueItem(second.ID, project.ID, project.Name, 10, second.Trigger, second.Branch)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateBuildQueueItem(second.ID, project.ID, project.Name, 0, second.Trigger, second.Branch); err != nil {
		t.Fatalf("queue insertion must be idempotent: %v", err)
	}

	active, err := store.ListBuildQueue("")
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 2 || active[0].BuildID != second.ID {
		t.Fatalf("active queue = %#v, want higher-priority build %d first", active, second.ID)
	}
	if active[0].QueuePosition != 1 || active[0].BuildNumber != second.Number || active[0].WaitReason != "dispatch" {
		t.Fatalf("queue explanation = %#v", active[0])
	}
	pending, err := store.ListPendingBuilds()
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 2 || pending[0].ID != second.ID {
		t.Fatalf("pending builds = %#v, want higher-priority build %d first", pending, second.ID)
	}

	if err := store.UpdateBuildQueueItemStatus(secondQueue.ID, "running"); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateBuildQueueItemStatusByBuildID(second.ID, "success"); err != nil {
		t.Fatal(err)
	}
	active, err = store.ListBuildQueue("")
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].BuildID != first.ID {
		t.Fatalf("terminal builds must leave the active queue, got %#v", active)
	}
}

func TestRecoverInterruptedBuildsRequeuesOnlyRunningBuilds(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()
	project, err := store.CreateProject("recovery-demo", "", "", "git", "main", `{}`, 0, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	running, err := store.CreateBuild(project.ID, 1, "manual", "main", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := store.CreateBuild(project.ID, 2, "manual", "main", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.StartBuild(running.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateBuildQueueItem(running.ID, project.ID, project.Name, 5, running.Trigger, running.Branch); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateBuildQueueItemStatusByBuildID(running.ID, "running"); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateBuildStatus(cancelled.ID, "cancelled"); err != nil {
		t.Fatal(err)
	}

	recovered, err := store.RecoverInterruptedBuilds()
	if err != nil || recovered != 1 {
		t.Fatalf("RecoverInterruptedBuilds() = %d, %v; want 1, nil", recovered, err)
	}
	current, err := store.GetBuild(running.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != "pending" || current.StartedAt != nil || !strings.Contains(current.Log, "Requeued after interrupted server restart") {
		t.Fatalf("recovered build = %#v", current)
	}
	queue, err := store.ListBuildQueue("")
	if err != nil || len(queue) != 1 || queue[0].Status != "queued" || queue[0].StartedAt != nil {
		t.Fatalf("recovered queue = %#v, %v", queue, err)
	}
	unchanged, err := store.GetBuild(cancelled.ID)
	if err != nil || unchanged.Status != "cancelled" {
		t.Fatalf("cancelled build = %#v, %v", unchanged, err)
	}
}

func TestCancelBuildIsTerminalSafeAndRecordsDuration(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	project, err := store.CreateProject("cancel-demo", "", "", "git", "main", `{}`, 0, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build, err := store.CreateBuild(project.ID, 1, "manual", "main", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.StartBuild(build.ID); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	if err := store.CancelBuild(build.ID); err != nil {
		t.Fatal(err)
	}
	cancelled, err := store.GetBuild(build.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != "cancelled" || cancelled.FinishedAt == nil || cancelled.DurationMs == nil || *cancelled.DurationMs < 0 {
		t.Fatalf("cancelled build metadata = %#v", cancelled)
	}
	if err := store.CancelBuild(build.ID); !errors.Is(err, ErrBuildNotCancellable) {
		t.Fatalf("second cancellation error = %v, want ErrBuildNotCancellable", err)
	}
	if err := store.FinishBuild(build.ID, "success", 42); err != nil {
		t.Fatal(err)
	}
	if err := store.CancelBuild(build.ID); !errors.Is(err, ErrBuildNotCancellable) {
		t.Fatalf("terminal build cancellation error = %v, want ErrBuildNotCancellable", err)
	}
	finished, err := store.GetBuild(build.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != "success" || finished.DurationMs == nil || *finished.DurationMs != 42 {
		t.Fatalf("terminal build was overwritten: %#v", finished)
	}
}

func TestProjectGroupHierarchyAssignmentAndSafeDeletion(t *testing.T) {
	data, cleanup := newTestStore(t)
	defer cleanup()

	root, err := data.CreateProjectGroup("products", "all products", nil)
	if err != nil {
		t.Fatal(err)
	}
	child, err := data.CreateProjectGroup("desktop", "desktop applications", &root.ID)
	if err != nil {
		t.Fatal(err)
	}
	grandchild, err := data.CreateProjectGroup("windows", "windows applications", &child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := data.CreateProjectGroup("products", "duplicate name", nil); !errors.Is(err, ErrProjectGroupNameExists) {
		t.Fatalf("duplicate create error = %v, want ErrProjectGroupNameExists", err)
	}
	if err := data.UpdateProjectGroup(child.ID, grandchild.Name, child.Description, child.ParentID); !errors.Is(err, ErrProjectGroupNameExists) {
		t.Fatalf("duplicate update error = %v, want ErrProjectGroupNameExists", err)
	}
	project, err := data.CreateProject("installer", "", "", "git", "main", `{}`, 0, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := data.SetProjectGroup(project.ID, &child.ID); err != nil {
		t.Fatal(err)
	}

	if err := data.UpdateProjectGroup(root.ID, root.Name, root.Description, &grandchild.ID); err == nil {
		t.Fatal("cyclic project group hierarchy should be rejected")
	}
	if err := data.DeleteProjectGroup(child.ID); err != nil {
		t.Fatal(err)
	}

	reparented, err := data.GetProjectGroup(grandchild.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reparented.ParentID == nil || *reparented.ParentID != root.ID {
		t.Fatalf("grandchild parent = %#v, want %d", reparented.ParentID, root.ID)
	}
	assigned, err := data.GetProject(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if assigned.GroupID == nil || *assigned.GroupID != root.ID {
		t.Fatalf("project group = %#v, want %d", assigned.GroupID, root.ID)
	}
	if err := data.DeleteProjectGroup(child.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("second delete error = %v, want sql.ErrNoRows", err)
	}
}

func TestDatabaseMigration(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-migrate-*.db")
	if err != nil {
		t.Fatal(err)
	}
	name := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(name)

	store, err := New(name)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	tables := []string{"users", "ssh_keys", "projects", "builds", "artifacts", "workers", "plugins"}
	for _, table := range tables {
		var count int
		err := store.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count)
		if err != nil {
			t.Errorf("table %q not queryable: %v", table, err)
		}
	}

	store.Close()

	store2, err := New(name)
	if err != nil {
		t.Fatalf("New() on existing db error = %v", err)
	}
	defer store2.Close()

	var count int
	err = store2.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		t.Errorf("users table not queryable after reopen: %v", err)
	}
}

func TestDatabaseClose(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-close-*.db")
	if err != nil {
		t.Fatal(err)
	}
	name := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(name)

	store, err := New(name)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	err = store.db.Ping()
	if err == nil {
		t.Fatal("db.Ping() after Close() expected error, got nil")
	}
}

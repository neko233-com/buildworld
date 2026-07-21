package store

import (
	"database/sql"
	"errors"
	"testing"
)

func TestDeleteProjectRemovesOwnedRecordsAndPreservesSharedResources(t *testing.T) {
	data, cleanup := newTestStore(t)
	defer cleanup()
	if _, err := data.db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		t.Fatal(err)
	}

	user, err := data.CreateUser("project-owner", "project-owner@example.test", "hash", "admin")
	if err != nil {
		t.Fatal(err)
	}
	vcsRoot, err := data.CreateVCSRoot("shared-repository", "git", "https://example.test/shared.git", "main", nil, 60, true, "{}")
	if err != nil {
		t.Fatal(err)
	}
	template, err := data.CreateBuildTemplate("shared-template", "jobs: {}", "shared")
	if err != nil {
		t.Fatal(err)
	}
	group, err := data.CreateProjectGroupWithColor("shared-group", "shared", "mint")
	if err != nil {
		t.Fatal(err)
	}
	target, err := data.CreateProject("delete-me", "", "https://example.test/delete.git", "git", "main", "jobs: {}", user.ID, &vcsRoot.ID, &template.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := data.SetProjectGroup(target.ID, &group.ID); err != nil {
		t.Fatal(err)
	}
	survivor, err := data.CreateProject("keep-me", "", "https://example.test/keep.git", "git", "main", "jobs: {}", user.ID, &vcsRoot.ID, &template.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := data.SetProjectGroup(survivor.ID, &group.ID); err != nil {
		t.Fatal(err)
	}

	first, err := data.CreateBuild(target.ID, 1, "manual", "main", "target-1", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := data.CreateBuild(target.ID, 2, "manual", "main", "target-2", "", &first.ID, &first.ID)
	if err != nil {
		t.Fatal(err)
	}
	survivingBuild, err := data.CreateBuild(survivor.ID, 1, "manual", "main", "survivor", "", &first.ID, &second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := data.SetBuildLog(first.ID, "owned build log"); err != nil {
		t.Fatal(err)
	}
	if _, err := data.CreateBuildQueueItem(first.ID, target.ID, target.Name, 10, "manual", "main"); err != nil {
		t.Fatal(err)
	}
	if _, err := data.CreateBuildQueueItem(survivingBuild.ID, survivor.ID, survivor.Name, 1, "manual", "main"); err != nil {
		t.Fatal(err)
	}
	if _, err := data.CreateArtifact(first.ID, "owned.txt", "owned.txt", 5, "sha", "text/plain"); err != nil {
		t.Fatal(err)
	}
	if _, err := data.CreateArtifact(survivingBuild.ID, "kept.txt", "kept.txt", 4, "sha", "text/plain"); err != nil {
		t.Fatal(err)
	}
	channel, err := data.GetNotificationChannelByName(DefaultWebNotificationChannelName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := data.CreateNotificationEvent(channel.ID, &first.ID, "build.completed", "owned"); err != nil {
		t.Fatal(err)
	}
	if _, err := data.CreateNotificationEvent(channel.ID, &survivingBuild.ID, "build.completed", "kept"); err != nil {
		t.Fatal(err)
	}
	if _, err := data.CreateBuildApproval(first.ID, user.ID, user.Username); err != nil {
		t.Fatal(err)
	}
	if _, err := data.CreateBuildApproval(survivingBuild.ID, user.ID, user.Username); err != nil {
		t.Fatal(err)
	}
	targetResult, err := data.CreateTestResult(first.ID, 1, 1, 0, 0, 10, "<testsuite/>")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := data.CreateTestResult(survivingBuild.ID, 1, 1, 0, 0, 10, "<testsuite/>"); err != nil {
		t.Fatal(err)
	}
	// Exercise defensive cleanup for a cross-project test-result reference.
	if err := data.SetBuildTestResult(survivingBuild.ID, targetResult.ID); err != nil {
		t.Fatal(err)
	}
	if err := data.SetEnvVar("project", &target.ID, "TARGET_SECRET", "secret", true, "owned"); err != nil {
		t.Fatal(err)
	}
	if err := data.SetEnvVar("global", nil, "GLOBAL_VALUE", "kept", false, "shared"); err != nil {
		t.Fatal(err)
	}
	if _, err := data.CreateBuildStat(target.ID, "2026-07-21", 2, 2, 0, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := data.CreateBuildStat(survivor.ID, "2026-07-21", 1, 1, 0, 10); err != nil {
		t.Fatal(err)
	}

	if err := data.DeleteProject(target.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := data.GetProject(target.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("deleted project error = %v", err)
	}
	for _, check := range []struct {
		name  string
		query string
		args  []interface{}
	}{
		{"builds", "SELECT COUNT(*) FROM builds WHERE project_id=?", []interface{}{target.ID}},
		{"queue", "SELECT COUNT(*) FROM build_queue_items WHERE project_id=?", []interface{}{target.ID}},
		{"artifacts", "SELECT COUNT(*) FROM artifacts WHERE build_id IN (?, ?)", []interface{}{first.ID, second.ID}},
		{"notifications", "SELECT COUNT(*) FROM notification_events WHERE build_id IN (?, ?)", []interface{}{first.ID, second.ID}},
		{"approvals", "SELECT COUNT(*) FROM build_approvals WHERE build_id IN (?, ?)", []interface{}{first.ID, second.ID}},
		{"test results", "SELECT COUNT(*) FROM test_results WHERE build_id IN (?, ?)", []interface{}{first.ID, second.ID}},
		{"environment", "SELECT COUNT(*) FROM env_vars WHERE project_id=?", []interface{}{target.ID}},
		{"statistics", "SELECT COUNT(*) FROM build_stats WHERE project_id=?", []interface{}{target.ID}},
	} {
		var count int
		if err := data.db.QueryRow(check.query, check.args...).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", check.name, err)
		}
		if count != 0 {
			t.Fatalf("%s count = %d, want 0", check.name, count)
		}
	}

	keptBuild, err := data.GetBuild(survivingBuild.ID)
	if err != nil {
		t.Fatal(err)
	}
	if keptBuild.WaitDependencyOn != nil || keptBuild.RetriedFrom != nil || keptBuild.TestResultID != nil {
		t.Fatalf("surviving build retained deleted references: %#v", keptBuild)
	}
	for name, load := range map[string]func() error{
		"user":     func() error { _, err := data.GetUser(user.ID); return err },
		"VCS root": func() error { _, err := data.GetVCSRoot(vcsRoot.ID); return err },
		"template": func() error { _, err := data.GetBuildTemplate(template.ID); return err },
		"group":    func() error { _, err := data.GetProjectGroup(group.ID); return err },
		"channel":  func() error { _, err := data.GetNotificationChannel(channel.ID); return err },
	} {
		if err := load(); err != nil {
			t.Fatalf("shared %s was deleted: %v", name, err)
		}
	}
	var globalEnv, survivorQueue, survivorArtifacts, survivorNotifications, survivorApprovals, survivorResults, survivorStats int
	if err := data.db.QueryRow("SELECT COUNT(*) FROM env_vars WHERE project_id IS NULL AND name='GLOBAL_VALUE'").Scan(&globalEnv); err != nil {
		t.Fatal(err)
	}
	if err := data.db.QueryRow("SELECT COUNT(*) FROM build_queue_items WHERE build_id=?", survivingBuild.ID).Scan(&survivorQueue); err != nil {
		t.Fatal(err)
	}
	if err := data.db.QueryRow("SELECT COUNT(*) FROM artifacts WHERE build_id=?", survivingBuild.ID).Scan(&survivorArtifacts); err != nil {
		t.Fatal(err)
	}
	if err := data.db.QueryRow("SELECT COUNT(*) FROM notification_events WHERE build_id=?", survivingBuild.ID).Scan(&survivorNotifications); err != nil {
		t.Fatal(err)
	}
	if err := data.db.QueryRow("SELECT COUNT(*) FROM build_approvals WHERE build_id=?", survivingBuild.ID).Scan(&survivorApprovals); err != nil {
		t.Fatal(err)
	}
	if err := data.db.QueryRow("SELECT COUNT(*) FROM test_results WHERE build_id=?", survivingBuild.ID).Scan(&survivorResults); err != nil {
		t.Fatal(err)
	}
	if err := data.db.QueryRow("SELECT COUNT(*) FROM build_stats WHERE project_id=?", survivor.ID).Scan(&survivorStats); err != nil {
		t.Fatal(err)
	}
	if globalEnv != 1 || survivorQueue != 1 || survivorArtifacts != 1 || survivorNotifications != 1 || survivorApprovals != 1 || survivorResults != 1 || survivorStats != 1 {
		t.Fatalf("shared/surviving rows global=%d queue=%d artifacts=%d notifications=%d approvals=%d results=%d stats=%d", globalEnv, survivorQueue, survivorArtifacts, survivorNotifications, survivorApprovals, survivorResults, survivorStats)
	}
	if err := assertNoForeignKeyViolations(data.db); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteProjectRollsBackAllOwnedDeletes(t *testing.T) {
	data, cleanup := newTestStore(t)
	defer cleanup()
	project, err := data.CreateProject("rollback-project-delete", "", "", "git", "main", "jobs: {}", 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build, err := data.CreateBuild(project.ID, 1, "manual", "main", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := data.CreateArtifact(build.ID, "rollback.txt", "rollback.txt", 1, "sha", "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	if err := data.SetEnvVar("project", &project.ID, "ROLLBACK", "kept", false, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := data.db.Exec(`CREATE TRIGGER block_project_delete BEFORE DELETE ON projects
		BEGIN SELECT RAISE(ABORT, 'project deletion blocked'); END`); err != nil {
		t.Fatal(err)
	}

	if err := data.DeleteProject(project.ID); err == nil {
		t.Fatal("DeleteProject succeeded despite blocking trigger")
	}
	for name, query := range map[string]string{
		"project":     "SELECT COUNT(*) FROM projects WHERE id=?",
		"build":       "SELECT COUNT(*) FROM builds WHERE project_id=?",
		"environment": "SELECT COUNT(*) FROM env_vars WHERE project_id=?",
	} {
		var count int
		if err := data.db.QueryRow(query, project.ID).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", name, err)
		}
		if count != 1 {
			t.Fatalf("%s count after rollback = %d, want 1", name, count)
		}
	}
	if _, err := data.GetArtifact(artifact.ID); err != nil {
		t.Fatalf("artifact was not rolled back: %v", err)
	}
}

func assertNoForeignKeyViolations(db *sql.DB) error {
	rows, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		var table string
		var rowID int64
		var parent string
		var foreignKeyID int
		if err := rows.Scan(&table, &rowID, &parent, &foreignKeyID); err != nil {
			return err
		}
		return errors.New("foreign key violation in " + table)
	}
	return rows.Err()
}

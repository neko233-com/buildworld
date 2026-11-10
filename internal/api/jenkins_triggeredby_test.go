package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/neko233-com/buildworld/internal/auth"
	"github.com/neko233-com/buildworld/internal/config"
	"github.com/neko233-com/buildworld/internal/engine"
	"github.com/neko233-com/buildworld/internal/store"
)

func TestAuthenticatedTriggerPreservesUserThroughJenkinsCondition(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell unavailable")
	}
	t.Setenv("PATH", filepath.Dir(sh)+string(os.PathListSeparator)+os.Getenv("PATH"))
	database, err := store.New(filepath.Join(t.TempDir(), "buildworld.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	user, err := database.CreateUser("alice", "alice@example.test", "unused", "admin")
	if err != nil {
		t.Fatal(err)
	}
	source := `pipeline { agent any stages { stage('guard') { when { triggeredBy cause: 'UserIdCause', detail: 'alice' } steps { sh 'echo AUTHENTICATED_CAUSE_OK' } } } }`
	project, err := database.CreateProject("trigger-user", "", "", "git", "main", source, user.ID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	jwt := auth.NewJWT("triggeredby-test-secret")
	if _, err := (&handlers{d: Deps{Store: database}}).attachBuildTriggerUser(`{}`, user.ID); err != nil {
		t.Fatalf("attach authenticated user: %v", err)
	}
	token, err := jwt.Generate(user.ID, user.Role, user.SessionVersion, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(Deps{Cfg: &config.Config{}, Store: database, JWT: jwt})
	rawToken := "bw_88888888888888888888888888888888"
	persistAPIToken(t, database, user.ID, rawToken, `["build:trigger"]`)
	for _, test := range []struct{ path, token string }{
		{fmt.Sprintf("/api/projects/%d/builds", project.ID), token},
		{"/api/trigger/" + project.Name, rawToken},
	} {
		response := apiTokenRequest(router, http.MethodPost, test.path, test.token, `{}`)
		if response.Code != http.StatusCreated {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
		var public store.Build
		if err := json.Unmarshal(response.Body.Bytes(), &public); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(public.Parameters, "triggerUser") || strings.Contains(public.Parameters, "alice") {
			t.Fatalf("internal context exposed: %q", public.Parameters)
		}
		build, err := database.GetBuild(public.ID)
		if err != nil || !strings.Contains(build.Parameters, "alice") {
			t.Fatalf("stored build=%+v err=%v", build, err)
		}
		runner := engine.NewBuildRunner(database, nil, filepath.Join(t.TempDir(), "workspaces"), nil)
		defer runner.Shutdown(context.Background())
		runner.Run(build.ID)
		for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
			build, err = database.GetBuild(build.ID)
			if err != nil {
				t.Fatal(err)
			}
			if build.Status == "success" || build.Status == "failed" {
				break
			}
		}
		if build.Status != "success" || !strings.Contains(build.Log, "AUTHENTICATED_CAUSE_OK") {
			t.Fatalf("status=%s log=%s", build.Status, build.Log)
		}
	}
}

func TestManualRetryUsesCurrentUserAndRebuildCause(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell unavailable")
	}
	t.Setenv("PATH", filepath.Dir(sh)+string(os.PathListSeparator)+os.Getenv("PATH"))
	database, err := store.New(filepath.Join(t.TempDir(), "buildworld.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	alice, err := database.CreateUser("alice", "alice@example.test", "unused", "admin")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := database.CreateUser("bob", "bob@example.test", "unused", "admin")
	if err != nil {
		t.Fatal(err)
	}
	source := `pipeline { agent any stages {
stage('alice') { when { triggeredBy cause: 'UserIdCause', detail: 'alice' } steps { sh 'echo ALICE_CAUSE_OK' } }
stage('bob') { when { triggeredBy cause: 'UserIdCause', detail: 'bob' } steps { sh 'echo BOB_CAUSE_OK' } }
stage('rebuild') { when { triggeredBy 'RebuildCause' } steps { sh 'echo REBUILD_CAUSE_OK' } }
} }`
	project, err := database.CreateProject("retry-user", "", "", "git", "main", source, alice.ID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	jwt := auth.NewJWT("retry-cause-test-secret")
	aliceToken, err := jwt.Generate(alice.ID, alice.Role, alice.SessionVersion, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	bobToken, err := jwt.Generate(bob.ID, bob.Role, bob.SessionVersion, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(Deps{Cfg: &config.Config{}, Store: database, JWT: jwt})
	runner := engine.NewBuildRunner(database, nil, filepath.Join(t.TempDir(), "workspaces"), nil)
	defer runner.Shutdown(context.Background())
	run := func(response *httptest.ResponseRecorder) *store.Build {
		t.Helper()
		if response.Code != http.StatusCreated {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
		var build store.Build
		if err := json.Unmarshal(response.Body.Bytes(), &build); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(build.Parameters, "buildworld.retry") {
			t.Fatalf("retry metadata exposed: %s", build.Parameters)
		}
		runner.Run(build.ID)
		for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
			current, err := database.GetBuild(build.ID)
			if err != nil {
				t.Fatal(err)
			}
			if current.Status == "success" || current.Status == "failed" {
				return current
			}
		}
		t.Fatal("build did not finish")
		return nil
	}
	original := run(apiTokenRequest(router, http.MethodPost, fmt.Sprintf("/api/projects/%d/builds", project.ID), aliceToken, `{}`))
	if original.Status != "success" || !strings.Contains(original.Log, "ALICE_CAUSE_OK") || strings.Contains(original.Log, "REBUILD_CAUSE_OK") {
		t.Fatalf("original: %+v", original)
	}
	retry := run(apiTokenRequest(router, http.MethodPost, fmt.Sprintf("/api/builds/%d/retry", original.ID), bobToken, `{}`))
	if retry.Status != "success" || strings.Contains(retry.Log, "ALICE_CAUSE_OK") || !strings.Contains(retry.Log, "BOB_CAUSE_OK") || !strings.Contains(retry.Log, "REBUILD_CAUSE_OK") {
		t.Fatalf("retry: %+v", retry)
	}
}

func TestProjectHTTPTriggerRunsRemoteCauseCondition(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell unavailable")
	}
	t.Setenv("PATH", filepath.Dir(sh)+string(os.PathListSeparator)+os.Getenv("PATH"))
	database, err := store.New(filepath.Join(t.TempDir(), "remote.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	source := `pipeline { agent any stages { stage('remote') { when { triggeredBy 'Remote' } steps { sh 'echo HTTP_CAUSE_OK' } } } }`
	project, err := database.CreateProject("remote-cause", "", "", "git", "main", source, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SetProjectHTTPTrigger(project.ID, true, "fixture-http-token"); err != nil {
		t.Fatal(err)
	}
	router := NewRouter(Deps{Cfg: &config.Config{}, Store: database, JWT: auth.NewJWT("remote-cause-test")})
	response := apiTokenRequest(router, http.MethodPost, fmt.Sprintf("/api/trigger/projects/%d/fixture-http-token", project.ID), "", `{}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var build store.Build
	if err := json.Unmarshal(response.Body.Bytes(), &build); err != nil {
		t.Fatal(err)
	}
	runner := engine.NewBuildRunner(database, nil, filepath.Join(t.TempDir(), "workspaces"), nil)
	defer runner.Shutdown(context.Background())
	runner.Run(build.ID)
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		current, err := database.GetBuild(build.ID)
		if err != nil {
			t.Fatal(err)
		}
		if current.Status == "success" || current.Status == "failed" {
			if current.Status != "success" || !strings.Contains(current.Log, "HTTP_CAUSE_OK") {
				t.Fatalf("status=%s log=%s", current.Status, current.Log)
			}
			return
		}
	}
	t.Fatal("build did not finish")
}

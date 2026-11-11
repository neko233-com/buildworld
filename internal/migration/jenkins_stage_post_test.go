package migration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/neko233-com/buildworld/internal/engine"
	"github.com/neko233-com/buildworld/internal/store"
)

// These marker orders were checked against a real Jenkins Declarative job.
func TestJenkinsStagePostMatchesJenkinsOutcomes(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell is required for the runner integration test")
	}
	t.Setenv("PATH", filepath.Dir(sh)+string(os.PathListSeparator)+os.Getenv("PATH"))
	tests := []struct {
		name, stages, status, markers string
	}{
		{"skip", `stage('skip') {
 when { expression { false } }
 steps { echo 'BWPROBE:SKIP_STEP' }
 post { always { echo 'BWPROBE:SKIP_POST' } }
}
stage('after') { steps { echo 'BWPROBE:AFTER' } }`, "success", "BWPROBE:AFTER,BWPROBE:PIPELINE_POST"},
		{"failure", `stage('fail') {
 steps { echo 'BWPROBE:START'; error 'expected failure'; echo 'BWPROBE:STEP_AFTER_ERROR' }
 post {
  always { echo 'BWPROBE:ALWAYS' }
  failure { echo 'BWPROBE:FAILURE' }
  success { echo 'BWPROBE:SUCCESS' }
  cleanup { echo 'BWPROBE:CLEANUP' }
 }
}`, "failed", "BWPROBE:START,BWPROBE:ALWAYS,BWPROBE:FAILURE,BWPROBE:CLEANUP,BWPROBE:PIPELINE_POST"},
		{"posterror", `stage('posterror') {
 steps { echo 'BWPROBE:STEP' }
 post {
  always { echo 'BWPROBE:ALWAYS'; error 'expected post failure'; echo 'BWPROBE:POST_AFTER_ERROR' }
  failure { echo 'BWPROBE:FAILURE' }
  success { echo 'BWPROBE:SUCCESS' }
  cleanup { echo 'BWPROBE:CLEANUP' }
 }
}`, "failed", "BWPROBE:STEP,BWPROBE:ALWAYS,BWPROBE:FAILURE,BWPROBE:CLEANUP,BWPROBE:PIPELINE_POST"},
		{"success", `stage('success') {
 environment { STAGE_ONLY = 'present' }
 steps { echo 'BWPROBE:STEP' }
 post {
  always { sh 'test "$STAGE_ONLY" = present; echo BWPROBE:ALWAYS' }
  failure { echo 'BWPROBE:FAILURE' }
  success { echo 'BWPROBE:SUCCESS' }
  cleanup { echo 'BWPROBE:CLEANUP' }
 }
}`, "success", "BWPROBE:STEP,BWPROBE:ALWAYS,BWPROBE:SUCCESS,BWPROBE:CLEANUP,BWPROBE:PIPELINE_POST"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			db, err := store.New(filepath.Join(root, "stage-post.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			source := "pipeline {\n agent any\n options { skipDefaultCheckout() }\n stages {\n" + tc.stages + "\n }\n post { always { echo 'BWPROBE:PIPELINE_POST'; deleteDir() } }\n}"
			project, err := db.CreateProject("oracle-"+tc.name, "", "", "git", "main", source, 0, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			build, err := db.CreateBuild(project.ID, 1, "manual", "main", "", `{}`, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			runner := engine.NewBuildRunner(db, nil, filepath.Join(root, "workspaces"), nil)
			defer runner.Shutdown(context.Background())
			runner.Run(build.ID)
			deadline := time.Now().Add(20 * time.Second)
			for time.Now().Before(deadline) {
				build, err = db.GetBuild(build.ID)
				if err != nil {
					t.Fatal(err)
				}
				if build.Status == "failed" || build.Status == "success" {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			markers := regexp.MustCompile(`BWPROBE:[A-Z_]+`).FindAllString(build.Log, -1)
			if build.Status != tc.status || strings.Join(markers, ",") != tc.markers {
				t.Fatalf("status=%s markers=%v log=%s", build.Status, markers, build.Log)
			}
		})
	}
}

func TestJenkinsStagePostRejectsUnsupportedCombinations(t *testing.T) {
	for _, body := range []string{
		`steps { echo 'x' } post { unstable { echo 'not yet supported' } }`,
		`steps { echo 'x' } post { always { echo 'one' } } post { cleanup { echo 'two' } }`,
		`steps { echo 'x' } post { always { archiveArtifacts artifacts: 'x', fingerprint: true } }`,
	} {
		source := "pipeline { agent any stages { stage('unsafe') { " + body + " } } }"
		if _, err := engine.ParseJenkinsfileConfig(source, "unsafe"); err == nil {
			t.Fatalf("unsupported stage post accepted: %s", body)
		}
	}
}

func TestSkippedStagePostDoesNotCreateGlobalArchive(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell is required for the runner integration test")
	}
	t.Setenv("PATH", filepath.Dir(sh)+string(os.PathListSeparator)+os.Getenv("PATH"))
	source := `pipeline {
 agent any
 options { skipDefaultCheckout() }
 stages {
  stage('skip') {
   when { expression { false } }
   steps { echo 'skip' }
   post { always { archiveArtifacts artifacts: 'dist/**' } }
  }
  stage('later') { steps { sh 'mkdir -p dist; echo later > dist/result.txt' } }
 }
}`
	config, err := engine.ParseJenkinsfileConfig(source, "skip-archive")
	if err != nil || len(config.Artifacts) != 0 {
		t.Fatalf("conditional archive became global: config=%#v error=%v", config, err)
	}
	root := t.TempDir()
	db, err := store.New(filepath.Join(root, "archive.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	project, err := db.CreateProject("skip-archive", "", "", "git", "main", source, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build, err := db.CreateBuild(project.ID, 1, "manual", "main", "", `{}`, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	runner := engine.NewBuildRunner(db, nil, filepath.Join(root, "workspaces"), nil)
	runner.SetArtifactManager(engine.NewArtifactManager(db, filepath.Join(root, "artifacts")))
	defer runner.Shutdown(context.Background())
	runner.Run(build.ID)
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		build, err = db.GetBuild(build.ID)
		if err != nil {
			t.Fatal(err)
		}
		if build.Status == "failed" || build.Status == "success" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	artifacts, err := db.ListArtifactsByBuild(build.ID)
	if err != nil || build.Status != "success" || len(artifacts) != 0 {
		t.Fatalf("status=%s artifacts=%#v err=%v log=%s", build.Status, artifacts, err, build.Log)
	}
}

func TestJenkinsStageAndPipelineAbortedPostOnUserCancel(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell is required for the cancellation test")
	}
	t.Setenv("PATH", filepath.Dir(sh)+string(os.PathListSeparator)+os.Getenv("PATH"))
	root := t.TempDir()
	db, err := store.New(filepath.Join(root, "cancel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	source := `pipeline {
 agent any
 options { skipDefaultCheckout() }
 stages { stage('wait') {
  steps { sh 'echo BWPROBE:START; sleep 20; echo BWPROBE:FINISHED' }
  post {
   always { echo 'BWPROBE:STAGE_ALWAYS' }
   aborted { echo 'BWPROBE:STAGE_ABORTED' }
   failure { echo 'BWPROBE:STAGE_FAILURE' }
   cleanup { echo 'BWPROBE:STAGE_CLEANUP' }
  }
 } }
 post {
  always { echo 'BWPROBE:PIPELINE_ALWAYS' }
  aborted { echo 'BWPROBE:PIPELINE_ABORTED' }
  failure { echo 'BWPROBE:PIPELINE_FAILURE' }
  cleanup { echo 'BWPROBE:PIPELINE_CLEANUP' }
 }
}`
	project, err := db.CreateProject("cancel-post", "", "", "git", "main", source, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build, err := db.CreateBuild(project.ID, 1, "manual", "main", "", `{}`, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	runner := engine.NewBuildRunner(db, nil, filepath.Join(root, "workspaces"), nil)
	defer runner.Shutdown(context.Background())
	runner.Run(build.ID)
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		build, err = db.GetBuild(build.ID)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(build.Log, "BWPROBE:START") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(build.Log, "BWPROBE:START") {
		t.Fatalf("stage did not start: %s", build.Log)
	}
	runner.Stop(build.ID)
	deadline = time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		build, err = db.GetBuild(build.ID)
		if err != nil {
			t.Fatal(err)
		}
		if build.Status == "cancelled" || build.Status == "failed" || build.Status == "success" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	markers := regexp.MustCompile(`BWPROBE:[A-Z_]+`).FindAllString(build.Log, -1)
	want := "BWPROBE:START,BWPROBE:STAGE_ALWAYS,BWPROBE:STAGE_ABORTED,BWPROBE:STAGE_CLEANUP,BWPROBE:PIPELINE_ALWAYS,BWPROBE:PIPELINE_ABORTED,BWPROBE:PIPELINE_CLEANUP"
	if build.Status != "cancelled" || strings.Join(markers, ",") != want {
		t.Fatalf("status=%s markers=%v log=%s", build.Status, markers, build.Log)
	}
}

func TestJenkinsStageTimeoutRunsAbortedPost(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell is required for the timeout test")
	}
	t.Setenv("PATH", filepath.Dir(sh)+string(os.PathListSeparator)+os.Getenv("PATH"))
	root := t.TempDir()
	db, err := store.New(filepath.Join(root, "timeout.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	source := `pipeline {
 agent any
 options { skipDefaultCheckout() }
 stages { stage('bounded') {
  options { timeout(time: 1, unit: 'SECONDS') }
  steps { sh 'echo BWPROBE:START; sleep 10; echo BWPROBE:STEP_FINISHED' }
  post {
   always { echo 'BWPROBE:ALWAYS' }
   failure { echo 'BWPROBE:FAILURE' }
   aborted { echo 'BWPROBE:ABORTED' }
   cleanup { echo 'BWPROBE:CLEANUP' }
  }
 } }
 post { always { echo 'BWPROBE:PIPELINE_POST' } }
}`
	project, err := db.CreateProject("timeout-post", "", "", "git", "main", source, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build, err := db.CreateBuild(project.ID, 1, "manual", "main", "", `{}`, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	runner := engine.NewBuildRunner(db, nil, filepath.Join(root, "workspaces"), nil)
	defer runner.Shutdown(context.Background())
	runner.Run(build.ID)
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		build, err = db.GetBuild(build.ID)
		if err != nil {
			t.Fatal(err)
		}
		if build.Status == "cancelled" || build.Status == "failed" || build.Status == "success" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	markers := regexp.MustCompile(`BWPROBE:[A-Z_]+`).FindAllString(build.Log, -1)
	want := "BWPROBE:START,BWPROBE:ALWAYS,BWPROBE:ABORTED,BWPROBE:CLEANUP,BWPROBE:PIPELINE_POST"
	if build.Status != "cancelled" || strings.Join(markers, ",") != want {
		t.Fatalf("status=%s markers=%v log=%s", build.Status, markers, build.Log)
	}
}

func TestJenkinsStageTimeoutDuringPostMatchesJenkins(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell is required for the timeout test")
	}
	t.Setenv("PATH", filepath.Dir(sh)+string(os.PathListSeparator)+os.Getenv("PATH"))
	root := t.TempDir()
	db, err := store.New(filepath.Join(root, "post-timeout.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	source := `pipeline {
 agent any
 options { skipDefaultCheckout() }
 stages { stage('bounded') {
  options { timeout(time: 1, unit: 'SECONDS') }
  steps { echo 'BWPROBE:STEP' }
  post {
   always { sh 'echo BWPROBE:ALWAYS_START; sleep 10; echo BWPROBE:ALWAYS_FINISHED' }
   failure { echo 'BWPROBE:FAILURE' }
   aborted { echo 'BWPROBE:ABORTED' }
   cleanup { echo 'BWPROBE:CLEANUP' }
  }
 } }
 post { always { echo 'BWPROBE:PIPELINE_POST' } }
}`
	project, err := db.CreateProject("post-timeout", "", "", "git", "main", source, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build, err := db.CreateBuild(project.ID, 1, "manual", "main", "", `{}`, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	runner := engine.NewBuildRunner(db, nil, filepath.Join(root, "workspaces"), nil)
	defer runner.Shutdown(context.Background())
	runner.Run(build.ID)
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		build, err = db.GetBuild(build.ID)
		if err != nil {
			t.Fatal(err)
		}
		if build.Status == "cancelled" || build.Status == "failed" || build.Status == "success" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	markers := regexp.MustCompile(`BWPROBE:[A-Z_]+`).FindAllString(build.Log, -1)
	want := "BWPROBE:STEP,BWPROBE:ALWAYS_START,BWPROBE:ABORTED,BWPROBE:CLEANUP,BWPROBE:PIPELINE_POST"
	if build.Status != "cancelled" || strings.Join(markers, ",") != want {
		t.Fatalf("status=%s markers=%v log=%s", build.Status, markers, build.Log)
	}
}

func TestJenkinsCancellationDuringStagePostRunsAbortedCleanup(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell is required for the cancellation test")
	}
	t.Setenv("PATH", filepath.Dir(sh)+string(os.PathListSeparator)+os.Getenv("PATH"))
	root := t.TempDir()
	db, err := store.New(filepath.Join(root, "post-cancel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	source := `pipeline {
 agent any
 options { skipDefaultCheckout() }
 stages { stage('build') {
  steps { echo 'BWPROBE:STEP' }
  post {
   always { sh 'echo BWPROBE:POST_START; sleep 20; echo BWPROBE:POST_FINISHED' }
   failure { echo 'BWPROBE:FAILURE' }
   aborted { echo 'BWPROBE:ABORTED' }
   cleanup { echo 'BWPROBE:CLEANUP' }
  }
 } }
 post { aborted { echo 'BWPROBE:PIPELINE_ABORTED' } }
}`
	project, err := db.CreateProject("post-cancel", "", "", "git", "main", source, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build, err := db.CreateBuild(project.ID, 1, "manual", "main", "", `{}`, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	runner := engine.NewBuildRunner(db, nil, filepath.Join(root, "workspaces"), nil)
	defer runner.Shutdown(context.Background())
	runner.Run(build.ID)
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		build, err = db.GetBuild(build.ID)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(build.Log, "BWPROBE:POST_START") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(build.Log, "BWPROBE:POST_START") {
		t.Fatalf("post did not start: %s", build.Log)
	}
	runner.Stop(build.ID)
	deadline = time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		build, err = db.GetBuild(build.ID)
		if err != nil {
			t.Fatal(err)
		}
		if build.Status == "cancelled" || build.Status == "failed" || build.Status == "success" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	markers := regexp.MustCompile(`BWPROBE:[A-Z_]+`).FindAllString(build.Log, -1)
	want := "BWPROBE:STEP,BWPROBE:POST_START,BWPROBE:ABORTED,BWPROBE:CLEANUP,BWPROBE:PIPELINE_ABORTED"
	if build.Status != "cancelled" || strings.Join(markers, ",") != want {
		t.Fatalf("status=%s markers=%v log=%s", build.Status, markers, build.Log)
	}
}

func TestNativePipelineStageTimeoutRemainsFailure(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell is required for the timeout test")
	}
	t.Setenv("PATH", filepath.Dir(sh)+string(os.PathListSeparator)+os.Getenv("PATH"))
	root := t.TempDir()
	db, err := store.New(filepath.Join(root, "native-timeout.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	source := `import { definePipeline } from '@buildworld/pipeline'
export default definePipeline({ stages: [{ name: 'bounded', timeoutSec: 1,
  steps: [{ name: 'wait', type: 'shell', shell: 'bash', command: 'sleep 10' }],
  post: { failure: [{ name: 'failure', type: 'shell', shell: 'bash', command: 'echo BWPROBE:NATIVE_FAILURE' }],
          aborted: [{ name: 'aborted', type: 'shell', shell: 'bash', command: 'echo BWPROBE:NATIVE_ABORTED' }] }
}] })`
	project, err := db.CreateProject("native-timeout", "", "", "git", "main", source, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build, err := db.CreateBuild(project.ID, 1, "manual", "main", "", `{}`, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	runner := engine.NewBuildRunner(db, nil, filepath.Join(root, "workspaces"), nil)
	defer runner.Shutdown(context.Background())
	runner.Run(build.ID)
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		build, err = db.GetBuild(build.ID)
		if err != nil {
			t.Fatal(err)
		}
		if build.Status == "cancelled" || build.Status == "failed" || build.Status == "success" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if build.Status != "failed" || !strings.Contains(build.Log, "BWPROBE:NATIVE_FAILURE") || strings.Contains(build.Log, "BWPROBE:NATIVE_ABORTED") {
		t.Fatalf("status=%s log=%s", build.Status, build.Log)
	}
}

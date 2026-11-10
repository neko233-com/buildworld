package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/neko233-com/buildworld/internal/engine"
	"github.com/neko233-com/buildworld/internal/store"
)

func TestJenkinsWhenEqualsValuesAndBuildNumber(t *testing.T) {
	for _, tc := range []struct {
		condition string
		params    map[string]interface{}
		env       map[string]string
		want      bool
	}{
		{`equals expected: 2, actual: currentBuild.number`, nil, map[string]string{"BUILD_NUMBER": "2"}, true},
		{`equals(expected: 2, actual: currentBuild.number)`, nil, map[string]string{"BUILD_NUMBER": "3"}, false},
		{`equals actual: params.COUNT, expected: 9007199254740993`, map[string]interface{}{"COUNT": json.Number("9007199254740993")}, nil, true},
		{`equals expected: 9007199254740993, actual: params.COUNT`, map[string]interface{}{"COUNT": json.Number("9007199254740992")}, nil, false},
		{`equals expected: -1, actual: params.COUNT`, map[string]interface{}{"COUNT": json.Number("-1.0")}, nil, true},
		{`equals expected: 2, actual: params.COUNT`, map[string]interface{}{"COUNT": "2"}, nil, false},
		{`equals expected: true, actual: params.FLAG`, map[string]interface{}{"FLAG": true}, nil, true},
		{`equals expected: null, actual: params.MISSING`, nil, nil, true},
		{`equals expected: 'production', actual: env.TARGET`, nil, map[string]string{"TARGET": "production"}, true},
		{`allOf { equals expected: 'yes', actual: params.FLAG; branch 'main' }`, map[string]interface{}{"FLAG": "yes"}, nil, true},
		{`expression { currentBuild.number >= 2 && params.COUNT > 9007199254740992 }`, map[string]interface{}{"COUNT": json.Number("9007199254740993")}, map[string]string{"BUILD_NUMBER": "2"}, true},
		{`expression { params.LABEL < 'release' }`, map[string]interface{}{"LABEL": "main"}, nil, true},
		{`expression { params.COUNT != 2 && params.COUNT <= 3 }`, map[string]interface{}{"COUNT": json.Number("3")}, nil, true},
		{`expression { params.A < params.B }`, map[string]interface{}{"A": "\ue000", "B": "\U00010000"}, nil, false},
	} {
		t.Run(tc.condition+fmt.Sprint(tc.want), func(t *testing.T) {
			source := fmt.Sprintf(`pipeline { agent any stages { stage('guard') { when { %s } steps { sh 'echo ok' } } } }`, tc.condition)
			config, err := engine.ParseJenkinsfileConfig(source, "equals")
			if err != nil {
				t.Fatal(err)
			}
			got, err := engine.ShouldRunStageWithJenkinsContext(config.Stages[0], nil, tc.params, tc.env, "main")
			if err != nil || got != tc.want {
				t.Fatalf("got=%t want=%t error=%v", got, tc.want, err)
			}
		})
	}
}

func TestJenkinsNumericConditionsFailClosedForInvalidInputs(t *testing.T) {
	for _, tc := range []struct {
		expression string
		parameters map[string]interface{}
	}{
		{`params.COUNT > 1`, map[string]interface{}{"COUNT": "2"}},
		{`params.COUNT == 1`, map[string]interface{}{"COUNT": json.Number("1e9999999")}},
	} {
		if _, err := engine.EvaluateJenkinsWhen(tc.expression, tc.parameters); err == nil {
			t.Fatalf("accepted invalid numeric condition %s", tc.expression)
		}
	}
}

func TestJenkinsWhenEqualsRejectsUnsafeOrMalformedExpressions(t *testing.T) {
	for _, condition := range []string{
		`equals expected: 2`,
		`equals expected: 2, expected: 3, actual: 2`,
		`equals expected: 2, actual: dangerous()`,
		`equals expected: 2, actual: other.number`,
		`equals expected: "${params.TARGET}", actual: 'production'`,
		`equals(expected: 2, actual: 2, surprise: 3)`,
	} {
		source := fmt.Sprintf(`pipeline { agent any stages { stage('guard') { when { %s } steps { sh 'echo ok' } } } }`, condition)
		if _, err := engine.ParseJenkinsfileConfig(source, "equals"); err == nil {
			t.Fatalf("accepted %s", condition)
		}
	}
}

func TestJenkinsWhenEqualsRunsAgainstActualBuildNumber(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("POSIX shell is unavailable")
	}
	t.Setenv("PATH", filepath.Dir(sh)+string(os.PathListSeparator)+os.Getenv("PATH"))
	root := t.TempDir()
	data, err := store.New(filepath.Join(root, "equals.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	source := `pipeline { agent any environment { BUILD_NUMBER = '99' } stages { stage('matched') { when { equals expected: 2, actual: currentBuild.number } steps { sh 'echo actual-match' } } stage('skipped') { when { equals expected: 1, actual: currentBuild.number } steps { sh 'echo wrong-match' } } } }`
	project, err := data.CreateProject("equals", "", "", "git", "main", source, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	build, err := data.CreateBuild(project.ID, 2, "manual", "main", "", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	runner := engine.NewBuildRunner(data, nil, filepath.Join(root, "workspaces"), nil)
	defer runner.Shutdown(context.Background())
	runner.Run(build.ID)
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		build, err = data.GetBuild(build.ID)
		if err != nil {
			t.Fatal(err)
		}
		if build.Status == "success" || build.Status == "failed" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if build.Status != "success" || !strings.Contains(build.Log, "actual-match") || strings.Contains(build.Log, "wrong-match") {
		t.Fatalf("unexpected build: status=%s log=%s", build.Status, build.Log)
	}
}

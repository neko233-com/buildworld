package migration

import (
	"fmt"
	"strings"
	"testing"

	"github.com/neko233-com/buildworld/internal/engine"
)

func TestJenkinsSCMWhenParsing(t *testing.T) {
	for _, condition := range []string{
		`changeset 'src/**/*.go'`,
		`changeset(pattern: 'src/**/*.go', comparator: 'GLOB', caseSensitive: true)`,
		`changeset pattern: 'src/.*\\.go', comparator: 'REGEXP'`,
		`changelog 'feat:.*'`,
		`changelog(pattern: '.*release.*')`,
	} {
		source := fmt.Sprintf(`pipeline { agent any stages { stage('build') { when { %s } steps { sh 'echo build' } } } }`, condition)
		config, err := engine.ParseJenkinsfileConfig(source, "scm")
		if err != nil || len(config.Stages) != 1 || !strings.Contains(config.Stages[0].JenkinsWhen, "jenkinsChange") {
			t.Fatalf("condition=%s config=%+v err=%v", condition, config, err)
		}
	}
	for _, condition := range []string{
		`changeset pattern: params.PATH`, `changeset pattern: '*.go', caseSensitive: 'true'`,
		`changeset pattern: '*.go', unknown: 'x'`, `changelog pattern: 'x', comparator: 'GLOB'`,
	} {
		source := fmt.Sprintf(`pipeline { agent any stages { stage('build') { when { %s } steps { sh 'echo build' } } } }`, condition)
		if _, err := engine.ParseJenkinsfileConfig(source, "scm"); err == nil {
			t.Fatalf("unsupported SCM condition accepted: %s", condition)
		}
	}
}

package migration

import (
	"fmt"
	"strings"
	"testing"

	"github.com/neko233-com/buildworld/internal/engine"
)

func TestJenkinsWhenGroovyLiteralEscapes(t *testing.T) {
	tests := []struct {
		condition   string
		branch      string
		environment map[string]string
		parameters  map[string]interface{}
		want        bool
	}{
		{`branch pattern: 'release-\\d+', comparator: 'REGEXP'`, "release-42", nil, nil, true},
		{`branch(pattern: 'release-\\d+', comparator: 'REGEXP')`, "release-a", nil, nil, false},
		{`branch 'release/\u4e2d\u6587'`, "release/中文", nil, nil, true},
		{`branch 'feature/\uD83D\uDE80'`, "feature/🚀", nil, nil, true},
		{`environment name: 'GREETING', value: 'it\'s ok'`, "main", map[string]string{"GREETING": "it's ok"}, nil, true},
		{`environment(name: 'PRICE', value: '$5')`, "main", map[string]string{"PRICE": "$5"}, nil, true},
		{`environment(name: 'PRICE', value: '\$5')`, "main", map[string]string{"PRICE": "$5"}, nil, true},
		{`environment(name: 'PRICE', value: "\$5")`, "main", map[string]string{"PRICE": "$5"}, nil, true},
		{`expression { params.VALUE == 'line\nnext' }`, "main", nil, map[string]interface{}{"VALUE": "line\nnext"}, true},
		{`expression { params.VALUE == "quote: \"yes\"" }`, "main", nil, map[string]interface{}{"VALUE": `quote: "yes"`}, true},
		{`branch pattern: 'release-(?=[0-9]{2}$)[0-9]+', comparator: 'REGEXP'`, "release-44", nil, nil, true},
		{`branch pattern: 'release-(?=[0-9]{3}$)[0-9]+', comparator: 'REGEXP'`, "release-44", nil, nil, false},
		{`branch pattern: 'release-([0-9])\\1', comparator: 'REGEXP'`, "release-44", nil, nil, true},
	}
	for _, tc := range tests {
		source := fmt.Sprintf(`pipeline { agent any stages { stage('test') { when { %s } steps { sh 'echo ok' } } } }`, tc.condition)
		config, err := engine.ParseJenkinsfileConfig(source, "escape-test")
		if err != nil {
			t.Fatalf("%s: %v", tc.condition, err)
		}
		got, err := engine.ShouldRunStageWithJenkinsContext(config.Stages[0], nil, tc.parameters, tc.environment, tc.branch)
		if err != nil || got != tc.want {
			t.Fatalf("%s: got %v, want %v, error %v", tc.condition, got, tc.want, err)
		}
	}
}

func TestJenkinsWhenLiteralRejectsInterpolationAndInvalidEscape(t *testing.T) {
	for _, condition := range []string{
		`branch "${params.BRANCH}"`, `environment name: 'MODE', value: "$MODE"`,
		`branch "\u0024{params.BRANCH}"`,
		`branch 'release-\d+'`, `branch 'bad\s'`, `branch 'bad\uD800'`, `branch 'bad\uDE80'`,
	} {
		source := fmt.Sprintf(`pipeline { agent any stages { stage('test') { when { %s } steps { sh 'echo ok' } } } }`, condition)
		_, err := engine.ParseJenkinsfileConfig(source, "escape-test")
		if err == nil || !strings.Contains(err.Error(), "when") {
			t.Fatalf("accepted %s: %v", condition, err)
		}
	}
}

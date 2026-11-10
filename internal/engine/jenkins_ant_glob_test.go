package engine

import "testing"

// Expected values were checked against the installed Jenkins Comparator.GLOB.
func TestJenkinsAntGlobPathSemantics(t *testing.T) {
	tests := []struct {
		pattern, path string
		want          bool
	}{
		{"src//*.go", "src/main.go", true},
		{"src/**/", "src/main.go", true},
		{"src/*.go", "src//main.go", true},
		{"/src/**", "src/main.go", false},
		{"/src/**", "/src/main.go", true},
		{"src/?.go", "src/😀.go", false},
		{"src/??.go", "src/😀.go", true},
		{"src/*.go", "src/one\ntwo.go", true},
		{"**", "/x/y", true},
		{"/*", "abc", false},
		{"src/**", "src", true},
		{"**/x", "x", true},
		{"src/**/x", "src/x", true},
		{"src/**/x", "src/a/b/x", true},
		{"x//y", "x/y/", true},
		{"x/?", "x/中", true},
		{"x/???", "x/😀中", true},
		{"x/I", "x/ı", true},
		{"x/k", "x/K", false},
		{"x/ς", "x/σ", true},
		{"x/*b", "x/a\nb", true},
		{"x/?", "x/\n", true},
	}
	for _, tc := range tests {
		got, err := matchJenkinsPath(tc.pattern, tc.path, false)
		if err != nil || got != tc.want {
			t.Errorf("pattern %q path %q: got %v, want %v (err=%v)", tc.pattern, tc.path, got, tc.want, err)
		}
	}
}

func TestJenkinsAntGlobConditionIntegration(t *testing.T) {
	env := map[string]string{
		"TAG_NAME":                  "release/😀",
		jenkinsSCMChangesContextKey: `[{"paths":["src/main.go"]}]`,
	}
	for _, tc := range []struct {
		condition string
		want      bool
	}{
		{`jenkinsBranch("feature//**")`, true},
		{`jenkinsTag("release/??")`, true},
		{`jenkinsTag("release/?")`, false},
		{`jenkinsChangeSet("src//*.go","GLOB","false")`, true},
		{`jenkinsChangeSet("SRC//*.go","GLOB","true")`, false},
	} {
		got, err := EvaluateJenkinsWhenContext(tc.condition, nil, env, "feature/main")
		if err != nil || got != tc.want {
			t.Errorf("%s: got %v, want %v (err=%v)", tc.condition, got, tc.want, err)
		}
	}
}

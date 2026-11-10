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
	got, err := EvaluateJenkinsWhenContext(`jenkinsChangeRequest("EQUALS","CHANGE_BRANCH","\u00a0")`, nil, map[string]string{"CHANGE_ID": "1", "CHANGE_BRANCH": "\u00a0"}, "")
	if err != nil || !got {
		t.Errorf("changeRequest must not treat non-breaking space as blank: got %v (err=%v)", got, err)
	}
}

// Jenkins Comparator normalizes Java-blank actual values, including changeset paths.
func TestJenkinsComparatorBlankActualValues(t *testing.T) {
	for _, tc := range []struct {
		comparator, pattern, actual string
		want                        bool
	}{
		{"EQUALS", " ", " ", false},
		{"EQUALS", "", " ", true},
		{"REGEXP", " *", " ", true},
		{"GLOB", "*", " ", false},
		{"GLOB", "**", " ", true},
		{"EQUALS", "\u00a0", "\u00a0", true},
		{"EQUALS", "", "\u0085", false},
		{"EQUALS", "", "\u001c", true},
		{"EQUALS", "", "\u2007", false},
		{"EQUALS", "", "\u2008", true},
	} {
		got, err := compareJenkinsString(tc.pattern, tc.actual, tc.comparator, "GLOB")
		if err != nil || got != tc.want {
			t.Errorf("%s pattern %q actual %q: got %v, want %v (err=%v)", tc.comparator, tc.pattern, tc.actual, got, tc.want, err)
		}
	}
	for _, tc := range []struct {
		pattern, comparator string
		want                bool
	}{{"*", "GLOB", false}, {" ", "EQUALS", false}, {"", "EQUALS", true}} {
		got, err := jenkinsMatchesChangeSet(tc.pattern, tc.comparator, false, []jenkinsSCMChange{{Paths: []string{" "}}})
		if err != nil || got != tc.want {
			t.Errorf("changeset %s pattern %q: got %v, want %v (err=%v)", tc.comparator, tc.pattern, got, tc.want, err)
		}
	}
}

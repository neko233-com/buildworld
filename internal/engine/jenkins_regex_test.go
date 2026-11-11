package engine

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestJenkinsRegexBranchMatchesJenkinsOracle(t *testing.T) {
	cases := []struct {
		name    string
		pattern string
		want    bool
	}{
		{"positive lookahead", `release-(?=[0-9]{2}$)[0-9]+`, true},
		{"negative lookahead", `release-(?![0-9]{3}$)[0-9]+`, true},
		{"lookbehind", `release-(?<=-)[0-9]+`, true},
		{"backreference", `release-([0-9])\1`, true},
		{"named backreference", `release-(?<digit>[0-9])\k<digit>`, true},
		{"nonmatch", `release-(?=[0-9]{3}$)[0-9]+`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			condition := fmt.Sprintf("jenkinsBranch(%q,%q)", tc.pattern, "REGEXP")
			got, err := EvaluateJenkinsWhenContext(condition, nil, nil, "release-44")
			if err != nil || got != tc.want {
				t.Fatalf("got %t, err %v, want %t", got, err, tc.want)
			}
		})
	}
}

func TestJenkinsRegexSCMConditions(t *testing.T) {
	changes := []jenkinsSCMChange{{Title: "fix: 123", Comment: "fix: 123", Paths: []string{"src/file.go"}}}
	got, err := jenkinsMatchesChangeSet(`src/(?=file)[a-z]+\.go`, "REGEXP", false, changes)
	if err != nil || !got {
		t.Fatalf("changeset got %t, err %v", got, err)
	}
	got, err = jenkinsMatchesChangeLog(`fix(?=: )[^\n]+`, changes)
	if err != nil || !got {
		t.Fatalf("changelog got %t, err %v", got, err)
	}
}

func TestJenkinsRegexDefaultCharacterClassesUseJavaASCII(t *testing.T) {
	for _, pattern := range []string{`(?=\w)\w+`, `(?=\d)\d+`} {
		value := "é"
		if strings.Contains(pattern, `\d`) {
			value = "٤"
		}
		re, err := compileJenkinsRegex(`\A(?:` + pattern + `)\z`)
		if err != nil {
			t.Fatal(err)
		}
		matched, err := re.match(value)
		if err != nil || matched {
			t.Fatalf("pattern %q matched non-ASCII %q: matched=%t err=%v", pattern, value, matched, err)
		}
	}
	re, err := compileJenkinsRegex(`\A(?:(?=\s)\s+)\z`)
	if err != nil {
		t.Fatal(err)
	}
	if matched, err := re.match(" "); err != nil || !matched {
		t.Fatalf("ASCII whitespace did not match: %t %v", matched, err)
	}
}

func TestJenkinsRegexRejectsInvalidAndBoundsBacktracking(t *testing.T) {
	for _, pattern := range []string{
		`\A(?:(?=)\z`,
		`\A(?:(?=a)\q)\z`,
		`\A(?:(?U)\w+)\z`,
		`\A(?:(?n)(?=a)a)\z`,
		`\A(?:(?P<name>a))\z`,
	} {
		if _, err := compileJenkinsRegex(pattern); err == nil {
			t.Fatalf("unsupported or invalid regex was accepted: %q", pattern)
		}
	}
	re, err := compileJenkinsRegex(`\A(?:(a+)+(?=b)b)\z`)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	matched, err := re.match(strings.Repeat("a", 32768))
	if err == nil || matched {
		t.Fatalf("pathological expression got %t, err %v; want bounded failure", matched, err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("regex exceeded bound: %v", elapsed)
	}
}

func TestJenkinsRegexRejectsUnsafeUnicodeCaseFolding(t *testing.T) {
	for _, pattern := range []string{`\A(?:(?i)é)\z`, `\A(?:(?i)\x{212a})\z`} {
		if _, err := compileJenkinsRegex(pattern); err == nil {
			t.Fatalf("Unicode case-folding pattern was accepted: %q", pattern)
		}
	}
	re, err := compileJenkinsRegex(`\A(?:(?i)(?=a)a)\z`)
	if err != nil {
		t.Fatal(err)
	}
	if matched, err := re.match("A"); err != nil || !matched {
		t.Fatalf("ASCII case-insensitive match failed: matched=%t err=%v", matched, err)
	}
	if _, err := re.match("K"); err == nil {
		t.Fatal("non-ASCII case folding must fail instead of guessing")
	}
}

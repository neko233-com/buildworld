package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"
	"unicode/utf8"

	"github.com/neko233-com/buildworld/internal/git"
	"github.com/neko233-com/buildworld/internal/processtree"
	"github.com/neko233-com/buildworld/internal/store"
)

const jenkinsSCMChangeLogMaxBytes = 16 << 20

type jenkinsSCMCaptureContextKey struct{}

type jenkinsSCMRunState struct {
	capture         bool
	checkoutVisible atomic.Bool
}

func pipelineUsesJenkinsSCMChanges(config *BuildConfig) bool {
	for _, stage := range config.Stages {
		if jenkinsWhenUsesSCMChanges(stage.JenkinsWhen) {
			return true
		}
	}
	return false
}

func jenkinsWhenUsesSCMChanges(condition string) bool {
	return strings.Contains(condition, "jenkinsChangeSet(") || strings.Contains(condition, "jenkinsChangeLog(")
}

type jenkinsSCMChange struct {
	Revision string   `json:"revision"`
	Title    string   `json:"title"`
	Comment  string   `json:"comment"`
	Paths    []string `json:"paths"`
}

type jenkinsSCMHeadChanges struct {
	Version int                `json:"version"`
	Changes []jenkinsSCMChange `json:"changes"`
}

// jenkinsPullRequestHead identifies a provider's explicit PR-head ref, not a
// user-supplied CHANGE_ID environment value.
func jenkinsPullRequestHead(ref string) (string, bool) {
	if !strings.HasPrefix(ref, "refs/pull/") || !strings.HasSuffix(ref, "/head") {
		return "", false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(ref, "refs/pull/"), "/head")
	if len(id) == 0 || len(id) > 20 || id[0] == '0' {
		return "", false
	}
	for _, ch := range id {
		if ch < '0' || ch > '9' {
			return "", false
		}
	}
	return id, true
}

// jenkinsPullRequestChanges aggregates only snapshots that explicitly record
// that changelog capture was enabled for that SCM head.
func jenkinsPullRequestChanges(records []store.JenkinsSCMCheckout) (string, bool, error) {
	changes := make([]jenkinsSCMChange, 0)
	for _, record := range records {
		if !record.HistoryKnown {
			return "", true, nil
		}
		var captured jenkinsSCMHeadChanges
		if err := json.Unmarshal([]byte(record.ChangesJSON), &captured); err != nil || captured.Version != 1 || captured.Changes == nil {
			return "", true, nil
		}
		changes = append(changes, captured.Changes...)
	}
	data, err := json.Marshal(changes)
	if err != nil {
		return "", false, fmt.Errorf("encode Jenkins SCM head history: %w", err)
	}
	if len(data) > jenkinsSCMChangeLogMaxBytes {
		return "", false, fmt.Errorf("Jenkins SCM head history exceeds %d bytes", jenkinsSCMChangeLogMaxBytes)
	}
	return string(data), false, nil
}

type boundedSCMOutput struct{ bytes.Buffer }

func (b *boundedSCMOutput) Write(data []byte) (int, error) {
	if len(data) > jenkinsSCMChangeLogMaxBytes-b.Len() {
		return 0, fmt.Errorf("Jenkins SCM changelog exceeds %d bytes", jenkinsSCMChangeLogMaxBytes)
	}
	return b.Buffer.Write(data)
}

// collectJenkinsSCMChanges follows the Git plugin's non-merge commit range.
// A missing baseline is the first observed checkout and has an empty changelog.
func collectJenkinsSCMChanges(ctx context.Context, workspace, revision, baseline string) ([]jenkinsSCMChange, error) {
	if baseline == "" || revision == baseline {
		return []jenkinsSCMChange{}, nil
	}
	if !validSCMRevision(revision) || !validSCMRevision(baseline) || (len(revision) != 40 && len(revision) != 64) || (len(baseline) != 40 && len(baseline) != 64) {
		return nil, fmt.Errorf("Jenkins SCM revisions must be full Git object IDs")
	}
	args := []string{"-C", workspace, "log", "--no-merges", "--no-renames", "--name-only", "-z", "--encoding=UTF-8", "--max-count=1024", "--format=format:%x00%H%x00%B%x00", revision, "--not", baseline, "--"}
	cmd := processtree.CommandContext(ctx, "git", args...)
	cmd.Env = git.NonInteractiveEnvironment(cmd.Environ())
	var output boundedSCMOutput
	cmd.Stdout = &output
	var stderr boundedSCMOutput
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("read Jenkins SCM changelog: %w: %s", err, stderr.String())
	}
	return parseJenkinsSCMLog(output.Bytes())
}

func parseJenkinsSCMLog(data []byte) ([]jenkinsSCMChange, error) {
	tokens := bytes.Split(data, []byte{0})
	changes := make([]jenkinsSCMChange, 0)
	for cursor := 0; cursor < len(tokens); {
		for cursor < len(tokens) && len(tokens[cursor]) == 0 {
			cursor++
		}
		if cursor == len(tokens) {
			break
		}
		revision := string(tokens[cursor])
		if !validSCMRevision(revision) || (len(revision) != 40 && len(revision) != 64) || cursor+1 >= len(tokens) || !utf8.Valid(tokens[cursor+1]) {
			return nil, fmt.Errorf("invalid Jenkins SCM changelog record")
		}
		comment := string(tokens[cursor+1])
		title := strings.TrimSpace(strings.SplitN(comment, "\n", 2)[0])
		change := jenkinsSCMChange{Revision: revision, Title: title, Comment: comment, Paths: []string{}}
		cursor += 2
		first := true
		for cursor < len(tokens) && len(tokens[cursor]) > 0 {
			path := tokens[cursor]
			if first {
				if path[0] != '\n' {
					return nil, fmt.Errorf("invalid Jenkins SCM path separator")
				}
				path = path[1:]
				first = false
			}
			if len(path) == 0 || !utf8.Valid(path) {
				return nil, fmt.Errorf("invalid Jenkins SCM path")
			}
			change.Paths = append(change.Paths, string(path))
			cursor++
		}
		changes = append(changes, change)
	}
	return changes, nil
}

func jenkinsMatchesChangeSet(pattern, comparator string, caseSensitive bool, changes []jenkinsSCMChange) (bool, error) {
	comparator = strings.ToUpper(comparator)
	if comparator != "EQUALS" && comparator != "REGEXP" {
		comparator = "GLOB"
	}
	var glob *jenkinsPathPattern
	var expression *regexp.Regexp
	for _, change := range changes {
		for _, path := range change.Paths {
			var matched bool
			var err error
			switch comparator {
			case "GLOB":
				if glob == nil {
					glob, err = compileJenkinsPathPattern(pattern, caseSensitive)
				}
				if err == nil {
					matched = glob.matches(path)
				}
			case "REGEXP":
				if expression == nil {
					expression, err = regexp.Compile(`\A(?:` + pattern + `)\z`)
				}
				if err == nil {
					matched = expression.MatchString(path)
				}
			default:
				matched = pattern == path
			}
			if err != nil || matched {
				return matched, err
			}
		}
	}
	return false, nil
}

func jenkinsMatchesChangeLog(pattern string, changes []jenkinsSCMChange) (bool, error) {
	titlePattern, err := regexp.Compile(`\A(?:` + pattern + `)\z`)
	if err != nil {
		return false, err
	}
	commentPattern, err := regexp.Compile(`(?ms)\A(?:[^\r\n]*?` + pattern + `[^\r\n]*?)\z`)
	if err != nil {
		return false, err
	}
	for _, change := range changes {
		if titlePattern.MatchString(change.Title) || commentPattern.MatchString(change.Comment) {
			return true, nil
		}
	}
	return false, nil
}

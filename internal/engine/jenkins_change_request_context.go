package engine

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/neko233-com/buildworld/internal/store"
)

// A NUL-containing name cannot be exposed as a process environment variable.
const jenkinsChangeRequestContextKey = "\x00buildworld.changeRequest"
const jenkinsTriggerUserContextKey = "\x00buildworld.triggerUser"
const jenkinsRetryContextKey = "\x00buildworld.retry"

type jenkinsRetryContext struct {
	Kind string `json:"kind"`
	User string `json:"user,omitempty"`
}

// AttachJenkinsRetryContext records the actual retry initiator without
// modifying user parameters or the authenticated change-request metadata.
func AttachJenkinsRetryContext(parameters, kind, username string) (string, error) {
	if (kind != "manual" && kind != "automatic") || (kind == "automatic" && username != "") || len(username) > 2048 || strings.ContainsRune(username, '\x00') {
		return "", fmt.Errorf("invalid retry context")
	}
	fields := make(map[string]json.RawMessage)
	if parameters != "" {
		if err := json.Unmarshal([]byte(parameters), &fields); err != nil || fields == nil {
			return "", fmt.Errorf("invalid retry parameters")
		}
	}
	encoded, err := json.Marshal(jenkinsRetryContext{Kind: kind, User: username})
	if err != nil {
		return "", err
	}
	fields[jenkinsRetryContextKey] = encoded
	result, err := json.Marshal(fields)
	return string(result), err
}

func readJenkinsRetryContext(parameters string) *jenkinsRetryContext {
	var fields map[string]json.RawMessage
	var retry jenkinsRetryContext
	if json.Unmarshal([]byte(parameters), &fields) != nil || json.Unmarshal(fields[jenkinsRetryContextKey], &retry) != nil ||
		(retry.Kind != "manual" && retry.Kind != "automatic") || (retry.Kind == "automatic" && retry.User != "") || len(retry.User) > 2048 || strings.ContainsRune(retry.User, '\x00') {
		return nil
	}
	return &retry
}

func comparableRetryParameters(parameters string) (string, error) {
	fields := make(map[string]json.RawMessage)
	if parameters != "" {
		if err := json.Unmarshal([]byte(parameters), &fields); err != nil || fields == nil {
			return "", fmt.Errorf("invalid retry parameters")
		}
	}
	delete(fields, jenkinsRetryContextKey)
	encoded, err := json.Marshal(fields)
	return string(encoded), err
}

// JenkinsChangeRequestContext is the authenticated, pinned webhook metadata.
type JenkinsChangeRequestContext struct {
	ID                string `json:"id"`
	Target            string `json:"target"`
	Branch            string `json:"branch"`
	Title             string `json:"title"`
	Author            string `json:"author"`
	AuthorDisplayName string `json:"author_display_name,omitempty"`
}

// AttachJenkinsChangeRequestContext preserves validated parameters and adds the
// provider metadata in a reserved field that cannot be a shell variable.
func AttachJenkinsChangeRequestContext(parameters string, change JenkinsChangeRequestContext) (string, error) {
	if change.ID == "" {
		return "", fmt.Errorf("missing PR identity")
	}
	for _, value := range []string{change.ID, change.Target, change.Branch, change.Title, change.Author, change.AuthorDisplayName} {
		if len(value) > 2048 || strings.ContainsRune(value, '\x00') {
			return "", fmt.Errorf("invalid PR context")
		}
	}
	fields := make(map[string]json.RawMessage)
	if parameters != "" {
		if err := json.Unmarshal([]byte(parameters), &fields); err != nil || fields == nil {
			return "", fmt.Errorf("invalid build parameters for PR context")
		}
	}
	if _, exists := fields[jenkinsChangeRequestContextKey]; exists {
		return "", fmt.Errorf("reserved PR context already exists")
	}
	encoded, err := json.Marshal(change)
	if err != nil {
		return "", err
	}
	fields[jenkinsChangeRequestContextKey] = encoded
	result, err := json.Marshal(fields)
	return string(result), err
}

func readJenkinsChangeRequestContext(parameters, expectedID string) *JenkinsChangeRequestContext {
	var fields map[string]json.RawMessage
	if json.Unmarshal([]byte(parameters), &fields) != nil {
		return nil
	}
	var change JenkinsChangeRequestContext
	if json.Unmarshal(fields[jenkinsChangeRequestContextKey], &change) != nil || change.ID != expectedID {
		return nil
	}
	return &change
}

// PublicBuildParameters removes internal webhook metadata before user parameter validation.
func PublicBuildParameters(parameters map[string]interface{}) {
	delete(parameters, jenkinsChangeRequestContextKey)
	delete(parameters, jenkinsTriggerUserContextKey)
	delete(parameters, jenkinsRetryContextKey)
}

// AttachJenkinsTriggerUser records the authenticated user at build creation.
func AttachJenkinsTriggerUser(parameters, username string) (string, error) {
	if username == "" || len(username) > 2048 || strings.ContainsRune(username, '\x00') {
		return "", fmt.Errorf("invalid trigger user")
	}
	fields := make(map[string]json.RawMessage)
	if parameters != "" {
		if err := json.Unmarshal([]byte(parameters), &fields); err != nil || fields == nil {
			return "", fmt.Errorf("invalid trigger parameters")
		}
	}
	if _, exists := fields[jenkinsTriggerUserContextKey]; exists {
		return "", fmt.Errorf("reserved trigger context already exists")
	}
	encoded, err := json.Marshal(username)
	if err != nil {
		return "", err
	}
	fields[jenkinsTriggerUserContextKey] = encoded
	result, err := json.Marshal(fields)
	return string(result), err
}

func readJenkinsTriggerUser(parameters string) string {
	var fields map[string]json.RawMessage
	var username string
	if json.Unmarshal([]byte(parameters), &fields) != nil || json.Unmarshal(fields[jenkinsTriggerUserContextKey], &username) != nil {
		return ""
	}
	return username
}

func (r *BuildRunner) retryLineage(build *store.Build) ([]*store.Build, error) {
	current := build
	seen := make(map[int64]bool)
	lineage := []*store.Build{build}
	baseline, err := comparableRetryParameters(build.Parameters)
	if err != nil {
		return nil, err
	}
	for current.Trigger == "retry" {
		if current.RetriedFrom == nil || seen[current.ID] || len(seen) >= 128 {
			return nil, fmt.Errorf("invalid retry lineage")
		}
		seen[current.ID] = true
		parent, err := r.store.GetBuild(*current.RetriedFrom)
		if err != nil || parent == nil || parent.ProjectID != build.ProjectID || parent.Branch != build.Branch || parent.CommitSHA != build.CommitSHA {
			return nil, fmt.Errorf("unavailable or inconsistent retry lineage")
		}
		parentParameters, err := comparableRetryParameters(parent.Parameters)
		if err != nil || parentParameters != baseline {
			return nil, fmt.Errorf("inconsistent retry parameters")
		}
		current = parent
		lineage = append(lineage, parent)
	}
	return lineage, nil
}

func (r *BuildRunner) changeRequestWebhookOrigin(build *store.Build) (bool, error) {
	lineage, err := r.retryLineage(build)
	if err != nil {
		return false, err
	}
	return lineage[len(lineage)-1].Trigger == "webhook", nil
}

type jenkinsTriggerCause struct {
	Class string `json:"class"`
	User  string `json:"user,omitempty"`
}

type jenkinsRetryCauses struct {
	Causes         []jenkinsTriggerCause `json:"causes"`
	UnknownNonUser bool                  `json:"unknown_non_user"`
	HasManualRetry bool                  `json:"has_manual_retry"`
}

func (r *BuildRunner) resolveJenkinsRetryCauses(build *store.Build) (jenkinsRetryCauses, error) {
	var result jenkinsRetryCauses
	lineage, err := r.retryLineage(build)
	if err != nil {
		return result, err
	}
	manualUser := ""
	for _, item := range lineage[:len(lineage)-1] {
		retry := readJenkinsRetryContext(item.Parameters)
		if retry == nil {
			return result, fmt.Errorf("retry initiator is unavailable")
		}
		if retry.Kind == "manual" && !result.HasManualRetry {
			result.HasManualRetry, manualUser = true, retry.User
		}
	}
	root := lineage[len(lineage)-1]
	switch root.Trigger {
	case "schedule":
		result.Causes = append(result.Causes, jenkinsTriggerCause{Class: "TimerTriggerCause"})
	case "vcs":
		result.Causes = append(result.Causes, jenkinsTriggerCause{Class: "SCMTriggerCause"})
	case "finish":
		result.Causes = append(result.Causes, jenkinsTriggerCause{Class: "BuildUpstreamCause"})
	case "http":
		result.Causes = append(result.Causes, jenkinsTriggerCause{Class: "RemoteCause"})
	case "manual", "api":
		if !result.HasManualRetry {
			result.Causes = append(result.Causes, jenkinsTriggerCause{Class: "UserIdCause", User: readJenkinsTriggerUser(root.Parameters)})
		}
	default:
		result.UnknownNonUser = true
	}
	if result.HasManualRetry {
		result.Causes = append(result.Causes, jenkinsTriggerCause{Class: "UserIdCause", User: manualUser}, jenkinsTriggerCause{Class: "RebuildCause"})
	}
	return result, nil
}

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

func (r *BuildRunner) changeRequestWebhookOrigin(build *store.Build) (bool, error) {
	current := build
	seen := make(map[int64]bool)
	for current.Trigger == "retry" {
		if current.RetriedFrom == nil || seen[current.ID] || len(seen) >= 128 {
			return false, fmt.Errorf("invalid change request retry lineage")
		}
		seen[current.ID] = true
		parent, err := r.store.GetBuild(*current.RetriedFrom)
		if err != nil || parent == nil || parent.ProjectID != build.ProjectID || parent.Branch != build.Branch || parent.CommitSHA != build.CommitSHA || parent.Parameters != build.Parameters {
			return false, fmt.Errorf("unavailable or inconsistent change request retry lineage")
		}
		current = parent
	}
	return current.Trigger == "webhook", nil
}

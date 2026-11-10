package engine

import (
	"encoding/json"
	"fmt"
	"strings"
)

// A NUL-containing name cannot be exposed as a process environment variable.
const jenkinsChangeRequestContextKey = "\x00buildworld.changeRequest"

// JenkinsChangeRequestContext is the authenticated, pinned webhook metadata.
type JenkinsChangeRequestContext struct {
	ID     string `json:"id"`
	Target string `json:"target"`
	Branch string `json:"branch"`
	Title  string `json:"title"`
	Author string `json:"author"`
}

// AttachJenkinsChangeRequestContext preserves validated parameters and adds the
// provider metadata in a reserved field that cannot be a shell variable.
func AttachJenkinsChangeRequestContext(parameters string, change JenkinsChangeRequestContext) (string, error) {
	if change.ID == "" {
		return "", fmt.Errorf("missing PR identity")
	}
	for _, value := range []string{change.ID, change.Target, change.Branch, change.Title, change.Author} {
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

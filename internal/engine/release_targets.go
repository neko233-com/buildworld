package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/neko233-com/buildworld/internal/releasemeta"
	"github.com/neko233-com/buildworld/internal/store"
)

// ReleaseTargetsVariable is project-owned public metadata, never a secret or global fallback.
const ReleaseTargetsVariable = "RELEASE_TARGETS_JSON"

type ReleaseTarget struct {
	Channel          string `json:"channel"`
	Environment      string `json:"environment"`
	ChannelLabel     string `json:"channelLabel,omitempty"`
	EnvironmentLabel string `json:"environmentLabel,omitempty"`
}

type ReleaseTargetCatalog struct {
	SchemaVersion int             `json:"schemaVersion"`
	Targets       []ReleaseTarget `json:"targets"`
}

func ScopedReleaseParameters(parameters []BuildParameter) bool {
	for _, parameter := range parameters {
		if name := strings.TrimSpace(parameter.Name); name == "RELEASE_CHANNEL" || name == "RELEASE_ENVIRONMENT" {
			return true
		}
	}
	return false
}

func validReleaseLabel(label string) bool {
	if utf8.RuneCountInString(label) > 80 || strings.TrimSpace(label) != label {
		return false
	}
	for _, character := range label {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func ParseReleaseTargetCatalog(value string) (*ReleaseTargetCatalog, error) {
	invalid := errors.New("RELEASE_TARGETS_JSON must contain a valid public schemaVersion=1 target catalog")
	if len(value) == 0 || len(value) > 65536 {
		return nil, invalid
	}
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.DisallowUnknownFields()
	var catalog ReleaseTargetCatalog
	if decoder.Decode(&catalog) != nil || decoder.Decode(new(any)) != io.EOF || catalog.SchemaVersion != 1 || len(catalog.Targets) == 0 || len(catalog.Targets) > 512 {
		return nil, invalid
	}
	seen := make(map[string]bool)
	channelLabels := make(map[string]string)
	for _, target := range catalog.Targets {
		key := target.Channel + "/" + target.Environment
		if !releasemeta.ValidEnvironment(target.Channel) || !releasemeta.ValidEnvironment(target.Environment) || seen[key] || !validReleaseLabel(target.ChannelLabel) || !validReleaseLabel(target.EnvironmentLabel) {
			return nil, invalid
		}
		if label, exists := channelLabels[target.Channel]; exists && label != target.ChannelLabel {
			return nil, invalid
		}
		seen[key] = true
		channelLabels[target.Channel] = target.ChannelLabel
	}
	return &catalog, nil
}

// ApplyProjectReleaseTargets validates the public matrix and makes its candidates
// authoritative. Parameter declarations retain their type and secrecy contract.
func ApplyProjectReleaseTargets(config *BuildConfig, variables []*store.EnvVar) error {
	var declaration *store.EnvVar
	for _, variable := range variables {
		if variable != nil && variable.Name == ReleaseTargetsVariable {
			if declaration != nil {
				return errors.New("RELEASE_TARGETS_JSON must have one project declaration")
			}
			declaration = variable
		}
	}
	if declaration == nil {
		return nil
	}
	if declaration.IsSecret || declaration.Scope != "project" {
		return errors.New("RELEASE_TARGETS_JSON must be a public project variable")
	}
	catalog, err := ParseReleaseTargetCatalog(declaration.Value)
	if err != nil {
		return err
	}
	channelIndex, environmentIndex := -1, -1
	for index, parameter := range config.Parameters {
		switch strings.TrimSpace(parameter.Name) {
		case "RELEASE_SHARED_SERVICE":
			return errors.New("shared release pipelines cannot declare a scoped target catalog")
		case "RELEASE_CHANNEL":
			if channelIndex >= 0 || parameter.IsSecret || !strings.EqualFold(strings.TrimSpace(parameter.Type), "choice") {
				return errors.New("RELEASE_CHANNEL must be one public choice declaration")
			}
			channelIndex = index
		case "RELEASE_ENVIRONMENT":
			if environmentIndex >= 0 || parameter.IsSecret || (!strings.EqualFold(strings.TrimSpace(parameter.Type), "string") && !strings.EqualFold(strings.TrimSpace(parameter.Type), "choice")) {
				return errors.New("RELEASE_ENVIRONMENT must be one public string or choice declaration")
			}
			environmentIndex = index
		}
	}
	if channelIndex < 0 || environmentIndex < 0 {
		return errors.New("scoped target catalogs require declared RELEASE_CHANNEL and RELEASE_ENVIRONMENT")
	}
	channels, environments := []string{}, []string{}
	channelSeen, environmentSeen := map[string]bool{}, map[string]bool{}
	for _, target := range catalog.Targets {
		if !channelSeen[target.Channel] {
			channels = append(channels, target.Channel)
			channelSeen[target.Channel] = true
		}
		if !environmentSeen[target.Environment] {
			environments = append(environments, target.Environment)
			environmentSeen[target.Environment] = true
		}
	}
	channel, _ := config.Parameters[channelIndex].Default.(string)
	if !channelSeen[channel] {
		channel = channels[0]
	}
	environment, _ := config.Parameters[environmentIndex].Default.(string)
	firstEnvironment := ""
	defaultExists, testExists := false, false
	for _, target := range catalog.Targets {
		if target.Channel != channel {
			continue
		}
		if firstEnvironment == "" {
			firstEnvironment = target.Environment
		}
		defaultExists = defaultExists || target.Environment == environment
		testExists = testExists || target.Environment == "test"
	}
	if testExists {
		environment = "test"
	} else if !defaultExists {
		environment = firstEnvironment
	}
	config.Parameters = append([]BuildParameter(nil), config.Parameters...)
	config.Parameters[channelIndex].Choices = channels
	config.Parameters[channelIndex].Default = channel
	config.Parameters[environmentIndex].Choices = environments
	config.Parameters[environmentIndex].Default = environment
	config.ReleaseTargets = catalog
	return nil
}

// ValidateConfiguredReleaseTarget rejects missing or stale combinations. History
// and pipeline choices do not establish that a pair is configured for deployment.
func ValidateConfiguredReleaseTarget(config *BuildConfig, parameters map[string]interface{}) error {
	if !ScopedReleaseParameters(config.Parameters) {
		return nil
	}
	if config.ReleaseTargets == nil {
		return errors.New("scoped releases require a public project RELEASE_TARGETS_JSON catalog")
	}
	channel, _ := parameters["RELEASE_CHANNEL"].(string)
	environment, _ := parameters["RELEASE_ENVIRONMENT"].(string)
	for _, target := range config.ReleaseTargets.Targets {
		if target.Channel == channel && target.Environment == environment {
			return nil
		}
	}
	return errors.New("selected channel and environment are not a configured deployment target")
}

// releaseParameterEnvironment is the final shell boundary: stage and step
// overlays cannot replace a persisted release selection. Values remain literal,
// and unrelated variables preserve the ordinary pipeline environment contract.
func releaseParameterEnvironment(environment []string, parameters map[string]interface{}) []string {
	names := []string{"RELEASE_CHANNEL", "RELEASE_ENVIRONMENT", "RELEASE_SHARED_SERVICE", "RELEASE_ACTION", "RELEASE_VERSION", "CONFIRM_PUBLISH_LATEST"}
	values := make(map[string]string, len(names))
	for _, name := range names {
		if value, exists := parameters[name]; exists {
			values[name] = fmt.Sprint(value)
		}
	}
	if len(values) == 0 {
		return environment
	}
	result := make([]string, 0, len(environment)+len(values))
	for _, entry := range environment {
		name, _, _ := strings.Cut(entry, "=")
		if _, reserved := values[name]; !reserved {
			result = append(result, entry)
		}
	}
	for _, name := range names {
		if value, exists := values[name]; exists {
			result = append(result, name+"="+value)
		}
	}
	return result
}

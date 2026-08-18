package engine

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/neko233-com/buildworld/internal/store"
)

const (
	maskedSecretValue  = "********"
	minMaskedSecretLen = 4
)

// secretMasker is immutable after construction and safe for concurrent use.
// Longest values are replaced first so overlapping credentials cannot leave a
// suffix visible after a shorter value has already been masked.
type secretMasker struct {
	secrets  []string
	replacer *strings.Replacer
}

func newSecretMasker(values ...string) *secretMasker {
	unique := make(map[string]struct{}, len(values)*2)
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if utf8.RuneCountInString(trimmed) < minMaskedSecretLen {
			continue
		}
		addMaskableSecret(unique, value)
		if trimmed != value {
			addMaskableSecret(unique, trimmed)
		}
		if strings.Contains(value, "\r\n") {
			addMaskableSecret(unique, strings.ReplaceAll(value, "\r\n", "\n"))
		}
	}
	secrets := make([]string, 0, len(unique))
	for secret := range unique {
		secrets = append(secrets, secret)
	}
	sort.Slice(secrets, func(i, j int) bool {
		if len(secrets[i]) == len(secrets[j]) {
			return secrets[i] < secrets[j]
		}
		return len(secrets[i]) > len(secrets[j])
	})
	pairs := make([]string, 0, len(secrets)*2)
	for _, secret := range secrets {
		pairs = append(pairs, secret, maskedSecretValue)
	}
	masker := &secretMasker{secrets: secrets}
	if len(pairs) > 0 {
		masker.replacer = strings.NewReplacer(pairs...)
	}
	return masker
}

func addMaskableSecret(values map[string]struct{}, value string) {
	if utf8.RuneCountInString(value) < minMaskedSecretLen {
		return
	}
	values[value] = struct{}{}
}

func (m *secretMasker) Mask(value string) string {
	if m == nil || m.replacer == nil || value == "" {
		return value
	}
	return m.replacer.Replace(value)
}

// streamingSecretMasker delays only a trailing fragment that could still
// become a secret. Complete safe output remains live; a secret split across
// arbitrary stdout/gRPC chunks can never be published piecemeal.
type streamingSecretMasker struct {
	mu      sync.Mutex
	masker  *secretMasker
	pending string
}

func newStreamingSecretMasker(masker *secretMasker) *streamingSecretMasker {
	return &streamingSecretMasker{masker: masker}
}

func (m *streamingSecretMasker) Write(chunk string) string {
	if m == nil {
		return chunk
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	combined := m.pending + chunk
	if m.masker == nil || len(m.masker.secrets) == 0 {
		m.pending = ""
		return combined
	}
	var safe strings.Builder
	position := 0
	for position < len(combined) {
		remaining := combined[position:]
		matched := ""
		for _, secret := range m.masker.secrets {
			if len(remaining) >= len(secret) && strings.HasPrefix(remaining, secret) {
				matched = secret
				break
			}
		}
		if matched != "" {
			safe.WriteString(maskedSecretValue)
			position += len(matched)
			continue
		}
		if isSecretPrefix(remaining, m.masker.secrets) || isIncompleteUTF8Prefix(remaining) {
			break
		}
		safe.WriteByte(combined[position])
		position++
	}
	m.pending = combined[position:]
	return safe.String()
}

func (m *streamingSecretMasker) Flush() string {
	if m == nil {
		return ""
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	pending := m.masker.Mask(m.pending)
	m.pending = ""
	return pending
}

func isSecretPrefix(value string, secrets []string) bool {
	for _, secret := range secrets {
		if len(value) < len(secret) && strings.HasPrefix(secret, value) {
			return true
		}
	}
	return false
}

func isIncompleteUTF8Prefix(value string) bool {
	if value == "" {
		return false
	}
	expected := 0
	switch first := value[0]; {
	case first >= 0xc2 && first <= 0xdf:
		expected = 2
	case first >= 0xe0 && first <= 0xef:
		expected = 3
	case first >= 0xf0 && first <= 0xf4:
		expected = 4
	default:
		return false
	}
	if len(value) >= expected {
		return false
	}
	for _, continuation := range []byte(value[1:]) {
		if continuation < 0x80 || continuation > 0xbf {
			return false
		}
	}
	return true
}

type buildSecretMasker struct {
	literal *secretMasker
	stream  *streamingSecretMasker
}

func (r *BuildRunner) configureBuildSecretMasker(build *store.Build, project *store.Project, config *BuildConfig) {
	if r == nil || r.store == nil || build == nil || project == nil || config == nil {
		return
	}
	values := r.collectBuildSecretValues(build, project, config)
	literal := newSecretMasker(values...)
	masker := &buildSecretMasker{literal: literal, stream: newStreamingSecretMasker(literal)}
	r.secretMaskersMu.Lock()
	if r.secretMaskers == nil {
		r.secretMaskers = make(map[int64]*buildSecretMasker)
	}
	r.secretMaskers[build.ID] = masker
	r.secretMaskersMu.Unlock()
}

func (r *BuildRunner) collectBuildSecretValues(build *store.Build, project *store.Project, config *BuildConfig) []string {
	values := make([]string, 0)
	secretNames := make(map[string]struct{})
	appendEnvSecrets := func(scope string, projectID *int64) {
		variables, err := r.store.ListEnvVars(scope, projectID)
		if err != nil {
			return
		}
		for _, variable := range variables {
			if variable != nil && variable.IsSecret {
				secretNames[variable.Name] = struct{}{}
				values = append(values, variable.Value)
			}
		}
	}
	appendEnvSecrets("global", nil)
	projectID := project.ID
	appendEnvSecrets("project", &projectID)

	// Also mask the effective expanded value. A secret env var may itself refer
	// to another variable, while the child process receives only the expansion.
	for _, entry := range r.buildEnv(build, config, project) {
		name, value, found := strings.Cut(entry, "=")
		if !found {
			continue
		}
		if _, secret := secretNames[name]; secret {
			values = append(values, value)
		}
	}

	params := parseParams(build.Parameters)
	for _, parameter := range config.Parameters {
		if !parameter.IsSecret && !strings.EqualFold(strings.TrimSpace(parameter.Type), "password") {
			continue
		}
		if value, exists := params[parameter.Name]; exists && value != nil {
			values = append(values, stringifySecretValue(value))
		} else if parameter.Default != nil {
			values = append(values, stringifySecretValue(parameter.Default))
		}
	}

	values = append(values, repositoryURLSecrets(project.RepoURL)...)
	if project.VCSRootID != nil {
		if root, err := r.store.GetVCSRoot(*project.VCSRootID); err == nil {
			values = append(values, repositoryURLSecrets(root.URL)...)
			if root.CredentialID != nil {
				if credential, err := r.store.GetCredential(*root.CredentialID); err == nil {
					values = append(values, credential.Password, credential.PrivateKey, credential.Token)
				}
			}
		}
	}
	return values
}

func stringifySecretValue(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func repositoryURLSecrets(raw string) []string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.User == nil {
		return nil
	}
	password, hasPassword := parsed.User.Password()
	if hasPassword {
		values := []string{password}
		if _, escaped, found := strings.Cut(parsed.User.String(), ":"); found {
			values = append(values, escaped)
		}
		return values
	}
	// Token-only HTTPS URLs commonly place the token in the user field.
	return []string{parsed.User.Username(), parsed.User.String()}
}

func (r *BuildRunner) buildSecretMasker(buildID int64) *buildSecretMasker {
	r.secretMaskersMu.Lock()
	masker := r.secretMaskers[buildID]
	r.secretMaskersMu.Unlock()
	return masker
}

func (r *BuildRunner) clearBuildSecretMasker(buildID int64) {
	r.secretMaskersMu.Lock()
	delete(r.secretMaskers, buildID)
	r.secretMaskersMu.Unlock()
}

func (r *BuildRunner) maskBuildText(buildID int64, value string) string {
	masker := r.buildSecretMasker(buildID)
	if masker == nil {
		return value
	}
	return masker.literal.Mask(value)
}

func (r *BuildRunner) logBuildOutput(buildID int64, stage, chunk string) error {
	masker := r.buildSecretMasker(buildID)
	if masker == nil {
		return r.log(buildID, stage, chunk)
	}
	if safe := masker.stream.Write(chunk); safe != "" {
		return r.logMasked(buildID, masker.literal.Mask(stage), safe)
	}
	return nil
}

// logBuildLine marks executor output as a complete line before streaming
// secret masking. Without the terminator, a final character such as the "r"
// in "server" can look like a credential prefix and be emitted later as a
// separate console line during masker flush.
func (r *BuildRunner) logBuildLine(buildID int64, stage, line string) error {
	if !strings.HasSuffix(line, "\n") {
		line += "\n"
	}
	masker := r.buildSecretMasker(buildID)
	if masker == nil {
		return r.log(buildID, stage, strings.TrimSuffix(line, "\n"))
	}
	safe := strings.TrimSuffix(masker.stream.Write(line), "\n")
	if safe == "" {
		return nil
	}
	return r.logMasked(buildID, masker.literal.Mask(stage), safe)
}

func (r *BuildRunner) flushBuildOutput(buildID int64, stage string) error {
	masker := r.buildSecretMasker(buildID)
	if masker == nil {
		return nil
	}
	if safe := masker.stream.Flush(); safe != "" {
		return r.logMasked(buildID, masker.literal.Mask(stage), safe)
	}
	return nil
}

func (r *BuildRunner) maskedNotificationObjects(buildID int64, build *store.Build, project *store.Project) (*store.Build, *store.Project) {
	if build == nil || project == nil {
		return build, project
	}
	buildCopy := *build
	buildCopy.Status = r.maskBuildText(buildID, build.Status)
	buildCopy.Trigger = r.maskBuildText(buildID, build.Trigger)
	buildCopy.Branch = r.maskBuildText(buildID, build.Branch)
	buildCopy.CommitSHA = r.maskBuildText(buildID, build.CommitSHA)
	buildCopy.Parameters = r.maskBuildText(buildID, build.Parameters)
	buildCopy.Log = r.maskBuildText(buildID, build.Log)
	projectCopy := *project
	projectCopy.Name = r.maskBuildText(buildID, project.Name)
	projectCopy.Description = r.maskBuildText(buildID, project.Description)
	projectCopy.RepoURL = r.maskBuildText(buildID, project.RepoURL)
	projectCopy.DefaultBranch = r.maskBuildText(buildID, project.DefaultBranch)
	projectCopy.Config = r.maskBuildText(buildID, project.Config)
	projectCopy.Tags = append([]string(nil), project.Tags...)
	for index := range projectCopy.Tags {
		projectCopy.Tags[index] = r.maskBuildText(buildID, projectCopy.Tags[index])
	}
	return &buildCopy, &projectCopy
}

func (r *BuildRunner) sendBuildEventAsync(buildID int64, build *store.Build, project *store.Project, event string) {
	if r.notificationService == nil || build == nil || project == nil {
		return
	}
	safeBuild, safeProject := r.maskedNotificationObjects(buildID, build, project)
	go r.notificationService.SendBuildEvent(safeBuild, safeProject, event)
}

package releasemeta

import "regexp"

var environmentName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

// ValidEnvironment accepts an environment identifier, not a filesystem path.
// Configured target existence remains the release pipeline's responsibility.
func ValidEnvironment(value string) bool {
	return environmentName.MatchString(value)
}

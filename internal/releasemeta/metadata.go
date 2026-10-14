package releasemeta

import "regexp"

var environmentName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

var releaseVersion = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

// ValidRecord accepts only a complete scoped release record with its exact identity.
func ValidRecord(branch, channel, environment, component, version string) bool {
	return ValidEnvironment(channel) && ValidEnvironment(environment) && ValidEnvironment(component) && releaseVersion.MatchString(version) && branch == "release/"+channel+"/"+environment+"/"+component+"/"+version
}

// ValidEnvironment accepts an environment identifier, not a filesystem path.
// Configured target existence remains the release pipeline's responsibility.
func ValidEnvironment(value string) bool {
	return environmentName.MatchString(value)
}

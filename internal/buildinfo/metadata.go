package buildinfo

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var commitPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

// ReleaseCommit reads the commit recorded beside a packaged server binary.
// Invalid, missing, or version-mismatched metadata never identifies a release.
func ReleaseCommit(executable, version string) string {
	if executable == "" || version == "" || version == "dev" {
		return ""
	}
	file, err := os.Open(filepath.Join(filepath.Dir(executable), "release-info.json"))
	if err != nil {
		return ""
	}
	defer file.Close()
	var info struct {
		SchemaVersion int    `json:"schemaVersion"`
		Repository    string `json:"repository"`
		Version       string `json:"version"`
		SourceCommit  string `json:"sourceCommit"`
	}
	if json.NewDecoder(io.LimitReader(file, 64<<10)).Decode(&info) != nil || info.SchemaVersion != 1 ||
		info.Repository != "https://github.com/neko233-com/buildworld" ||
		strings.TrimPrefix(info.Version, "v") != strings.TrimPrefix(version, "v") || !commitPattern.MatchString(info.SourceCommit) {
		return ""
	}
	return info.SourceCommit
}

package jenkins

import (
	"fmt"
	"regexp"
	"strings"
)

// JobDefinition is the normalized result of parsing a Jenkins job config.xml.
// It maps directly onto BuildWorld's project model so the importer can persist
// it without further transformation.
type JobDefinition struct {
	JobName       string
	Format        string // "jenkinsfile" | "yaml"
	SourceMode    string // "inline" | "scm"
	Script        string // pipeline source (Jenkinsfile text) for inline pipelines
	SCMRepo       string
	SCMBranch     string
	SCMPath       string
	RepoURL       string // convenience repo URL surfaced to the project
	DefaultBranch string
	Warnings      []string
}

var (
	scriptRe       = regexp.MustCompile(`(?s)<script>(.*?)</script>`)
	gitURLRe       = regexp.MustCompile(`(?s)<scm[^>]*>.*?<url>([^<]+)</url>`)
	gitBranchRe    = regexp.MustCompile(`(?s)<scm[^>]*>.*?<name>([^<]+)</name>`)
	scriptPathRe   = regexp.MustCompile(`(?s)<scriptPath>([^<]+)</scriptPath>`)
	envRepoRe      = regexp.MustCompile(`GIT_REPO_URL\s*=\s*"([^"]+)"`)
	envBranchRe    = regexp.MustCompile(`GIT_BRANCH\s*=\s*"([^"]+)"`)
	freestyleCmdRe = regexp.MustCompile(`(?s)<hudson\.tasks\.Shell>(.*?)</hudson\.tasks\.Shell>`)
	commandRe      = regexp.MustCompile(`(?s)<command>([^<]*)</command>`)
)

// ParseConfig turns a Jenkins job config.xml into a BuildWorld JobDefinition.
// jobName is the leaf job name (used as the default project name and title).
func ParseConfig(xmlData []byte, jobName string) (*JobDefinition, error) {
	raw := string(xmlData)

	if !strings.Contains(raw, "<project") && !strings.Contains(raw, "<flow-definition") && !strings.Contains(raw, "<maven") && !strings.Contains(raw, "<matrix") {
		return nil, fmt.Errorf("unsupported jenkins job type (not a project/flow-definition/maven/matrix config)")
	}

	def := &JobDefinition{JobName: jobName}

	// Pipeline (Declarative or Scripted) stored inline.
	if strings.Contains(raw, "flow-definition") {
		if m := scriptRe.FindStringSubmatch(raw); m != nil {
			def.Format = "jenkinsfile"
			def.SourceMode = "inline"
			def.Script = xmlUnescape(m[1])
			applySCM(def, raw)
			return def, nil
		}
		// Pipeline script from SCM: the Jenkinsfile lives in the repository.
		if gitURLRe.MatchString(raw) {
			def.Format = "jenkinsfile"
			def.SourceMode = "scm"
			applySCM(def, raw)
			if def.SCMPath == "" {
				def.SCMPath = "Jenkinsfile"
			}
			return def, nil
		}
		return nil, fmt.Errorf("pipeline job has neither inline script nor scm definition")
	}

	// Freestyle project: collect shell build steps and wrap them in a BuildWorld
	// YAML pipeline so they remain executable.
	if m := freestyleCmdRe.FindAllStringSubmatch(raw, -1); m != nil {
		var steps []string
		for _, block := range m {
			for _, cmd := range commandRe.FindAllStringSubmatch(block[1], -1) {
				steps = append(steps, xmlUnescape(cmd[1]))
			}
		}
		if len(steps) == 0 {
			return nil, fmt.Errorf("freestyle job has no shell build steps to import")
		}
		def.Format = "yaml"
		def.SourceMode = "inline"
		def.Script = freestyleToYAML(jobName, steps)
		applySCM(def, raw)
		return def, nil
	}

	return nil, fmt.Errorf("unsupported jenkins job type; only Pipeline and Freestyle are importable")
}

// applySCM extracts the git repository, branch and Jenkinsfile path from the
// config.xml <scm> block, then lets environment assignments inside the pipeline
// script override them (common Jenkins convention: GIT_REPO_URL / GIT_BRANCH).
func applySCM(def *JobDefinition, raw string) {
	if m := gitURLRe.FindStringSubmatch(raw); m != nil {
		def.SCMRepo = strings.TrimSpace(m[1])
		def.RepoURL = def.SCMRepo
	}
	if m := gitBranchRe.FindStringSubmatch(raw); m != nil {
		def.SCMBranch = normalizeSCMBranch(m[1])
		def.DefaultBranch = def.SCMBranch
	}
	if m := scriptPathRe.FindStringSubmatch(raw); m != nil {
		def.SCMPath = strings.TrimSpace(m[1])
	}
	if def.Script != "" {
		if m := envRepoRe.FindStringSubmatch(def.Script); m != nil {
			def.RepoURL = m[1]
			def.SCMRepo = m[1]
		}
		if m := envBranchRe.FindStringSubmatch(def.Script); m != nil {
			def.DefaultBranch = normalizeSCMBranch(m[1])
			def.SCMBranch = def.DefaultBranch
		}
	}
	if def.RepoURL == "" {
		def.Warnings = append(def.Warnings, "no git repository detected; set repo_url to enable SCM-triggered builds")
	}
}

// normalizeSCMBranch turns Jenkins Git BranchSpec patterns into the concrete
// branch names accepted by git clone --branch. Jenkins emits */main for the
// common "any remote main" selector; preserving that pattern makes a migrated
// Pipeline script from SCM fail before its Jenkinsfile can be read.
func normalizeSCMBranch(value string) string {
	branch := strings.TrimSpace(value)
	branch = strings.TrimPrefix(branch, "*/")
	branch = strings.TrimPrefix(branch, "refs/heads/")
	branch = strings.TrimPrefix(branch, "refs/remotes/origin/")
	branch = strings.TrimPrefix(branch, "origin/")
	return branch
}

func freestyleToYAML(name string, steps []string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("name: %s\n", name))
	b.WriteString("on: [manual]\n")
	b.WriteString("jobs:\n")
	b.WriteString("  build:\n")
	b.WriteString("    runs-on: local\n")
	b.WriteString("    steps:\n")
	b.WriteString("      - name: Run build steps\n")
	b.WriteString("        run: |\n")
	for _, s := range steps {
		for _, line := range strings.Split(s, "\n") {
			b.WriteString("          ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return b.String()
}

func xmlUnescape(s string) string {
	// Jenkins stores inline pipeline scripts as XML-escaped text. The standard
	// XML entity set is sufficient to recover the original Jenkinsfile.
	replacer := strings.NewReplacer(
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", "\"",
		"&apos;", "'",
		"&amp;", "&",
	)
	return replacer.Replace(s)
}

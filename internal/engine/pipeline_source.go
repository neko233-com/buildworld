package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/neko233-com/buildworld/internal/git"
	"github.com/neko233-com/buildworld/internal/processtree"
	"github.com/neko233-com/buildworld/internal/store"
)

// PipelineFormat identifies the authoring syntax of a pipeline definition.
// BuildWorld accepts TypeScript, jobs-based YAML, and Jenkinsfile sources.
type PipelineFormat string

const (
	FormatTypeScript  PipelineFormat = "typescript"
	FormatYAML        PipelineFormat = "yaml"
	FormatJenkinsfile PipelineFormat = "jenkinsfile"
	FormatAuto        PipelineFormat = "auto"
)

// NormalizeFormat maps an empty/unknown format to FormatAuto so callers can
// pass through a stored hint without special-casing.
func NormalizeFormat(value string) PipelineFormat {
	switch PipelineFormat(strings.ToLower(strings.TrimSpace(value))) {
	case FormatTypeScript:
		return FormatTypeScript
	case FormatYAML:
		return FormatYAML
	case FormatJenkinsfile:
		return FormatJenkinsfile
	default:
		return FormatAuto
	}
}

// JenkinsfileConverter is registered by the migration package at init time. The
// engine deliberately does not import migration (that would create a cycle);
// instead migration provides the Jenkinsfile -> BuildConfig converter so the
// live pipeline engine can treat Jenkinsfile as a first-class source format.
var JenkinsfileConverter func(source, name string) (*BuildConfig, []string, error)

// ParseJenkinsfileConfig parses a Jenkinsfile into the canonical BuildConfig.
// It returns an error if Jenkinsfile support was not registered.
func ParseJenkinsfileConfig(source, name string) (*BuildConfig, error) {
	if JenkinsfileConverter == nil {
		return nil, fmt.Errorf("Jenkinsfile pipeline support is not registered; ensure the migration package is imported")
	}
	cfg, _, err := JenkinsfileConverter(source, name)
	if err != nil {
		return nil, err
	}
	if err := ValidatePipelineSemantics(cfg); err != nil {
		return nil, err
	}
	if _, err := ResolveApprovalPolicy(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// IsJenkinsfilePipeline reports whether the source is a Jenkinsfile rather than
// a BuildWorld TypeScript or YAML pipeline. It deliberately rejects JSON and
// Markdown so those rejections stay precise.
func IsJenkinsfilePipeline(source string) bool {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return false
	}
	if strings.HasPrefix(trimmed, "#") {
		return false
	}
	// Declarative pipeline { ... } or scripted node { ... }.
	if strings.Contains(trimmed, "pipeline {") || strings.Contains(trimmed, "node {") {
		return true
	}
	// Scripted stage('name') { ... } with shell commands is the most common
	// hand-written Jenkinsfile shape.
	if strings.Contains(trimmed, "stage(") &&
		(strings.Contains(trimmed, "steps") || strings.Contains(trimmed, "sh ") || strings.Contains(trimmed, "echo ") || strings.Contains(trimmed, "script ")) {
		return true
	}
	return false
}

// DetectPipelineFormat infers the source syntax from its content.
func DetectPipelineFormat(source string) PipelineFormat {
	trimmed := strings.TrimSpace(source)
	if IsTypeScriptPipeline(trimmed) {
		return FormatTypeScript
	}
	if IsJenkinsfilePipeline(trimmed) {
		return FormatJenkinsfile
	}
	return FormatYAML
}

// ParsePipelineConfigWithFormat parses a pipeline source using an explicit
// format hint. An empty or auto hint triggers content-based detection.
func ParsePipelineConfigWithFormat(format PipelineFormat, source, name string) (*BuildConfig, error) {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return nil, fmt.Errorf("pipeline config cannot be empty")
	}
	if format == "" || format == FormatAuto {
		format = DetectPipelineFormat(trimmed)
	}
	var config *BuildConfig
	var err error
	switch format {
	case FormatTypeScript:
		config, err = ParseTypeScriptPipeline(source)
	case FormatJenkinsfile:
		config, err = ParseJenkinsfileConfig(source, name)
	case FormatYAML:
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			return nil, fmt.Errorf("JSON pipeline configs are not supported; use TypeScript, YAML, or Jenkinsfile")
		}
		if strings.HasPrefix(trimmed, "#") {
			return nil, fmt.Errorf("Markdown pipeline configs are not supported; use TypeScript, YAML, or Jenkinsfile")
		}
		config, err = ParseYAMLConfig(source)
	default:
		return nil, fmt.Errorf("unsupported pipeline format %q", format)
	}
	if err != nil {
		return nil, err
	}
	if err := ValidatePipelineSemantics(config); err != nil {
		return nil, err
	}
	if _, err := ResolveApprovalPolicy(config); err != nil {
		return nil, err
	}
	return config, nil
}

// SCMPipelineSource describes a pipeline definition fetched from a VCS, exactly
// like Jenkins' "Pipeline script from SCM": a repository, branch, and path to
// the definition file. It supports TypeScript, YAML, and Jenkinsfile sources.
type SCMPipelineSource struct {
	RepoURL    string
	Branch     string
	Path       string
	Format     PipelineFormat
	VCSRootID  *int64
	CredToken  string
	CredUser   string
	CredSSHKey string
}

// SCMFetcher retrieves a pipeline file from a remote repository. The default
// engine implementation clones anonymously; servers register a credential-aware
// fetcher through DefaultSCMFetcher.
type SCMFetcher interface {
	FetchSCMPipeline(ctx context.Context, src SCMPipelineSource) (content string, format PipelineFormat, err error)
}

// DefaultSCMFetcher resolves SCM pipeline sources. The server overrides this
// with a store-aware implementation that attaches credentials from the project
// VCS root. Until then, anonymous clones are used.
var DefaultSCMFetcher SCMFetcher = anonymousSCMFetcher{}

type anonymousSCMFetcher struct{}

func (anonymousSCMFetcher) FetchSCMPipeline(ctx context.Context, src SCMPipelineSource) (string, PipelineFormat, error) {
	return FetchSCMPipeline(ctx, src)
}

// ResolveProjectPipelineSource returns the raw pipeline definition text for a
// project. For inline projects it returns the stored config; for SCM-sourced
// projects it clones the repository and reads the definition file. The runner
// dispatches this text to the worker, which re-parses it, so the SCM fetch must
// happen once on the server and the resolved text travels with the build.
func ResolveProjectPipelineSource(ctx context.Context, project *store.Project) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(project.PipelineSourceMode))
	if mode != "scm" {
		return project.Config, nil
	}
	src := SCMPipelineSource{
		RepoURL:   project.PipelineSCMRepo,
		Branch:    project.PipelineSCMBranch,
		Path:      project.PipelineSCMPath,
		Format:    NormalizeFormat(project.PipelineFormat),
		VCSRootID: project.VCSRootID,
	}
	content, _, err := DefaultSCMFetcher.FetchSCMPipeline(ctx, src)
	if err != nil {
		return "", fmt.Errorf("fetch SCM pipeline: %w", err)
	}
	return content, nil
}

// ResolveProjectBuildConfig produces the executable BuildConfig for a project.
// When the project uses SCM-sourced pipeline definitions it fetches the file
// from the configured repository first; otherwise it parses the inline config.
func ResolveProjectBuildConfig(ctx context.Context, project *store.Project) (*BuildConfig, error) {
	source, err := ResolveProjectPipelineSource(ctx, project)
	if err != nil {
		return nil, err
	}
	format := NormalizeFormat(project.PipelineFormat)
	// Projects created before pipeline_format existed were backfilled as YAML.
	// Preserve those TypeScript/Jenkinsfile jobs during rolling upgrades.
	if format == FormatYAML {
		if detected := DetectPipelineFormat(source); detected != FormatYAML {
			format = detected
		}
	}
	return ParsePipelineConfigWithFormat(format, source, project.Name)
}

// FetchSCMPipeline clones the repository at the configured branch and returns
// the pipeline file content with its detected format. The clone is created in
// a temporary directory that is always removed.
func FetchSCMPipeline(ctx context.Context, src SCMPipelineSource) (string, PipelineFormat, error) {
	if strings.TrimSpace(src.RepoURL) == "" {
		return "", "", fmt.Errorf("SCM pipeline source requires a repository URL")
	}
	if strings.TrimSpace(src.Path) == "" {
		return "", "", fmt.Errorf("SCM pipeline source requires a file path")
	}
	branch := strings.TrimSpace(src.Branch)
	if branch == "" {
		branch = "main"
	}
	dest, err := os.MkdirTemp("", "buildworld-scm-pipeline-*")
	if err != nil {
		return "", "", fmt.Errorf("create SCM temp dir: %w", err)
	}
	defer os.RemoveAll(dest)

	cloneURL := src.RepoURL
	if src.CredToken != "" && strings.HasPrefix(cloneURL, "https://") {
		cloneURL = embedCredentialToken(cloneURL, src.CredToken)
	}

	client := git.NewClient()
	if src.CredSSHKey != "" {
		if err := client.CloneWithSSHContext(ctx, cloneURL, dest, src.CredUser, src.CredSSHKey); err != nil {
			return "", "", err
		}
	} else {
		cmd := processtree.CommandContext(ctx, "git", "clone", "--branch", branch, "--depth", "1", cloneURL, dest)
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", "", fmt.Errorf("git clone failed: %w, output: %s", err, out)
		}
	}

	filePath := filepath.Join(dest, filepath.Clean(src.Path))
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", "", fmt.Errorf("read SCM pipeline file %q: %w", src.Path, err)
	}
	content := string(data)

	format := src.Format
	if format == "" || format == FormatAuto {
		format = detectFormatByPath(src.Path)
		if format == FormatAuto {
			format = DetectPipelineFormat(content)
		} else if format == FormatYAML && IsJenkinsfilePipeline(content) {
			format = FormatJenkinsfile
		}
	}
	return content, format, nil
}

// detectFormatByPath infers the pipeline format from the file extension, used
// when an SCM source does not declare an explicit format.
func detectFormatByPath(path string) PipelineFormat {
	base := strings.ToLower(filepath.Base(path))
	switch {
	case base == "jenkinsfile", strings.HasSuffix(base, ".jenkinsfile"), strings.HasSuffix(base, ".groovy"):
		return FormatJenkinsfile
	case strings.HasSuffix(base, ".yml"), strings.HasSuffix(base, ".yaml"):
		return FormatYAML
	case strings.HasSuffix(base, ".ts"):
		return FormatTypeScript
	default:
		return FormatAuto
	}
}

// embedCredentialToken injects a personal access token into an HTTPS git URL so
// authenticated clones work without an interactive credential prompt.
func embedCredentialToken(rawURL, token string) string {
	if !strings.HasPrefix(rawURL, "https://") {
		return rawURL
	}
	trimmed := strings.TrimPrefix(rawURL, "https://")
	return "https://" + token + "@" + trimmed
}

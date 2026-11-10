package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/neko233-com/buildworld/internal/git"
	"github.com/neko233-com/buildworld/internal/processtree"
	"github.com/neko233-com/buildworld/internal/store"
)

// PipelineFormat identifies the authoring syntax of a pipeline definition.
// BuildWorld accepts TypeScript, jobs-based YAML, and Jenkinsfile sources.
type PipelineFormat string

const (
	FormatTypeScript        PipelineFormat = "typescript"
	FormatYAML              PipelineFormat = "yaml"
	FormatJenkinsfile       PipelineFormat = "jenkinsfile"
	FormatAuto              PipelineFormat = "auto"
	scmPipelineFetchTimeout                = 5 * time.Minute
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
	Revision   string
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

// SCMRevisionFetcher resolves the definition and immutable Git revision together.
type SCMRevisionFetcher interface {
	FetchSCMPipelineWithRevision(ctx context.Context, src SCMPipelineSource) (content string, format PipelineFormat, revision string, err error)
}

// SCMObjectRevisionFetcher also proves the requested object ID when an
// annotated tag resolves to a different commit ID.
type SCMObjectRevisionFetcher interface {
	SCMRevisionFetcher
	FetchSCMPipelineWithObject(ctx context.Context, src SCMPipelineSource) (content string, format PipelineFormat, revision, objectID string, err error)
}

// DefaultSCMFetcher resolves SCM pipeline sources. The server overrides this
// with a store-aware implementation that attaches credentials from the project
// VCS root. Until then, anonymous clones are used.
var DefaultSCMFetcher SCMFetcher = anonymousSCMFetcher{}

type anonymousSCMFetcher struct{}

func (anonymousSCMFetcher) FetchSCMPipeline(ctx context.Context, src SCMPipelineSource) (string, PipelineFormat, error) {
	return FetchSCMPipeline(ctx, src)
}

func (anonymousSCMFetcher) FetchSCMPipelineWithRevision(ctx context.Context, src SCMPipelineSource) (string, PipelineFormat, string, error) {
	return FetchSCMPipelineWithRevision(ctx, src)
}

func (anonymousSCMFetcher) FetchSCMPipelineWithObject(ctx context.Context, src SCMPipelineSource) (string, PipelineFormat, string, string, error) {
	return fetchSCMPipelineWithObject(ctx, src)
}

// ResolveBuildPipelineSource pins SCM definitions to the build's immutable Git revision.
func ResolveBuildPipelineSource(ctx context.Context, project *store.Project, build *store.Build) (string, string, error) {
	if !strings.EqualFold(strings.TrimSpace(project.PipelineSourceMode), "scm") {
		if strings.HasPrefix(strings.TrimSpace(build.Branch), "refs/tags/") {
			fetchCtx, cancel := context.WithTimeout(ctx, scmPipelineFetchTimeout)
			defer cancel()
			revision, err := ResolveSCMTagCommit(fetchCtx, project.RepoURL, strings.TrimSpace(build.Branch), build.CommitSHA)
			if err != nil {
				return "", "", fmt.Errorf("resolve inline pipeline tag: %w", err)
			}
			return project.Config, revision, nil
		}
		return project.Config, "", nil
	}
	fetcher, ok := DefaultSCMFetcher.(SCMRevisionFetcher)
	if !ok {
		return "", "", errors.New("SCM fetcher cannot resolve an immutable pipeline revision")
	}
	branch := strings.TrimSpace(build.Branch)
	if branch == "" {
		branch = strings.TrimSpace(project.PipelineSCMBranch)
	}
	if branch == "" {
		branch = strings.TrimSpace(project.DefaultBranch)
	}
	src := SCMPipelineSource{RepoURL: project.PipelineSCMRepo, Branch: branch, Revision: build.CommitSHA,
		Path: project.PipelineSCMPath, Format: NormalizeFormat(project.PipelineFormat), VCSRootID: project.VCSRootID}
	fetchCtx, cancel := context.WithTimeout(ctx, scmPipelineFetchTimeout)
	defer cancel()
	var content, revision, objectID string
	var err error
	if objectFetcher, ok := DefaultSCMFetcher.(SCMObjectRevisionFetcher); ok {
		content, _, revision, objectID, err = objectFetcher.FetchSCMPipelineWithObject(fetchCtx, src)
	} else {
		content, _, revision, err = fetcher.FetchSCMPipelineWithRevision(fetchCtx, src)
		objectID = revision
	}
	if err != nil {
		return "", "", fmt.Errorf("fetch immutable SCM pipeline: %w", err)
	}
	if !validSCMRevision(revision) || (len(revision) != 40 && len(revision) != 64) {
		return "", "", errors.New("SCM fetcher returned an invalid immutable revision")
	}
	if src.Revision != "" && (!validSCMRevision(objectID) || (len(objectID) != 40 && len(objectID) != 64) || !strings.HasPrefix(strings.ToLower(objectID), strings.ToLower(src.Revision))) {
		return "", "", errors.New("SCM pipeline revision does not match the requested object")
	}
	if src.Revision != "" && !strings.HasPrefix(branch, "refs/tags/") && !strings.EqualFold(objectID, revision) {
		return "", "", errors.New("non-tag SCM builds require a commit object")
	}
	return content, revision, nil
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
	fetchCtx, cancel := context.WithTimeout(ctx, scmPipelineFetchTimeout)
	content, _, err := DefaultSCMFetcher.FetchSCMPipeline(fetchCtx, src)
	fetchErr := fetchCtx.Err()
	cancel()
	if errors.Is(fetchErr, context.DeadlineExceeded) {
		return "", fmt.Errorf("fetch SCM pipeline timed out: %w", fetchErr)
	}
	if errors.Is(fetchErr, context.Canceled) {
		return "", fmt.Errorf("fetch SCM pipeline canceled: %w", fetchErr)
	}
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

// FetchSCMPipeline returns a pipeline file from an isolated Git checkout.
func FetchSCMPipeline(ctx context.Context, src SCMPipelineSource) (string, PipelineFormat, error) {
	content, format, _, err := FetchSCMPipelineWithRevision(ctx, src)
	return content, format, err
}

func validSCMRevision(revision string) bool {
	if len(revision) < 7 || len(revision) > 64 {
		return false
	}
	for _, c := range revision {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// FetchSCMPipelineWithRevision returns the definition and the exact checked-out commit.
func FetchSCMPipelineWithRevision(ctx context.Context, src SCMPipelineSource) (string, PipelineFormat, string, error) {
	content, format, revision, _, err := fetchSCMPipelineWithObject(ctx, src)
	return content, format, revision, err
}

func fetchSCMPipelineWithObject(ctx context.Context, src SCMPipelineSource) (string, PipelineFormat, string, string, error) {
	return fetchSCMObject(ctx, src, true)
}

// ResolveSCMTagCommit resolves a tag or tag object to a verified commit for an
// inline pipeline that has an explicitly configured SCM repository.
func ResolveSCMTagCommit(ctx context.Context, repository, tagRef, objectID string) (string, error) {
	if !strings.HasPrefix(tagRef, "refs/tags/") || strings.TrimPrefix(tagRef, "refs/tags/") == "" {
		return "", errors.New("SCM tag ref is required")
	}
	_, _, revision, actualObject, err := fetchSCMObject(ctx, SCMPipelineSource{RepoURL: repository, Branch: tagRef, Revision: objectID}, false)
	if err != nil {
		return "", err
	}
	if objectID != "" && !strings.HasPrefix(strings.ToLower(actualObject), strings.ToLower(objectID)) {
		return "", errors.New("SCM tag object does not match the requested object")
	}
	return revision, nil
}

func fetchSCMObject(ctx context.Context, src SCMPipelineSource, requirePipeline bool) (string, PipelineFormat, string, string, error) {
	if strings.TrimSpace(src.RepoURL) == "" {
		return "", "", "", "", fmt.Errorf("SCM pipeline source requires a repository URL")
	}
	if requirePipeline && strings.TrimSpace(src.Path) == "" {
		return "", "", "", "", fmt.Errorf("SCM pipeline source requires a file path")
	}
	if src.Revision != "" && !validSCMRevision(src.Revision) {
		return "", "", "", "", fmt.Errorf("SCM commit must be a hexadecimal Git object ID")
	}
	branch := strings.TrimSpace(src.Branch)
	if branch == "" {
		branch = "main"
	}
	if strings.HasPrefix(branch, "-") || strings.ContainsAny(branch, "\r\n\x00") {
		return "", "", "", "", fmt.Errorf("invalid SCM branch")
	}
	dest, err := os.MkdirTemp("", "buildworld-scm-pipeline-*")
	if err != nil {
		return "", "", "", "", fmt.Errorf("create SCM temp dir: %w", err)
	}
	defer os.RemoveAll(dest)

	cloneURL := src.RepoURL
	if src.CredToken != "" && strings.HasPrefix(cloneURL, "https://") {
		cloneURL = embedCredentialToken(cloneURL, src.CredToken)
	}

	client := git.NewClient()
	if src.CredSSHKey != "" {
		if err := client.CloneWithSSHContext(ctx, cloneURL, dest, src.CredUser, src.CredSSHKey); err != nil {
			return "", "", "", "", err
		}
	} else {
		args := []string{"clone"}
		if src.Revision == "" && !strings.HasPrefix(branch, "refs/tags/") {
			args = append(args, "--branch", branch, "--depth", "1")
		}
		args = append(args, "--", cloneURL, dest)
		cmd := processtree.CommandContext(ctx, "git", args...)
		cmd.Env = git.NonInteractiveEnvironment(cmd.Environ())
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", "", "", "", fmt.Errorf("git clone failed: %w, output: %s", err, out)
		}
	}
	if src.Revision != "" || src.CredSSHKey != "" || strings.HasPrefix(branch, "refs/tags/") {
		ref := branch
		if src.Revision != "" {
			ref = src.Revision
		}
		cmd := processtree.CommandContext(ctx, "git", "-C", dest, "checkout", "--detach", ref)
		cmd.Env = git.NonInteractiveEnvironment(cmd.Environ())
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", "", "", "", fmt.Errorf("git checkout failed: %w, output: %s", err, out)
		}
	}
	revCmd := processtree.CommandContext(ctx, "git", "-C", dest, "rev-parse", "--verify", "HEAD^{commit}")
	revCmd.Env = git.NonInteractiveEnvironment(revCmd.Environ())
	rev, err := revCmd.CombinedOutput()
	revision := strings.TrimSpace(string(rev))
	if err != nil || (len(revision) != 40 && len(revision) != 64) || !validSCMRevision(revision) {
		return "", "", "", "", fmt.Errorf("cannot resolve SCM pipeline commit")
	}
	objectID := revision
	if src.Revision != "" {
		objectCmd := processtree.CommandContext(ctx, "git", "-C", dest, "rev-parse", "--verify", src.Revision+"^{object}")
		objectCmd.Env = git.NonInteractiveEnvironment(objectCmd.Environ())
		object, err := objectCmd.Output()
		if err != nil {
			return "", "", "", "", errors.New("cannot resolve requested SCM object")
		}
		objectID = strings.TrimSpace(string(object))
		commitCmd := processtree.CommandContext(ctx, "git", "-C", dest, "rev-parse", "--verify", src.Revision+"^{commit}")
		commitCmd.Env = git.NonInteractiveEnvironment(commitCmd.Environ())
		commit, err := commitCmd.Output()
		if err != nil || !strings.EqualFold(strings.TrimSpace(string(commit)), revision) {
			return "", "", "", "", errors.New("requested SCM object does not resolve to the checked-out commit")
		}
	}
	if !requirePipeline {
		return "", "", revision, objectID, nil
	}

	cleanPath := filepath.Clean(src.Path)
	if filepath.IsAbs(cleanPath) || cleanPath == ".." || strings.HasPrefix(cleanPath, ".."+string(filepath.Separator)) {
		return "", "", "", "", fmt.Errorf("SCM pipeline path escapes the repository")
	}
	filePath, err := filepath.EvalSymlinks(filepath.Join(dest, cleanPath))
	if err != nil {
		return "", "", "", "", fmt.Errorf("resolve SCM pipeline file: %w", err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(dest)
	if err != nil {
		return "", "", "", "", fmt.Errorf("resolve SCM checkout: %w", err)
	}
	relative, err := filepath.Rel(resolvedRoot, filePath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", "", "", "", fmt.Errorf("SCM pipeline path escapes the repository")
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", "", "", "", fmt.Errorf("read SCM pipeline file %q: %w", src.Path, err)
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
	return content, format, revision, objectID, nil
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

package worker

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/neko233-com/buildworld/internal/bytemsg"
	"github.com/neko233-com/buildworld/internal/engine"
	"github.com/neko233-com/buildworld/internal/plugin"
	"github.com/neko233-com/buildworld/internal/processtree"
	pb "github.com/neko233-com/buildworld/internal/rpc/generated"
	"google.golang.org/grpc"
)

// Executor is the remote worker's gRPC execution endpoint. It deliberately
// executes the same Buildworld pipeline format as the local runner.
type Executor struct {
	pb.UnimplementedWorkerServiceServer
	mu          sync.Mutex
	running     map[string]context.CancelFunc
	plugins     *plugin.Loader
	environment *engine.BuildEnvironment
	pluginMu    sync.Mutex
}

const (
	remoteArtifactChunkSize = 512 * 1024
	remoteArtifactMaxSize   = 256 * 1024 * 1024
)

func NewExecutor() *Executor {
	return NewExecutorAt("")
}

func NewExecutorAt(buildRoot string) *Executor {
	environment := engine.NewBuildEnvironment(buildRoot)
	loader := plugin.NewLoader(environment.PluginCacheRoot())
	_ = loader.LoadAll()
	return NewExecutorWithPluginsAt(loader, environment)
}

func NewExecutorWithPlugins(plugins *plugin.Loader) *Executor {
	return NewExecutorWithPluginsAt(plugins, engine.NewBuildEnvironment(""))
}

func NewExecutorWithPluginsAt(plugins *plugin.Loader, environment *engine.BuildEnvironment) *Executor {
	return &Executor{running: make(map[string]context.CancelFunc), plugins: plugins, environment: environment}
}

func (e *Executor) ExecuteBuild(req *pb.BuildRequest, stream grpc.ServerStreamingServer[pb.BuildResponse]) error {
	ctx := stream.Context()
	if err := bytemsg.Validate(req.Protocol); err != nil {
		return e.send(stream, req.BuildId, "", "", err.Error(), "failed", true)
	}
	buildCtx, cancel := context.WithCancel(ctx)
	e.mu.Lock()
	e.running[req.BuildId] = cancel
	e.mu.Unlock()
	defer func() { cancel(); e.mu.Lock(); delete(e.running, req.BuildId); e.mu.Unlock() }()

	cfg, err := engine.ParsePipelineConfig(req.PipelineConfig)
	if err != nil {
		return e.send(stream, req.BuildId, "", "", err.Error(), "failed", true)
	}
	if err := e.ensurePlugins(buildCtx, req.Plugins); err != nil {
		return e.send(stream, req.BuildId, "", "", "remote plugin sync: "+err.Error(), "failed", true)
	}
	if e.environment == nil {
		e.environment = engine.NewBuildEnvironment("")
	}
	if err := e.environment.Ensure(); err != nil {
		return err
	}
	workspace, err := os.MkdirTemp(e.environment.WorkspacesRoot(), "buildworld-worker-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workspace)
	if req.RepoUrl != "" {
		if err := processtree.CommandContext(buildCtx, "git", "clone", req.RepoUrl, workspace).Run(); err != nil {
			return e.send(stream, req.BuildId, "checkout", "clone", err.Error(), "failed", true)
		}
		if req.Branch != "" {
			if err := processtree.CommandContext(buildCtx, "git", "-C", workspace, "checkout", req.Branch).Run(); err != nil {
				return e.send(stream, req.BuildId, "checkout", "branch", err.Error(), "failed", true)
			}
		}
	}
	env := engine.AppendPipelineEnvironment(nil, cfg.Environment)
	env = engine.AppendPipelineEnvironment(env, req.Environment)
	isolated, err := e.environment.Environment(workspace)
	if err != nil {
		return err
	}
	env = append(env, isolated...)
	runner := engine.NewExecutor()
	stageStatuses := make(map[string]string, len(cfg.Stages))
	var failedStages []string
	buildFailed := false
	for _, stage := range cfg.Stages {
		stageKey := engine.StageKey(stage)
		if len(stage.Branches) > 0 && !matchesBranch(stage.Branches, req.Branch) {
			if err := e.send(stream, req.BuildId, stage.Name, "", "skipped: branch does not match "+strings.Join(stage.Branches, ", "), "skipped", false); err != nil {
				return err
			}
			stageStatuses[stageKey] = engine.StageStatusSkipped
			continue
		}
		shouldRun, err := engine.ShouldRunStage(stage, stageStatuses)
		if err != nil {
			return e.send(stream, req.BuildId, stage.Name, "", err.Error(), "failed", true)
		}
		skipMessage := "skipped: dependency/if condition evaluated to false"
		if buildFailed {
			if strings.TrimSpace(stage.If) == "" {
				if shouldRun {
					shouldRun = false
					skipMessage = "skipped: fail-fast after previous stage failure"
				}
			} else {
				shouldRun, err = engine.EvaluatePipelineCondition(stage.If, engine.ConditionContext{Success: false, Failure: true})
				if err != nil {
					return e.send(stream, req.BuildId, stage.Name, "", err.Error(), "failed", true)
				}
			}
		}
		if !shouldRun {
			if err := e.send(stream, req.BuildId, stage.Name, "", skipMessage, "skipped", false); err != nil {
				return err
			}
			stageStatuses[stageKey] = engine.StageStatusSkipped
			continue
		}

		stageCtx := buildCtx
		cancelStage := func() {}
		if stage.TimeoutSec > 0 {
			stageCtx, cancelStage = context.WithTimeout(buildCtx, time.Duration(stage.TimeoutSec)*time.Second)
		}
		stageEnv := engine.AppendPipelineEnvironment(env, stage.Environment)
		stageBaseEnvLength := len(stageEnv)
		stageFailed := false
		for _, configuredStep := range stage.Steps {
			step := configuredStep
			if stage.WorkingDirectory != "" {
				if step.Config == nil {
					step.Config = make(map[string]string)
				} else {
					step.Config = cloneStringMap(step.Config)
				}
				if step.Config["working-directory"] == "" {
					step.Config["working-directory"] = stage.WorkingDirectory
				}
			}
			if stageFailed && strings.TrimSpace(step.If) == "" {
				if err := e.send(stream, req.BuildId, stage.Name, step.Name, "skipped: fail-fast after previous step failure", "skipped", false); err != nil {
					cancelStage()
					return err
				}
				continue
			}
			stepShouldRun := true
			if strings.TrimSpace(step.If) != "" {
				var conditionErr error
				stepShouldRun, conditionErr = engine.EvaluatePipelineCondition(step.If, engine.ConditionContext{Success: !stageFailed, Failure: stageFailed})
				if conditionErr != nil {
					cancelStage()
					return e.send(stream, req.BuildId, stage.Name, step.Name, conditionErr.Error(), "failed", true)
				}
			}
			if !stepShouldRun {
				if err := e.send(stream, req.BuildId, stage.Name, step.Name, "skipped: if condition evaluated to false", "skipped", false); err != nil {
					cancelStage()
					return err
				}
				continue
			}
			if step.Type == "git" {
				message := "checkout already completed by remote worker preflight"
				if req.RepoUrl == "" {
					message = "checkout skipped: no repository URL was supplied"
				}
				if err := e.send(stream, req.BuildId, stage.Name, step.Name, message, "success", false); err != nil {
					cancelStage()
					return err
				}
				continue
			}
			if err := e.send(stream, req.BuildId, stage.Name, step.Name, "started", "running", false); err != nil {
				cancelStage()
				return err
			}
			stepEnv := engine.AppendRuntimeEnvironment(stageEnv, cfg, step.Runtime)
			var outputs []string
			onOutput := func(line string) {
				outputs = engine.AppendBuildOutputs(outputs, line)
				_ = e.send(stream, req.BuildId, stage.Name, step.Name, line, "running", false)
			}
			err := e.runStep(stageCtx, runner, step, workspace, stepEnv, onOutput)
			if err != nil {
				stageFailed = true
				if sendErr := e.send(stream, req.BuildId, stage.Name, step.Name, "ERROR: "+err.Error(), "running", false); sendErr != nil {
					cancelStage()
					return sendErr
				}
				if stageCtx.Err() == context.DeadlineExceeded {
					if sendErr := e.send(stream, req.BuildId, stage.Name, step.Name, fmt.Sprintf("stage timed out after %ds", stage.TimeoutSec), "running", false); sendErr != nil {
						cancelStage()
						return sendErr
					}
					break
				}
				continue
			}
			stageEnv = append(stageEnv, outputs...)
		}
		cancelStage()
		env = append(env, stageEnv[stageBaseEnvLength:]...)
		if stageFailed {
			stageStatuses[stageKey] = engine.StageStatusFailed
			failedStages = append(failedStages, stage.Name)
			buildFailed = true
		} else {
			stageStatuses[stageKey] = engine.StageStatusSuccess
		}
	}
	if len(failedStages) > 0 {
		e.runPost(cfg, "failure", workspace, env, runner, stream, req.BuildId)
		return e.send(stream, req.BuildId, "done", "", "failed stage(s): "+strings.Join(failedStages, ", "), "failed", true)
	}
	e.runPost(cfg, "success", workspace, env, runner, stream, req.BuildId)
	if err := e.streamArtifacts(stream, req.BuildId, workspace, cfg.Artifacts); err != nil {
		return e.send(stream, req.BuildId, "artifacts", "", err.Error(), "failed", true)
	}
	return e.send(stream, req.BuildId, "done", "", "completed", "success", false)
}

func (e *Executor) runPost(cfg *engine.BuildConfig, outcome, workspace string, env []string, runner *engine.Executor, stream grpc.ServerStreamingServer[pb.BuildResponse], buildID string) {
	if len(cfg.Post) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	for _, condition := range []string{"always", outcome, "cleanup"} {
		for _, step := range cfg.Post[condition] {
			stage := "post " + condition
			_ = e.send(stream, buildID, stage, step.Name, "started", "running", false)
			err := e.runStep(ctx, runner, step, workspace, engine.AppendRuntimeEnvironment(env, cfg, step.Runtime), func(line string) {
				_ = e.send(stream, buildID, stage, step.Name, line, "running", false)
			})
			if err != nil {
				_ = e.send(stream, buildID, stage, step.Name, err.Error(), "failed", false)
			}
		}
	}
}

func matchesBranch(allowed []string, branch string) bool {
	for _, candidate := range allowed {
		if branch == candidate {
			return true
		}
	}
	return false
}

func (e *Executor) ensurePlugins(ctx context.Context, references []*pb.PluginReference) error {
	if len(references) == 0 {
		return nil
	}
	if e.plugins == nil {
		return fmt.Errorf("worker plugin cache is not configured")
	}
	// Plugin installation writes the cache directory. Serialize cache misses so
	// multiple concurrent builds cannot download or compile the same plugin over
	// each other before the manifest verification below.
	e.pluginMu.Lock()
	defer e.pluginMu.Unlock()
	for _, ref := range references {
		if ref == nil {
			return fmt.Errorf("invalid empty plugin reference")
		}
		if err := e.plugins.EnsureBinaryReference(ctx, plugin.BinaryReference{Name: ref.Name, Version: ref.Version, Source: ref.Source, ManifestSHA256: ref.ManifestSha256}); err != nil {
			return err
		}
	}
	return nil
}

func (e *Executor) streamArtifacts(stream grpc.ServerStreamingServer[pb.BuildResponse], buildID, workspace string, patterns []string) error {
	files := artifactFiles(workspace, patterns)
	for _, artifact := range files {
		file, err := os.Open(artifact.path)
		if err != nil {
			return fmt.Errorf("open artifact %s: %w", artifact.name, err)
		}
		info, err := file.Stat()
		if err != nil {
			file.Close()
			return fmt.Errorf("stat artifact %s: %w", artifact.name, err)
		}
		if info.Size() > remoteArtifactMaxSize {
			file.Close()
			return fmt.Errorf("artifact %s exceeds the %d MiB remote transfer limit", artifact.name, remoteArtifactMaxSize/(1024*1024))
		}
		hash := sha256.New()
		buffer := make([]byte, remoteArtifactChunkSize)
		for {
			n, readErr := file.Read(buffer)
			if n > 0 {
				_, _ = hash.Write(buffer[:n])
				if err := stream.Send(&pb.BuildResponse{BuildId: buildID, ArtifactChunk: &pb.ArtifactChunk{Name: artifact.name, Data: append([]byte(nil), buffer[:n]...)}, Protocol: bytemsg.NewProtocolInfo()}); err != nil {
					file.Close()
					return err
				}
			}
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				file.Close()
				return fmt.Errorf("read artifact %s: %w", artifact.name, readErr)
			}
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("close artifact %s: %w", artifact.name, err)
		}
		if err := stream.Send(&pb.BuildResponse{BuildId: buildID, ArtifactChunk: &pb.ArtifactChunk{Name: artifact.name, FinalChunk: true, Sha256: fmt.Sprintf("%x", hash.Sum(nil))}, Protocol: bytemsg.NewProtocolInfo()}); err != nil {
			return err
		}
	}
	return nil
}

type artifactFile struct {
	name string
	path string
}

func artifactFiles(workspace string, patterns []string) []artifactFile {
	seen := make(map[string]bool)
	var files []artifactFile
	for _, pattern := range patterns {
		if !filepath.IsAbs(pattern) {
			pattern = filepath.Join(workspace, pattern)
		}
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		for _, match := range matches {
			info, err := os.Stat(match)
			if err != nil || info.IsDir() {
				continue
			}
			rel, err := filepath.Rel(workspace, match)
			if err != nil {
				rel = filepath.Base(match)
			}
			name := filepath.Base(strings.ReplaceAll(filepath.ToSlash(rel), "/", "_"))
			if name == "." || name == "" || seen[name] {
				continue
			}
			seen[name] = true
			files = append(files, artifactFile{name: name, path: match})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
	return files
}

func (e *Executor) runStep(ctx context.Context, runner *engine.Executor, step engine.Step, workspace string, env []string, onOutput func(string)) error {
	if err := e.runBaseStep(ctx, runner, step, workspace, env, onOutput); err != nil {
		return err
	}
	if addition := step.PlatformAdditions[engine.PlatformName()]; addition != "" {
		platformStep := step
		platformStep.Command = addition
		platformStep.PlatformAdditions = nil
		return e.runBaseStep(ctx, runner, platformStep, workspace, env, onOutput)
	}
	return nil
}

func (e *Executor) runBaseStep(ctx context.Context, runner *engine.Executor, step engine.Step, workspace string, env []string, onOutput func(string)) error {
	stepEnv := engine.AppendStepEnvironment(env, step.Config)
	workingDirectory := workspace
	if configured := step.Config["working-directory"]; configured != "" {
		resolved, err := engine.ResolveWorkspaceDirectory(workspace, engine.ResolvePipelineVariables(configured, stepEnv, nil))
		if err != nil {
			return err
		}
		workingDirectory = resolved
	}
	command := engine.ResolvePipelineVariables(step.Command, stepEnv, nil)
	switch step.Type {
	case "service_watch":
		config := make(map[string]string, len(step.Config))
		for key, value := range step.Config {
			config[key] = engine.ResolvePipelineVariables(value, stepEnv, nil)
		}
		return engine.WatchService(ctx, workingDirectory, config, onOutput)
	case "", "shell", "tail":
		if step.Shell != "" {
			return runner.RunMultiShell(ctx, step.Shell, command, workingDirectory, stepEnv, onOutput)
		}
		return runner.RunShell(ctx, command, workingDirectory, stepEnv, onOutput)
	case "powershell", "ps1", "pwsh", "bash", "sh", "python", "python3", "cmd":
		return runner.RunMultiShell(ctx, step.Type, command, workingDirectory, stepEnv, onOutput)
	case "script":
		return runner.RunMultiShell(ctx, "sh", command, workingDirectory, stepEnv, onOutput)
	case "git":
		onOutput("checkout already handled by remote worker preflight")
		return nil
	case "notify":
		onOutput("Notification skipped: remote workers do not own notification credentials")
		return nil
	default:
		if e.plugins != nil {
			if handler := e.plugins.LookupStep(step.Type); handler != nil {
				sc := &plugin.StepContext{Workspace: workingDirectory, Config: step.Config}
				if err := handler(ctx, sc); err != nil {
					return err
				}
				for _, line := range sc.Logs {
					onOutput(line)
				}
				for key, value := range sc.Outputs {
					onOutput(fmt.Sprintf("::buildworld:set %s=%s", key, value))
				}
				for key, value := range sc.Env {
					onOutput(fmt.Sprintf("::buildworld:set %s=%s", key, value))
				}
				return nil
			}
		}
		return fmt.Errorf("remote worker does not support step type: %s", step.Type)
	}
}

func cloneStringMap(source map[string]string) map[string]string {
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func (e *Executor) send(stream grpc.ServerStreamingServer[pb.BuildResponse], id, stage, step, output, status string, isError bool) error {
	return stream.Send(&pb.BuildResponse{BuildId: id, Stage: stage, Step: step, Output: output, Status: status, IsError: isError, Protocol: bytemsg.NewProtocolInfo()})
}

func (e *Executor) CancelBuild(buildID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if cancel := e.running[buildID]; cancel != nil {
		cancel()
		return true
	}
	return false
}

package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/neko233-com/buildworld233/internal/git"
	"github.com/neko233-com/buildworld233/internal/plugin"
	"github.com/neko233-com/buildworld233/internal/store"
	"github.com/neko233-com/buildworld233/internal/ws"
)

type BuildRunner struct {
	store               *store.Store
	hub                 *ws.Hub
	workspaces          *WorkspaceManager
	executor            *Executor
	gitClient           *git.Client
	plugins             *plugin.Loader
	artifacts           *ArtifactManager
	triggerChecker      *TriggerChecker
	notificationService *NotificationService
	statisticsService   *StatisticsService
}

func NewBuildRunner(s *store.Store, hub *ws.Hub, wsRoot string, plugins *plugin.Loader) *BuildRunner {
	return &BuildRunner{
		store:      s,
		hub:        hub,
		workspaces: NewWorkspaceManager(wsRoot),
		executor:   NewExecutor(),
		gitClient:  git.NewClient(),
		plugins:    plugins,
	}
}

func (r *BuildRunner) SetArtifactManager(am *ArtifactManager) {
	r.artifacts = am
}

func (r *BuildRunner) SetTriggerChecker(tc *TriggerChecker) {
	r.triggerChecker = tc
}

func (r *BuildRunner) SetNotificationService(ns *NotificationService) {
	r.notificationService = ns
}

func (r *BuildRunner) SetStatisticsService(ss *StatisticsService) {
	r.statisticsService = ss
}

func (r *BuildRunner) Run(buildID int64) {
	go r.run(context.Background(), buildID)
}

func (r *BuildRunner) run(ctx context.Context, buildID int64) {
	start := time.Now()

	build, err := r.store.GetBuild(buildID)
	if err != nil {
		return
	}
	// 超时控制
	if build.TimeoutSec > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(build.TimeoutSec)*time.Second)
		defer cancel()
	}
	// 需要审批的构建在审批通过前不执行
	if build.ApprovalRequired && build.ApprovedBy == nil {
		return
	}

	project, err := r.store.GetProject(build.ProjectID)
	if err != nil {
		r.fail(buildID, start, fmt.Sprintf("project not found: %v", err), nil)
		return
	}

	if build.WaitDependencyOn != nil {
		depBuild, err := r.store.GetBuild(*build.WaitDependencyOn)
		if err == nil && depBuild.Status != "success" {
			return
		}
	}

	if !r.matchAgent(build, project) {
		return
	}

	cfg, err := r.loadBuildConfig(project)
	if err != nil {
		r.fail(buildID, start, fmt.Sprintf("invalid pipeline config: %v", err), project)
		return
	}

	workspace, err := r.workspaces.Prepare(project.ID, build.Number)
	if err != nil {
		r.fail(buildID, start, fmt.Sprintf("workspace error: %v", err), project)
		return
	}
	defer r.workspaces.Clean(workspace)

	_ = r.store.StartBuild(buildID)
	r.broadcastStatus(buildID, "running", "", 0)

	env := r.buildEnv(build, cfg, project)
	params := parseParams(build.Parameters)

	totalStages := len(cfg.Stages)
	for i, stage := range cfg.Stages {
		r.log(buildID, stage.Name, fmt.Sprintf("=== Stage: %s ===", stage.Name))
		r.broadcastStatus(buildID, "running", stage.Name, float64(i)/float64(totalStages))

		for _, step := range stage.Steps {
			r.log(buildID, stage.Name, fmt.Sprintf("--- Step: %s ---", step.Name))
			if err := r.execStep(ctx, step, workspace, project, build, env, params, stage.Name); err != nil {
				r.log(buildID, stage.Name, fmt.Sprintf("ERROR: %v", err))
				if ctx.Err() == context.DeadlineExceeded {
					r.fail(buildID, start, fmt.Sprintf("step %q timed out after %ds", step.Name, build.TimeoutSec), project)
				} else {
					r.fail(buildID, start, fmt.Sprintf("step %q failed: %v", step.Name, err), project)
				}
				r.finishCleanup(buildID)
				return
			}
		}
	}

	if r.artifacts != nil && len(cfg.Artifacts) > 0 {
		r.log(buildID, "", "Collecting artifacts...")
		if err := r.artifacts.CollectGlob(buildID, workspace, cfg.Artifacts); err != nil {
			r.log(buildID, "", fmt.Sprintf("artifact collection error: %v", err))
		} else {
			arts, _ := r.artifacts.ListByBuild(buildID)
			r.log(buildID, "", fmt.Sprintf("Saved %d artifact(s)", len(arts)))
		}
	}

	duration := time.Since(start).Milliseconds()
	_ = r.store.FinishBuild(buildID, "success", duration)
	r.log(buildID, "", fmt.Sprintf("Build #%d succeeded in %s", build.Number, time.Since(start)))
	r.broadcastStatus(buildID, "success", "", 1.0)

	if r.triggerChecker != nil {
		finishedBuild, _ := r.store.GetBuild(buildID)
		if finishedBuild != nil {
			r.triggerChecker.HandleBuildFinish(finishedBuild)
		}
	}

	if r.notificationService != nil {
		finishedBuild, _ := r.store.GetBuild(buildID)
		if finishedBuild != nil {
			go r.notificationService.SendBuildNotifications(finishedBuild, project)
		}
	}

	r.recordStatistics(buildID)
}

func (r *BuildRunner) loadBuildConfig(project *store.Project) (*BuildConfig, error) {
	cfg, err := ParsePipelineConfig(project.Config)
	if err != nil {
		return nil, err
	}
	if project.TemplateID != nil {
		tmpl, err := r.store.GetBuildTemplate(*project.TemplateID)
		if err == nil {
			tmplCfg, err := ParseBuildConfig(tmpl.Config)
			if err == nil {
				cfg = MergeBuildConfig(tmplCfg, cfg)
			}
		}
	}
	if cfg.Environment == nil {
		cfg.Environment = map[string]string{}
	}
	return cfg, nil
}

func (r *BuildRunner) matchAgent(build *store.Build, project *store.Project) bool {
	cfg, err := r.loadBuildConfig(project)
	if err != nil {
		return true
	}
	if len(cfg.AgentRequirements) == 0 {
		return true
	}
	_ = r.store.MarkOfflineWorkers()
	workers, err := r.store.ListOnlineWorkers()
	if err != nil {
		return true
	}
	for _, w := range workers {
		if r.workerMatchesRequirements(w, cfg.AgentRequirements) {
			return true
		}
	}
	return false
}

func (r *BuildRunner) workerMatchesRequirements(w *store.Worker, reqs []string) bool {
	var labels []string
	if w.Labels != "" {
		json.Unmarshal([]byte(w.Labels), &labels)
	}
	labelSet := map[string]bool{}
	for _, l := range labels {
		labelSet[l] = true
	}
	for _, req := range reqs {
		if strings.HasPrefix(req, "pool=") {
			pool := strings.TrimPrefix(req, "pool=")
			if w.Pool != pool {
				return false
			}
			continue
		}
		if !labelSet[req] {
			found := false
			for _, l := range labels {
				if l == req {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}
	return true
}

func (r *BuildRunner) finishCleanup(buildID int64) {
}

func (r *BuildRunner) execStep(ctx context.Context, step Step, workspace string, project *store.Project, build *store.Build, env []string, params map[string]interface{}, stage string) error {
	switch step.Type {
	case "shell", "":
		command := resolveVars(step.Command, env, params)
		if step.Shell != "" {
			return r.executor.RunMultiShell(ctx, step.Shell, command, workspace, env, func(line string) {
				r.log(buildIDOf(build), stage, strings.TrimRight(line, "\r\n"))
			})
		}
		return r.executor.RunShell(ctx, command, workspace, env, func(line string) {
			r.log(buildIDOf(build), stage, strings.TrimRight(line, "\r\n"))
		})
	case "powershell", "ps1", "pwsh":
		command := resolveVars(step.Command, env, params)
		return r.executor.RunMultiShell(ctx, step.Type, command, workspace, env, func(line string) {
			r.log(buildIDOf(build), stage, strings.TrimRight(line, "\r\n"))
		})
	case "bash", "sh", "python", "python3", "cmd":
		command := resolveVars(step.Command, env, params)
		return r.executor.RunMultiShell(ctx, step.Type, command, workspace, env, func(line string) {
			r.log(buildIDOf(build), stage, strings.TrimRight(line, "\r\n"))
		})
	case "git":
		url := project.RepoURL
		if c, ok := step.Config["url"]; ok && c != "" {
			url = c
		}
		branch := build.Branch
		if branch == "" {
			branch = project.DefaultBranch
		}
		r.log(buildIDOf(build), stage, fmt.Sprintf("git clone %s (branch=%s)", url, branch))
		if err := r.gitClient.Clone(url, workspace); err != nil {
			return fmt.Errorf("git clone: %w", err)
		}
		if branch != "" {
			_ = r.gitClient.Checkout(workspace, branch)
		}
		return nil
	case "script":
		scriptPath := filepath.Join(workspace, ".bw_step.sh")
		if err := os.WriteFile(scriptPath, []byte(step.Command), 0o755); err != nil {
			return err
		}
		return r.executor.RunShell(ctx, "sh .bw_step.sh", workspace, env, func(line string) {
			r.log(buildIDOf(build), stage, strings.TrimRight(line, "\r\n"))
		})
	default:
		if r.plugins != nil {
			if handler := r.plugins.LookupStep(step.Type); handler != nil {
				sc := &plugin.StepContext{
					Workspace: workspace,
					Branch:    build.Branch,
					Commit:    build.CommitSHA,
					Config:    step.Config,
				}
				if err := handler(ctx, sc); err != nil {
					return err
				}
				for _, l := range sc.Logs {
					r.log(buildIDOf(build), stage, l)
				}
				for k, v := range sc.Env {
					env = append(env, k+"="+v)
				}
				return nil
			}
		}
		return fmt.Errorf("unsupported step type: %s", step.Type)
	}
}

func buildIDOf(b *store.Build) int64 { return b.ID }

func (r *BuildRunner) buildEnv(build *store.Build, cfg *BuildConfig, project *store.Project) []string {
	envMap := map[string]string{}
	for k, v := range cfg.Environment {
		envMap[k] = v
	}
	if globals, err := r.store.ListEnvVars("global", nil); err == nil {
		for _, v := range globals {
			envMap[v.Name] = v.Value
		}
	}
	pid := project.ID
	if projectVars, err := r.store.ListEnvVars("project", &pid); err == nil {
		for _, v := range projectVars {
			envMap[v.Name] = v.Value
		}
	}
	var env []string
	for k, v := range envMap {
		env = append(env, k+"="+v)
	}
	return env
}

var varPattern = regexp.MustCompile(`\$\{(global|project|parameter|env)\.([A-Za-z_][A-Za-z0-9_]*)\}`)

func resolveVars(s string, env []string, params map[string]interface{}) string {
	envMap := map[string]string{}
	for _, e := range env {
		if i := strings.Index(e, "="); i > 0 {
			envMap[e[:i]] = e[i+1:]
		}
	}
	return varPattern.ReplaceAllStringFunc(s, func(m string) string {
		sub := varPattern.FindStringSubmatch(m)
		if len(sub) != 3 {
			return m
		}
		scope, name := sub[1], sub[2]
		switch scope {
		case "parameter":
			if v, ok := params[name]; ok {
				return fmt.Sprintf("%v", v)
			}
		case "env":
			// 从系统环境变量获取（os.Getenv）
			if v, ok := lookupOSEnv(name); ok {
				return v
			}
		default:
			if v, ok := envMap[name]; ok {
				return v
			}
		}
		return m
	})
}

func parseParams(jsonStr string) map[string]interface{} {
	if jsonStr == "" {
		return nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &m); err != nil {
		return nil
	}
	return m
}

func (r *BuildRunner) log(buildID int64, stage, line string) {
	ts := time.Now().Format("15:04:05")
	entry := fmt.Sprintf("[%s] [%s] %s\n", ts, stage, line)
	_ = r.store.AppendBuildLog(buildID, entry)
	if r.hub != nil {
		ws.BroadcastBuildLog(r.hub, fmt.Sprintf("%d", buildID), map[string]interface{}{
			"timestamp": ts, "stage": stage, "line": line, "level": "info",
		})
	}
}

func (r *BuildRunner) broadcastStatus(buildID int64, status, stage string, progress float64) {
	if r.hub == nil {
		return
	}
	ws.BroadcastBuildStatus(r.hub, fmt.Sprintf("%d", buildID), map[string]interface{}{
		"status": status, "stage": stage, "progress": progress,
	})
}

func (r *BuildRunner) fail(buildID int64, start time.Time, msg string, project *store.Project) {
	r.log(buildID, "", "BUILD FAILED: "+msg)
	_ = r.store.FinishBuild(buildID, "failed", time.Since(start).Milliseconds())
	r.broadcastStatus(buildID, "failed", "", 1.0)

	if r.notificationService != nil && project != nil {
		finishedBuild, _ := r.store.GetBuild(buildID)
		if finishedBuild != nil {
			go r.notificationService.SendBuildNotifications(finishedBuild, project)
		}
	}

	r.recordStatistics(buildID)
}

// recordStatistics 在构建结束后异步记录统计。失败不影响构建结果。
func (r *BuildRunner) recordStatistics(buildID int64) {
	if r.statisticsService == nil {
		return
	}
	build, err := r.store.GetBuild(buildID)
	if err != nil || build == nil {
		return
	}
	_ = r.statisticsService.RecordBuildCompletion(build)
}

// lookupOSEnv 包装 os.LookupEnv 便于测试。
var lookupOSEnv = func(key string) (string, bool) {
	return os.LookupEnv(key)
}

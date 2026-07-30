package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/neko233-com/buildworld/internal/bytemsg"
	"github.com/neko233-com/buildworld/internal/git"
	"github.com/neko233-com/buildworld/internal/plugin"
	pb "github.com/neko233-com/buildworld/internal/rpc/generated"
	"github.com/neko233-com/buildworld/internal/store"
	"github.com/neko233-com/buildworld/internal/ws"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type BuildRunner struct {
	store                   *store.Store
	hub                     *ws.Hub
	workspaces              *WorkspaceManager
	buildEnvironment        *BuildEnvironment
	executor                *Executor
	gitClient               *git.Client
	plugins                 *plugin.Loader
	artifacts               *ArtifactManager
	triggerChecker          *TriggerChecker
	notificationService     *NotificationService
	statisticsService       *StatisticsService
	runsMu                  sync.Mutex
	runs                    map[int64]context.CancelFunc
	queueMu                 sync.Mutex
	queueCancel             context.CancelFunc
	queueWake               chan struct{}
	remoteWaitMu            sync.Mutex
	remoteWaiting           map[int64]bool
	workerDispatchToken     string
	poolCursorMu            sync.Mutex
	poolCursor              map[string]int // round-robin index per pool key
	policyMu                sync.Mutex
	executionPolicy         ExecutionPolicy
	activeBuilds            int
	activeLocalBuilds       int
	policyWake              chan struct{}
	durableLogMu            sync.Mutex
	durableLogs             map[int64]*durableBuildLogBatch
	durableLogAppend        func(int64, string) error
	durableLogErrorReporter func(int64, error)
	secretMaskersMu         sync.Mutex
	secretMaskers           map[int64]*buildSecretMasker
	liveLogBroadcast        func(int64, map[string]interface{})
}

const remoteArtifactMaxSize = 256 * 1024 * 1024

type ExecutionPolicy struct {
	DefaultTimeoutSec        int
	MaxConcurrentBuilds      int
	MaxConcurrentLocalBuilds int
	RetryLimit               int
	FailFast                 bool
	CPUPercent               int
	BackgroundMode           bool
}

type remoteArtifactBuffer struct {
	name string
	data bytes.Buffer
}

func NewBuildRunner(s *store.Store, hub *ws.Hub, wsRoot string, plugins *plugin.Loader) *BuildRunner {
	buildRoot := filepath.Dir(wsRoot)
	runner := &BuildRunner{
		store:            s,
		hub:              hub,
		workspaces:       NewWorkspaceManager(wsRoot),
		buildEnvironment: NewBuildEnvironment(buildRoot),
		executor:         NewExecutor(),
		gitClient:        git.NewClient(),
		plugins:          plugins,
		runs:             make(map[int64]context.CancelFunc),
		queueWake:        make(chan struct{}, 1),
		remoteWaiting:    make(map[int64]bool),
		poolCursor:       make(map[string]int),
		executionPolicy: ExecutionPolicy{
			DefaultTimeoutSec:        1800,
			MaxConcurrentBuilds:      2,
			MaxConcurrentLocalBuilds: 1,
			RetryLimit:               1,
			FailFast:                 true,
			CPUPercent:               25,
			BackgroundMode:           true,
		},
		policyWake:    make(chan struct{}, 1),
		durableLogs:   make(map[int64]*durableBuildLogBatch),
		secretMaskers: make(map[int64]*buildSecretMasker),
	}
	_ = runner.ReloadExecutionPolicy()
	return runner
}

func (r *BuildRunner) ConfigureExecutionPolicy(policy ExecutionPolicy) error {
	if policy.DefaultTimeoutSec < 1 {
		return fmt.Errorf("default build timeout must be positive")
	}
	if policy.MaxConcurrentBuilds < 1 {
		return fmt.Errorf("build concurrency must be positive")
	}
	if policy.MaxConcurrentLocalBuilds < 1 {
		return fmt.Errorf("local build concurrency must be positive")
	}
	if policy.RetryLimit < 0 || policy.RetryLimit > 2 {
		return fmt.Errorf("retry limit must be between 0 and 2")
	}
	if policy.CPUPercent == 0 {
		policy.CPUPercent = 25
	}
	if policy.CPUPercent < 5 || policy.CPUPercent > 100 {
		return fmt.Errorf("CPU limit must be between 5 and 100 percent")
	}
	if err := applyProcessBackgroundMode(policy.BackgroundMode); err != nil {
		return fmt.Errorf("apply background process priority: %w", err)
	}
	runtime.GOMAXPROCS(cpuThreadBudget(policy.CPUPercent))
	r.policyMu.Lock()
	r.executionPolicy = policy
	r.policyMu.Unlock()
	r.signalPolicyChange()
	return nil
}

// ReloadExecutionPolicy applies the durable global execution settings without
// requiring a server restart. Missing keys preserve the current/default value.
func (r *BuildRunner) ReloadExecutionPolicy() error {
	if r.store == nil {
		return nil
	}
	values, err := r.store.ListEnvVars("system", nil)
	if err != nil {
		return fmt.Errorf("load build execution policy: %w", err)
	}
	policy := r.executionPolicySnapshot()
	for _, value := range values {
		switch value.Name {
		case "build_timeout":
			parsed, parseErr := strconv.Atoi(value.Value)
			if parseErr != nil {
				return fmt.Errorf("parse build_timeout: %w", parseErr)
			}
			policy.DefaultTimeoutSec = parsed
		case "build_concurrency":
			parsed, parseErr := strconv.Atoi(value.Value)
			if parseErr != nil {
				return fmt.Errorf("parse build_concurrency: %w", parseErr)
			}
			policy.MaxConcurrentBuilds = parsed
		case "local_agent_concurrency":
			parsed, parseErr := strconv.Atoi(value.Value)
			if parseErr != nil {
				return fmt.Errorf("parse local_agent_concurrency: %w", parseErr)
			}
			policy.MaxConcurrentLocalBuilds = parsed
		case "retry_policy":
			switch value.Value {
			case "never":
				policy.RetryLimit = 0
			case "failed_once":
				policy.RetryLimit = 1
			case "failed_twice":
				policy.RetryLimit = 2
			default:
				return fmt.Errorf("parse retry_policy: unsupported value %q", value.Value)
			}
		case "validation_fail_fast":
			parsed, parseErr := strconv.ParseBool(value.Value)
			if parseErr != nil {
				return fmt.Errorf("parse validation_fail_fast: %w", parseErr)
			}
			policy.FailFast = parsed
		case "cpu_limit_percent":
			parsed, parseErr := strconv.Atoi(value.Value)
			if parseErr != nil {
				return fmt.Errorf("parse cpu_limit_percent: %w", parseErr)
			}
			policy.CPUPercent = parsed
		case "background_mode":
			parsed, parseErr := strconv.ParseBool(value.Value)
			if parseErr != nil {
				return fmt.Errorf("parse background_mode: %w", parseErr)
			}
			policy.BackgroundMode = parsed
		}
	}
	return r.ConfigureExecutionPolicy(policy)
}

func (r *BuildRunner) executionPolicySnapshot() ExecutionPolicy {
	r.policyMu.Lock()
	defer r.policyMu.Unlock()
	return r.executionPolicy
}

// ExecutionPolicySnapshot exposes the effective hot-reloaded resource policy
// for metrics and administrative UI without allowing callers to mutate it.
func (r *BuildRunner) ExecutionPolicySnapshot() ExecutionPolicy {
	return r.executionPolicySnapshot()
}

func cpuThreadBudget(percent int) int {
	threads := runtime.NumCPU() * percent / 100
	if threads < 1 {
		return 1
	}
	if threads > runtime.NumCPU() {
		return runtime.NumCPU()
	}
	return threads
}

func executionResourceEnvironment(policy ExecutionPolicy) []string {
	threads := cpuThreadBudget(policy.CPUPercent)
	return []string{
		"BUILDWORLD_CPU_LIMIT_PERCENT=" + strconv.Itoa(policy.CPUPercent),
		"BUILDWORLD_CPU_THREADS=" + strconv.Itoa(threads),
		"GOMAXPROCS=" + strconv.Itoa(threads),
		"CARGO_BUILD_JOBS=" + strconv.Itoa(threads),
		"CMAKE_BUILD_PARALLEL_LEVEL=" + strconv.Itoa(threads),
		"UV_THREADPOOL_SIZE=" + strconv.Itoa(threads),
		"NPM_CONFIG_JOBS=" + strconv.Itoa(threads),
	}
}

func (r *BuildRunner) signalPolicyChange() {
	select {
	case r.policyWake <- struct{}{}:
	default:
	}
}

func (r *BuildRunner) acquireExecutionSlot(ctx context.Context, local bool) bool {
	for {
		r.policyMu.Lock()
		globalAvailable := r.activeBuilds < r.executionPolicy.MaxConcurrentBuilds
		localAvailable := !local || r.activeLocalBuilds < r.executionPolicy.MaxConcurrentLocalBuilds
		if globalAvailable && localAvailable {
			r.activeBuilds++
			if local {
				r.activeLocalBuilds++
			}
			hasGlobalCapacity := r.activeBuilds < r.executionPolicy.MaxConcurrentBuilds
			hasLocalCapacity := !local || r.activeLocalBuilds < r.executionPolicy.MaxConcurrentLocalBuilds
			r.policyMu.Unlock()
			if hasGlobalCapacity && hasLocalCapacity {
				r.signalPolicyChange()
			}
			return true
		}
		r.policyMu.Unlock()
		select {
		case <-ctx.Done():
			return false
		case <-r.policyWake:
		}
	}
}

func (r *BuildRunner) releaseExecutionSlot(local bool) {
	r.policyMu.Lock()
	if r.activeBuilds > 0 {
		r.activeBuilds--
	}
	if local && r.activeLocalBuilds > 0 {
		r.activeLocalBuilds--
	}
	r.policyMu.Unlock()
	r.signalPolicyChange()
	r.WakeQueue()
}

func (r *BuildRunner) SetBuildEnvironment(environment *BuildEnvironment) {
	if environment == nil {
		return
	}
	r.buildEnvironment = environment
	r.workspaces = NewWorkspaceManager(environment.WorkspacesRoot())
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

// SetWorkerDispatchToken configures the shared enrollment secret used to
// authenticate server-to-worker gRPC dispatches. Remote execution stays off
// until this is set, preventing an accidentally exposed worker port.
func (r *BuildRunner) SetWorkerDispatchToken(token string) {
	r.workerDispatchToken = token
}

// StartQueue restores interrupted work and coordinates pending builds from the
// durable store. Recovery is on by default: an unexpected service restart
// requeues only builds that were actively running, never cancelled work.
// A single dispatcher replaces one timer per waiting build, which keeps worker
// saturation bounded even when many projects target the same remote pool.
func (r *BuildRunner) StartQueue(parent context.Context) {
	r.queueMu.Lock()
	if r.queueCancel != nil {
		r.queueMu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(parent)
	r.queueCancel = cancel
	r.queueMu.Unlock()
	if r.store != nil {
		if recovered, err := r.store.RecoverInterruptedBuilds(); err != nil {
			// The periodic scan remains useful for existing pending work even when
			// recovery encounters a transient database error.
			fmt.Printf("build queue recovery failed: %v\n", err)
		} else if recovered > 0 {
			fmt.Printf("requeued %d build(s) interrupted by server restart\n", recovered)
		}
	}
	go r.dispatchPendingLoop(ctx)
	r.WakeQueue()
}

func (r *BuildRunner) StopQueue() {
	r.queueMu.Lock()
	cancel := r.queueCancel
	r.queueCancel = nil
	r.queueMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// WakeQueue asks the central dispatcher to reconcile pending builds promptly.
// It is intentionally non-blocking; the next durable scan will still catch any
// signal that arrives while a reconciliation is already running.
func (r *BuildRunner) WakeQueue() {
	select {
	case r.queueWake <- struct{}{}:
	default:
	}
}

func (r *BuildRunner) dispatchPendingLoop(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		r.dispatchPending()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-r.queueWake:
		}
	}
}

func (r *BuildRunner) dispatchPending() {
	if r.store == nil {
		return
	}
	builds, err := r.store.ListPendingBuilds()
	if err != nil {
		return
	}
	r.runsMu.Lock()
	inFlight := len(r.runs)
	r.runsMu.Unlock()
	r.policyMu.Lock()
	reservedGlobal := r.activeBuilds
	if inFlight > reservedGlobal {
		reservedGlobal = inFlight
	}
	globalAvailable := r.executionPolicy.MaxConcurrentBuilds - reservedGlobal
	localAvailable := r.executionPolicy.MaxConcurrentLocalBuilds - r.activeLocalBuilds
	// A newly launched run is registered synchronously before its goroutine
	// acquires a slot. Conservatively reserve local capacity for that tiny
	// window so a second wake cannot over-dispatch the same queue.
	if unacquired := inFlight - r.activeBuilds; unacquired > 0 {
		localAvailable -= unacquired
	}
	r.policyMu.Unlock()
	pending := make(map[int64]bool, len(builds))
	for _, build := range builds {
		pending[build.ID] = true
		if globalAvailable <= 0 || r.isBuildRunning(build.ID) {
			continue
		}
		project, projectErr := r.store.GetProject(build.ProjectID)
		if projectErr == nil {
			_, _ = r.store.CreateBuildQueueItem(build.ID, project.ID, project.Name, 0, build.Trigger, build.Branch)
		}
		if build.ApprovalRequired && build.ApprovedBy == nil {
			_ = r.store.UpdateBuildQueueItemStatusByBuildID(build.ID, "pending_approval")
			continue
		}
		if build.WaitDependencyOn != nil {
			dependency, dependencyErr := r.store.GetBuild(*build.WaitDependencyOn)
			if dependencyErr == nil && dependency.Status != "success" {
				continue
			}
		}

		localExecution := true
		if projectErr == nil {
			if cfg, configErr := r.loadBuildConfig(context.Background(), project); configErr == nil {
				localExecution = len(cfg.AgentRequirements) == 0
			}
		}
		if localExecution && localAvailable <= 0 {
			continue
		}
		r.Run(build.ID)
		globalAvailable--
		if localExecution {
			localAvailable--
		}
	}
	r.remoteWaitMu.Lock()
	for buildID := range r.remoteWaiting {
		if !pending[buildID] {
			delete(r.remoteWaiting, buildID)
		}
	}
	r.remoteWaitMu.Unlock()
}

func (r *BuildRunner) isBuildRunning(buildID int64) bool {
	r.runsMu.Lock()
	defer r.runsMu.Unlock()
	return r.runs[buildID] != nil
}

// Enqueue persists the visible queue item and wakes the single durable
// dispatcher. Production trigger paths use this instead of starting one
// waiting goroutine per build, so priority changes continue to affect the next
// build that actually acquires capacity.
func (r *BuildRunner) Enqueue(buildID int64) error {
	build, err := r.store.GetBuild(buildID)
	if err != nil {
		return err
	}
	project, err := r.store.GetProject(build.ProjectID)
	if err != nil {
		return err
	}
	if _, err := r.store.CreateBuildQueueItem(build.ID, project.ID, project.Name, 0, build.Trigger, build.Branch); err != nil {
		return err
	}
	if build.ApprovalRequired && build.ApprovedBy == nil {
		if err := r.store.UpdateBuildQueueItemStatusByBuildID(build.ID, "pending_approval"); err != nil {
			return err
		}
	}
	r.queueMu.Lock()
	queueStarted := r.queueCancel != nil
	r.queueMu.Unlock()
	if queueStarted {
		r.WakeQueue()
	} else {
		// Preserve the direct runner contract used by embedded callers and
		// focused tests that intentionally do not start the durable dispatcher.
		r.Run(buildID)
	}
	return nil
}

func (r *BuildRunner) Run(buildID int64) {
	ctx, cancel := context.WithCancel(context.Background())
	r.runsMu.Lock()
	if r.runs[buildID] != nil {
		r.runsMu.Unlock()
		cancel()
		return
	}
	r.runs[buildID] = cancel
	r.runsMu.Unlock()
	go func() {
		defer func() {
			r.runsMu.Lock()
			delete(r.runs, buildID)
			r.runsMu.Unlock()
			cancel()
		}()
		r.run(ctx, buildID)
	}()
}

// Stop cancels the live process context, including intentional long-running
// tail nodes. Store cancellation remains the source of truth for the UI.
func (r *BuildRunner) Stop(buildID int64) {
	r.runsMu.Lock()
	cancel := r.runs[buildID]
	r.runsMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (r *BuildRunner) run(ctx context.Context, buildID int64) {
	start := time.Now()
	_ = r.ReloadExecutionPolicy()

	build, err := r.store.GetBuild(buildID)
	if err != nil {
		return
	}
	project, err := r.store.GetProject(build.ProjectID)
	if err != nil {
		r.fail(buildID, start, fmt.Sprintf("project not found: %v", err), nil)
		return
	}
	_, _ = r.store.CreateBuildQueueItem(build.ID, project.ID, project.Name, 0, build.Trigger, build.Branch)
	// 需要审批的构建在审批通过前不执行
	if build.ApprovalRequired && build.ApprovedBy == nil {
		_ = r.store.UpdateBuildQueueItemStatusByBuildID(build.ID, "pending_approval")
		return
	}

	if build.WaitDependencyOn != nil {
		depBuild, err := r.store.GetBuild(*build.WaitDependencyOn)
		if err == nil && depBuild.Status != "success" {
			return
		}
	}

	cfg, err := r.loadBuildConfig(ctx, project)
	if err != nil {
		r.fail(buildID, start, fmt.Sprintf("invalid pipeline config: %v", err), project)
		return
	}
	r.configureBuildSecretMasker(build, project, cfg)
	defer r.clearBuildSecretMasker(buildID)
	localExecution := len(cfg.AgentRequirements) == 0
	if !r.acquireExecutionSlot(ctx, localExecution) {
		return
	}
	defer r.releaseExecutionSlot(localExecution)

	build, err = r.store.GetBuild(buildID)
	if err != nil || build.Status != "pending" {
		return
	}
	if build.TimeoutSec <= 0 {
		build.TimeoutSec = cfg.TimeoutSec
		if build.TimeoutSec <= 0 && !cfg.AllowLongRunning {
			build.TimeoutSec = r.executionPolicySnapshot().DefaultTimeoutSec
		}
		if build.TimeoutSec > 0 {
			_ = r.store.SetBuildTimeout(buildID, build.TimeoutSec)
		}
	}
	if cfg.DisableConcurrent {
		if r.deferForProjectConcurrency(buildID, project.ID, cfg.AbortPrevious) {
			return
		}
	}
	if build.TimeoutSec > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(build.TimeoutSec)*time.Second)
		defer cancel()
	}
	if len(cfg.AgentRequirements) > 0 {
		worker := r.selectRemoteWorker(cfg)
		if worker == nil {
			if r.markRemoteWaiting(buildID) {
				r.log(buildID, "queue", "Waiting for a matching remote worker slot")
			}
			return
		}
		r.clearRemoteWaiting(buildID)
		r.runRemote(ctx, build, project, cfg, worker, start)
		return
	}

	workspace, err := r.workspaces.Prepare(project.ID, build.Number)
	if err != nil {
		r.fail(buildID, start, fmt.Sprintf("workspace error: %v", err), project)
		return
	}
	defer r.workspaces.Clean(workspace)

	_ = r.store.StartBuild(buildID)
	_ = r.store.UpdateBuildQueueItemStatusByBuildID(buildID, "running")
	r.log(buildID, "", encodeTimelinePlan(cfg))
	r.broadcastStatus(buildID, "running", "", 0)
	if r.notificationService != nil {
		if startedBuild, err := r.store.GetBuild(buildID); err == nil {
			r.sendBuildEventAsync(buildID, startedBuild, project, "build.started")
		}
	}
	// Jenkins' "Pipeline script from SCM" checks out the same repository before
	// executing the Jenkinsfile.  The source fetch used to read the Jenkinsfile
	// itself must not be mistaken for that workspace checkout: shell steps still
	// need a real .git directory and the complete repository contents.
	if strings.EqualFold(strings.TrimSpace(project.PipelineSourceMode), "scm") {
		repository := strings.TrimSpace(project.PipelineSCMRepo)
		if repository == "" {
			r.fail(buildID, start, "SCM pipeline repository is empty", project)
			return
		}
		branch := strings.TrimSpace(build.Branch)
		if branch == "" {
			branch = strings.TrimSpace(project.PipelineSCMBranch)
		}
		if branch == "" {
			branch = strings.TrimSpace(project.DefaultBranch)
		}
		r.log(buildID, "SCM Checkout", fmt.Sprintf("Checking out Jenkins SCM repository %s (branch=%s)", repository, branch))
		if err := r.gitClient.CloneContext(ctx, repository, workspace); err != nil {
			r.fail(buildID, start, fmt.Sprintf("SCM checkout: git clone: %v", err), project)
			return
		}
		if branch != "" {
			if err := r.gitClient.CheckoutContext(ctx, workspace, branch); err != nil {
				r.fail(buildID, start, fmt.Sprintf("SCM checkout: git checkout: %v", err), project)
				return
			}
		}
	}

	env := r.buildEnvAt(build, cfg, project, workspace)
	if r.buildEnvironment != nil {
		isolated, environmentErr := r.buildEnvironment.Environment(workspace)
		if environmentErr != nil {
			r.fail(buildID, start, fmt.Sprintf("build environment error: %v", environmentErr), project)
			return
		}
		env = append(env, isolated...)
	}
	env = append(env, executionResourceEnvironment(r.executionPolicySnapshot())...)
	params := parseParams(build.Parameters)
	env, err = r.runPluginHook(ctx, "build.before", "", workspace, project, build, env)
	if err != nil {
		r.runPostSteps(ctx, cfg, "failure", workspace, project, build, env, params)
		r.fail(buildID, start, fmt.Sprintf("plugin build.before hook: %v", err), project)
		return
	}

	totalStages := len(cfg.Stages)
	var failedSteps []string
	stageStatuses := make(map[string]string, totalStages)
	for i, stage := range cfg.Stages {
		stageKey := StageKey(stage)
		branch := build.Branch
		if branch == "" {
			branch = project.DefaultBranch
		}
		if !stageMatchesBranch(stage, branch) {
			r.log(buildID, stage.Name, fmt.Sprintf("SKIPPED: branch %q does not match %s", branch, strings.Join(stage.Branches, ", ")))
			stageStatuses[stageKey] = StageStatusSkipped
			continue
		}
		shouldRun, conditionErr := ShouldRunStage(stage, stageStatuses)
		if conditionErr != nil {
			r.runPostSteps(ctx, cfg, "failure", workspace, project, build, env, params)
			r.fail(buildID, start, fmt.Sprintf("stage %q condition: %v", stage.Name, conditionErr), project)
			return
		}
		if !shouldRun {
			r.log(buildID, stage.Name, "SKIPPED: dependency/if condition evaluated to false")
			stageStatuses[stageKey] = StageStatusSkipped
			continue
		}
		r.log(buildID, stage.Name, fmt.Sprintf("=== Stage: %s ===", stage.Name))
		r.broadcastStatus(buildID, "running", stage.Name, float64(i)/float64(totalStages))

		stageCtx := ctx
		cancelStage := func() {}
		if stage.TimeoutSec > 0 {
			stageCtx, cancelStage = context.WithTimeout(ctx, time.Duration(stage.TimeoutSec)*time.Second)
		}
		stageEnv := AppendPipelineEnvironment(env, stage.Environment)
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
			stepShouldRun := true
			if strings.TrimSpace(step.If) != "" {
				var conditionErr error
				stepShouldRun, conditionErr = EvaluatePipelineCondition(step.If, ConditionContext{Success: !stageFailed, Failure: stageFailed})
				if conditionErr != nil {
					cancelStage()
					r.runPostSteps(ctx, cfg, "failure", workspace, project, build, env, params)
					r.fail(buildID, start, fmt.Sprintf("step %q condition: %v", step.Name, conditionErr), project)
					return
				}
			}
			if !stepShouldRun {
				r.log(buildID, stage.Name, fmt.Sprintf("SKIPPED: step %q if condition evaluated to false", step.Name))
				continue
			}
			r.log(buildID, stage.Name, fmt.Sprintf("--- Step: %s ---", step.Name))
			stepEnv := appendRuntimeEnv(stageEnv, cfg, step.Runtime)
			var outputs []string
			onOutput := func(line string) {
				r.logBuildOutput(buildID, stage.Name, line)
				outputs = appendBuildKV(outputs, line)
			}
			// Run the step in its own goroutine so the step loop can keep
			// flushing buffered output while a long-running step (e.g.
			// service_watch used as a temporary log monitor) stays alive.
			// Without this, the secret-masker stream only flushes at step
			// completion, so never-ending steps would emit nothing to the
			// live log or the durable store.
			stepErrCh := make(chan error, 1)
			go func() {
				stepErrCh <- r.execStep(stageCtx, step, workspace, project, build, stepEnv, params, stage.Name, onOutput)
			}()
			stepFlushTicker := time.NewTicker(durableBuildLogFlushInterval)
			var stepErr error
			stepSettled := false
			for !stepSettled {
				select {
				case stepErr = <-stepErrCh:
					stepSettled = true
				case <-stepFlushTicker.C:
					_ = r.flushBuildOutput(buildID, stage.Name)
				case <-stageCtx.Done():
					cancelStage()
					stepErr = <-stepErrCh
					stepSettled = true
				}
			}
			stepFlushTicker.Stop()
			_ = r.flushBuildOutput(buildID, stage.Name)
			stepErr = completedStepError(stageCtx, stepErr)
			if stepErr != nil {
				if ctx.Err() == context.Canceled {
					cancelStage()
					r.runPostSteps(ctx, cfg, "failure", workspace, project, build, env, params)
					r.log(buildID, stage.Name, fmt.Sprintf("CANCELLED: step %q stopped by user", step.Name))
					r.flushDurableBuildLog(buildID, true)
					_ = r.store.CancelBuild(buildID)
					_ = r.store.UpdateBuildQueueItemStatusByBuildID(buildID, "cancelled")
					r.broadcastStatus(buildID, "cancelled", stage.Name, 1)
					return
				}
				r.log(buildID, stage.Name, fmt.Sprintf("ERROR: %v", stepErr))
				if ctx.Err() == context.DeadlineExceeded {
					cancelStage()
					r.runPostSteps(ctx, cfg, "failure", workspace, project, build, env, params)
					r.fail(buildID, start, fmt.Sprintf("step %q timed out after %ds", step.Name, build.TimeoutSec), project)
					r.finishCleanup(buildID)
					return
				}
				stageFailed = true
				if stageCtx.Err() == context.DeadlineExceeded {
					failedSteps = append(failedSteps, stage.Name+" (timeout)")
					r.log(buildID, stage.Name, fmt.Sprintf("Stage timed out after %ds", stage.TimeoutSec))
					if r.executionPolicySnapshot().FailFast {
						cancelStage()
						r.runPostSteps(ctx, cfg, "failure", workspace, project, build, env, params)
						r.fail(buildID, start, fmt.Sprintf("stage %q timed out after %ds", stage.Name, stage.TimeoutSec), project)
						r.finishCleanup(buildID)
						return
					}
					break
				}
				if r.executionPolicySnapshot().FailFast {
					cancelStage()
					r.runPostSteps(ctx, cfg, "failure", workspace, project, build, env, params)
					r.fail(buildID, start, fmt.Sprintf("step %q failed: %v", step.Name, stepErr), project)
					r.finishCleanup(buildID)
					return
				}
				failedSteps = append(failedSteps, stage.Name+" / "+step.Name)
				r.log(buildID, stage.Name, "Failure recorded; continuing because fail-fast is disabled")
				continue
			}
			r.log(buildID, stage.Name, fmt.Sprintf("--- Step complete: %s ---", step.Name))
			stageEnv = append(stageEnv, outputs...)
		}
		cancelStage()
		env = append(env, stageEnv[stageBaseEnvLength:]...)
		if stageFailed {
			stageStatuses[stageKey] = StageStatusFailed
			r.log(buildID, stage.Name, fmt.Sprintf("=== Stage failed: %s ===", stage.Name))
		} else {
			stageStatuses[stageKey] = StageStatusSuccess
			r.log(buildID, stage.Name, fmt.Sprintf("=== Stage complete: %s ===", stage.Name))
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

	if len(failedSteps) > 0 {
		r.runPostSteps(ctx, cfg, "failure", workspace, project, build, env, params)
		r.fail(buildID, start, fmt.Sprintf("%d step(s) failed: %s", len(failedSteps), strings.Join(failedSteps, ", ")), project)
		r.finishCleanup(buildID)
		return
	}
	r.runPostSteps(ctx, cfg, "success", workspace, project, build, env, params)

	duration := time.Since(start).Milliseconds()
	r.log(buildID, "", fmt.Sprintf("Build #%d succeeded in %s", build.Number, time.Since(start)))
	r.flushDurableBuildLog(buildID, true)
	_ = r.store.FinishBuild(buildID, "success", duration)
	_ = r.store.UpdateBuildQueueItemStatusByBuildID(buildID, "success")
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
			r.sendBuildEventAsync(buildID, finishedBuild, project, "build.completed")
		}
	}
	r.cleanupCompleted(project.ID, cfg.RetentionCompleted)

	r.recordStatistics(buildID)
}

func stageMatchesBranch(stage Stage, branch string) bool {
	if len(stage.Branches) == 0 {
		return true
	}
	for _, allowed := range stage.Branches {
		if branch == allowed {
			return true
		}
	}
	return false
}

func (r *BuildRunner) cleanupCompleted(projectID int64, retain int) {
	if retain == 0 {
		retain = 30
	}
	if retain < 0 {
		return
	}
	if err := r.store.PruneCompletedBuilds(projectID, retain); err != nil {
		// Retention is best-effort and must never change a finished build result.
		return
	}
}

func (r *BuildRunner) deferForProjectConcurrency(buildID, projectID int64, abortPrevious bool) bool {
	builds, err := r.store.ListBuildsByProject(projectID)
	if err != nil {
		return false
	}
	for _, other := range builds {
		if other.ID == buildID || other.Status != "running" {
			continue
		}
		if abortPrevious {
			r.log(buildID, "queue", fmt.Sprintf("Cancelling Build #%d due to disableConcurrentBuilds", other.Number))
			r.Stop(other.ID)
		} else {
			r.log(buildID, "queue", fmt.Sprintf("Waiting for Build #%d due to disableConcurrentBuilds", other.Number))
		}
		return true
	}
	return false
}

func (r *BuildRunner) runPostSteps(_ context.Context, cfg *BuildConfig, outcome, workspace string, project *store.Project, build *store.Build, env []string, params map[string]interface{}) {
	postCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	conditions := []string{"always", outcome, "cleanup"}
	for _, condition := range conditions {
		for _, step := range cfg.Post[condition] {
			stage := "post " + condition
			r.log(build.ID, stage, fmt.Sprintf("--- Step: %s ---", step.Name))
			stepEnv := appendRuntimeEnv(env, cfg, step.Runtime)
			err := r.execStep(postCtx, step, workspace, project, build, stepEnv, params, stage, func(line string) {
				r.logBuildOutput(build.ID, stage, line)
			})
			_ = r.flushBuildOutput(build.ID, stage)
			if err != nil {
				r.log(build.ID, stage, fmt.Sprintf("ERROR: %v", err))
			}
		}
		if _, err := r.runPluginHook(postCtx, "build."+condition, outcome, workspace, project, build, env); err != nil {
			r.log(build.ID, "plugin build."+condition, fmt.Sprintf("ERROR: %v", err))
		}
	}
}

func (r *BuildRunner) runPluginHook(ctx context.Context, hook, outcome, workspace string, project *store.Project, build *store.Build, env []string) ([]string, error) {
	if r.plugins == nil {
		return env, nil
	}
	environment := make(map[string]string, len(env))
	for _, entry := range env {
		if key, value, ok := strings.Cut(entry, "="); ok {
			environment[key] = value
		}
	}
	sc := &plugin.StepContext{
		Workspace:   workspace,
		ProjectID:   project.ID,
		ProjectName: project.Name,
		BuildID:     build.ID,
		BuildNumber: build.Number,
		Branch:      build.Branch,
		Commit:      build.CommitSHA,
		Outcome:     outcome,
		Environment: environment,
	}
	if err := r.plugins.RunHooks(ctx, hook, sc); err != nil {
		return env, err
	}
	stage := "plugin " + hook
	for _, line := range sc.Logs {
		r.logBuildOutput(build.ID, stage, line)
	}
	keys := make([]string, 0, len(sc.Env)+len(sc.Outputs))
	values := make(map[string]string, len(sc.Env)+len(sc.Outputs))
	for key, value := range sc.Env {
		keys = append(keys, key)
		values[key] = value
	}
	for key, value := range sc.Outputs {
		if _, exists := values[key]; !exists {
			keys = append(keys, key)
		}
		values[key] = value
	}
	sort.Strings(keys)
	for _, key := range keys {
		env = append(env, key+"="+values[key])
	}
	_ = r.flushBuildOutput(build.ID, stage)
	return env, nil
}

func (r *BuildRunner) loadBuildConfig(ctx context.Context, project *store.Project) (*BuildConfig, error) {
	source, err := ResolveProjectPipelineSource(ctx, project)
	if err != nil {
		return nil, err
	}
	cfg, err := ParsePipelineConfig(source)
	if err != nil {
		return nil, err
	}
	if project.TemplateID != nil {
		tmpl, err := r.store.GetBuildTemplate(*project.TemplateID)
		if err == nil {
			tmplCfg, err := ParsePipelineConfig(tmpl.Config)
			if err == nil {
				cfg = MergeBuildConfig(tmplCfg, cfg)
			}
		}
	}
	if err := ValidatePipelineSemantics(cfg); err != nil {
		return nil, err
	}
	if cfg.Environment == nil {
		cfg.Environment = map[string]string{}
	}
	return cfg, nil
}

func (r *BuildRunner) matchAgent(build *store.Build, project *store.Project) bool {
	cfg, err := r.loadBuildConfig(context.Background(), project)
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

// selectRemoteWorker only dispatches when a pipeline explicitly asks for an
// agent label/pool. Empty requirements intentionally keep Buildworld's embedded
// local worker as the default scheduler. When multiple workers in the same pool
// match, round-robin distributes builds across them.
func (r *BuildRunner) selectRemoteWorker(cfg *BuildConfig) *store.Worker {
	if len(cfg.AgentRequirements) == 0 {
		return nil
	}
	_ = r.store.MarkOfflineWorkers()
	workers, err := r.store.ListOnlineWorkers()
	if err != nil {
		return nil
	}
	var candidates []*store.Worker
	for _, w := range workers {
		if r.workerMatchesRequirements(w, cfg.AgentRequirements) && w.ActiveBuilds < w.MaxConcurrentBuilds {
			candidates = append(candidates, w)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	// Build a stable pool key from the requirements so round-robin state is
	// shared across identical agent_requirements values.
	poolKey := strings.Join(cfg.AgentRequirements, "|")
	startIdx := 0
	if len(candidates) > 1 {
		r.poolCursorMu.Lock()
		startIdx = r.poolCursor[poolKey] % len(candidates)
		r.poolCursor[poolKey] = startIdx + 1
		r.poolCursorMu.Unlock()
	}
	for i := 0; i < len(candidates); i++ {
		idx := (startIdx + i) % len(candidates)
		w := candidates[idx]
		acquired, err := r.store.TryAcquireWorker(w.ID)
		if err == nil && acquired {
			w.ActiveBuilds++
			return w
		}
	}
	return nil
}

func (r *BuildRunner) markRemoteWaiting(buildID int64) bool {
	r.remoteWaitMu.Lock()
	defer r.remoteWaitMu.Unlock()
	if r.remoteWaiting[buildID] {
		return false
	}
	r.remoteWaiting[buildID] = true
	return true
}

func (r *BuildRunner) clearRemoteWaiting(buildID int64) {
	r.remoteWaitMu.Lock()
	delete(r.remoteWaiting, buildID)
	r.remoteWaitMu.Unlock()
}

func (r *BuildRunner) runRemote(ctx context.Context, build *store.Build, project *store.Project, cfg *BuildConfig, worker *store.Worker, start time.Time) {
	defer r.store.ReleaseWorker(worker.ID)
	if r.workerDispatchToken == "" {
		r.fail(build.ID, start, "remote worker dispatch requires workers.enrollment_token", project)
		return
	}
	pluginRefs, err := r.remotePluginReferences(cfg)
	if err != nil {
		r.fail(build.ID, start, err.Error(), project)
		return
	}
	_ = r.store.StartBuild(build.ID)
	_ = r.store.UpdateBuildQueueItemStatusByBuildID(build.ID, "running")
	r.log(build.ID, "", encodeTimelinePlan(cfg))
	r.broadcastStatus(build.ID, "running", "remote", 0)
	r.log(build.ID, "remote", "Dispatching to worker "+worker.Name+" at "+worker.Address)
	conn, err := grpc.DialContext(ctx, worker.Address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		r.fail(build.ID, start, fmt.Sprintf("remote worker dial: %v", err), project)
		return
	}
	defer conn.Close()
	env := map[string]string{}
	for _, entry := range r.buildEnv(build, cfg, project) {
		if i := strings.Index(entry, "="); i > 0 {
			env[entry[:i]] = entry[i+1:]
		}
	}
	dispatchCtx := bytemsg.WithDispatchCredential(ctx, r.workerDispatchToken)
	pipelineConfig, err := ResolveProjectPipelineSource(ctx, project)
	if err != nil {
		r.fail(build.ID, start, fmt.Sprintf("resolve pipeline source: %v", err), project)
		return
	}
	stream, err := pb.NewWorkerServiceClient(conn).ExecuteBuild(dispatchCtx, &pb.BuildRequest{BuildId: fmt.Sprintf("%d", build.ID), ProjectName: project.Name, RepoUrl: project.RepoURL, Branch: build.Branch, CommitSha: build.CommitSHA, PipelineConfig: pipelineConfig, Environment: env, Protocol: bytemsg.NewProtocolInfo(), Plugins: pluginRefs})
	if err != nil {
		r.fail(build.ID, start, fmt.Sprintf("remote worker start: %v", err), project)
		return
	}
	artifacts := make(map[string]*remoteArtifactBuffer)
	completed := false
	for {
		response, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			_ = r.flushBuildOutput(build.ID, "remote")
			if ctx.Err() == context.Canceled {
				r.flushDurableBuildLog(build.ID, true)
				_ = r.store.CancelBuild(build.ID)
				_ = r.store.UpdateBuildQueueItemStatusByBuildID(build.ID, "cancelled")
				r.broadcastStatus(build.ID, "cancelled", "remote", 1)
				return
			}
			r.fail(build.ID, start, fmt.Sprintf("remote worker stream: %v", err), project)
			return
		}
		if err := bytemsg.Validate(response.Protocol); err != nil {
			_ = r.flushBuildOutput(build.ID, "remote")
			r.fail(build.ID, start, "remote worker response: "+err.Error(), project)
			return
		}
		if chunk := response.GetArtifactChunk(); chunk != nil {
			if err := r.receiveRemoteArtifact(build.ID, artifacts, chunk); err != nil {
				_ = r.flushBuildOutput(build.ID, "remote")
				r.fail(build.ID, start, fmt.Sprintf("remote artifact: %v", err), project)
				return
			}
			continue
		}
		if response.Output != "" {
			r.logBuildOutput(build.ID, response.Stage, response.Output)
		}
		if response.IsError || response.Status == "failed" {
			_ = r.flushBuildOutput(build.ID, response.Stage)
			r.fail(build.ID, start, response.Output, project)
			return
		}
		if response.Status == "success" {
			completed = true
		}
	}
	_ = r.flushBuildOutput(build.ID, "remote")
	if !completed {
		r.fail(build.ID, start, "remote worker stream ended without a successful terminal response", project)
		return
	}
	if len(artifacts) != 0 {
		r.fail(build.ID, start, "remote worker stream ended before all artifacts were finalized", project)
		return
	}
	duration := time.Since(start).Milliseconds()
	r.flushDurableBuildLog(build.ID, true)
	_ = r.store.FinishBuild(build.ID, "success", duration)
	_ = r.store.UpdateBuildQueueItemStatusByBuildID(build.ID, "success")
	r.broadcastStatus(build.ID, "success", "", 1)
	r.cleanupCompleted(project.ID, cfg.RetentionCompleted)
	r.recordStatistics(build.ID)
}

func (r *BuildRunner) remotePluginReferences(cfg *BuildConfig) ([]*pb.PluginReference, error) {
	if r.plugins == nil || cfg == nil {
		return nil, nil
	}
	stepTypes := make([]string, 0)
	for _, stage := range cfg.Stages {
		for _, step := range stage.Steps {
			stepTypes = append(stepTypes, step.Type)
		}
	}
	references, err := r.plugins.BinaryReferencesForStepTypes(stepTypes)
	if err != nil {
		return nil, err
	}
	result := make([]*pb.PluginReference, 0, len(references))
	for _, ref := range references {
		result = append(result, &pb.PluginReference{Name: ref.Name, Version: ref.Version, Source: ref.Source, ManifestSha256: ref.ManifestSHA256})
	}
	return result, nil
}

func (r *BuildRunner) receiveRemoteArtifact(buildID int64, pending map[string]*remoteArtifactBuffer, chunk *pb.ArtifactChunk) error {
	if r.artifacts == nil {
		return fmt.Errorf("artifact storage is not configured")
	}
	name := filepath.Base(chunk.Name)
	if name == "." || name == "" {
		return fmt.Errorf("invalid artifact name")
	}
	artifact := pending[name]
	if artifact == nil {
		artifact = &remoteArtifactBuffer{name: name}
		pending[name] = artifact
	}
	if artifact.data.Len()+len(chunk.Data) > remoteArtifactMaxSize {
		return fmt.Errorf("artifact %s exceeds the %d MiB remote transfer limit", name, remoteArtifactMaxSize/(1024*1024))
	}
	if _, err := artifact.data.Write(chunk.Data); err != nil {
		return err
	}
	if !chunk.FinalChunk {
		return nil
	}
	if chunk.Sha256 == "" {
		return fmt.Errorf("artifact %s is missing its checksum", name)
	}
	actual := fmt.Sprintf("%x", sha256.Sum256(artifact.data.Bytes()))
	if !strings.EqualFold(actual, chunk.Sha256) {
		return fmt.Errorf("artifact %s checksum mismatch", name)
	}
	if _, err := r.artifacts.Save(buildID, name, bytes.NewReader(artifact.data.Bytes())); err != nil {
		return err
	}
	r.log(buildID, "artifacts", fmt.Sprintf("Saved remote artifact %s (%d bytes)", name, artifact.data.Len()))
	delete(pending, name)
	return nil
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

func (r *BuildRunner) execStep(ctx context.Context, step Step, workspace string, project *store.Project, build *store.Build, env []string, params map[string]interface{}, stage string, onOutput func(string)) error {
	if err := r.execBaseStep(ctx, step, workspace, project, build, env, params, stage, onOutput); err != nil {
		return err
	}
	platform := PlatformName()
	if addition := step.PlatformAdditions[platform]; strings.TrimSpace(addition) != "" {
		r.log(buildIDOf(build), stage, fmt.Sprintf("--- Platform addition: %s ---", platform))
		platformStep := step
		platformStep.Command = addition
		platformStep.PlatformAdditions = nil
		return r.execBaseStep(ctx, platformStep, workspace, project, build, env, params, stage, onOutput)
	}
	return nil
}

// completedStepError closes the race between a command returning and its
// context being cancelled. A cancelled step must never emit a success marker.
func completedStepError(ctx context.Context, stepErr error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		if stepErr != nil {
			return fmt.Errorf("%w (process: %v)", ctxErr, stepErr)
		}
		return ctxErr
	}
	return stepErr
}

func (r *BuildRunner) execBaseStep(ctx context.Context, step Step, workspace string, project *store.Project, build *store.Build, env []string, params map[string]interface{}, stage string, onOutput func(string)) error {
	stepEnv := AppendStepEnvironment(env, step.Config)
	workingDirectory := workspace
	if configured := step.Config["working-directory"]; configured != "" {
		resolved, err := ResolveWorkspaceDirectory(workspace, resolveVars(configured, stepEnv, params))
		if err != nil {
			return err
		}
		workingDirectory = resolved
	}
	switch step.Type {
	case "service_watch":
		return WatchService(ctx, workingDirectory, resolveWatchConfig(step.Config, stepEnv, params), onOutput)
	case "shell", "", "tail":
		command := resolveVars(step.Command, stepEnv, params)
		if step.Shell != "" {
			return r.executor.RunMultiShell(ctx, step.Shell, command, workingDirectory, stepEnv, func(line string) {
				onOutput(strings.TrimRight(line, "\r\n"))
			})
		}
		return r.executor.RunShell(ctx, command, workingDirectory, stepEnv, func(line string) {
			onOutput(strings.TrimRight(line, "\r\n"))
		})
	case "powershell", "ps1", "pwsh":
		command := resolveVars(step.Command, stepEnv, params)
		return r.executor.RunMultiShell(ctx, step.Type, command, workingDirectory, stepEnv, func(line string) {
			onOutput(strings.TrimRight(line, "\r\n"))
		})
	case "bash", "sh", "python", "python3", "cmd":
		command := resolveVars(step.Command, stepEnv, params)
		return r.executor.RunMultiShell(ctx, step.Type, command, workingDirectory, stepEnv, func(line string) {
			onOutput(strings.TrimRight(line, "\r\n"))
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
		if err := r.gitClient.CloneContext(ctx, url, workspace); err != nil {
			return fmt.Errorf("git clone: %w", err)
		}
		if branch != "" {
			if err := r.gitClient.CheckoutContext(ctx, workspace, branch); err != nil {
				return fmt.Errorf("git checkout: %w", err)
			}
		}
		return nil
	case "notify":
		if r.notificationService == nil {
			onOutput("Notification skipped: no notification service is configured")
			return nil
		}
		currentBuild, err := r.store.GetBuild(build.ID)
		if err != nil {
			return fmt.Errorf("load build for pipeline notification: %w", err)
		}
		safeBuild, safeProject := r.maskedNotificationObjects(build.ID, currentBuild, project)
		if err := r.notificationService.SendBuildEvent(safeBuild, safeProject, "pipeline.notification"); err != nil {
			return fmt.Errorf("send pipeline notification: %w", err)
		}
		onOutput("Pipeline notification delivered")
		return nil
	case "script":
		scriptPath := filepath.Join(workingDirectory, ".bw_step.sh")
		if err := os.WriteFile(scriptPath, []byte(step.Command), 0o755); err != nil {
			return err
		}
		return r.executor.RunShell(ctx, "sh .bw_step.sh", workingDirectory, stepEnv, func(line string) {
			onOutput(strings.TrimRight(line, "\r\n"))
		})
	default:
		if r.plugins != nil {
			if handler := r.plugins.LookupStep(step.Type); handler != nil {
				sc := &plugin.StepContext{
					Workspace: workingDirectory,
					Branch:    build.Branch,
					Commit:    build.CommitSHA,
					Config:    step.Config,
				}
				if err := handler(ctx, sc); err != nil {
					return err
				}
				for _, l := range sc.Logs {
					onOutput(l)
				}
				// Plugin output uses the same explicit marker as shell nodes so the
				// enclosing runner can make it available to all following nodes.
				for k, v := range sc.Outputs {
					onOutput(fmt.Sprintf("::buildworld:set %s=%s", k, v))
				}
				for k, v := range sc.Env {
					onOutput(fmt.Sprintf("::buildworld:set %s=%s", k, v))
				}
				return nil
			}
		}
		return fmt.Errorf("unsupported step type: %s", step.Type)
	}
}

func resolveWatchConfig(config map[string]string, env []string, params map[string]interface{}) map[string]string {
	resolved := make(map[string]string, len(config))
	for key, value := range config {
		resolved[key] = resolveVars(value, env, params)
	}
	return resolved
}

// appendRuntimeEnv resolves a single configured version automatically. Multiple
// installed versions remain explicit through a fence header such as `go@1.26`.
func appendRuntimeEnv(env []string, cfg *BuildConfig, selector string) []string {
	if selector == "" || cfg == nil {
		return env
	}
	parts := strings.SplitN(strings.ToLower(selector), "@", 2)
	language := parts[0]
	version := ""
	if len(parts) == 2 {
		version = parts[1]
	}
	if version == "" && len(cfg.Toolchains[language]) == 1 {
		version = cfg.Toolchains[language][0]
	}
	if version == "" {
		return env
	}
	env = append(env, "BUILDWORLD_RUNTIME_"+strings.ToUpper(strings.ReplaceAll(language, "-", "_"))+"="+version)
	if language == "go" {
		env = append(env, "GOTOOLCHAIN=go"+version)
	}
	return env
}

// AppendRuntimeEnvironment exposes the runtime selector used by both the
// embedded runner and remote workers. A pipeline therefore has one runtime
// resolution rule regardless of where it is scheduled.
func AppendRuntimeEnvironment(env []string, cfg *BuildConfig, selector string) []string {
	return appendRuntimeEnv(env, cfg, selector)
}

// ResolvePipelineVariables resolves Buildworld variable scopes for a worker.
func ResolvePipelineVariables(command string, env []string, params map[string]interface{}) string {
	return resolveVars(command, env, params)
}

// AppendBuildOutputs promotes explicit node-output markers to later steps.
func AppendBuildOutputs(env []string, output string) []string {
	return appendBuildKV(env, output)
}

// PlatformName is the pipeline platform key for the current host.
func PlatformName() string {
	if runtime.GOOS == "darwin" {
		return "macos"
	}
	return runtime.GOOS
}

func buildIDOf(b *store.Build) int64 { return b.ID }

func (r *BuildRunner) buildEnv(build *store.Build, cfg *BuildConfig, project *store.Project) []string {
	return r.buildEnvAt(build, cfg, project, "")
}

func (r *BuildRunner) buildEnvAt(build *store.Build, cfg *BuildConfig, project *store.Project, workspace string) []string {
	// Keep the Jenkins environment contract for Jenkinsfile pipelines.  Define
	// these before configured environments so an explicitly configured pipeline
	// value retains Jenkins' normal override behaviour.
	buildNumber := strconv.Itoa(build.Number)
	envMap := map[string]string{
		"BUILD_NUMBER":       buildNumber,
		"BUILD_ID":           buildNumber,
		"BUILD_DISPLAY_NAME": "#" + buildNumber,
		"BUILD_TAG":          "buildworld-" + project.Name + "-" + buildNumber,
		"JOB_NAME":           project.Name,
		"JOB_BASE_NAME":      project.Name,
	}
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
	if workspace != "" {
		envMap["WORKSPACE"] = workspace
	}
	return ExpandConfiguredEnvironment(envMap, os.Environ())
}

// ExpandConfiguredEnvironment applies Jenkins-compatible environment
// interpolation before local or remote execution. A self reference such as
// PATH=${PATH}:/opt/go resolves against the worker process environment, while
// references to other configured variables resolve in stable passes.
func ExpandConfiguredEnvironment(configured map[string]string, base []string) []string {
	baseValues := make(map[string]string, len(base))
	for _, entry := range base {
		if key, value, found := strings.Cut(entry, "="); found {
			baseValues[key] = value
		}
	}
	keys := make([]string, 0, len(configured))
	for key := range configured {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	resolved := make(map[string]string, len(configured))
	for pass := 0; pass <= len(keys); pass++ {
		for _, current := range keys {
			resolved[current] = os.Expand(configured[current], func(reference string) string {
				if reference == current {
					if value, exists := baseValues[reference]; exists {
						return value
					}
					return "${" + reference + "}"
				}
				if value, exists := resolved[reference]; exists {
					return value
				}
				if value, exists := configured[reference]; exists {
					return value
				}
				if value, exists := baseValues[reference]; exists {
					return value
				}
				return "${" + reference + "}"
			})
		}
	}
	environment := make([]string, 0, len(keys))
	for _, key := range keys {
		environment = append(environment, key+"="+resolved[key])
	}
	return environment
}

var varPattern = regexp.MustCompile(`\$\{(global|project|parameter|env|build)\.([A-Za-z_][A-Za-z0-9_]*)\}`)
var buildKVPattern = regexp.MustCompile(`(?m)::buildworld:set\s+([A-Za-z_][A-Za-z0-9_]*)=([^\r\n]+)`)

// appendBuildKV promotes explicit Buildworld output markers to the following
// nodes. Example: echo "::buildworld:set IMAGE=registry/app:${BUILD_ID}" and
// then use ${build.IMAGE} in any later node.
func appendBuildKV(env []string, command string) []string {
	for _, match := range buildKVPattern.FindAllStringSubmatch(command, -1) {
		if len(match) == 3 {
			env = append(env, match[1]+"="+strings.TrimSpace(match[2]))
		}
	}
	return env
}

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
			if v, ok := envMap[name]; ok {
				return v
			}
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

func (r *BuildRunner) log(buildID int64, stage, line string) error {
	return r.logMasked(buildID, r.maskBuildText(buildID, stage), r.maskBuildText(buildID, line))
}

func (r *BuildRunner) logMasked(buildID int64, stage, line string) error {
	line = boundLiveBuildLogLine(line)
	ts := time.Now().Format("15:04:05")
	entry := fmt.Sprintf("[%s] [%s] %s\n", ts, stage, line)
	payload := map[string]interface{}{
		"timestamp": ts, "stage": stage, "line": line, "level": "info",
	}
	if r.liveLogBroadcast != nil {
		r.liveLogBroadcast(buildID, payload)
	} else if r.hub != nil {
		ws.BroadcastBuildLog(r.hub, fmt.Sprintf("%d", buildID), payload)
	}
	return r.appendDurableBuildLog(buildID, entry)
}

func (r *BuildRunner) broadcastStatus(buildID int64, status, stage string, progress float64) {
	switch status {
	case "success", "failed", "cancelled":
		r.flushDurableBuildLog(buildID, true)
	}
	if r.hub == nil {
		return
	}
	ws.BroadcastBuildStatus(r.hub, fmt.Sprintf("%d", buildID), map[string]interface{}{
		"status": r.maskBuildText(buildID, status), "stage": r.maskBuildText(buildID, stage), "progress": progress,
	})
}

func (r *BuildRunner) fail(buildID int64, start time.Time, msg string, project *store.Project) {
	r.log(buildID, "", "BUILD FAILED: "+msg)
	// Persist the complete failure summary before status becomes terminal. Store
	// status is used as the completion barrier by API clients and test runners.
	r.flushDurableBuildLog(buildID, true)
	_ = r.store.FinishBuild(buildID, "failed", time.Since(start).Milliseconds())
	_ = r.store.UpdateBuildQueueItemStatusByBuildID(buildID, "failed")

	if r.notificationService != nil && project != nil {
		finishedBuild, _ := r.store.GetBuild(buildID)
		if finishedBuild != nil {
			r.sendBuildEventAsync(buildID, finishedBuild, project, "build.completed")
		}
	}

	r.recordStatistics(buildID)
	r.scheduleAutomaticRetry(buildID)
	// Automatic-retry diagnostics belong to the failed build. Flush them before
	// publishing terminal status so they cannot recreate leaked per-build state.
	r.flushDurableBuildLog(buildID, true)
	r.broadcastStatus(buildID, "failed", "", 1.0)
}

func (r *BuildRunner) scheduleAutomaticRetry(buildID int64) {
	limit := r.executionPolicySnapshot().RetryLimit
	if limit == 0 {
		return
	}
	build, err := r.store.GetBuild(buildID)
	if err != nil {
		return
	}
	depth := 0
	seen := map[int64]struct{}{build.ID: {}}
	for build.RetriedFrom != nil {
		depth++
		if depth >= limit {
			return
		}
		parentID := *build.RetriedFrom
		if _, exists := seen[parentID]; exists {
			return
		}
		seen[parentID] = struct{}{}
		build, err = r.store.GetBuild(parentID)
		if err != nil {
			return
		}
	}
	retry, err := r.store.RetryBuild(buildID)
	if err != nil {
		r.log(buildID, "", fmt.Sprintf("Automatic retry could not be scheduled: %v", err))
		return
	}
	r.log(buildID, "", fmt.Sprintf("Automatic retry scheduled as build #%d", retry.Number))
	if err := r.Enqueue(retry.ID); err != nil {
		r.log(buildID, "", fmt.Sprintf("Automatic retry could not enter the queue: %v", err))
	}
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

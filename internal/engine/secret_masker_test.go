package engine

import (
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/neko233-com/buildworld/internal/bytemsg"
	pb "github.com/neko233-com/buildworld/internal/rpc/generated"
	"github.com/neko233-com/buildworld/internal/store"
	"google.golang.org/grpc"
)

func TestSecretMaskerMasksLiteralAndIgnoresEmptyOrShortValues(t *testing.T) {
	masker := newSecretMasker("", "    ", "abc", " abc ", "literal-secret", "literal-secret")
	got := masker.Mask("abc literal-secret remains protected; short padded value= abc ")
	if got != "abc ******** remains protected; short padded value= abc " {
		t.Fatalf("Mask() = %q", got)
	}
}

func TestStreamingSecretMaskerMasksAcrossChunkBoundariesWithoutHoldingSafeTail(t *testing.T) {
	masker := newStreamingSecretMasker(newSecretMasker("cross-chunk-secret"))
	first := masker.Write("live output cross-ch")
	if first != "live output " {
		t.Fatalf("first safe output = %q", first)
	}
	second := masker.Write("unk-secret continues")
	if second != "******** continues" {
		t.Fatalf("second safe output = %q", second)
	}
	if tail := masker.Flush(); tail != "" {
		t.Fatalf("unexpected tail = %q", tail)
	}
}

func TestStreamingSecretMaskerHandlesOverlappingSecretPrefixes(t *testing.T) {
	masker := newStreamingSecretMasker(newSecretMasker("abcd", "bcde"))
	output := masker.Write("abcd") + masker.Flush()
	if output != maskedSecretValue {
		t.Fatalf("overlapping output = %q", output)
	}
}

func TestStreamingSecretMaskerMasksEveryTwoChunkSplit(t *testing.T) {
	const secret = "every-boundary-secret"
	input := "before=" + secret + ";after"
	for split := 0; split <= len(input); split++ {
		masker := newStreamingSecretMasker(newSecretMasker(secret))
		output := masker.Write(input[:split]) + masker.Write(input[split:]) + masker.Flush()
		if strings.Contains(output, secret) || output != "before=********;after" {
			t.Fatalf("split %d output = %q", split, output)
		}
	}
}

func TestBuildRunnerMasksLocalSecretOutputFailureProblemsBroadcastAndNotifications(t *testing.T) {
	root := t.TempDir()
	database, err := store.New(filepath.Join(root, "local-secret-mask.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	const (
		globalSecret  = "global-secret-8192"
		projectSecret = "project-secret-4096"
		paramSecret   = "parameter-secret-2048"
	)
	command := fmt.Sprintf("echo %s && echo %s && echo %s && buildworld-command-that-does-not-exist", globalSecret, projectSecret, paramSecret)
	config := fmt.Sprintf(`import { definePipeline, parameter, shell, stage } from "@buildworld/pipeline"
export default definePipeline({
  parameters: [parameter("token", "password", { required: true, isSecret: true })],
  stages: [stage("Leak check", shell("Fail safely", %s))],
})`, strconv.Quote(command))
	project, err := database.CreateProject("local-secret-mask", "", "", "git", paramSecret, config, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SetEnvVar("global", nil, "GLOBAL_SECRET", globalSecret, true, ""); err != nil {
		t.Fatal(err)
	}
	if err := database.SetEnvVar("project", &project.ID, "PROJECT_SECRET", projectSecret, true, ""); err != nil {
		t.Fatal(err)
	}
	params, _ := json.Marshal(map[string]interface{}{"token": paramSecret})
	build, err := database.CreateBuild(project.ID, 1, "manual", paramSecret, "", string(params), nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	runner := NewBuildRunner(database, nil, filepath.Join(root, "workspaces"), nil)
	if err := runner.ConfigureExecutionPolicy(ExecutionPolicy{DefaultTimeoutSec: 10, MaxConcurrentBuilds: 1, MaxConcurrentLocalBuilds: 1, RetryLimit: 0, FailFast: true}); err != nil {
		t.Fatal(err)
	}
	runner.SetNotificationService(NewNotificationService(database))
	var broadcastMu sync.Mutex
	var broadcasts []string
	runner.liveLogBroadcast = func(_ int64, payload map[string]interface{}) {
		encoded, _ := json.Marshal(payload)
		broadcastMu.Lock()
		broadcasts = append(broadcasts, string(encoded))
		broadcastMu.Unlock()
	}

	runner.Run(build.ID)
	finished := waitForBuildStatus(t, database, build.ID, 8*time.Second, "failed")
	waitForBuildNotificationEvents(t, database, build.ID, 2)

	assertNoSecretValues(t, "durable build log", finished.Log, globalSecret, projectSecret, paramSecret)
	if !strings.Contains(finished.Log, maskedSecretValue) {
		t.Fatalf("durable build log did not contain mask:\n%s", finished.Log)
	}
	problems, _ := json.Marshal(AnalyzeBuildProblems(finished.Status, finished.Log))
	assertNoSecretValues(t, "failure problem report", string(problems), globalSecret, projectSecret, paramSecret)

	broadcastMu.Lock()
	liveOutput := strings.Join(broadcasts, "\n")
	broadcastMu.Unlock()
	assertNoSecretValues(t, "live WebSocket payload", liveOutput, globalSecret, projectSecret, paramSecret)
	assertNotificationEventsContainNoSecrets(t, database, globalSecret, projectSecret, paramSecret)
}

type secretMaskRemoteWorker struct {
	pb.UnimplementedWorkerServiceServer
	parameterSecret  string
	credentialSecret string
}

func (w *secretMaskRemoteWorker) ExecuteBuild(request *pb.BuildRequest, stream grpc.ServerStreamingServer[pb.BuildResponse]) error {
	cut := len(w.parameterSecret) / 2
	responses := []*pb.BuildResponse{
		{BuildId: request.BuildId, Stage: "remote", Output: "remote=" + w.parameterSecret[:cut], Status: "running", Protocol: bytemsg.NewProtocolInfo()},
		{BuildId: request.BuildId, Stage: "remote", Output: w.parameterSecret[cut:] + " credential=" + w.credentialSecret, Status: "running", Protocol: bytemsg.NewProtocolInfo()},
		{BuildId: request.BuildId, Stage: "remote", Output: "remote failed with " + w.parameterSecret, Status: "failed", IsError: true, Protocol: bytemsg.NewProtocolInfo()},
	}
	for _, response := range responses {
		if err := stream.Send(response); err != nil {
			return err
		}
	}
	return nil
}

func TestBuildRunnerMasksRemoteChunkedParameterAndCredentialOutput(t *testing.T) {
	root := t.TempDir()
	database, err := store.New(filepath.Join(root, "remote-secret-mask.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	const (
		parameterSecret  = "remote-parameter-secret-7777"
		credentialSecret = "vcs-credential-secret-8888"
	)
	credential, err := database.CreateCredential("remote-vcs", store.CredentialTypeGit, "", "buildworld", credentialSecret, "", "", "", "", true)
	if err != nil {
		t.Fatal(err)
	}
	vcsRoot, err := database.CreateVCSRoot("remote-vcs", "git", "", "main", &credential.ID, 0, true, "{}")
	if err != nil {
		t.Fatal(err)
	}
	config := `import { definePipeline, parameter } from "@buildworld/pipeline"
export default definePipeline({
  agentRequirements: ["secret-mask-worker"],
  parameters: [parameter("token", "password", { required: true, isSecret: true })],
  stages: [],
})`
	project, err := database.CreateProject("remote-secret-mask", "", "", "git", parameterSecret, config, 0, &vcsRoot.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	params, _ := json.Marshal(map[string]interface{}{"token": parameterSecret})
	build, err := database.CreateBuild(project.ID, 1, "manual", parameterSecret, "", string(params), nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	server := grpc.NewServer()
	pb.RegisterWorkerServiceServer(server, &secretMaskRemoteWorker{parameterSecret: parameterSecret, credentialSecret: credentialSecret})
	t.Cleanup(server.Stop)
	go func() { _ = server.Serve(listener) }()
	if _, err := database.CreateWorker("secret-mask-worker", "secret-mask-worker", listener.Addr().String(), "unused", []string{"secret-mask-worker"}, 1, ""); err != nil {
		t.Fatal(err)
	}

	runner := NewBuildRunner(database, nil, filepath.Join(root, "workspaces"), nil)
	runner.SetWorkerDispatchToken("local-test-dispatch-token")
	if err := runner.ConfigureExecutionPolicy(ExecutionPolicy{DefaultTimeoutSec: 10, MaxConcurrentBuilds: 1, MaxConcurrentLocalBuilds: 1, RetryLimit: 0, FailFast: true}); err != nil {
		t.Fatal(err)
	}
	runner.SetNotificationService(NewNotificationService(database))
	var broadcastMu sync.Mutex
	var broadcasts []string
	runner.liveLogBroadcast = func(_ int64, payload map[string]interface{}) {
		encoded, _ := json.Marshal(payload)
		broadcastMu.Lock()
		broadcasts = append(broadcasts, string(encoded))
		broadcastMu.Unlock()
	}

	runner.Run(build.ID)
	finished := waitForBuildStatus(t, database, build.ID, 8*time.Second, "failed")
	waitForBuildNotificationEvents(t, database, build.ID, 1)
	assertNoSecretValues(t, "remote durable build log", finished.Log, parameterSecret, credentialSecret)
	if strings.Count(finished.Log, maskedSecretValue) < 2 {
		t.Fatalf("remote log did not mask parameter and credential:\n%s", finished.Log)
	}
	broadcastMu.Lock()
	liveOutput := strings.Join(broadcasts, "\n")
	broadcastMu.Unlock()
	assertNoSecretValues(t, "remote live WebSocket payload", liveOutput, parameterSecret, credentialSecret)
	problems, _ := json.Marshal(AnalyzeBuildProblems(finished.Status, finished.Log))
	assertNoSecretValues(t, "remote failure problem report", string(problems), parameterSecret, credentialSecret)
	assertNotificationEventsContainNoSecrets(t, database, parameterSecret, credentialSecret)
}

func waitForBuildNotificationEvents(t *testing.T, database *store.Store, buildID int64, minimum int) {
	t.Helper()
	channels, err := database.ListNotificationChannels()
	if err != nil || len(channels) == 0 {
		t.Fatalf("notification channels = %#v, err=%v", channels, err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		events, listErr := database.ListNotificationEvents(channels[0].ID, 20)
		if listErr != nil {
			t.Fatal(listErr)
		}
		count := 0
		for _, event := range events {
			if event.BuildID != nil && *event.BuildID == buildID {
				count++
			}
		}
		if count >= minimum {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d notification event(s) for build %d", minimum, buildID)
}

func assertNotificationEventsContainNoSecrets(t *testing.T, database *store.Store, secrets ...string) {
	t.Helper()
	channels, err := database.ListNotificationChannels()
	if err != nil {
		t.Fatal(err)
	}
	for _, channel := range channels {
		events, listErr := database.ListNotificationEvents(channel.ID, 100)
		if listErr != nil {
			t.Fatal(listErr)
		}
		for _, event := range events {
			assertNoSecretValues(t, "notification event payload", event.Payload+event.ErrorMessage, secrets...)
		}
	}
}

func assertNoSecretValues(t *testing.T, surface, value string, secrets ...string) {
	t.Helper()
	for _, secret := range secrets {
		if strings.Contains(value, secret) {
			t.Fatalf("%s contains plaintext secret %q: %s", surface, secret, value)
		}
	}
}

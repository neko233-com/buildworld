package worker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/neko233-com/buildworld/internal/bytemsg"
	pb "github.com/neko233-com/buildworld/internal/rpc/generated"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestWorkerExecutesStructuredProtocolPipeline(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	pb.RegisterWorkerServiceServer(server, NewExecutor())
	go server.Serve(listener)
	defer server.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	pipeline := "jobs:\n  output:\n    steps:\n      - run: echo remote-ok\n"
	stream, err := pb.NewWorkerServiceClient(conn).ExecuteBuild(ctx, &pb.BuildRequest{BuildId: "1", PipelineConfig: pipeline, Protocol: bytemsg.NewProtocolInfo()})
	if err != nil {
		t.Fatal(err)
	}
	var success bool
	for {
		message, err := stream.Recv()
		if err != nil {
			break
		}
		if message.Status == "success" {
			success = true
		}
		if err := bytemsg.Validate(message.Protocol); err != nil {
			t.Fatalf("response protocol = %v", err)
		}
	}
	if !success {
		t.Fatal("worker did not return a successful terminal response")
	}
}

func TestWorkerDaemonRejectsUnauthenticatedDispatch(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	daemon := &Daemon{dispatchToken: "shared-secret", executor: NewExecutor()}
	server := grpc.NewServer(grpc.StreamInterceptor(daemon.authorizeDispatch))
	pb.RegisterWorkerServiceServer(server, daemon.executor)
	go server.Serve(listener)
	defer server.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	request := &pb.BuildRequest{BuildId: "auth", PipelineConfig: "jobs:\n  check:\n    steps:\n      - run: echo ok\n", Protocol: bytemsg.NewProtocolInfo()}
	stream, err := pb.NewWorkerServiceClient(conn).ExecuteBuild(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Recv(); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("unauthenticated dispatch error = %v", err)
	}

	stream, err = pb.NewWorkerServiceClient(conn).ExecuteBuild(bytemsg.WithDispatchCredential(ctx, "shared-secret"), request)
	if err != nil {
		t.Fatal(err)
	}
	var completed bool
	for {
		response, err := stream.Recv()
		if err != nil {
			break
		}
		if response.Status == "success" {
			completed = true
		}
	}
	if !completed {
		t.Fatal("authenticated worker dispatch did not complete")
	}
}

func TestWorkerRejectsStaleProtocol(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	pb.RegisterWorkerServiceServer(server, NewExecutor())
	go server.Serve(listener)
	defer server.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	stream, err := pb.NewWorkerServiceClient(conn).ExecuteBuild(ctx, &pb.BuildRequest{BuildId: "stale", PipelineConfig: "jobs:\n  check:\n    steps:\n      - run: echo ok\n", Protocol: &pb.ProtocolInfo{Name: bytemsg.Name, Major: 3}})
	if err != nil {
		t.Fatal(err)
	}
	message, err := stream.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if !message.IsError || message.Status != "failed" || !strings.Contains(message.Output, "unsupported worker protocol") {
		t.Fatalf("stale protocol response = %#v", message)
	}
}

func TestWorkerRejectsMissingStructuredProtocol(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	pb.RegisterWorkerServiceServer(server, NewExecutor())
	go server.Serve(listener)
	defer server.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	stream, err := pb.NewWorkerServiceClient(conn).ExecuteBuild(ctx, &pb.BuildRequest{BuildId: "missing", PipelineConfig: "jobs:\n  check:\n    steps:\n      - run: echo ok\n"})
	if err != nil {
		t.Fatal(err)
	}
	message, err := stream.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if !message.IsError || message.Status != "failed" || !strings.Contains(message.Output, "missing structured protocol info") {
		t.Fatalf("missing protocol response = %#v", message)
	}
	if err := bytemsg.Validate(message.Protocol); err != nil {
		t.Fatalf("error response protocol = %v", err)
	}
}

func TestWorkerUsesSharedPlatformRuntimeAndOutputRules(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	pb.RegisterWorkerServiceServer(server, NewExecutor())
	go server.Serve(listener)
	defer server.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	platform := runtime.GOOS
	runtimeVariable := "$BUILDWORLD_RUNTIME_NODE"
	if platform == "darwin" {
		platform = "macos"
	}
	if platform == "windows" {
		runtimeVariable = "%BUILDWORLD_RUNTIME_NODE%"
	}
	pipeline := fmt.Sprintf(`import { definePipeline, shell, stage } from "@buildworld/pipeline"
export default definePipeline({
  toolchains: { node: ["22"] },
  stages: [stage("build", [
    shell("emit", "echo ::buildworld:set IMAGE=remote-ok", { runtime: "node", platformAdditions: { %q: %q } }),
    shell("consume", "echo ${build.IMAGE}"),
  ])],
})`, platform, "echo platform-"+runtimeVariable)
	stream, err := pb.NewWorkerServiceClient(conn).ExecuteBuild(ctx, &pb.BuildRequest{BuildId: "2", PipelineConfig: pipeline, Protocol: bytemsg.NewProtocolInfo()})
	if err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	for {
		message, err := stream.Recv()
		if err != nil {
			break
		}
		output.WriteString(message.Output)
	}
	for _, want := range []string{"platform-22", "remote-ok"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output %q does not contain %q", output.String(), want)
		}
	}
}

func TestWorkerCancelBuildTerminatesCommandTree(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	executor := NewExecutorAt(t.TempDir())
	server := grpc.NewServer()
	pb.RegisterWorkerServiceServer(server, executor)
	go server.Serve(listener)
	defer server.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	pidFile := filepath.Join(t.TempDir(), "worker-child.pid")
	pipeline := `import { definePipeline, shell, stage } from "@buildworld/pipeline"
export default definePipeline({
  stages: [stage("long-running", shell("child", ` + strconv.Quote(workerProcessTreeShellCommand(t)) + `))],
})`
	const buildID = "cancel-process-tree"
	stream, err := pb.NewWorkerServiceClient(conn).ExecuteBuild(ctx, &pb.BuildRequest{
		BuildId:        buildID,
		PipelineConfig: pipeline,
		Protocol:       bytemsg.NewProtocolInfo(),
		Environment: map[string]string{
			"BUILDWORLD_PROCESS_TREE_HELPER":   "1",
			"BUILDWORLD_PROCESS_TREE_PID_FILE": pidFile,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	streamDone := make(chan string, 1)
	go func() {
		var output strings.Builder
		for {
			message, err := stream.Recv()
			if err != nil {
				streamDone <- fmt.Sprintf("%v; output: %s", err, output.String())
				return
			}
			output.WriteString(message.Output)
			output.WriteByte('\n')
		}
	}()

	pid := waitForWorkerProcessTreePID(t, pidFile, streamDone)
	if !testProcessRunning(pid) {
		t.Fatalf("worker child process %d was not running before cancellation", pid)
	}
	if !executor.CancelBuild(buildID) {
		t.Fatal("CancelBuild() did not find the active build")
	}
	select {
	case <-streamDone:
	case <-time.After(10 * time.Second):
		t.Fatal("worker stream did not finish after cancellation")
	}
	waitForWorkerProcessTreeExit(t, pid)
}

func TestWorkerProcessTreeHelper(t *testing.T) {
	if os.Getenv("BUILDWORLD_PROCESS_TREE_HELPER") != "1" {
		t.Skip("helper process")
	}
	pidFile := os.Getenv("BUILDWORLD_PROCESS_TREE_PID_FILE")
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}
	for {
		time.Sleep(time.Hour)
	}
}

func TestWorkerStreamsConfiguredArtifacts(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	pb.RegisterWorkerServiceServer(server, NewExecutor())
	go server.Serve(listener)
	defer server.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	pipeline := `import { definePipeline, shell, stage } from "@buildworld/pipeline"
export default definePipeline({
  artifacts: ["remote-artifact.txt"],
  stages: [stage("build", shell("write", "echo remote-artifact > remote-artifact.txt"))],
})`
	stream, err := pb.NewWorkerServiceClient(conn).ExecuteBuild(ctx, &pb.BuildRequest{BuildId: "artifact", PipelineConfig: pipeline, Protocol: bytemsg.NewProtocolInfo()})
	if err != nil {
		t.Fatal(err)
	}
	var received bytes.Buffer
	var checksum string
	var finalized, success bool
	for {
		message, err := stream.Recv()
		if err != nil {
			break
		}
		if chunk := message.GetArtifactChunk(); chunk != nil {
			if chunk.Name != "remote-artifact.txt" {
				t.Fatalf("artifact name = %q", chunk.Name)
			}
			received.Write(chunk.Data)
			if chunk.FinalChunk {
				finalized = true
				checksum = chunk.Sha256
			}
		}
		if message.Status == "success" {
			success = true
		}
	}
	if !finalized || !success {
		t.Fatalf("artifact finalized=%v success=%v", finalized, success)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(received.Bytes())); got != checksum {
		t.Fatalf("artifact checksum = %q, want %q", checksum, got)
	}
	if !strings.Contains(received.String(), "remote-artifact") {
		t.Fatalf("artifact payload = %q", received.String())
	}
}

func workerProcessTreeShellCommand(t *testing.T) string {
	t.Helper()
	executable, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if strings.ContainsAny(executable, " \t\"") {
			t.Skip("Windows process-tree helper path requires quoting that cmd.exe rewrites")
		}
		return executable + " -test.run=TestWorkerProcessTreeHelper"
	}
	quoted := "'" + strings.ReplaceAll(executable, "'", `'"'"'`) + "'"
	return quoted + ` -test.run='^TestWorkerProcessTreeHelper$' & child=$!; wait "$child"`
}

func waitForWorkerProcessTreePID(t *testing.T, path string, streamDone <-chan string) int {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err == nil && pid > 0 {
				return pid
			}
		}
		select {
		case result := <-streamDone:
			t.Fatalf("worker stream ended before writing child PID: %s", result)
		case <-time.After(20 * time.Millisecond):
		}
	}
	t.Fatalf("timed out waiting for worker child PID file %s", path)
	return 0
}

func waitForWorkerProcessTreeExit(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if !testProcessRunning(pid) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("worker child process %d survived build cancellation", pid)
}

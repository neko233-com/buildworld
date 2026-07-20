package worker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/neko233-com/buildworld/internal/bytemsg"
	"github.com/neko233-com/buildworld/internal/plugin"
	pb "github.com/neko233-com/buildworld/internal/rpc/generated"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestWorkerExecutesVersionedPipeline(t *testing.T) {
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
	pipeline := "# Test\n\n## Pipeline\n\n### Output\n\n```default shell\necho remote-ok\n```\n"
	stream, err := pb.NewWorkerServiceClient(conn).ExecuteBuild(ctx, &pb.BuildRequest{BuildId: "1", PipelineConfig: pipeline, ProtocolVersion: ProtocolVersion, Protocol: bytemsg.NewProtocolInfo()})
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
		if err := bytemsg.Validate(message.Protocol, ""); err != nil {
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
	request := &pb.BuildRequest{BuildId: "auth", PipelineConfig: "# Auth\n\n## Pipeline\n\n### Check\n```default shell\necho ok\n```", ProtocolVersion: ProtocolVersion, Protocol: bytemsg.NewProtocolInfo()}
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
	stream, err := pb.NewWorkerServiceClient(conn).ExecuteBuild(ctx, &pb.BuildRequest{BuildId: "stale", PipelineConfig: `{"stages":[]}`, ProtocolVersion: "bytemsg233/v2"})
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
	pipeline := fmt.Sprintf(`{"toolchains":{"node":["22"]},"stages":[{"name":"build","steps":[{"name":"emit","type":"shell","runtime":"node","command":"echo ::buildworld:set IMAGE=remote-ok","platform_additions":{"%s":"echo platform-%s"}},{"name":"consume","type":"shell","command":"echo ${build.IMAGE}"}]}]}`, platform, runtimeVariable)
	stream, err := pb.NewWorkerServiceClient(conn).ExecuteBuild(ctx, &pb.BuildRequest{BuildId: "2", PipelineConfig: pipeline, ProtocolVersion: ProtocolVersion})
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
	pipeline := `{"artifacts":["remote-artifact.txt"],"stages":[{"name":"build","steps":[{"name":"write","type":"shell","command":"echo remote-artifact > remote-artifact.txt"}]}]}`
	stream, err := pb.NewWorkerServiceClient(conn).ExecuteBuild(ctx, &pb.BuildRequest{BuildId: "artifact", PipelineConfig: pipeline, ProtocolVersion: ProtocolVersion})
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

func TestWorkerRunsLoadedPluginAndPassesKVOutput(t *testing.T) {
	root := t.TempDir()
	loader := plugin.NewLoader(root)
	if err := loader.InstallPlugin("emit", "1.0.0", "test", "test", `registerStep("emit-output", function(ctx) { ctx.log("plugin-ran"); ctx.output("IMAGE", "remote-plugin"); });`, "", "js", "", "", "none"); err != nil {
		t.Fatal(err)
	}
	if err := loader.Load("emit"); err != nil {
		t.Fatal(err)
	}
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	pb.RegisterWorkerServiceServer(server, NewExecutorWithPlugins(loader))
	go server.Serve(listener)
	defer server.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	pipeline := `{"stages":[{"name":"plugin","steps":[{"name":"emit","type":"emit-output"},{"name":"read","type":"shell","command":"echo ${build.IMAGE}"}]}]}`
	stream, err := pb.NewWorkerServiceClient(conn).ExecuteBuild(ctx, &pb.BuildRequest{BuildId: "plugin", PipelineConfig: pipeline, ProtocolVersion: ProtocolVersion})
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
	for _, want := range []string{"plugin-ran", "remote-plugin"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("output %q does not contain %q", output.String(), want)
		}
	}
}

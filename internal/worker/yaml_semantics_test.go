package worker

import (
	"context"
	"net"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/neko233-com/buildworld/internal/bytemsg"
	pb "github.com/neko233-com/buildworld/internal/rpc/generated"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func TestRemoteWorkerYAMLDependencyConditionsBuiltinsAndTimeout(t *testing.T) {
	sleepCommand := "sleep 2"
	if runtime.GOOS == "windows" {
		sleepCommand = "ping -n 4 127.0.0.1 >nul"
	}
	pipeline := `
jobs:
  checkout:
    steps:
      - name: checkout
        uses: actions/checkout@v4
  notify:
    steps:
      - name: notify
        type: notify
  fail:
    steps:
      - name: expected failure
        run: exit 9
  blocked:
    needs: fail
    steps:
      - name: should not run
        run: echo SHOULD_NOT_RUN
  recover:
    needs: fail
    if: failure()
    steps:
      - name: recovery
        run: echo REMOTE_RECOVERY_RAN
  timeout:
    if: always()
    timeout-minutes: 0.001
    steps:
      - name: wait
        run: ` + sleepCommand + `
`
	output, status := executeWorkerYAMLForTest(t, pipeline)
	if status != "failed" {
		t.Fatalf("terminal status = %q\n%s", status, output)
	}
	for _, expected := range []string{
		"checkout skipped: no repository URL was supplied",
		"Notification skipped: remote workers do not own notification credentials",
		"REMOTE_RECOVERY_RAN",
		"stage timed out after 1s",
		"dependency/if condition evaluated to false",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("output does not contain %q:\n%s", expected, output)
		}
	}
	if strings.Contains(output, "SHOULD_NOT_RUN") || strings.Contains(output, "unsupported step type") {
		t.Fatalf("remote worker used incorrect dependency/builtin semantics:\n%s", output)
	}
}

func TestRemoteWorkerFailFastSkipsOrdinaryWorkAndRunsDiagnostics(t *testing.T) {
	pipeline := `jobs:
  fail:
    steps:
      - {name: expected failure, run: exit 9}
      - {name: ordinary step, run: echo ORDINARY_STEP_SHOULD_NOT_RUN}
      - {name: failure diagnostic, run: echo FAILURE_STEP_RAN, if: failure()}
      - {name: success diagnostic, run: echo SUCCESS_STEP_SHOULD_NOT_RUN, if: success()}
      - {name: always diagnostic, run: echo ALWAYS_STEP_RAN, if: always()}
  ordinary:
    name: ordinary stage
    steps:
      - {name: ordinary stage step, run: echo ORDINARY_STAGE_SHOULD_NOT_RUN}
  failure:
    name: failure stage
    if: failure()
    steps:
      - {name: failure stage diagnostic, run: echo FAILURE_STAGE_RAN}
  always:
    name: always stage
    if: always()
    steps:
      - {name: always stage diagnostic, run: echo ALWAYS_STAGE_RAN}
`
	output, status := executeWorkerYAMLForTest(t, pipeline)
	if status != "failed" {
		t.Fatalf("terminal status = %q\n%s", status, output)
	}
	for _, expected := range []string{
		"FAILURE_STEP_RAN",
		"ALWAYS_STEP_RAN",
		"FAILURE_STAGE_RAN",
		"ALWAYS_STAGE_RAN",
		"skipped: fail-fast after previous step failure",
		"skipped: fail-fast after previous stage failure",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("output does not contain %q:\n%s", expected, output)
		}
	}
	for _, forbidden := range []string{
		"ORDINARY_STEP_SHOULD_NOT_RUN",
		"SUCCESS_STEP_SHOULD_NOT_RUN",
		"ORDINARY_STAGE_SHOULD_NOT_RUN",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("fail-fast executed %q:\n%s", forbidden, output)
		}
	}
}

func TestRemoteWorkerYAMLAppliesJobStepEnvAndWorkingDirectory(t *testing.T) {
	mkdirCommand := "mkdir -p nested"
	echoCommand := `echo REMOTE-$JOB_VALUE-$STEP_VALUE`
	if runtime.GOOS == "windows" {
		mkdirCommand = "mkdir nested"
		echoCommand = "echo REMOTE-%JOB_VALUE%-%STEP_VALUE%"
	}
	pipeline := `
jobs:
  verify:
    env:
      JOB_VALUE: job
    defaults:
      run:
        working-directory: nested
    steps:
      - name: prepare
        working-directory: .
        run: ` + mkdirCommand + `
      - name: inspect
        env:
          STEP_VALUE: step
        run: ` + echoCommand + `
`
	output, status := executeWorkerYAMLForTest(t, pipeline)
	if status != "success" {
		t.Fatalf("terminal status = %q\n%s", status, output)
	}
	if !strings.Contains(output, "REMOTE-job-step") {
		t.Fatalf("job/step environment not applied:\n%s", output)
	}
}

func executeWorkerYAMLForTest(t *testing.T, pipeline string) (string, string) {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	pb.RegisterWorkerServiceServer(server, NewExecutorAt(t.TempDir()))
	go server.Serve(listener)
	t.Cleanup(server.Stop)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	connection, err := grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	stream, err := pb.NewWorkerServiceClient(connection).ExecuteBuild(ctx, &pb.BuildRequest{
		BuildId:        "yaml-semantics",
		PipelineConfig: pipeline,
		Protocol:       bytemsg.NewProtocolInfo(),
	})
	if err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	var terminalStatus string
	for {
		message, receiveErr := stream.Recv()
		if receiveErr != nil {
			break
		}
		if message.Output != "" {
			output.WriteString(message.Output)
			output.WriteByte('\n')
		}
		if message.Status == "success" || message.Status == "failed" {
			terminalStatus = message.Status
		}
	}
	return output.String(), terminalStatus
}

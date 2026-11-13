package worker

import (
	"context"
	"io"
	"net"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/neko233-com/buildworld/internal/bytemsg"
	pb "github.com/neko233-com/buildworld/internal/rpc/generated"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func TestWorkerEnforcesConcurrencyAndPublishesExecutionSnapshots(t *testing.T) {
	executor := NewExecutorAt(t.TempDir())
	if err := executor.Configure(2, false); err != nil {
		t.Fatal(err)
	}
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	pb.RegisterWorkerServiceServer(server, executor)
	go server.Serve(listener)
	defer server.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	command := "sleep 2"
	if runtime.GOOS == "windows" {
		command = "ping -n 4 127.0.0.1 > NUL"
	}
	client := pb.NewWorkerServiceClient(conn)
	consume := func(id string) bool {
		stream, err := client.ExecuteBuild(ctx, &pb.BuildRequest{BuildId: id, ProjectName: "concurrent", PipelineConfig: "jobs:\n  probe:\n    steps:\n      - run: " + command + "\n", Protocol: bytemsg.NewProtocolInfo()})
		if err != nil {
			return false
		}
		var done bool
		for {
			message, e := stream.Recv()
			if e == io.EOF {
				return done
			}
			if e != nil {
				return false
			}
			if message.Status == "failed" {
				return false
			}
			if message.Status == "success" {
				done = true
			}
		}
	}
	var wait sync.WaitGroup
	results := make(chan bool, 2)
	for _, id := range []string{"one", "two"} {
		wait.Add(1)
		go func() { defer wait.Done(); results <- consume(id) }()
	}
	deadline := time.Now().Add(3 * time.Second)
	for len(executor.Snapshot().Tasks) != 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	snapshot := executor.Snapshot()
	if len(snapshot.Tasks) != 2 || snapshot.PID <= 0 || snapshot.Capacity != 2 {
		t.Fatal("missing active snapshot", snapshot)
	}
	if consume("three") {
		t.Fatal("third task bypassed local capacity")
	}
	if err = executor.Configure(1, true); err != nil {
		t.Fatal(err)
	}
	wait.Wait()
	close(results)
	for result := range results {
		if !result {
			t.Fatal("existing task was interrupted by drain or limit reduction")
		}
	}
	if len(executor.Snapshot().Tasks) != 0 {
		t.Fatal("execution permit leaked")
	}
	if consume("four") {
		t.Fatal("draining worker accepted dispatch")
	}
}

func TestDaemonStatePreservesIdentityAndAdmission(t *testing.T) {
	root := t.TempDir()
	first := &Daemon{server: "http://controller", id: "stable-id", token: "private-test-token", stateRoot: root, executor: NewExecutorAt(filepath.Join(root, "build"))}
	_ = first.executor.Configure(7, true)
	if err := first.saveState(); err != nil {
		t.Fatal(err)
	}
	second := &Daemon{server: first.server, stateRoot: root, executor: NewExecutorAt(filepath.Join(root, "build"))}
	if err := second.loadState(); err != nil {
		t.Fatal(err)
	}
	if second.id != first.id || second.token != first.token || second.executor.Snapshot().Capacity != 7 || !second.executor.Snapshot().Draining {
		t.Fatal("worker state was not restored")
	}
	second.server = "http://other"
	if second.loadState() == nil {
		t.Fatal("state identity crossed controllers")
	}
}

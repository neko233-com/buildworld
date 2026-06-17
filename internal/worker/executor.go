package worker

import (
	"context"
	"log"
	"sync"

	pb "github.com/neko233-com/buildworld233/internal/rpc/generated"
)

type Executor struct {
	mu      sync.Mutex
	running map[string]context.CancelFunc
}

func NewExecutor() *Executor {
	return &Executor{
		running: make(map[string]context.CancelFunc),
	}
}

func (e *Executor) ExecuteBuild(ctx context.Context, req *pb.BuildRequest, stream pb.WorkerService_ExecuteBuildServer) error {
	buildCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	e.mu.Lock()
	e.running[req.BuildId] = cancel
	e.mu.Unlock()

	defer func() {
		e.mu.Lock()
		delete(e.running, req.BuildId)
		e.mu.Unlock()
	}()

	log.Printf("Executing build %s for project %s", req.BuildId, req.ProjectName)

	if err := stream.Send(&pb.BuildResponse{
		BuildId: req.BuildId,
		Stage:   "init",
		Step:    "checkout",
		Output:  "Cloning repository " + req.RepoUrl,
		Status:  "running",
	}); err != nil {
		return err
	}

	<-buildCtx.Done()

	return stream.Send(&pb.BuildResponse{
		BuildId: req.BuildId,
		Stage:   "done",
		Status:  "completed",
	})
}

func (e *Executor) CancelBuild(buildID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	cancel, ok := e.running[buildID]
	if ok {
		cancel()
	}
	return ok
}

package worker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/neko233-com/buildworld/internal/buildinfo"
	"github.com/neko233-com/buildworld/internal/workercontrol"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/neko233-com/buildworld/internal/bytemsg"
	"github.com/neko233-com/buildworld/internal/config"
	pb "github.com/neko233-com/buildworld/internal/rpc/generated"
)

type Daemon struct {
	id, name, server, token, dispatchToken, listen, advertise, pool string
	labels                                                          []string
	maxBuilds                                                       int
	executor                                                        *Executor
	stateRoot                                                       string
	state                                                           daemonState
	shutdown                                                        context.CancelFunc
}

func NewDaemon(cfg *config.Config, serverAddr, enrollToken, listen, advertise string) *Daemon {
	if listen == "" {
		listen = ":6051"
	}
	daemon := &Daemon{id: uuid.New().String(), name: cfg.Workers.Local.Workspace, server: serverAddr, token: enrollToken, dispatchToken: enrollToken, listen: listen, advertise: advertise, pool: cfg.Workers.Local.Pool, labels: cfg.Workers.Local.Labels, maxBuilds: cfg.Workers.Local.MaxConcurrentBuilds, executor: NewExecutorAt(cfg.Storage.BuildTemp)}
	identity := sha256.Sum256([]byte(serverAddr + "\x00" + listen))
	daemon.stateRoot = filepath.Join(filepath.Dir(daemon.executor.environment.Root()), "worker-state", fmt.Sprintf("%x", identity[:8]))
	_ = daemon.executor.Configure(daemon.maxBuilds, false)
	return daemon
}

func (d *Daemon) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	d.shutdown = cancel
	if d.maxBuilds < 1 || d.maxBuilds > 64 {
		return fmt.Errorf("worker max-builds must be between 1 and 64")
	}
	if err := os.MkdirAll(d.stateRoot, 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(d.stateRoot, "daemon.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if !tryFileLock(lock) {
		return fmt.Errorf("another worker owns this state directory")
	}
	defer unlockFile(lock)
	if err = d.loadState(); err != nil {
		return err
	}
	listener, err := net.Listen("tcp", d.listen)
	if err != nil {
		return fmt.Errorf("listen worker: %w", err)
	}
	grpcServer := grpc.NewServer(grpc.StreamInterceptor(d.authorizeDispatch))
	pb.RegisterWorkerServiceServer(grpcServer, d.executor)
	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			log.Printf("worker grpc: %v", err)
		}
	}()
	address := d.advertise
	if address == "" {
		address = listener.Addr().String()
	}
	if err := d.ensureRegistration(address); err != nil {
		grpcServer.Stop()
		return err
	}
	log.Printf("buildworld-worker %s listening on %s", d.id, listener.Addr())
	if d.state.Update != nil && d.state.Update.OriginalPID != os.Getpid() && buildVersion(buildinfo.Version) == buildVersion(d.state.Update.Version) {
		d.state.Completion = &workercontrol.Completion{ID: d.state.Update.OperationID, Status: "success", Message: "Updated worker connected using the expected release version"}
		if err = d.saveState(); err != nil {
			grpcServer.Stop()
			return err
		}
	}
	_ = d.heartbeat()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)
	for {
		select {
		case <-ticker.C:
			_ = d.heartbeat()
		case <-signals:
			grpcServer.GracefulStop()
			return nil
		case <-ctx.Done():
			grpcServer.GracefulStop()
			return nil
		}
	}
}

func (d *Daemon) authorizeDispatch(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	if info.FullMethod == pb.WorkerService_ExecuteBuild_FullMethodName {
		if err := bytemsg.ValidateDispatchCredential(stream.Context(), d.dispatchToken); err != nil {
			return status.Error(codes.Unauthenticated, err.Error())
		}
	}
	return handler(srv, stream)
}

func (d *Daemon) register(address string) error {
	payload, _ := json.Marshal(map[string]interface{}{"name": d.name, "address": address, "labels": d.labels, "pool": d.pool, "max_builds": d.maxBuilds})
	req, err := http.NewRequest(http.MethodPost, d.server+"/api/agents/auto-register", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Buildworld-Enroll-Token", d.token)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("worker registration status: %s", resp.Status)
	}
	var result struct {
		Agent struct {
			ID string `json:"id"`
		} `json:"agent"`
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	d.id, d.token = result.Agent.ID, result.Token
	return d.saveState()
}

func (d *Daemon) ensureRegistration(address string) error {
	if d.state.ID != "" {
		return nil
	}
	return d.register(address)
}

func (d *Daemon) heartbeat() error {
	payload, _ := json.Marshal(workercontrol.Heartbeat{Runtime: d.executor.Snapshot(), Completion: d.state.Completion})
	req, err := http.NewRequest(http.MethodPost, d.server+"/api/agents/heartbeat/"+d.id, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("X-Buildworld-Worker-Token", d.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("worker heartbeat status: %s", resp.Status)
	}
	var reply workercontrol.Reply
	if err = json.NewDecoder(resp.Body).Decode(&reply); err != nil {
		return err
	}
	if d.state.Update != nil && d.state.Completion != nil && d.state.Completion.ID == d.state.Update.OperationID && d.state.Completion.Status == "success" {
		if err = atomicJSON(filepath.Join(d.state.Update.Directory, "ready.json"), map[string]any{"id": d.state.Update.OperationID, "version": strings.TrimPrefix(buildinfo.Version, "v"), "pid": os.Getpid()}); err != nil {
			return err
		}
	}
	return d.control(reply.Operation)
}

// SetStateDirectory explicitly isolates multiple worker instances on one host.
func (d *Daemon) SetStateDirectory(path string) error {
	if path == "" {
		return nil
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	d.stateRoot = absolute
	return nil
}

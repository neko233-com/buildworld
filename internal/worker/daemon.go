package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
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
}

func NewDaemon(cfg *config.Config, serverAddr, enrollToken, listen, advertise string) *Daemon {
	if listen == "" {
		listen = ":6051"
	}
	return &Daemon{id: uuid.New().String(), name: cfg.Workers.Local.Workspace, server: serverAddr, token: enrollToken, dispatchToken: enrollToken, listen: listen, advertise: advertise, pool: cfg.Workers.Local.Pool, labels: cfg.Workers.Local.Labels, maxBuilds: cfg.Workers.Local.MaxConcurrentBuilds, executor: NewExecutorAt(cfg.Storage.BuildTemp)}
}

func (d *Daemon) Start(ctx context.Context) error {
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
	if err := d.register(address); err != nil {
		grpcServer.Stop()
		return err
	}
	log.Printf("buildworld-worker %s listening on %s", d.id, listener.Addr())
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
	return nil
}

func (d *Daemon) heartbeat() error {
	req, err := http.NewRequest(http.MethodPost, d.server+"/api/agents/heartbeat/"+d.id, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Buildworld-Worker-Token", d.token)
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("worker heartbeat status: %s", resp.Status)
	}
	return nil
}

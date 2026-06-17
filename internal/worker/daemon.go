package worker

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/neko233-com/buildworld233/internal/config"
	pb "github.com/neko233-com/buildworld233/internal/rpc/generated"
)

type Daemon struct {
	id        string
	name      string
	server    string
	token     string
	labels    []string
	maxBuilds int

	conn     *grpc.ClientConn
	client   pb.WorkerServiceClient
	health   *HealthMonitor
	executor *Executor

	stopCh chan struct{}
}

func NewDaemon(cfg *config.Config, serverAddr, token string) *Daemon {
	return &Daemon{
		id:        uuid.New().String(),
		name:      cfg.Workers.Local.Workspace,
		server:    serverAddr,
		token:     token,
		labels:    cfg.Workers.Local.Labels,
		maxBuilds: cfg.Workers.Local.MaxConcurrentBuilds,
		health:    NewHealthMonitor(),
		executor:  NewExecutor(),
		stopCh:    make(chan struct{}),
	}
}

func (d *Daemon) Start(ctx context.Context) error {
	log.Printf("Starting worker daemon %s", d.id)

	conn, err := grpc.Dial(d.server, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("connect to server: %w", err)
	}
	d.conn = conn
	d.client = pb.NewWorkerServiceClient(conn)

	if err := d.register(); err != nil {
		return fmt.Errorf("register: %w", err)
	}

	go d.heartbeatLoop(ctx)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigCh:
		log.Println("Received shutdown signal")
	case <-ctx.Done():
		log.Println("Context cancelled")
	case <-d.stopCh:
		log.Println("Stop requested")
	}

	return d.Stop()
}

func (d *Daemon) Stop() error {
	log.Printf("Stopping worker daemon %s", d.id)

	if d.conn != nil {
		d.conn.Close()
	}

	return nil
}

func (d *Daemon) register() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := d.client.Register(ctx, &pb.RegisterRequest{
		WorkerId:            d.id,
		Name:                d.name,
		Address:             d.server,
		Token:               d.token,
		Labels:              d.labels,
		MaxConcurrentBuilds: int32(d.maxBuilds),
	})
	if err != nil {
		return err
	}

	if !resp.Success {
		return fmt.Errorf("registration failed: %s", resp.Message)
	}

	log.Printf("Registered with server as %s", d.id)
	return nil
}

func (d *Daemon) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.sendHeartbeat()
		case <-ctx.Done():
			return
		case <-d.stopCh:
			return
		}
	}
}

func (d *Daemon) sendHeartbeat() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	metrics := d.health.GetMetrics()

	_, err := d.client.Heartbeat(ctx, &pb.HeartbeatRequest{
		WorkerId:     d.id,
		CpuUsage:     metrics.CPU,
		MemoryUsage:  metrics.Memory,
		DiskUsage:    metrics.Disk,
		ActiveBuilds: int32(metrics.ActiveBuilds),
	})
	if err != nil {
		log.Printf("Heartbeat failed: %v", err)
	}
}

package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/neko233-com/buildworld/internal/api"
	"github.com/neko233-com/buildworld/internal/auth"
	"github.com/neko233-com/buildworld/internal/config"
	"github.com/neko233-com/buildworld/internal/engine"
	"github.com/neko233-com/buildworld/internal/plugin"
	"github.com/neko233-com/buildworld/internal/store"
	"github.com/neko233-com/buildworld/internal/ws"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Config file path")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	if directory := filepath.Dir(cfg.Database.Path); directory != "" {
		_ = os.MkdirAll(directory, 0o700)
	}

	db, err := store.New(cfg.Database.Path)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()
	if err := api.ApplyStoredSettings(cfg, db); err != nil {
		log.Printf("Warning: failed to restore runtime settings: %v", err)
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(cfg.Server.Host, fmt.Sprint(cfg.Server.Port)))
	if err != nil {
		log.Fatalf("Failed to reserve HTTP listener: %v", err)
	}
	defer listener.Close()

	// Keep all mutable data in configured storage. A LaunchAgent has no stable
	// shell working directory, so relative roots otherwise lose build output.
	artifactsRoot := cfg.Storage.Artifacts
	for _, dir := range []string{cfg.Storage.BuildTemp, cfg.Plugins.Path, artifactsRoot} {
		if dir != "" {
			_ = os.MkdirAll(dir, 0o755)
		}
	}

	if err := auth.SetupDefaultAdmin(db); err != nil {
		log.Printf("Warning: Failed to setup default admin: %v", err)
	}

	jwtSecret, jwtSecretPath, generated, err := auth.LoadOrCreateJWTSecret(cfg.Auth.JWTSecret, cfg.Database.Path)
	if err != nil {
		log.Fatalf("Failed to initialize JWT secret: %v", err)
	}
	cfg.Auth.JWTSecret = jwtSecret
	if generated {
		log.Printf("Generated a persistent JWT signing secret at %s", jwtSecretPath)
	}
	jwtInstance := auth.NewJWT(jwtSecret)

	// WebSocket hub.
	hub := ws.NewHub()
	go hub.Run()

	// Plugin loader.
	loader := plugin.NewLoader(cfg.Plugins.Path)
	if err := loader.LoadAll(); err != nil {
		log.Printf("Warning: plugin load error: %v", err)
	}

	// Reconcile only validated binary manifests. Existing activation choices are
	// preserved; script plugin directories are ignored by Loader.LoadAll.
	for _, loaded := range loader.ListAll() {
		stored, storeErr := db.GetPluginByName(loaded.Name)
		switch {
		case storeErr == nil:
			if err := db.UpdatePluginMetadata(stored.ID, loaded.Version, loaded.Description, loaded.Author, loaded.Path, loaded.InstallSource()); err != nil {
				log.Printf("Warning: failed to sync binary plugin %s: %v", loaded.Name, err)
				continue
			}
			loader.SetEnabled(loaded.Name, stored.Enabled)
		case errors.Is(storeErr, sql.ErrNoRows):
			if _, err := db.CreatePlugin(loaded.Name, loaded.Version, loaded.Description, loaded.Author, "", loaded.Path, loaded.InstallSource()); err != nil {
				log.Printf("Warning: failed to register binary plugin %s: %v", loaded.Name, err)
			}
		default:
			log.Printf("Warning: failed to inspect binary plugin %s: %v", loaded.Name, storeErr)
		}
	}

	// Build runner (executes pipelines in-process — the "local agent").
	buildEnvironment := engine.NewBuildEnvironment(cfg.Storage.BuildTemp)
	if err := buildEnvironment.Ensure(); err != nil {
		log.Fatalf("Failed to prepare build environment: %v", err)
	}
	runner := engine.NewBuildRunner(db, hub, buildEnvironment.WorkspacesRoot(), loader)
	runner.SetBuildEnvironment(buildEnvironment)
	runner.SetWorkerDispatchToken(cfg.Workers.EnrollmentToken)
	artifactMgr := engine.NewArtifactManager(db, artifactsRoot)
	runner.SetArtifactManager(artifactMgr)
	notificationService := engine.NewNotificationService(db)
	runner.SetNotificationService(notificationService)
	statisticsService := engine.NewStatisticsService(db)
	runner.SetStatisticsService(statisticsService)
	runner.StartQueue(context.Background())
	defer runner.StopQueue()
	approvalService := engine.NewApprovalService(db)
	bigScreenService := engine.NewBigScreenService(db)
	triggerChecker := engine.NewTriggerChecker(db, runner, hub)
	triggerChecker.Start()
	defer triggerChecker.Stop()

	var staticFS fs.FS
	var staticDir string
	for _, distDir := range staticDirectories() {
		if stat, err := os.Stat(distDir); err == nil && stat.IsDir() {
			staticFS = os.DirFS(distDir)
			staticDir = distDir
			break
		}
	}
	var liveReload *api.LiveReload
	if staticDir != "" {
		liveReload, err = api.NewLiveReload(staticDir)
		if err != nil {
			log.Printf("Web hot reload unavailable: %v", err)
		} else {
			defer liveReload.Stop()
			log.Printf("Web hot reload watching %s", staticDir)
		}
	}

	server := api.NewServer(api.Deps{
		Cfg:        cfg,
		Store:      db,
		Hub:        hub,
		Runner:     runner,
		JWT:        jwtInstance,
		Loader:     loader,
		Artifacts:  artifactMgr,
		StaticFS:   staticFS,
		LiveReload: liveReload,
		Statistics: statisticsService,
		Approval:   approvalService,
		BigScreen:  bigScreenService,
	})

	// Graceful shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	cleanupPID, err := writeServerPID()
	if err != nil {
		log.Printf("Warning: failed to write server PID: %v", err)
	} else {
		defer cleanupPID()
	}

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- server.Serve(listener)
	}()

	log.Println("buildworld server started")
	log.Printf("Web UI: http://localhost:%d", cfg.Server.Port)
	log.Printf("API:    http://localhost:%d/api", cfg.Server.Port)

	select {
	case <-sigCh:
		log.Println("Shutting down...")
	case err := <-serverErrCh:
		if err != nil {
			log.Printf("Server startup failed: %v", err)
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	server.Stop(ctx)
}

// staticDirectories supports repository development and release bundles that
// ship web/dist beside the server binary.
func staticDirectories() []string {
	directories := []string{filepath.Join("web", "dist")}
	if executable, err := os.Executable(); err == nil {
		directories = append(directories, filepath.Join(filepath.Dir(executable), "web", "dist"))
	}
	return directories
}

func writeServerPID() (func(), error) {
	directory, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	directory = filepath.Join(directory, "buildworld")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(directory, "server.pid")
	pid := fmt.Sprint(os.Getpid())
	if err := os.WriteFile(path, []byte(pid), 0o600); err != nil {
		return nil, err
	}
	return func() {
		current, err := os.ReadFile(path)
		if err == nil && strings.TrimSpace(string(current)) == pid {
			_ = os.Remove(path)
		}
	}, nil
}

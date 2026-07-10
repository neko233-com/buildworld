package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"log"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/neko233-com/buildworld233/internal/api"
	"github.com/neko233-com/buildworld233/internal/auth"
	"github.com/neko233-com/buildworld233/internal/config"
	"github.com/neko233-com/buildworld233/internal/engine"
	"github.com/neko233-com/buildworld233/internal/plugin"
	"github.com/neko233-com/buildworld233/internal/store"
	"github.com/neko233-com/buildworld233/internal/ws"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Config file path")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Ensure required directories exist.
	artifactsRoot := "./artifacts"
	for _, dir := range []string{filepath.Dir(cfg.Database.Path), cfg.Storage.Workspace, cfg.Plugins.Path, artifactsRoot} {
		if dir != "" {
			_ = os.MkdirAll(dir, 0o755)
		}
	}

	db, err := store.New(cfg.Database.Path)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	if err := auth.SetupDefaultAdmin(db); err != nil {
		log.Printf("Warning: Failed to setup default admin: %v", err)
	}

	// JWT secret: use config or generate a random one for this process.
	jwtSecret := cfg.Auth.JWTSecret
	if jwtSecret == "" {
		jwtSecret = randomSecret()
		log.Println("WARNING: auth.jwt_secret is empty — generated a random secret for this session.")
		log.Println("Set a permanent secret in config.yaml to keep tokens valid across restarts.")
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

	// Build runner (executes pipelines in-process — the "local agent").
	wsRoot := cfg.Storage.Workspace
	if wsRoot == "" {
		wsRoot = "./workspace"
	}
	runner := engine.NewBuildRunner(db, hub, wsRoot, loader)
	artifactMgr := engine.NewArtifactManager(db, artifactsRoot)
	runner.SetArtifactManager(artifactMgr)
	notificationService := engine.NewNotificationService(db)
	runner.SetNotificationService(notificationService)
	statisticsService := engine.NewStatisticsService(db)
	runner.SetStatisticsService(statisticsService)
	approvalService := engine.NewApprovalService(db)
	triggerChecker := engine.NewTriggerChecker(db, runner, hub)
	triggerChecker.Start()
	defer triggerChecker.Stop()

	distDir := filepath.Join("web", "dist")
	var staticFS fs.FS
	if stat, err := os.Stat(distDir); err == nil && stat.IsDir() {
		staticFS = os.DirFS(distDir)
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
		Statistics: statisticsService,
		Approval:   approvalService,
	})

	// Config hot-reload.
	watcher, err := config.Watch(*configPath, func(newCfg *config.Config) {
		log.Println("Config reloaded")
		cfg = newCfg
	})
	if err != nil {
		log.Printf("Failed to watch config: %v", err)
	}
	defer watcher.Stop()

	// Graceful shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := server.Start(); err != nil {
			log.Printf("Server error: %v", err)
		}
	}()

	log.Println("buildworld233 server started")
	log.Printf("Web UI: http://localhost:%d", cfg.Server.Port)
	log.Printf("API:    http://localhost:%d/api", cfg.Server.Port)
	log.Println("Default login: root / root")

	<-sigCh
	log.Println("Shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	server.Stop(ctx)
}

// randomSecret returns a 32-byte hex-encoded random string for JWT signing.
func randomSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// Fallback — should never happen.
		return "buildworld233-fallback-secret-change-me"
	}
	return hex.EncodeToString(b)
}

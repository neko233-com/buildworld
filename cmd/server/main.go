package main

import (
	"context"
	"encoding/json"
	"flag"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"path/filepath"
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
	if err := api.ApplyStoredSettings(cfg, db); err != nil {
		log.Printf("Warning: failed to restore runtime settings: %v", err)
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

	// Sync plugin state with DB.
	dbPlugins, err := db.ListPlugins()
	if err == nil {
		for _, p := range dbPlugins {
			if !p.Enabled {
				loader.SetEnabled(p.Name, false)
				loader.Unload(p.Name)
			} else {
				if p.Source == "upload" {
					if err := loader.Load(p.Name); err != nil {
						log.Printf("Warning: failed to load upload plugin %s: %v", p.Name, err)
					}
				}
				if rp := loader.Get(p.Name); rp != nil {
					stepsJSON, _ := json.Marshal(rp.StepTypes())
					triggersJSON, _ := json.Marshal(nil)
					uiExtJSON, _ := json.Marshal(rp.UIExtensions())
					_ = db.UpdatePluginStatus(p.Name, p.Enabled, string(stepsJSON), string(triggersJSON), string(uiExtJSON))
				}
			}
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
	for _, distDir := range staticDirectories() {
		if stat, err := os.Stat(distDir); err == nil && stat.IsDir() {
			staticFS = os.DirFS(distDir)
			break
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
		Statistics: statisticsService,
		Approval:   approvalService,
		BigScreen:  bigScreenService,
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

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- server.Start()
	}()

	log.Println("buildworld server started")
	log.Printf("Web UI: http://localhost:%d", cfg.Server.Port)
	log.Printf("API:    http://localhost:%d/api", cfg.Server.Port)
	log.Println("Default login: root / root")

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

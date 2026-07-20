package main

import (
	"context"
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

// buildworld-server is the control-plane binary. It also starts the local build
// runner, so a small deployment does not need a separate worker process.
func main() {
	configPath := flag.String("config", "config.yaml", "Config file path")
	flag.Parse()
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	for _, dir := range []string{filepath.Dir(cfg.Database.Path), cfg.Storage.Workspace, cfg.Plugins.Path, "./artifacts"} {
		if dir != "" {
			_ = os.MkdirAll(dir, 0o755)
		}
	}
	db, err := store.New(cfg.Database.Path)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err := api.ApplyStoredSettings(cfg, db); err != nil {
		log.Printf("runtime settings: %v", err)
	}
	if err := auth.SetupDefaultAdmin(db); err != nil {
		log.Printf("default admin: %v", err)
	}
	secret, secretPath, generated, err := auth.LoadOrCreateJWTSecret(cfg.Auth.JWTSecret, cfg.Database.Path)
	if err != nil {
		log.Fatal(err)
	}
	cfg.Auth.JWTSecret = secret
	if generated {
		log.Printf("Generated a persistent JWT signing secret at %s", secretPath)
	}
	hub := ws.NewHub()
	go hub.Run()
	loader := plugin.NewLoader(cfg.Plugins.Path)
	if err := loader.LoadAll(); err != nil {
		log.Printf("plugin load: %v", err)
	}
	buildEnvironment := engine.NewBuildEnvironment(cfg.Storage.BuildTemp)
	if err := buildEnvironment.Ensure(); err != nil {
		log.Fatal(err)
	}
	runner := engine.NewBuildRunner(db, hub, buildEnvironment.WorkspacesRoot(), loader)
	runner.SetBuildEnvironment(buildEnvironment)
	runner.SetWorkerDispatchToken(cfg.Workers.EnrollmentToken)
	runner.SetArtifactManager(engine.NewArtifactManager(db, "./artifacts"))
	runner.SetNotificationService(engine.NewNotificationService(db))
	runner.SetStatisticsService(engine.NewStatisticsService(db))
	checker := engine.NewTriggerChecker(db, runner, hub)
	checker.Start()
	defer checker.Stop()
	var staticFS fs.FS
	if stat, err := os.Stat(filepath.Join("web", "dist")); err == nil && stat.IsDir() {
		staticFS = os.DirFS(filepath.Join("web", "dist"))
	}
	server := api.NewServer(api.Deps{Cfg: cfg, Store: db, Hub: hub, Runner: runner, JWT: auth.NewJWT(secret), Loader: loader, StaticFS: staticFS, Statistics: engine.NewStatisticsService(db), Approval: engine.NewApprovalService(db), BigScreen: engine.NewBigScreenService(db)})
	go func() {
		if err := server.Start(); err != nil {
			log.Printf("server: %v", err)
		}
	}()
	log.Printf("Buildworld server: http://localhost:%d", cfg.Server.Port)
	log.Println("Default login: root / root")
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	<-signals
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Stop(ctx)
}

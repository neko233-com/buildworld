package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/neko233-com/buildworld233/internal/api"
	"github.com/neko233-com/buildworld233/internal/auth"
	"github.com/neko233-com/buildworld233/internal/config"
	"github.com/neko233-com/buildworld233/internal/engine"
	"github.com/neko233-com/buildworld233/internal/store"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Config file path")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	db, err := store.New(cfg.Database.Path)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	if err := auth.SetupDefaultAdmin(db); err != nil {
		log.Printf("Warning: Failed to setup default admin: %v", err)
	}

	scheduler := engine.NewScheduler()
	_ = scheduler

	server := api.NewServer(cfg)

	watcher, err := config.Watch(*configPath, func(newCfg *config.Config) {
		log.Println("Config reloaded")
		cfg = newCfg
	})
	if err != nil {
		log.Printf("Failed to watch config: %v", err)
	}
	defer watcher.Stop()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := server.Start(); err != nil {
			log.Printf("Server error: %v", err)
		}
	}()

	log.Println("buildworld233 server started")
	log.Printf("Web UI: http://localhost:%d", cfg.Server.Port)
	log.Printf("API: http://localhost:%d/api", cfg.Server.Port)
	log.Println("Default login: root / root")

	<-sigCh
	log.Println("Shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	server.Stop(ctx)
}

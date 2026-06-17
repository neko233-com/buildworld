package main

import (
	"context"
	"flag"
	"log"

	"github.com/neko233-com/buildworld233/internal/config"
	"github.com/neko233-com/buildworld233/internal/worker"
)

func main() {
	server := flag.String("server", "http://localhost:6050", "Server address")
	token := flag.String("token", "", "Registration token")
	flag.Parse()

	cfg := &config.Config{
		Workers: config.WorkersConfig{
			Local: config.LocalWorkerConfig{
				Enabled:             true,
				MaxConcurrentBuilds: 4,
				Labels:              []string{"default"},
			},
		},
	}

	daemon := worker.NewDaemon(cfg, *server, *token)

	if err := daemon.Start(context.Background()); err != nil {
		log.Fatalf("Failed to start worker: %v", err)
	}
}

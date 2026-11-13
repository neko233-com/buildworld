package main

import (
	"context"
	"encoding/json"
	"flag"
	"github.com/neko233-com/buildworld/internal/buildinfo"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/neko233-com/buildworld/internal/config"
	_ "github.com/neko233-com/buildworld/internal/migration" // registers the Jenkinsfile -> BuildConfig converter
	"github.com/neko233-com/buildworld/internal/worker"
)

func main() {
	version := flag.Bool("version", false, "Print worker release identity")
	applyUpdate := flag.String("apply-worker-update", "", "Private update helper plan")
	server := flag.String("server", "http://localhost:8080", "Server address")
	token := flag.String("token", "", "Registration token")
	listen := flag.String("listen", ":6051", "Worker gRPC listen address")
	advertise := flag.String("advertise", "", "Worker address reachable from buildworld-server")
	name := flag.String("name", "buildworld-worker", "Worker name")
	labels := flag.String("labels", "go,nodejs,typescript", "Comma-separated worker labels")
	pool := flag.String("pool", "validation", "Worker pool")
	maxBuilds := flag.Int("max-builds", 4, "Maximum concurrent builds")
	stateDir := flag.String("state-dir", "", "Private persistent worker identity and update state directory")
	buildTemp := flag.String("build-temp", "./build_temp", "Disposable workspace and language-cache root; relative paths use the process working directory")
	flag.Parse()
	if *version {
		executable, _ := os.Executable()
		_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"version": buildinfo.Version, "source_commit": buildinfo.WorkerReleaseCommit(filepath.Clean(executable), buildinfo.Version)})
		return
	}
	if *applyUpdate != "" {
		if err := worker.ApplyUpdate(*applyUpdate); err != nil {
			log.Print("Worker update helper failed; inspect retained update state")
			os.Exit(1)
		}
		return
	}

	parsedLabels := make([]string, 0)
	for _, label := range strings.Split(*labels, ",") {
		if label = strings.TrimSpace(label); label != "" {
			parsedLabels = append(parsedLabels, label)
		}
	}
	cfg := &config.Config{
		Storage: config.StorageConfig{BuildTemp: *buildTemp},
		Workers: config.WorkersConfig{
			Local: config.LocalWorkerConfig{
				Workspace:           *name,
				Pool:                *pool,
				MaxConcurrentBuilds: *maxBuilds,
				Labels:              parsedLabels,
			},
		},
	}

	daemon := worker.NewDaemon(cfg, *server, *token, *listen, *advertise)

	if err := daemon.SetStateDirectory(*stateDir); err != nil {
		log.Fatal("Invalid worker state directory")
	}
	if err := daemon.Start(context.Background()); err != nil {
		log.Fatalf("Failed to start worker: %v", err)
	}
}

package main

import (
	"context"
	"flag"
	"log"
	"strings"

	"github.com/neko233-com/buildworld/internal/config"
	"github.com/neko233-com/buildworld/internal/worker"
)

func main() {
	server := flag.String("server", "http://localhost:8700", "Buildworld server address")
	token := flag.String("token", "", "Worker enrollment token")
	listen := flag.String("listen", ":6051", "Worker gRPC listen address")
	advertise := flag.String("advertise", "", "Worker address reachable from buildworld-server")
	name := flag.String("name", "buildworld-worker", "Worker name")
	labels := flag.String("labels", "go,nodejs,typescript", "Comma-separated worker labels")
	pool := flag.String("pool", "validation", "Worker pool")
	maxBuilds := flag.Int("max-builds", 4, "Maximum concurrent builds")
	buildTemp := flag.String("build-temp", "./build_temp", "Disposable workspace and language-cache root beside the binary")
	flag.Parse()
	parsedLabels := splitLabels(*labels)
	cfg := &config.Config{Storage: config.StorageConfig{BuildTemp: *buildTemp}, Workers: config.WorkersConfig{Local: config.LocalWorkerConfig{Enabled: true, Workspace: *name, Pool: *pool, MaxConcurrentBuilds: *maxBuilds, Labels: parsedLabels}}}
	if err := worker.NewDaemon(cfg, *server, *token, *listen, *advertise).Start(context.Background()); err != nil {
		log.Fatal(err)
	}
}

func splitLabels(value string) []string {
	var labels []string
	for _, label := range strings.Split(value, ",") {
		if label = strings.TrimSpace(label); label != "" {
			labels = append(labels, label)
		}
	}
	return labels
}

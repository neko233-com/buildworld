package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/neko233-com/buildworld233/internal/config"
)

type Server struct {
	httpServer *http.Server
	cfg        *config.Config
}

func NewServer(cfg *config.Config) *Server {
	router := NewRouter(cfg)

	return &Server{
		httpServer: &http.Server{
			Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
			Handler:      router,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		cfg: cfg,
	}
}

func (s *Server) Start() error {
	log.Printf("Starting server on %s", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

func (s *Server) Stop(ctx context.Context) error {
	log.Println("Stopping server...")
	return s.httpServer.Shutdown(ctx)
}

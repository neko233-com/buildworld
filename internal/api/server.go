package api

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/neko233-com/buildworld/internal/auth"
	"github.com/neko233-com/buildworld/internal/config"
	"github.com/neko233-com/buildworld/internal/engine"
	"github.com/neko233-com/buildworld/internal/plugin"
	"github.com/neko233-com/buildworld/internal/store"
	"github.com/neko233-com/buildworld/internal/ws"
)

type Server struct {
	cfg     *config.Config
	store   *store.Store
	hub     *ws.Hub
	runner  *engine.BuildRunner
	jwt     *auth.JWT
	loader  *plugin.Loader
	httpSrv *http.Server
}

type Deps struct {
	Cfg        *config.Config
	Store      *store.Store
	Hub        *ws.Hub
	Runner     *engine.BuildRunner
	Artifacts  *engine.ArtifactManager
	JWT        *auth.JWT
	Loader     *plugin.Loader
	StaticFS   fs.FS
	LiveReload *LiveReload
	Statistics *engine.StatisticsService
	Approval   *engine.ApprovalService
	BigScreen  *engine.BigScreenService
}

func NewServer(d Deps) *Server {
	router := NewRouter(d)
	addr := fmt.Sprintf("%s:%d", d.Cfg.Server.Host, d.Cfg.Server.Port)
	return &Server{
		cfg:    d.Cfg,
		store:  d.Store,
		hub:    d.Hub,
		runner: d.Runner,
		jwt:    d.JWT,
		loader: d.Loader,
		httpSrv: &http.Server{
			Addr:         addr,
			Handler:      router,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  120 * time.Second,
		},
	}
}

func (s *Server) Start() error {
	log.Printf("HTTP server listening on %s", s.httpSrv.Addr)
	return s.httpSrv.ListenAndServe()
}

// Serve starts the API on a listener reserved before side-effectful services
// such as queue recovery. A duplicate server therefore cannot mutate builds
// and only discover its port conflict afterward.
func (s *Server) Serve(listener net.Listener) error {
	log.Printf("HTTP server listening on %s", listener.Addr())
	return s.httpSrv.Serve(listener)
}

func (s *Server) Stop(ctx context.Context) error {
	log.Println("HTTP server shutting down...")
	return s.httpSrv.Shutdown(ctx)
}

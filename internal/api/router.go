package api

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/neko233-com/buildworld233/internal/auth"
	"github.com/neko233-com/buildworld233/internal/ws"
)

func responseHeaderMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := middleware.GetReqID(r.Context())
		if rid != "" {
			w.Header().Set("X-Request-ID", rid)
		}
		next.ServeHTTP(w, r)
	})
}

func contentTypeJSONMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

// publicPrefixes are API paths that skip JWT auth (login/register/health/version/webhooks).
var publicPrefixes = []string{
	"/api/health",
	"/api/version",
	"/api/auth/login",
	"/api/auth/register",
	"/api/webhooks/",
}

// NewRouter wires all REST routes + WebSocket + webhooks onto a chi router.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(responseHeaderMiddleware)

	h := &handlers{d: d}

	r.Route("/api", func(r chi.Router) {
		r.Use(contentTypeJSONMiddleware)
		r.Use(auth.Middleware(d.JWT, publicPrefixes...))
		// --- health & version ---
		r.Get("/health", h.health)
		r.Get("/version", h.version)

		// --- auth ---
		r.Route("/auth", func(r chi.Router) {
			r.Post("/login", h.login)
			r.Post("/register", h.register)
			r.Get("/me", h.me)
		})

		// --- vcs roots ---
		r.Route("/vcs-roots", func(r chi.Router) {
			r.Get("/", h.listVCSRoots)
			r.Post("/", h.createVCSRoot)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", h.getVCSRoot)
				r.Put("/", h.updateVCSRoot)
				r.Delete("/", h.deleteVCSRoot)
			})
		})

		// --- templates ---
		r.Route("/templates", func(r chi.Router) {
			r.Get("/", h.listTemplates)
			r.Post("/", h.createTemplate)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", h.getTemplate)
				r.Put("/", h.updateTemplate)
				r.Delete("/", h.deleteTemplate)
			})
		})

		// --- artifacts (must be before builds/{id} to avoid path collision) ---
		r.Route("/artifacts", func(r chi.Router) {
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/download", h.downloadArtifact)
			})
		})

		// --- projects ---
		r.Route("/projects", func(r chi.Router) {
			r.Get("/", h.listProjects)
			r.Post("/", h.createProject)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", h.getProject)
				r.Put("/", h.updateProject)
				r.Delete("/", h.deleteProject)
				r.Get("/builds", h.listProjectBuilds)
				r.Post("/builds", h.triggerBuild)
			})
		})

		// --- builds ---
		r.Route("/builds", func(r chi.Router) {
			r.Get("/", h.listBuilds)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", h.getBuild)
				r.Get("/logs", h.getBuildLogs)
				r.Post("/stop", h.stopBuild)
				r.Post("/retry", h.retryBuild)
				r.Post("/pin", h.pinBuild)
				r.Get("/artifacts", h.listBuildArtifacts)
				r.Post("/artifacts", h.uploadArtifact)
			})
		})

		// --- agents/workers ---
		r.Route("/agents", func(r chi.Router) {
			r.Get("/", h.listAgents)
			r.Post("/register", h.registerAgent)
			r.Route("/{id}", func(r chi.Router) {
				r.Delete("/", h.deleteAgent)
			})
			r.Post("/generate-token", h.generateAgentToken)
		})

		// --- plugins ---
		r.Route("/plugins", func(r chi.Router) {
			r.Get("/", h.listPlugins)
			r.Post("/", h.installPlugin)
			r.Route("/{id}", func(r chi.Router) {
				r.Delete("/", h.deletePlugin)
				r.Put("/enable", h.togglePlugin)
			})
		})

		// --- users ---
		r.Route("/users", func(r chi.Router) {
			r.Get("/", h.listUsers)
			r.Post("/", h.createUser)
			r.Route("/{id}", func(r chi.Router) {
				r.Delete("/", h.deleteUser)
				r.Put("/role", h.updateUserRole)
				r.Put("/password", h.updateUserPassword)
			})
		})

		// --- env vars ---
		r.Route("/env-vars", func(r chi.Router) {
			r.Get("/", h.listEnvVars)
			r.Post("/", h.setEnvVar)
			r.Delete("/{id}", h.deleteEnvVar)
		})

		// --- credentials ---
		r.Route("/credentials", func(r chi.Router) {
			r.Get("/", h.listCredentials)
			r.Post("/", h.createCredential)
			r.Get("/lookup", h.lookupCredential)
			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", h.getCredential)
				r.Put("/", h.updateCredential)
				r.Delete("/", h.deleteCredential)
			})
		})

		// --- notifications ---
		r.Route("/notifications", func(r chi.Router) {
			r.Route("/channels", func(r chi.Router) {
				r.Get("/", h.listNotificationChannels)
				r.Post("/", h.createNotificationChannel)
				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", h.getNotificationChannel)
					r.Put("/", h.updateNotificationChannel)
					r.Delete("/", h.deleteNotificationChannel)
					r.Get("/events", h.listNotificationEvents)
				})
			})
		})

		// --- webhooks (public) ---
		r.Route("/webhooks", func(r chi.Router) {
			r.Post("/github", h.githubWebhook)
			r.Post("/gitlab", h.gitlabWebhook)
			r.Post("/gitea", h.giteaWebhook)
		})
	})

	// WebSocket endpoint.
	r.Get("/ws", func(w http.ResponseWriter, r *http.Request) {
		ws.HandleWebSocket(d.Hub, w, r)
	})

	// Static frontend (SPA) — served from embedded or disk in main.
	if d.StaticFS != nil {
		fileServer := http.FileServer(http.FS(d.StaticFS))
		r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			if path == "/" {
				fileServer.ServeHTTP(w, r)
				return
			}
			// Try exact file first.
			if f, err := d.StaticFS.Open(strings.TrimPrefix(path, "/")); err == nil {
				f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}
			// Also try assets/ directory.
			if strings.HasPrefix(path, "/assets/") {
				fileServer.ServeHTTP(w, r)
				return
			}
			// SPA fallback: serve index.html.
			indexHTML, err := fs.ReadFile(d.StaticFS, "index.html")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			w.Write(indexHTML)
		})
	}

	return r
}

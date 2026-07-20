package api

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/neko233-com/buildworld/internal/auth"
	"github.com/neko233-com/buildworld/internal/migration"
	"github.com/neko233-com/buildworld/internal/portability"
	"github.com/neko233-com/buildworld/internal/ws"
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
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		next.ServeHTTP(w, r)
	})
}

// publicPrefixes are API paths that skip JWT auth. Account provisioning and
// operational dashboards intentionally stay authenticated.
var publicPrefixes = []string{
	"/api/health",
	"/api/version",
	"/api/auth/login",
	"/api/webhooks/",
	"/api/badge/",
	"/api/trigger/",
	"/api/agents/auto-register",
	"/api/agents/heartbeat/",
}

// NewRouter wires all REST routes + WebSocket + webhooks onto a chi router.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()

	// Do not use chi's synchronous request logger here. It writes every request
	// to stdout and can stall the entire HTTP response path when the server is
	// launched in the background with an unconsumed redirected pipe. Durable
	// operational events are already recorded through the audit/event stores.
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(responseHeaderMiddleware)
	r.Use(middleware.Compress(5))

	portabilityRegistry, _ := portability.NewDefaultRegistry(d.Store, d.Loader)
	h := &handlers{d: d, portability: portabilityRegistry, migrations: migration.NewDefaultRegistry()}
	apiTokenValidator := auth.NewAPITokenValidator(d.Store)
	adminOnly := auth.RequireRoles("admin")
	editors := auth.RequireRoles("admin", "developer")

	r.Route("/api", func(r chi.Router) {
		r.Use(contentTypeJSONMiddleware)
		r.Group(func(r chi.Router) {
			r.Use(auth.Middleware(d.JWT, apiTokenValidator, publicPrefixes...))
			// --- health & version ---
			r.Get("/health", h.health)
			r.Get("/version", h.version)

			// --- auth ---
			r.Route("/auth", func(r chi.Router) {
				r.Post("/login", h.login)
				r.With(adminOnly).Post("/register", h.register)
				r.Get("/me", h.me)
			})

			// --- vcs roots ---
			r.Route("/vcs-roots", func(r chi.Router) {
				r.Get("/", h.listVCSRoots)
				r.With(editors).Post("/", h.createVCSRoot)
				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", h.getVCSRoot)
					r.With(editors).Put("/", h.updateVCSRoot)
					r.With(editors).Delete("/", h.deleteVCSRoot)
				})
			})

			// --- templates ---
			r.Route("/templates", func(r chi.Router) {
				r.Get("/", h.listTemplates)
				r.With(editors).Post("/", h.createTemplate)
				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", h.getTemplate)
					r.With(editors).Put("/", h.updateTemplate)
					r.With(editors).Delete("/", h.deleteTemplate)
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
				r.With(editors).Post("/", h.createProject)
				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", h.getProject)
					r.With(editors).Put("/", h.updateProject)
					r.With(editors).Post("/validate", h.validateProjectConfig)
					r.With(editors).Delete("/", h.deleteProject)
					r.Get("/builds", h.listProjectBuilds)
					r.With(editors).Post("/builds", h.triggerBuild)
					r.Get("/stats", h.getProjectStats)
					r.Route("/hooks", func(r chi.Router) {
						r.Get("/", h.listGitHooks)
						r.With(editors).Post("/", h.createGitHook)
					})
				})
			})

			// --- external pipeline migration strategies ---
			r.Route("/pipeline-migrations", func(r chi.Router) {
				r.With(editors).Post("/{format}", h.migratePipeline)
			})

			// --- builds ---
			r.Route("/builds", func(r chi.Router) {
				r.Get("/", h.listBuilds)
				r.Get("/search", h.searchBuilds)
				r.Route("/{id}", func(r chi.Router) {
					r.Get("/", h.getBuild)
					r.Get("/logs", h.getBuildLogs)
					r.Get("/timeline", h.getBuildTimeline)
					r.Get("/problems", h.getBuildProblems)
					r.Get("/chain", h.getBuildChain)
					r.Get("/logs/download", h.downloadBuildLogs)
					r.Get("/logs/search", h.searchBuildLogs)
					r.With(editors).Post("/stop", h.stopBuild)
					r.With(editors).Post("/retry", h.retryBuild)
					r.With(editors).Post("/pin", h.pinBuild)
					r.With(editors).Post("/approve", h.approveBuild)
					r.With(editors).Post("/reject", h.rejectBuild)
					r.Get("/artifacts", h.listBuildArtifacts)
					r.With(editors).Post("/artifacts", h.uploadArtifact)
					r.Get("/test-results", h.getBuildTestResults)
					r.With(editors).Post("/test-results", h.uploadTestResults)
				})
			})

			// --- agents/workers ---
			r.Route("/agents", func(r chi.Router) {
				r.Get("/", h.listAgents)
				r.With(adminOnly).Post("/register", h.registerAgent)
				r.Post("/auto-register", h.registerAgent)
				r.Post("/heartbeat/{id}", h.workerHeartbeat)
				r.Route("/{id}", func(r chi.Router) {
					r.With(adminOnly).Delete("/", h.deleteAgent)
				})
				r.With(adminOnly).Post("/generate-token", h.generateAgentToken)
			})

			// --- plugins ---
			r.Route("/plugins", func(r chi.Router) {
				r.Get("/", h.listPlugins)
				r.With(adminOnly).Post("/", h.installPlugin)
				r.With(adminOnly).Post("/github", h.installGitHubPlugin)
				r.Get("/ui-extensions", h.listUIExtensions)
				r.Route("/{name}", func(r chi.Router) {
					r.With(adminOnly).Delete("/", h.deletePlugin)
					r.With(adminOnly).Put("/enable", h.togglePlugin)
					r.With(adminOnly).Post("/reload", h.reloadPlugin)
					r.Get("/ui.js", h.getPluginUI)
					r.With(adminOnly).Get("/source", h.getPluginSource)
					r.With(adminOnly).Put("/source", h.updatePluginSource)
				})
			})

			// --- users ---
			r.Route("/users", func(r chi.Router) {
				r.Use(adminOnly)
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
				r.Use(adminOnly)
				r.Get("/", h.listEnvVars)
				r.Post("/", h.setEnvVar)
				r.Delete("/{id}", h.deleteEnvVar)
			})

			// --- credentials ---
			r.Route("/credentials", func(r chi.Router) {
				r.Use(adminOnly)
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
				r.Get("/in-app", h.listInAppNotifications)
				r.Post("/in-app/read", h.markInAppNotificationsRead)
				r.Route("/channels", func(r chi.Router) {
					r.Use(adminOnly)
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

			// --- statistics ---
			r.Get("/stats/dashboard", h.getDashboardStats)

			// --- git hooks ---
			r.Route("/hooks/{id}", func(r chi.Router) {
				r.Get("/", h.getGitHook)
				r.With(editors).Put("/", h.updateGitHook)
				r.With(editors).Delete("/", h.deleteGitHook)
			})

			// --- big screen ---
			r.Get("/bigscreen", h.getBigScreenData)

			// --- audit logs ---
			r.With(adminOnly).Get("/audit-logs", h.listAuditLogs)

			// --- api tokens ---
			r.Route("/api-tokens", func(r chi.Router) {
				r.Get("/", h.listAPITokens)
				r.Post("/", h.createAPIToken)
				r.Delete("/{id}", h.deleteAPIToken)
			})

			// --- approvals ---
			r.Get("/approvals/pending", h.listPendingApprovals)

			// --- deployment environments ---
			r.Route("/deployment-envs", func(r chi.Router) {
				r.Get("/", h.listDeploymentEnvs)
				r.With(editors).Post("/", h.createDeploymentEnv)
				r.With(editors).Put("/{id}", h.updateDeploymentEnv)
				r.With(editors).Delete("/{id}", h.deleteDeploymentEnv)
				r.With(editors).Post("/{id}/deploy/{buildId}", h.deployBuild)
			})

			// --- project groups ---
			r.Route("/project-groups", func(r chi.Router) {
				r.Get("/", h.listProjectGroups)
				r.With(editors).Post("/", h.createProjectGroup)
				r.With(editors).Put("/{id}", h.updateProjectGroup)
				r.With(editors).Delete("/{id}", h.deleteProjectGroup)
			})

			// --- build queue ---
			r.Route("/build-queue", func(r chi.Router) {
				r.Get("/", h.listBuildQueue)
				r.With(editors).Put("/{id}", h.reorderBuildQueue)
			})

			// --- global settings ---
			r.With(adminOnly).Get("/settings", h.getGlobalSettings)
			r.With(adminOnly).Put("/settings", h.updateGlobalSettings)
			r.With(adminOnly).Get("/settings/package-proxies", h.getPackageProxies)
			r.With(adminOnly).Post("/settings/package-proxies/apply", h.applyPackageProxies)

			// --- versioned configuration portability ---
			r.Route("/portability", func(r chi.Router) {
				r.Use(adminOnly)
				r.Get("/capabilities", h.portabilityCapabilities)
				r.Post("/export", h.exportPortabilityBundle)
				r.Post("/inspect", h.inspectPortabilityBundle)
				r.Post("/import", h.importPortabilityBundle)
			})

			// --- server metrics ---
			r.With(adminOnly).Get("/metrics", h.serverMetrics)

			// --- badge (public) ---
			r.Get("/badge/{projectName}", h.buildBadge)

			// --- http trigger (public, validates api token internally) ---
			r.Post("/trigger/{projectName}", h.httpTriggerBuild)
		})
	})
	// WebSocket endpoint.
	r.With(auth.Middleware(d.JWT, apiTokenValidator)).Get("/ws", func(w http.ResponseWriter, r *http.Request) {
		ws.HandleWebSocket(d.Hub, w, r)
	})

	// Disk-backed frontend assets can notify an open browser after HTML, CSS,
	// JavaScript, or TypeScript output changes. These endpoints deliberately sit
	// outside /api and do not expose project workspaces.
	if d.LiveReload != nil {
		r.Get("/__buildworld/livereload", d.LiveReload.ServeEvents)
		r.Get("/__buildworld/livereload.js", d.LiveReload.ServeScript)
	}

	// Static frontend (SPA) — served from embedded or disk in main.
	if d.StaticFS != nil {
		fileServer := http.FileServer(http.FS(d.StaticFS))
		serveIndex := func(w http.ResponseWriter, r *http.Request) {
			indexHTML, err := fs.ReadFile(d.StaticFS, "index.html")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			if d.LiveReload != nil {
				indexHTML = []byte(strings.Replace(string(indexHTML), "</head>", "<script src=\"/__buildworld/livereload.js\"></script></head>", 1))
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(indexHTML)
		}
		r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			if path == "/" || path == "/index.html" {
				serveIndex(w, r)
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
			serveIndex(w, r)
		})
	}

	return r
}

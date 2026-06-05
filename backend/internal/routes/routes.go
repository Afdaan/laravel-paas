// ===========================================
// Routes Package
// ===========================================
// Defines all API endpoints and middleware
// ===========================================
package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"gorm.io/gorm"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/backend/internal/handlers"
	domainHandlerPkg "github.com/laravel-paas/backend/internal/handlers/domain"
	projectHandlerPkg "github.com/laravel-paas/backend/internal/handlers/project"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/infrastructure/docker"
	"github.com/laravel-paas/backend/internal/middleware"
	"github.com/laravel-paas/shared/pkg/metrics"
	"github.com/laravel-paas/backend/internal/services"
	projectServicePkg "github.com/laravel-paas/backend/internal/services/project"
	"github.com/laravel-paas/shared/services/setting"
)

// Setup initializes the Fiber app with all routes
func Setup(
	db *gorm.DB,
	cfg *config.Config,
	redisService *infrastructure.RedisService,
	dockerService *docker.DockerService,
	storageService *infrastructure.StorageService,
	projectService *projectServicePkg.ProjectService,
	userService *services.UserService,
	settingService *setting.SettingService,
	authService *services.AuthService,
	databaseService *services.DatabaseService,
	feedbackService *services.FeedbackService,
	domainHandler *domainHandlerPkg.DomainHandler,
	secretStoreService *services.SecretStoreService,
) *fiber.App {
	app := fiber.New(fiber.Config{
		ErrorHandler: handlers.ErrorHandler,
		AppName:      "Laravel PaaS API",
		ProxyHeader:  "X-Forwarded-For", // Correctly detect client IP behind Nginx/Traefik
	})

	// ===========================================
	// Global Middlewares
	// ===========================================
	app.Use(recover.New())
	app.Use(logger.New(logger.Config{
		Next: func(c *fiber.Ctx) bool {
			// Skip logging for high-frequency polling endpoints to keep console clean
			path := c.Path()
			return path == "/health" ||
				path == "/api/internal/traefik/config" ||
				(c.Method() == "GET" && (path == "/api/projects/stats" ||
					path == "/api/admin/stats" ||
					path == "/api/queue/stats" ||
					(len(path) > 15 && path[len(path)-11:] == "/build-logs") ||
					(len(path) > 12 && path[len(path)-12:] == "/logs/stream") ||
					(len(path) > 10 && path[len(path)-5:] == "/logs")))
		},
	}))
	app.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.FrontendURL,
		AllowMethods:     "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders:     "Origin,Content-Type,Accept,Authorization",
		AllowCredentials: true,
	}))

	// ===========================================
	// Health Check
	// ===========================================
	app.Get("/health", func(c *fiber.Ctx) error {
		// Check Database
		dbConn, err := db.DB()
		if err != nil {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"status": "error"})
		}
		if err := dbConn.Ping(); err != nil {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"status": "error"})
		}

		// Check Redis
		if redisService != nil {
			if err := redisService.Ping(c.Context()); err != nil {
				return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"status": "error"})
			}
		}

		return c.JSON(fiber.Map{"status": "ok"})
	})

	// Prometheus Metrics Endpoint
	app.Get("/metrics", metrics.PrometheusHandler())

	// API Routes
	api := app.Group("/api")

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(authService, cfg, userService)
	userHandler := handlers.NewUserHandler(userService)
	projectHandler := projectHandlerPkg.NewProjectHandler(cfg, db, redisService, projectService, userService, dockerService, secretStoreService)
	settingHandler := handlers.NewSettingHandler(settingService)
	systemHandler := handlers.NewSystemHandler(userService, dockerService)
	feedbackHandler := handlers.NewFeedbackHandler(feedbackService)
	databaseHandler := handlers.NewDatabaseHandler(db, cfg, databaseService, projectService, redisService, secretStoreService)
	githubService := infrastructure.NewGithubService(cfg, redisService)
	githubAppHandler := handlers.NewGithubAppHandler(db, cfg, githubService, redisService, projectService)
	secretStoreHandler := handlers.NewSecretStoreHandler(db, cfg, secretStoreService)

	// ===========================================
	// Subdomain Proxy for User Projects (protected + rate limited)
	// ===========================================
	proxyGroup := app.Group("/proxy", middleware.ProxyAuth(cfg.JWTSecret, redisService, userService))
	proxyGroup.Use(middleware.RateLimitProxy())
	proxyGroup.Use(middleware.ValidateProxyTarget())
	proxyGroup.All("/*", projectHandler.ProxyToProject)

	// -----------------------------
	// Auth Routes (public, rate limited)
	// -----------------------------
	auth := api.Group("/auth")
	auth.Post("/login", middleware.RateLimitLogin(), authHandler.Login)
	api.Post("/webhooks/github-app", githubAppHandler.Webhook)

	// -----------------------------
	// System Init (public, rate limited)
	// -----------------------------
	systemInit := api.Group("/system", middleware.RateLimitLogin())
	systemInit.Get("/init-status", systemHandler.GetInitStatus)
	systemInit.Post("/initialize", systemHandler.InitializeSystem)

	// -----------------------------
	// Internal System Routes (Publicly accessible but meant for internal mesh network)
	// -----------------------------
	internal := api.Group("/internal")
	internal.Get("/traefik/config", domainHandler.GetTraefikConfig)

	// -----------------------------
	// Protected Routes
	// -----------------------------
	protected := api.Group("", middleware.JWTAuth(cfg.JWTSecret, redisService, userService))

	// Auth (protected)
	protected.Post("/auth/logout", authHandler.Logout)
	protected.Get("/auth/me", authHandler.Me)
	protected.Put("/auth/profile", authHandler.UpdateProfile)
	protected.Post("/auth/stream-token", authHandler.GenerateStreamToken)

	// GitHub Integration
	protected.Get("/github/installations", githubAppHandler.ListInstallations)
	protected.Post("/github/installations/link", githubAppHandler.LinkInstallation)
	protected.Get("/github/installations/:id/repositories", githubAppHandler.ListRepositories)
	protected.Get("/github/repositories/:owner/:repo/branches", githubAppHandler.ListBranches)

	// Feedback (common)
	protected.Post("/feedback", feedbackHandler.Create)
	protected.Get("/feedback", feedbackHandler.ListOwn)

	// Domains (Centralized User view)
	protected.Get("/domains", domainHandler.ListAll)

	// -----------------------------
	// Admin Routes
	// -----------------------------
	admin := protected.Group("/admin", middleware.RequireAdmin())

	// User management
	admin.Get("/users", userHandler.List)
	admin.Post("/users", userHandler.Create)
	admin.Post("/users/import", userHandler.ImportExcel)
	admin.Get("/users/:id", userHandler.Get)
	admin.Put("/users/:id", userHandler.Update)
	admin.Delete("/users/:id", userHandler.Delete)
	admin.Post("/users/:id/login-as", authHandler.LoginAsUser)

	// Settings
	admin.Get("/settings", settingHandler.List)
	admin.Put("/settings", settingHandler.Update)

	// Admin project overview
	admin.Get("/projects", projectHandler.ListAll)
	admin.Get("/stats", projectHandler.AdminStats)

	// Feedback management (Admin)
	admin.Get("/feedback", feedbackHandler.ListAll)
	admin.Put("/feedback/:id/status", feedbackHandler.UpdateStatus)
	admin.Delete("/feedback/:id", feedbackHandler.Delete)

	// Queue statistics (admin only)
	admin.Get("/queue/stats", projectHandler.GetQueueStats)
	admin.Post("/queue/cancel/:id", projectHandler.CancelQueueJob)
	admin.Post("/queue/requeue/:id", projectHandler.RequeueJob)
	admin.Get("/projects/stats", projectHandler.GetProjectsStats)
	admin.Get("/databases", databaseHandler.AdminListAll)
	admin.Get("/domains", domainHandler.ListGlobal)
	admin.Get("/domains/metrics", domainHandler.ListGlobalMetrics)

	// System monitoring (PaaS style)
	admin.Get("/system/stats", systemHandler.GetStats)
	admin.Post("/system/prune", systemHandler.PruneSystem)

	// SecretStore (Admin)
	admin.Get("/secretstores", secretStoreHandler.AdminListAll)
	admin.Put("/secretstores/:id/disable", secretStoreHandler.AdminDisable)
	admin.Get("/secretstores/logs", secretStoreHandler.AdminListLogs)

	// SecretStore (User)
	secretstores := protected.Group("/secretstores")
	secretstores.Get("/", secretStoreHandler.List)
	secretstores.Post("/", secretStoreHandler.Create)
	secretstores.Get("/:id", secretStoreHandler.Get)
	secretstores.Put("/:id", secretStoreHandler.Update)
	secretstores.Delete("/:id", secretStoreHandler.Delete)
	secretstores.Post("/:id/secrets", secretStoreHandler.SetSecret)
	secretstores.Get("/:id/items/:itemID/reveal", secretStoreHandler.RevealSecret)
	secretstores.Post("/:id/bindings", secretStoreHandler.Bind)
	secretstores.Delete("/:id/bindings/:bindingID", secretStoreHandler.Unbind)
	secretstores.Get("/:id/export", secretStoreHandler.Export)
	secretstores.Post("/:id/import", secretStoreHandler.Import)

	// SecretStore Item Management (Milestone 2)
	secretstores.Post("/:id/items", secretStoreHandler.CreateItem)
	secretstores.Put("/:id/items/:itemID", secretStoreHandler.UpdateItem)
	secretstores.Delete("/:id/items/:itemID", secretStoreHandler.DeleteItem)
	secretstores.Get("/:id/items/:itemID/history", secretStoreHandler.History)

	// -----------------------------
	// Project Routes (Users)
	// -----------------------------
	projects := protected.Group("/projects")
	projects.Get("/", projectHandler.ListOwn)
	projects.Post("/", projectHandler.Create)
	projects.Get("/:id", projectHandler.Get)
	projects.Get("/:id/branches", projectHandler.ListBranches)
	projects.Put("/:id", projectHandler.Update)
	projects.Post("/:id/redeploy", projectHandler.Redeploy)
	projects.Post("/:id/rollback", projectHandler.Rollback)
	projects.Post("/:id/stop", projectHandler.Stop)
	projects.Post("/:id/start", projectHandler.Start)
	projects.Post("/:id/restart", projectHandler.Restart)
	projects.Delete("/:id", projectHandler.Delete)
	projects.Get("/:id/logs", projectHandler.Logs)
	projects.Get("/:id/logs/stream", projectHandler.StreamLogs)
	projects.Get("/:id/build-logs", projectHandler.BuildLogs)
	projects.Get("/:id/build-logs/stream", projectHandler.StreamBuildLogs)
	projects.Get("/:id/deployment-events", projectHandler.GetDeploymentEvents)
	projects.Get("/:id/deployment-events/stream", projectHandler.StreamDeploymentEvents)
	projects.Get("/:id/stats", projectHandler.Stats)
	projects.Post("/:id/artisan", middleware.RateLimitArtisan(), projectHandler.RunArtisan)
	projects.Get("/:id/env", projectHandler.GetEnv)
	projects.Put("/:id/env", projectHandler.UpdateEnv)

	// Domain Management Routes
	domainHandler.RegisterRoutes(projects.Group("/:id/domains"))

	// -----------------------------
	// Database Management Routes
	// -----------------------------
	projects.Get("/:id/database/credentials", databaseHandler.GetCredentials)
	projects.Post("/:id/database/rotate-credentials", databaseHandler.RotateCredentials)
	projects.Post("/:id/database/restart", databaseHandler.RestartDatabase)
	projects.Post("/:id/database/status", databaseHandler.UpdateStatus)
	projects.Get("/:id/database/overview", databaseHandler.GetOverview)
	projects.Get("/:id/database/schema", databaseHandler.GetSchema)
	projects.Post("/:id/database/designer", databaseHandler.ExecuteDesignerAction)
	projects.Get("/:id/database/backups", databaseHandler.ListBackups)
	projects.Post("/:id/database/backups", databaseHandler.CreateBackup)
	projects.Post("/:id/database/backups/:backup/restore", databaseHandler.RestoreBackup)
	projects.Delete("/:id/database/backups/:backup", databaseHandler.DeleteBackup)
	projects.Get("/:id/database/backups/:backup/download", databaseHandler.DownloadBackup)
	projects.Get("/:id/database/metrics", databaseHandler.GetMetrics)
	projects.Post("/:id/database/transfer", databaseHandler.TransferDatabase)

	// Fallback/Legacy endpoints
	projects.Get("/:id/database/tables", databaseHandler.ListTables)
	projects.Get("/:id/database/tables/:table", databaseHandler.GetTableStructure)
	projects.Get("/:id/database/tables/:table/data", databaseHandler.GetTableData)
	projects.Delete("/:id/database/tables/:table/rows", databaseHandler.DeleteTableRow)
	projects.Put("/:id/database/tables/:table/rows", databaseHandler.UpdateTableRow)
	projects.Post("/:id/database/query", middleware.RateLimitQuery(), databaseHandler.ExecuteQuery)
	projects.Get("/:id/database/export", databaseHandler.ExportDatabase)
	projects.Post("/:id/database/import", middleware.RateLimitImport(), databaseHandler.ImportDatabase)
	projects.Post("/:id/database/reset", databaseHandler.ResetDatabase)

	return app
}

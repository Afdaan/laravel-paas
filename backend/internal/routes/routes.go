// ===========================================
// Routes Package
// ===========================================
// Defines all API endpoints and middleware
// ===========================================
package routes

import (
	"net"
	"net/url"
	"strings"
	"time"



	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"gorm.io/gorm"

	"github.com/laravel-paas/backend/internal/handlers"
	domainHandlerPkg "github.com/laravel-paas/backend/internal/handlers/domain"
	projectHandlerPkg "github.com/laravel-paas/backend/internal/handlers/project"
	"github.com/laravel-paas/backend/internal/middleware"
	"github.com/laravel-paas/backend/internal/services"
	"github.com/laravel-paas/backend/internal/services/billing"
	projectServicePkg "github.com/laravel-paas/backend/internal/services/project"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/infrastructure/docker"
	"github.com/laravel-paas/shared/pkg/metrics"
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
		ErrorHandler:            handlers.ErrorHandler,
		AppName:                 "Runara API",
		BodyLimit:               4 * 1024 * 1024,
		ReadTimeout:             15 * time.Second,
		WriteTimeout:            30 * time.Second,
		IdleTimeout:             60 * time.Second,
		ProxyHeader:             fiber.HeaderXForwardedFor,
		EnableTrustedProxyCheck: true,
		EnableIPValidation:      true,
		TrustedProxies:          cfg.TrustedProxyCIDRs,
	})

	isControlPlaneHost := func(host string) bool {
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		host = strings.ToLower(host)

		if host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "paas-backend" {
			return true
		}

		if cfg != nil {
			if cfg.BaseDomain != "" {
				base := strings.ToLower(cfg.BaseDomain)
				if host == base || host == "app."+base || host == "paas."+base || host == "api."+base || host == "admin."+base {
					return true
				}
			}
			if cfg.FrontendURL != "" {
				if u, err := url.Parse(cfg.FrontendURL); err == nil && u.Hostname() != "" {
					if strings.EqualFold(u.Hostname(), host) {
						return true
					}
				}
			}
		}

		return false
	}


	// ===========================================
	// Global Middlewares
	// ===========================================
	app.Use(recover.New())
	app.Use(middleware.RequestSecurity())
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
		AllowHeaders:     "Origin,Content-Type,Accept,Authorization,X-CSRF-Token,Idempotency-Key",
		AllowCredentials: true,
	}))

	// ===========================================
	// Health Check
	// ===========================================
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	// Prometheus Metrics Endpoint
	app.Get("/metrics", middleware.InternalOnly(cfg), metrics.PrometheusHandler())

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(authService, cfg, userService, db)
	userHandler := handlers.NewUserHandler(userService)
	projectHandler := projectHandlerPkg.NewProjectHandler(cfg, db, redisService, projectService, userService, dockerService, secretStoreService)
	settingHandler := handlers.NewSettingHandler(settingService, cfg)
	systemHandler := handlers.NewSystemHandler(userService, dockerService)
	feedbackHandler := handlers.NewFeedbackHandler(feedbackService)
	databaseHandler := handlers.NewDatabaseHandler(db, cfg, databaseService, projectService, redisService, secretStoreService)
	githubService := infrastructure.NewGithubService(cfg, redisService)
	githubAppHandler := handlers.NewGithubAppHandler(db, cfg, githubService, redisService, projectService)
	secretStoreHandler := handlers.NewSecretStoreHandler(db, cfg, secretStoreService)
	walletService := billing.NewWalletService(db)
	billingProfileService := billing.NewBillingProfileService(db)
	topupService := billing.NewTopupService(db, walletService, cfg, billing.NewMidtransClient(cfg), billing.NewPakasirClient(cfg))
	topupService.SetSettingService(settingService)
	catalogService := billing.NewCatalogServiceWithWallets(db, walletService, cfg)
	billingHandler := handlers.NewBillingHandlerWithTopups(
		catalogService,
		topupService,
		billing.NewSuspensionService(db, cfg),
	)
	billingHandler.SetBillingProfileService(billingProfileService)

	// API Routes
	api := app.Group("/api")

	// Ensure PaaS Control Plane API routes are only evaluated for Control Plane hosts.
	// If a request hits /api/* on a project subdomain or custom domain, forward it to projectHandler.ProxyToProject!
	api.Use(func(c *fiber.Ctx) error {
		if !isControlPlaneHost(c.Hostname()) {
			return projectHandler.ProxyToProject(c)
		}
		return c.Next()
	})


	// ===========================================
	// Subdomain Proxy for User Projects (protected + rate limited)
	// ===========================================
	proxyGroup := app.Group("/proxy", middleware.ProxyAuth())
	proxyGroup.Use(middleware.RateLimitProxy())
	proxyGroup.Use(middleware.ValidateProxyTarget())
	proxyGroup.All("/*", projectHandler.ProxyToProject)
	proxyGroup.All("", projectHandler.ProxyToProject)


	// -----------------------------
	// Auth Routes (public, rate limited)
	// -----------------------------
	auth := api.Group("/auth", middleware.NoStore(), middleware.MaxBody(8*1024))
	auth.Post("/login", middleware.RateLimitLogin(redisService), authHandler.Login)
	api.Post("/webhooks/github-app", githubAppHandler.Webhook)
	api.Post("/webhooks/midtrans", middleware.NoStore(), middleware.RateLimitMidtransWebhook(redisService), middleware.MaxBody(8*1024), billingHandler.MidtransWebhook)
	api.Post("/webhooks/pakasir", middleware.NoStore(), middleware.RateLimitPakasirWebhook(redisService), middleware.MaxBody(8*1024), billingHandler.PakasirWebhook)

	// -----------------------------
	// System Init (public, rate limited)
	// -----------------------------
	systemInit := api.Group("/system", middleware.RateLimitLogin(redisService))
	systemInit.Get("/init-status", systemHandler.GetInitStatus)
	systemInit.Post("/initialize", systemHandler.InitializeSystem)

	// -----------------------------
	// Internal System Routes (Publicly accessible but meant for internal mesh network)
	// -----------------------------
	internal := api.Group("/internal", middleware.InternalOnly(cfg))
	internal.Get("/traefik/config", domainHandler.GetTraefikConfig)

	// Stream routes accept short-lived stream tokens only on explicit endpoints.
	streamAuth := middleware.JWTStreamAuth(cfg, authService, userService, userService)
	api.Get("/projects/:id/logs/stream", streamAuth, projectHandler.StreamLogs)
	api.Get("/projects/:id/build-logs/stream", streamAuth, projectHandler.StreamBuildLogs)
	api.Get("/projects/:id/deployment-events/stream", streamAuth, projectHandler.StreamDeploymentEvents)
	api.Get("/projects/:id/domains/:domainID/events/stream", streamAuth, domainHandler.StreamEvents)
	api.Get("/projects/:id/domains/events/stream", streamAuth, domainHandler.StreamProjectEvents)

	// -----------------------------
	// Protected Routes
	// -----------------------------
	protected := api.Group("", middleware.JWTAuth(cfg, authService, userService, userService))

	// Auth (protected)
	protected.Post("/auth/logout", middleware.NoStore(), middleware.MaxBody(8*1024), authHandler.Logout)
	protected.Get("/auth/me", middleware.NoStore(), authHandler.Me)
	protected.Put("/auth/profile", middleware.NoStore(), middleware.MaxBody(8*1024), authHandler.UpdateProfile)
	protected.Post("/auth/stream-token", middleware.NoStore(), middleware.MaxBody(8*1024), authHandler.GenerateStreamToken)
	protected.Post("/auth/return-to-admin", middleware.NoStore(), middleware.MaxBody(8*1024), authHandler.ReturnToAdmin)
	protected.Post("/auth/re-auth", middleware.NoStore(), middleware.MaxBody(8*1024), middleware.RateLimitLogin(redisService), authHandler.Reauthenticate)

	// Billing catalog remains available while payment collection stays disabled.
	billingRoutes := protected.Group("/billing", middleware.NoStore())
	billingRoutes.Get("/catalog", billingHandler.ListActiveCatalog)
	billingRoutes.Get("/overview", billingHandler.GetOwnBillingOverview)
	billingRoutes.Get("/status", billingHandler.GetOwnPaymentDueStatus)
	billingRoutes.Get("/profile", billingHandler.GetBillingProfile)
	billingMutations := billingRoutes.Group("", middleware.RequireNoBillingImpersonation())
	billingMutations.Post("/topups", middleware.MaxBody(8*1024), billingHandler.CreateTopup)
	billingMutations.Post("/topups/by-ref/:topupRef/reconcile", middleware.MaxBody(8*1024), billingHandler.ReconcileTopupByRef)
	billingMutations.Post("/topups/:topupID/reconcile", middleware.MaxBody(8*1024), billingHandler.ReconcileTopup)
	billingMutations.Put("/profile", middleware.MaxBody(16*1024), billingHandler.UpdateBillingProfile)
	billingMutations.Put("/resources/auto-renew", middleware.MaxBody(4*1024), middleware.RateLimitAutoRenew(), billingHandler.UpdateAutoRenew)
	billingMutations.Post("/resources/:resourceType/:resourceID/pay", middleware.MaxBody(1024), billingHandler.PayDueResource)

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
	adminBilling := admin.Group("/billing", middleware.NoStore())
	adminBilling.Get("/catalog", billingHandler.ListCatalog)
	adminBilling.Get("/wallets", billingHandler.ListWallets)
	adminBilling.Get("/wallets/:userID", billingHandler.GetWallet)
	adminBilling.Get("/users/:userID/billing-profile", billingHandler.GetUserBillingProfileAdmin)
	adminBilling.Get("/invoices", billingHandler.ListInvoices)
	adminBilling.Get("/topups", billingHandler.ListTopups)
	adminBilling.Get("/suspensions", billingHandler.ListSuspensions)
	superadminBilling := adminBilling.Group("", middleware.RequireSuperAdmin(), middleware.RequireNoBillingImpersonation(), middleware.RequireRecentBillingAuthentication(cfg))
	superadminBilling.Post("/specs", middleware.MaxBody(8*1024), billingHandler.CreateBillableSpec)
	superadminBilling.Post("/topup-packages", middleware.MaxBody(8*1024), billingHandler.CreateTopupPackage)
	superadminBilling.Put("/topup-packages/:id", middleware.MaxBody(8*1024), billingHandler.UpdateTopupPackage)
	superadminBilling.Post("/wallets/:userID/credits", middleware.MaxBody(8*1024), billingHandler.AdjustWalletCredits)
	superadminBilling.Put("/payment-provider", middleware.MaxBody(8*1024), billingHandler.UpdatePaymentProvider)

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

	// System monitoring (Runara style)
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
	secretstores.Post("/:id/items/:itemID/reveal", middleware.RequireNoBillingImpersonation(), middleware.RequireRecentBillingAuthentication(cfg), secretStoreHandler.RevealSecret)
	secretstores.Post("/:id/bindings", secretStoreHandler.Bind)
	secretstores.Delete("/:id/bindings/:bindingID", secretStoreHandler.Unbind)
	secretstores.Post("/:id/export", middleware.RequireNoBillingImpersonation(), middleware.RequireRecentBillingAuthentication(cfg), secretStoreHandler.Export)
	secretstores.Post("/:id/import", secretStoreHandler.Import)

	// SecretStore Item Management (Milestone 2)
	secretstores.Post("/:id/items", secretStoreHandler.CreateItem)
	secretstores.Put("/:id/items/:itemID", secretStoreHandler.UpdateItem)
	secretstores.Delete("/:id/items/:itemID", secretStoreHandler.DeleteItem)
	secretstores.Get("/:id/items/:itemID/history", secretStoreHandler.History)

	// -----------------------------
	// Centralized Database Routes
	// -----------------------------
	databases := protected.Group("/databases")
	databases.Get("/", databaseHandler.ListUserDatabases)
	databases.Post("/", middleware.RequireNoBillingImpersonation(), databaseHandler.CreateDatabase)
	databases.Delete("/:uid", middleware.RequireNoBillingImpersonation(), databaseHandler.DeleteDatabase)
	databases.Post("/:uid/attach", middleware.RequireNoBillingImpersonation(), databaseHandler.AttachDatabase)
	databases.Post("/:uid/detach", middleware.RequireNoBillingImpersonation(), databaseHandler.DetachDatabase)
	databases.Post("/:uid/reset", middleware.RequireNoBillingImpersonation(), databaseHandler.ResetDatabaseInstance)
	databases.Post("/:uid/reinstall", middleware.RequireNoBillingImpersonation(), databaseHandler.ReinstallDatabaseInstance)

	// -----------------------------
	// Project Routes (Users)
	// -----------------------------
	projects := protected.Group("/projects")
	projects.Get("/", projectHandler.ListOwn)
	projects.Post("/", middleware.RequireNoBillingImpersonation(), projectHandler.Create)
	projects.Get("/:id", projectHandler.Get)
	projects.Get("/:id/branches", projectHandler.ListBranches)
	projects.Put("/:id", projectHandler.Update)
	projects.Post("/:id/redeploy", projectHandler.Redeploy)
	projects.Post("/:id/rollback", projectHandler.Rollback)
	projects.Post("/:id/stop", projectHandler.Stop)
	projects.Post("/:id/start", projectHandler.Start)
	projects.Post("/:id/restart", projectHandler.Restart)
	projects.Delete("/:id", middleware.RequireNoBillingImpersonation(), projectHandler.Delete)
	projects.Get("/:id/logs", projectHandler.Logs)
	projects.Get("/:id/build-logs", projectHandler.BuildLogs)
	projects.Get("/:id/deployment-events", projectHandler.GetDeploymentEvents)
	projects.Get("/:id/stats", projectHandler.Stats)
	projects.Post("/:id/console", middleware.RateLimitConsole(), projectHandler.RunConsoleCommand)
	projects.Get("/:id/env", projectHandler.GetEnv)
	projects.Put("/:id/env", projectHandler.UpdateEnv)

	// Domain Management Routes
	domainHandler.RegisterRoutes(projects.Group("/:id/domains"))

	// -----------------------------
	// Database Management Routes
	// -----------------------------
	projectDatabases := projects.Group("/:id/database")
	projectDatabases.Get("/overview", databaseHandler.GetOverview)
	projectDatabases.Get("/schema", databaseHandler.GetSchema)
	projectDatabases.Get("/backups", databaseHandler.ListBackups)
	projectDatabases.Get("/backups/:backup/download", databaseHandler.DownloadBackup)
	projectDatabases.Get("/metrics", databaseHandler.GetMetrics)

	projectDatabaseMutations := projectDatabases.Group("", middleware.RequireNoBillingImpersonation())
	projectDatabaseMutations.Post("/credentials", middleware.RequireRecentBillingAuthentication(cfg), databaseHandler.GetCredentials)
	projectDatabaseMutations.Post("/rotate-credentials", databaseHandler.RotateCredentials)
	projectDatabaseMutations.Post("/status", databaseHandler.UpdateStatus)
	projectDatabaseMutations.Post("/designer", databaseHandler.ExecuteDesignerAction)
	projectDatabaseMutations.Post("/backups", databaseHandler.CreateBackup)
	projectDatabaseMutations.Post("/backups/:backup/restore", databaseHandler.RestoreBackup)
	projectDatabaseMutations.Delete("/backups/:backup", databaseHandler.DeleteBackup)
	projectDatabaseMutations.Post("/transfer", databaseHandler.TransferDatabase)

	// Fallback/Legacy endpoints
	projectDatabases.Get("/tables", databaseHandler.ListTables)
	projectDatabases.Get("/tables/:table", databaseHandler.GetTableStructure)
	projectDatabases.Get("/tables/:table/data", databaseHandler.GetTableData)
	projectDatabases.Get("/export", databaseHandler.ExportDatabase)
	projectDatabaseMutations.Delete("/tables/:table/rows", databaseHandler.DeleteTableRow)
	projectDatabaseMutations.Put("/tables/:table/rows", databaseHandler.UpdateTableRow)
	projectDatabaseMutations.Post("/query", middleware.RateLimitQuery(), databaseHandler.ExecuteQuery)
	projectDatabaseMutations.Post("/import", middleware.RateLimitImport(), databaseHandler.ImportDatabase)
	projectDatabaseMutations.Post("/reset", databaseHandler.ResetDatabase)

	// ===========================================
	// Subdomain & Custom Domain Project Ingress Proxy Fallback
	// Proxy requests to project containers. Only bypass /api to system handlers if host is Control Plane host.
	// ===========================================
	app.Use(middleware.ProxyAuth(), middleware.RateLimitProxy(), middleware.ValidateProxyTarget(), func(c *fiber.Ctx) error {
		host := c.Hostname()
		path := c.Path()

		// Bypass to system API handlers only if the request is for the Control Plane host itself
		if isControlPlaneHost(host) {
			if strings.HasPrefix(path, "/api") || path == "/health" || path == "/metrics" {
				return c.Next()
			}
		}

		// User project subdomains or custom domains proxy ALL requests (including /api/*) to project containers
		return projectHandler.ProxyToProject(c)
	})



	return app
}

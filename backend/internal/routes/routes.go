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
	"github.com/laravel-paas/backend/internal/config"
	"github.com/laravel-paas/backend/internal/handlers"
	"github.com/laravel-paas/backend/internal/middleware"
	"github.com/laravel-paas/backend/internal/services"
	"gorm.io/gorm"
)

// Setup initializes the Fiber app with all routes
func Setup(db *gorm.DB, cfg *config.Config, redisService *services.RedisService) *fiber.App {
	app := fiber.New(fiber.Config{
		ErrorHandler: handlers.ErrorHandler,
		AppName:      "Laravel PaaS API",
	})

	// ===========================================
	// Global Middlewares
	// ===========================================
	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders: "Origin,Content-Type,Accept,Authorization",
	}))

	// ===========================================
	// Health Check
	// ===========================================
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	// ===========================================
	// API Routes
	// ===========================================
	api := app.Group("/api")

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(db, cfg)
	userHandler := handlers.NewUserHandler(db)
	projectHandler := handlers.NewProjectHandler(db, cfg, redisService)
	settingHandler := handlers.NewSettingHandler(db)
	dockerService := services.NewDockerService(cfg)
	systemHandler := handlers.NewSystemHandler(dockerService)

	// ===========================================
	// Subdomain Proxy for Student Projects
	// ===========================================
	app.All("/proxy/*", projectHandler.ProxyToProject)

	// -----------------------------
	// Auth Routes (public)
	// -----------------------------
	auth := api.Group("/auth")
	auth.Post("/login", authHandler.Login)

	// -----------------------------
	// Protected Routes
	// -----------------------------
	protected := api.Group("", middleware.JWTAuth(cfg.JWTSecret))
	
	// Auth (protected)
	protected.Post("/auth/logout", authHandler.Logout)
	protected.Get("/auth/me", authHandler.Me)

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

	// Settings
	admin.Get("/settings", settingHandler.List)
	admin.Put("/settings", settingHandler.Update)

	// Admin project overview
	admin.Get("/projects", projectHandler.ListAll)
	admin.Get("/stats", projectHandler.AdminStats)
	
	// Queue statistics (admin only)
	admin.Get("/queue/stats", projectHandler.GetQueueStats)
	admin.Get("/projects/stats", projectHandler.GetProjectsStats)

	// System monitoring (PaaS style)
	admin.Get("/system/stats", systemHandler.GetStats)
	admin.Post("/system/prune", systemHandler.PruneSystem)

	// -----------------------------
	// Project Routes (Students)
	// -----------------------------
	projects := protected.Group("/projects")
	projects.Get("/", projectHandler.ListOwn)
	projects.Post("/", projectHandler.Create)
	projects.Get("/:id", projectHandler.Get)
	projects.Put("/:id", projectHandler.Update)
	projects.Post("/:id/redeploy", projectHandler.Redeploy)
	projects.Delete("/:id", projectHandler.Delete)
	projects.Get("/:id/logs", projectHandler.Logs)
	projects.Get("/:id/stats", projectHandler.Stats)
	projects.Post("/:id/artisan", projectHandler.RunArtisan)
	projects.Get("/:id/env", projectHandler.GetEnv)
	projects.Put("/:id/env", projectHandler.UpdateEnv)

	// -----------------------------
	// Database Management Routes
	// -----------------------------
	databaseHandler := handlers.NewDatabaseHandler(db, cfg)
	projects.Get("/:id/database/credentials", databaseHandler.GetCredentials)
	projects.Get("/:id/database/tables", databaseHandler.ListTables)
	projects.Get("/:id/database/tables/:table", databaseHandler.GetTableStructure)
	projects.Get("/:id/database/tables/:table/data", databaseHandler.GetTableData)
	projects.Post("/:id/database/query", databaseHandler.ExecuteQuery)
	projects.Get("/:id/database/export", databaseHandler.ExportDatabase)
	projects.Post("/:id/database/import", databaseHandler.ImportDatabase)
	projects.Post("/:id/database/reset", databaseHandler.ResetDatabase)

	return app
}

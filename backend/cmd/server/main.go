// ===========================================
// Laravel PaaS Backend - Main Entry Point
// ===========================================
package main

import (
	"log/slog"
	"os"

	"github.com/joho/godotenv"
	"github.com/laravel-paas/backend/internal/config"
	"github.com/laravel-paas/backend/internal/database"
	"github.com/laravel-paas/backend/internal/logger"
	"github.com/laravel-paas/backend/internal/routes"
	"github.com/laravel-paas/backend/internal/services"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		slog.Warn("No .env file found, using system environment variables")
	}

	// Initialize configuration
	cfg := config.Load()

	// Initialize structured logger
	logger.Setup(cfg)

	// Initialize database connection
	db, err := database.Connect(cfg)
	if err != nil {
		slog.Error("Failed to connect to database", "error", err)
		os.Exit(1)
	}

	// Run migrations
	if err := database.Migrate(db); err != nil {
		slog.Error("Failed to run migrations", "error", err)
		os.Exit(1)
	}

	// Seed default data (superadmin, settings)
	if err := database.Seed(db, cfg); err != nil {
		slog.Error("Failed to seed database", "error", err)
		os.Exit(1)
	}

	// Initialize Redis service
	slog.Info("Connecting to Redis...")
	redisService, err := services.NewRedisService(cfg)
	if err != nil {
		slog.Error("Failed to connect to Redis", "error", err)
		os.Exit(1)
	}
	defer redisService.Close()
	slog.Info("Redis connected successfully")

	// Initialize and start deployment worker
	worker := services.NewDeploymentWorker(db, cfg, redisService)
	worker.Start()
	defer worker.Stop()

	// Initialize and start server
	app := routes.Setup(db, cfg, redisService)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	slog.Info("Server starting", "port", port)
	if err := app.Listen(":" + port); err != nil {
		slog.Error("Failed to start server", "error", err)
		os.Exit(1)
	}
}

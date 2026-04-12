// ===========================================
// Laravel PaaS Backend - Main Entry Point
// ===========================================
package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/laravel-paas/backend/internal/config"
	"github.com/laravel-paas/backend/internal/database"
	"github.com/laravel-paas/backend/internal/logger"
	"github.com/laravel-paas/backend/internal/repositories"
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

	// Initialize Infrastructure Services
	storageService := services.NewStorageService(cfg)
	dockerService := services.NewDockerService(cfg, storageService)
	gitService := services.NewGitService(cfg)
	versionService := services.NewVersionService()
	mysqlService := services.NewMySQLService()
	
	// Initialize Repositories
	userRepo := repositories.NewUserRepository(db)
	projectRepo := repositories.NewProjectRepository(db)
	settingRepo := repositories.NewSettingRepository(db)
	feedbackRepo := repositories.NewFeedbackRepository(db)

	// Initialize Core Services
	settingService := services.NewSettingService(settingRepo, redisService)
	feedbackService := services.NewFeedbackService(feedbackRepo)
	projectService := services.NewProjectService(cfg, projectRepo, settingService, dockerService, storageService, mysqlService, redisService)
	userService := services.NewUserService(userRepo, projectService)
	authService := services.NewAuthService(userRepo, cfg, redisService)
	databaseService := services.NewDatabaseService(db, cfg)

	// Initialize and start deployment worker
	worker := services.NewDeploymentWorker(cfg, projectRepo, settingService, redisService, dockerService, gitService, versionService, mysqlService, projectService)
	worker.Start()

	// Initialize server
	app := routes.Setup(db, cfg, redisService, dockerService, storageService, projectService, userService, settingService, authService, databaseService, feedbackService)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Create channel for idle connections.
	idleConnsClosed := make(chan struct{})

	go func() {
		// Listen for termination signals
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)
		<-sigint

		slog.Warn("Graceful shutdown initiated...")

		// 1. Stop the Worker (Finish current deployment)
		slog.Info("Shutting down worker...")
		worker.Stop()

		// 2. Stop accepting new requests (Fiber)
		// We give it a 10s timeout to finish current requests
		if err := app.ShutdownWithTimeout(10 * time.Second); err != nil {
			slog.Error("Fiber shutdown error", "error", err)
		}

		slog.Info("All systems stopped. Goodbye!")
		close(idleConnsClosed)
	}()

	slog.Info("Server starting", "port", port)
	if err := app.Listen(":" + port); err != nil {
		slog.Info("Server stopped", "info", err)
	}

	<-idleConnsClosed
}

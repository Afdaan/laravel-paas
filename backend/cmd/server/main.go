// ===========================================
// Laravel PaaS Backend - Main Entry Point
// ===========================================
package main

import (
	"context"
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
	"github.com/laravel-paas/backend/internal/services/setting"
	projectServicePkg "github.com/laravel-paas/backend/internal/services/project"
	"github.com/laravel-paas/backend/internal/workers"
	"github.com/laravel-paas/backend/internal/infrastructure"
	"github.com/laravel-paas/backend/internal/infrastructure/docker"
	domainServicePkg "github.com/laravel-paas/backend/internal/services/domain"
	domainHandlerPkg "github.com/laravel-paas/backend/internal/handlers/domain"
	"github.com/laravel-paas/backend/internal/services/worker"
	"github.com/laravel-paas/backend/internal/services/deployment"
)

func main() {
	// Load environment variables (Local .env takes precedence over Root .env)
	// godotenv.Load does not overwrite existing environment variables
	_ = godotenv.Load()        // Try local backend/.env
	_ = godotenv.Load("../.env") // Try project root .env

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
	redisService, err := infrastructure.NewRedisService(cfg)
	if err != nil {
		slog.Error("Failed to connect to Redis", "error", err)
		os.Exit(1)
	}
	defer redisService.Close()
	slog.Info("Redis connected successfully")

	// Initialize Infrastructure Services
	storageService := infrastructure.NewStorageService(cfg)
	dockerService := docker.NewDockerService(cfg, storageService)
	mysqlService := infrastructure.NewMySQLService()
	
	// Initialize Repositories
	userRepo := repositories.NewUserRepository(db)
	projectRepo := repositories.NewProjectRepository(db)
	settingRepo := repositories.NewSettingRepository(db)
	feedbackRepo := repositories.NewFeedbackRepository(db)

	// Initialize Core Services
	settingService := setting.NewSettingService(settingRepo, redisService)
	feedbackService := services.NewFeedbackService(feedbackRepo)
	transitionManager := deployment.NewTransitionManager(db, redisService)
	projectService := projectServicePkg.NewProjectService(cfg, projectRepo, settingService, dockerService, storageService, mysqlService, redisService, transitionManager)
	userService := services.NewUserService(userRepo, projectService)
	authService := services.NewAuthService(userRepo, cfg, redisService)
	databaseService := services.NewDatabaseService(db, cfg)
	domainService := domainServicePkg.NewDomainService(cfg, db, redisService, projectService, projectRepo)
	domainHandler := domainHandlerPkg.NewDomainHandler(domainService, projectService)

	// Initialize and start WorkerManager and CentralWatchdog
	workerManager := worker.NewWorkerManager(cfg, dockerService, redisService, settingService)
	workerManager.StartWatchdog()

	watchdog := worker.NewCentralWatchdog(projectRepo, redisService, dockerService, projectService)
	watchdog.Start()

	// Initialize and start domain verification worker (MXToolbox-style background check)
	domainWorker := workers.NewDomainWorker(db, domainService, projectService)
	domainCtx, cancelDomainWorker := context.WithCancel(context.Background())
	go domainWorker.Start(domainCtx)

	// Initialize server
	app := routes.Setup(db, cfg, redisService, dockerService, storageService, projectService, userService, settingService, authService, databaseService, feedbackService, domainHandler)

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

		// 0. Stop the Domain Worker
		cancelDomainWorker()

		// 1. Stop WorkerManager and CentralWatchdog
		slog.Info("Shutting down worker manager and central watchdog...")
		workerManager.Stop()
		watchdog.Stop()

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

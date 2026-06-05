// ===========================================
// Laravel PaaS Standalone Worker Daemon
// ===========================================
// Entry point for containerized worker cluster nodes.
// Processes deployment queue, builds containers,
// and drains gracefully on SIGINT/SIGTERM.
// ===========================================
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/database"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/worker/internal/infrastructure/docker"
	"github.com/laravel-paas/shared/logger"
	"github.com/laravel-paas/shared/repositories"
	"github.com/laravel-paas/shared/services/deployment"
	domainPkg "github.com/laravel-paas/shared/services/domain"
	projectServicePkg "github.com/laravel-paas/worker/internal/services/project"
	"github.com/laravel-paas/shared/services/setting"
	workerPkg "github.com/laravel-paas/worker/internal/services/worker"
	"github.com/laravel-paas/worker/internal/workers"
)

func main() {
	_ = godotenv.Load()
	_ = godotenv.Load("../.env")

	cfg := config.Load()
	logger.Setup(cfg)

	slot := os.Getenv("SLOT")
	version := os.Getenv("VERSION")
	slog.Info("Worker daemon starting up", "slot", slot, "version", version)

	db, err := database.Connect(cfg)
	if err != nil {
		slog.Error("Worker failed to connect to database", "error", err)
		os.Exit(1)
	}

	redisService, err := infrastructure.NewRedisService(cfg)
	if err != nil {
		slog.Error("Worker failed to connect to Redis", "error", err)
		os.Exit(1)
	}
	defer redisService.Close()

	storageService := infrastructure.NewStorageService(cfg)
	dockerService := docker.NewDockerService(cfg, storageService, db)
	gitService := infrastructure.NewGitService(cfg)
	versionService := infrastructure.NewVersionService()
	mysqlService := infrastructure.NewMySQLService()

	projectRepo := repositories.NewProjectRepository(db)
	settingRepo := repositories.NewSettingRepository(db)
	settingService := setting.NewSettingService(settingRepo, redisService)

	transitionManager := deployment.NewTransitionManager(db, redisService)
	projectService := projectServicePkg.NewProjectService(cfg, projectRepo, settingService, dockerService, storageService, mysqlService, redisService, transitionManager)

	if slot != "" {
		worker := workers.NewDeploymentWorker(cfg, projectRepo, settingService, redisService, dockerService, gitService, versionService, mysqlService, projectService)
		worker.Start()

		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)
		<-sigint

		slog.Info("Worker received shutdown signal, initiating drain mode...")
		worker.Stop()
		slog.Info("Worker drain complete. Exiting.")
	} else {
		slog.Info("Running in MANAGER mode. Starting watchdog, domain worker, and container manager...")

		managerCtx, cancelManager := context.WithCancel(context.Background())
		defer cancelManager()

		domainService := domainPkg.NewDomainService(cfg, db, redisService, projectService, projectRepo)
		domainWorker := workers.NewDomainWorker(db, domainService, projectService, redisService)
		go domainWorker.Start(managerCtx)

		githubService := infrastructure.NewGithubService(cfg, redisService)
		watchdog := workerPkg.NewCentralWatchdog(cfg, projectRepo, redisService, dockerService, projectService, settingService, githubService)
		watchdog.Start()

		workerManager := workerPkg.NewWorkerManager(cfg, dockerService, redisService, settingService)
		workerManager.StartWatchdog()

		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)
		<-sigint

		slog.Info("Manager received shutdown signal, stopping loops...")
		workerManager.Stop()
		watchdog.Stop()
		cancelManager()
		domainService.Shutdown()
		slog.Info("Manager shutdown complete. Exiting.")
	}
}

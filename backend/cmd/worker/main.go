// ===========================================
// Laravel PaaS Standalone Worker Daemon
// ===========================================
// Entry point for containerized worker cluster nodes.
// Processes deployment queue, builds containers,
// and drains gracefully on SIGINT/SIGTERM.
// ===========================================
package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/laravel-paas/backend/internal/config"
	"github.com/laravel-paas/backend/internal/database"
	"github.com/laravel-paas/backend/internal/infrastructure"
	"github.com/laravel-paas/backend/internal/infrastructure/docker"
	"github.com/laravel-paas/backend/internal/logger"
	"github.com/laravel-paas/backend/internal/repositories"
	projectServicePkg "github.com/laravel-paas/backend/internal/services/project"
	"github.com/laravel-paas/backend/internal/services/setting"
	"github.com/laravel-paas/backend/internal/workers"
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
	dockerService := docker.NewDockerService(cfg, storageService)
	gitService := infrastructure.NewGitService(cfg)
	versionService := infrastructure.NewVersionService()
	mysqlService := infrastructure.NewMySQLService()

	projectRepo := repositories.NewProjectRepository(db)
	settingRepo := repositories.NewSettingRepository(db)
	settingService := setting.NewSettingService(settingRepo, redisService)

	projectService := projectServicePkg.NewProjectService(cfg, projectRepo, settingService, dockerService, storageService, mysqlService, redisService)

	worker := workers.NewDeploymentWorker(cfg, projectRepo, settingService, redisService, dockerService, gitService, versionService, mysqlService, projectService)
	worker.Start()

	sigint := make(chan os.Signal, 1)
	signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)
	<-sigint

	slog.Info("Worker received shutdown signal, initiating drain mode...")
	worker.Stop()
	slog.Info("Worker drain complete. Exiting.")
}

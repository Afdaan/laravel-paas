// ===========================================
// Laravel PaaS Backend - Main Entry Point
// ===========================================
package main

import (
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/laravel-paas/backend/internal/config"
	"github.com/laravel-paas/backend/internal/database"
	"github.com/laravel-paas/backend/internal/routes"
	"github.com/laravel-paas/backend/internal/services"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using system environment variables")
	}

	// Initialize configuration
	cfg := config.Load()

	// Initialize database connection
	db, err := database.Connect(cfg)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// Run migrations
	if err := database.Migrate(db); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Seed default data (superadmin, settings)
	if err := database.Seed(db, cfg); err != nil {
		log.Fatalf("Failed to seed database: %v", err)
	}

	// Initialize Redis service
	log.Println("🔌 Connecting to Redis...")
	redisService, err := services.NewRedisService(cfg)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer redisService.Close()
	log.Println("✅ Redis connected successfully")

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

	log.Printf("🚀 Server starting on port %s", port)
	if err := app.Listen(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

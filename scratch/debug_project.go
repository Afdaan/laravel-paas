package main

import (
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/database"
	"github.com/laravel-paas/shared/models"
)

func main() {
	_ = godotenv.Load("../.env")
	_ = godotenv.Load(".env")
	_ = godotenv.Load()

	cfg := config.Load()
	cfg.PGHost = "127.0.0.1" // Override for host connectivity
	db, err := database.Connect(cfg)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	fmt.Println("Database connection successful.")

	var project models.Project
	subdomain := "afdaan-portofolio-3y7h4y"
	err = db.Where("subdomain = ?", subdomain).First(&project).Error
	if err != nil {
		log.Fatalf("Failed to retrieve project for subdomain %s: %v", subdomain, err)
	}

	fmt.Printf("\n=== Project Details ===\n")
	fmt.Printf("ID:                     %d\n", project.ID)
	fmt.Printf("Name:                   %s\n", project.Name)
	fmt.Printf("GithubURL:              %s\n", project.GithubURL)
	fmt.Printf("Branch:                 %s\n", project.Branch)
	fmt.Printf("UserID:                 %d\n", project.UserID)
	if project.GithubInstallationID != nil {
		fmt.Printf("GithubInstallationID:   %d\n", *project.GithubInstallationID)
	} else {
		fmt.Printf("GithubInstallationID:   <nil>\n")
	}

	var installations []models.GithubAppInstallation
	err = db.Where("user_id = ?", project.UserID).Find(&installations).Error
	if err != nil {
		log.Printf("Failed to retrieve installations for user ID %d: %v", project.UserID, err)
	} else {
		fmt.Printf("\n=== User installations ===\n")
		fmt.Printf("Found %d installations for User ID %d:\n", len(installations), project.UserID)
		for _, inst := range installations {
			fmt.Printf("- ID: %d | AccountName: %s | InstallationID: %d\n", inst.ID, inst.AccountName, inst.InstallationID)
		}
	}

	os.Exit(0)
}

package project

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/repositories"
	"gorm.io/gorm"
)

func TestDeleteProjectSuspendsBillingBeforeDispatch(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Project{}, &models.ProjectDeletionTask{}, &models.BillableSpec{}, &models.BillableResource{}); err != nil {
		t.Fatal(err)
	}

	user := models.User{Email: "delete-billing@example.test", Password: "test", Name: "Delete billing"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	project := models.Project{UserID: user.ID, Name: "Delete billing", GithubURL: "https://github.com/example/delete-billing", Subdomain: "delete-billing", Status: models.StatusRunning}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	spec := models.BillableSpec{Type: models.BillableTypeProject, Name: "Delete billing", Slug: "delete-billing", CPUMillicores: 500, MemoryMB: 512, StorageGB: 10, MonthlyCredits: 100, Version: 1, IsActive: true}
	if err := db.Create(&spec).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	resource := models.BillableResource{UserID: user.ID, Type: models.BillableTypeProject, ResourceID: project.ID, SpecID: spec.ID, BillingStatus: models.BillableResourceStatusActive, CurrentPeriodStart: now, NextInvoiceAt: now.AddDate(0, 1, 0), BillingAnchorDay: now.Day()}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}

	service := &ProjectService{projectRepo: repositories.NewProjectRepository(db)}
	if err := service.DeleteProject(&project); err != nil {
		t.Fatal(err)
	}

	if err := db.First(&project, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	if project.Status != models.StatusDeleting {
		t.Fatalf("project status=%s", project.Status)
	}
	if err := db.First(&resource, resource.ID).Error; err != nil {
		t.Fatal(err)
	}
	if resource.BillingStatus != models.BillableResourceStatusSuspended {
		t.Fatalf("billing status=%s", resource.BillingStatus)
	}
	var task models.ProjectDeletionTask
	if err := db.Where("project_id = ?", project.ID).First(&task).Error; err != nil {
		t.Fatal(err)
	}
}

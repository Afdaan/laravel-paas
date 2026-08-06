package worker

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/repositories"
	"gorm.io/gorm"
)

func TestBillingAllowsAutoHealingOnlyForActiveResource(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.BillableResource{}); err != nil {
		t.Fatal(err)
	}
	resource := models.BillableResource{UserID: 1, Type: models.BillableTypeProject, ResourceID: 1, SpecID: 1, BillingStatus: models.BillableResourceStatusSuspended}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}
	watchdog := &CentralWatchdog{cfg: &config.Config{BillingEnabled: true}, projectRepo: repositories.NewProjectRepository(db)}
	if watchdog.billingAllowsAutoHealing(resource.ResourceID) {
		t.Fatal("billing-suspended project allowed watchdog restart")
	}
	if err := db.Model(&resource).Update("billing_status", models.BillableResourceStatusActive).Error; err != nil {
		t.Fatal(err)
	}
	if !watchdog.billingAllowsAutoHealing(resource.ResourceID) {
		t.Fatal("active billing resource blocked watchdog restart")
	}
}

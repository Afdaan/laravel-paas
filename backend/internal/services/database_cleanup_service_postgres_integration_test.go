package services

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestDatabaseCleanupServicePostgresClaimsTaskOnce(t *testing.T) {
	db := databaseCleanupPostgresTestDB(t)
	task := models.DatabaseCleanupTask{
		Engine:        "postgresql",
		Name:          "cleanup_claim_" + uuid.NewString()[:12],
		Username:      "cleanup_user_" + uuid.NewString()[:12],
		DatabaseOwned: true,
		UserOwned:     true,
		LastError:     "initial",
		RetryCount:    1,
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Delete(&models.DatabaseCleanupTask{}, task.ID).Error }()

	var calls atomic.Int32
	cleanup := func(_ string, _ string, _ string, _ infrastructure.ProvisioningOwnership) (infrastructure.ProvisioningOwnership, error) {
		calls.Add(1)
		time.Sleep(100 * time.Millisecond)
		return infrastructure.ProvisioningOwnership{}, nil
	}
	first := newDatabaseCleanupService(db, cleanup)
	second := newDatabaseCleanupService(db, cleanup)

	start := make(chan struct{})
	errs := make(chan error, 2)
	var workers sync.WaitGroup
	for _, service := range []*DatabaseCleanupService{first, second} {
		workers.Add(1)
		go func(service *DatabaseCleanupService) {
			defer workers.Done()
			<-start
			errs <- service.processPending(context.Background())
		}(service)
	}
	close(start)
	workers.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	// Cleanup checkpoints database and user deletion separately. One claimed task
	// therefore executes exactly two ownership-specific cleanup operations.
	if calls.Load() != 2 {
		t.Fatalf("cleanup calls=%d, want 2", calls.Load())
	}
	var count int64
	if err := db.Model(&models.DatabaseCleanupTask{}).Where("id = ?", task.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("task count=%d, want 0", count)
	}
}

func databaseCleanupPostgresTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("BILLING_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("BILLING_TEST_DATABASE_URL not set")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Path != "/runara_billing_test" {
		t.Fatalf("BILLING_TEST_DATABASE_URL must target PostgreSQL database runara_billing_test")
	}
	query := parsed.Query()
	if query.Has("dbname") || query.Has("database") {
		t.Fatal("BILLING_TEST_DATABASE_URL must not override database")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.DatabaseCleanupTask{}); err != nil {
		t.Fatal(fmt.Errorf("migrate database cleanup task: %w", err))
	}
	return db
}

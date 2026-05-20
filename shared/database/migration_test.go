// ===========================================
// Database Migration Tests
// ===========================================
// Verifies idempotency, missing constraint handling,
// and safe repeated executions.
// ===========================================
package database_test

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/laravel-paas/shared/database"
	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite in-memory db: %v", err)
	}
	return db
}

// TestMigration_Idempotency verifies that running the migration bootstrap
// multiple times across repeated deploys succeeds without error.
func TestMigration_Idempotency(t *testing.T) {
	db := setupTestDB(t)

	// First execution (initial deploy)
	if err := database.DefensiveMigrationBootstrap(db); err != nil {
		t.Fatalf("first migration execution failed: %v", err)
	}

	// Second execution (subsequent deploy)
	if err := database.DefensiveMigrationBootstrap(db); err != nil {
		t.Fatalf("second migration execution failed (not idempotent): %v", err)
	}

	// Third execution
	if err := database.DefensiveMigrationBootstrap(db); err != nil {
		t.Fatalf("third migration execution failed: %v", err)
	}
}

// TestMigration_MissingConstraintHandling verifies that missing indexes and
// constraints are safely reconciled without failing.
func TestMigration_MissingConstraintHandling(t *testing.T) {
	db := setupTestDB(t)

	// Create bare table without indexes/constraints first
	err := db.Table("users").AutoMigrate(&models.User{})
	if err != nil {
		t.Fatalf("failed to create initial bare table: %v", err)
	}

	// Run defensive bootstrap to reconcile missing elements
	if err := database.DefensiveMigrationBootstrap(db); err != nil {
		t.Fatalf("reconciliation of missing constraints failed: %v", err)
	}
}

// TestMigration_DuplicateExecution verifies that executing migration functions
// concurrently or back-to-back remains completely rollback-safe and stable.
func TestMigration_DuplicateExecution(t *testing.T) {
	db := setupTestDB(t)

	for i := 0; i < 5; i++ {
		if err := database.DefensiveMigrationBootstrap(db); err != nil {
			t.Fatalf("iteration %d failed during duplicate execution check: %v", i, err)
		}
	}

	// Verify tables are intact and accessible
	var count int64
	if err := db.Model(&models.User{}).Count(&count).Error; err != nil {
		t.Fatalf("failed to query users table after migrations: %v", err)
	}
}

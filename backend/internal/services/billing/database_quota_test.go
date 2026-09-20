package billing

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
)

func TestLoadActiveDatabaseQuotaTx(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.BillableSpec{}); err != nil {
		t.Fatal(err)
	}
	connectionLimit := 42
	spec := models.BillableSpec{Type: models.BillableTypeDatabase, Name: "Database", Slug: "database", CPUMillicores: 500, MemoryMB: 512, StorageGB: 25, MonthlyCredits: 100, ConnectionLimit: &connectionLimit, BackupRetentionDays: intPointer(7), Version: 1, IsActive: true}
	if err := db.Create(&spec).Error; err != nil {
		t.Fatal(err)
	}

	quota, err := LoadActiveDatabaseQuotaTx(db, spec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if quota.ConnectionLimit != connectionLimit {
		t.Fatalf("quota=%#v", quota)
	}
}

func TestLoadActiveDatabaseQuotaTxRejectsInactiveOrIncompleteSpec(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.BillableSpec{}); err != nil {
		t.Fatal(err)
	}
	inactiveConnectionLimit := 10
	inactive := models.BillableSpec{Type: models.BillableTypeDatabase, Name: "Inactive", Slug: "inactive", CPUMillicores: 500, MemoryMB: 512, StorageGB: 10, MonthlyCredits: 100, ConnectionLimit: &inactiveConnectionLimit, Version: 1, IsActive: false}
	if err := db.Create(&inactive).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&inactive).Update("is_active", false).Error; err != nil {
		t.Fatal(err)
	}
	incomplete := models.BillableSpec{Type: models.BillableTypeDatabase, Name: "Incomplete", Slug: "incomplete", CPUMillicores: 500, MemoryMB: 512, StorageGB: 10, MonthlyCredits: 100, Version: 1, IsActive: true}
	if err := db.Create(&incomplete).Error; err != nil {
		t.Fatal(err)
	}
	for _, specID := range []uint{inactive.ID, incomplete.ID} {
		if _, err := LoadActiveDatabaseQuotaTx(db, specID); err != ErrInvalidInvoiceInput {
			t.Fatalf("spec=%d error=%v", specID, err)
		}
	}
}

//go:build integration

package billing

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/laravel-paas/shared/models"
)

func TestCatalogServicePostgresSerializesIdentityWrites(t *testing.T) {
	db := walletPostgresTestDB(t)
	service := NewCatalogService(db)
	ctx := context.Background()
	unique := time.Now().UTC().UnixNano()
	slug := fmt.Sprintf("concurrent-%d", unique)
	credits := int64(900000000 + unique%99999999)

	for _, err := range runConcurrentCalls(t, []func() error{
		func() error {
			_, err := service.CreateBillableSpec(ctx, catalogAudit(fmt.Sprintf("spec-%d-a", unique), "Concurrent specification pricing"), BillableSpecInput{
				Type: models.BillableTypeProject, Name: "Concurrent", Slug: slug, CPUMillicores: 500, MemoryMB: 1024, StorageGB: 5, MonthlyCredits: 100000, Reason: "Concurrent specification pricing",
			})
			return err
		},
		func() error {
			_, err := service.CreateBillableSpec(ctx, catalogAudit(fmt.Sprintf("spec-%d-b", unique), "Concurrent specification pricing"), BillableSpecInput{
				Type: models.BillableTypeProject, Name: "Concurrent", Slug: slug, CPUMillicores: 500, MemoryMB: 1024, StorageGB: 5, MonthlyCredits: 125000, Reason: "Concurrent specification pricing",
			})
			return err
		},
	}) {
		if err != nil {
			t.Fatalf("concurrent billable spec write: %v", err)
		}
	}

	for _, err := range runConcurrentCalls(t, []func() error{
		func() error {
			_, err := service.CreateTopupPackage(ctx, catalogAudit(fmt.Sprintf("package-%d-a", unique), "Concurrent package pricing"), TopupPackageInput{
				Credits: credits, AmountMinor: credits, SortOrder: 1, Reason: "Concurrent package pricing",
			})
			return err
		},
		func() error {
			_, err := service.CreateTopupPackage(ctx, catalogAudit(fmt.Sprintf("package-%d-b", unique), "Concurrent package pricing"), TopupPackageInput{
				Credits: credits, AmountMinor: credits + 1, SortOrder: 1, Reason: "Concurrent package pricing",
			})
			return err
		},
	}) {
		if err != nil {
			t.Fatalf("concurrent topup package write: %v", err)
		}
	}

	var specs []models.BillableSpec
	if err := db.Where("type = ? AND slug = ?", models.BillableTypeProject, slug).Order("version ASC").Find(&specs).Error; err != nil {
		t.Fatal(err)
	}
	if len(specs) != 2 || specs[0].Version != 1 || specs[0].IsActive || specs[1].Version != 2 || !specs[1].IsActive {
		t.Fatalf("serialized billable spec versions = %#v", specs)
	}

	var packages []models.TopupPackage
	if err := db.Where("provider = ? AND currency = ? AND credits = ?", models.BillingProviderMidtrans, models.BillingCurrencyIDR, credits).Order("version ASC").Find(&packages).Error; err != nil {
		t.Fatal(err)
	}
	if len(packages) != 2 || packages[0].Version != 1 || packages[0].IsActive || packages[1].Version != 2 || !packages[1].IsActive {
		t.Fatalf("serialized topup package versions = %#v", packages)
	}
}

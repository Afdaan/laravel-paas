package billing

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
)

func billingProfileTestFixture(t *testing.T) (*gorm.DB, models.User, *BillingProfileService) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.BillingProfile{}); err != nil {
		t.Fatal(err)
	}
	user := models.User{Name: "Billing Profile", Email: "profile@example.test", Password: "test"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	return db, user, NewBillingProfileService(db)
}

func TestBillingProfileValidationAndNormalization(t *testing.T) {
	_, user, service := billingProfileTestFixture(t)
	valid := UpdateBillingProfileInput{
		CompanyName: "  PT Runara  ", TaxID: "12.345.678.9-012.345", Email: "  BILLING@EXAMPLE.COM ",
		Phone: " 0812 3456 7890 ", AddressLine1: "  Jalan Runara 123  ", AddressLine2: "  Lantai 2 ",
		City: " Jakarta ", StateProvince: " DKI Jakarta ", PostalCode: " 12345 ", Country: " idn ",
	}
	profile, err := service.UpsertProfile(context.Background(), user.ID, valid)
	if err != nil {
		t.Fatal(err)
	}
	if profile.CompanyName != "PT Runara" || profile.Email != "billing@example.com" || profile.Country != "ID" || profile.PostalCode != "12345" {
		t.Fatalf("normalized profile=%#v", profile)
	}
	if err := service.RequireComplete(context.Background(), user.ID); err != nil {
		t.Fatalf("profile should be ready: %v", err)
	}

	invalidInputs := []UpdateBillingProfileInput{
		{},
		{CompanyName: "A", Email: "invalid", Phone: "123", AddressLine1: "x", City: "J", PostalCode: "1", Country: "ID"},
		{CompanyName: strings.Repeat("x", 256), Email: "billing@example.com", Phone: "081234567890", AddressLine1: "Jalan Runara", City: "Jakarta", PostalCode: "12345", Country: "ID"},
		{CompanyName: "Runara", Email: "billing@example.com", Phone: "081234567890", AddressLine1: "Jalan Runara", City: "Jakarta", PostalCode: "12345", Country: "US"},
		{CompanyName: "Runara", TaxID: "123", Email: "billing@example.com", Phone: "081234567890", AddressLine1: "Jalan Runara", City: "Jakarta", PostalCode: "12345", Country: "ID"},
	}
	for index, input := range invalidInputs {
		if _, err := service.UpsertProfile(context.Background(), user.ID, input); !errors.Is(err, ErrInvalidBillingProfile) && !errors.Is(err, ErrInvalidBillingEmail) {
			t.Fatalf("invalid input %d accepted: %v", index, err)
		}
	}
}

func TestTopupRequiresCompleteBillingProfile(t *testing.T) {
	db, user, service, _ := topupServiceFixture(t)
	if err := db.Where("user_id = ?", user.ID).Delete(&models.BillingProfile{}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(context.Background(), user.ID, "profile-required", TopupInput{PackageID: 1}); !errors.Is(err, ErrBillingProfileRequired) {
		t.Fatalf("expected profile requirement, got %v", err)
	}
}

func TestTopupPropagatesBillingProfileStorageFailure(t *testing.T) {
	db, user, service, gateway := topupServiceFixture(t)
	if err := db.Migrator().DropTable(&models.BillingProfile{}); err != nil {
		t.Fatal(err)
	}
	_, err := service.Create(context.Background(), user.ID, "profile-storage-failure", TopupInput{PackageID: 1})
	if err == nil || errors.Is(err, ErrBillingProfileRequired) || !strings.Contains(err.Error(), "validate billing profile") {
		t.Fatalf("profile storage error=%v", err)
	}
	if gateway.createCalls != 0 {
		t.Fatalf("gateway calls=%d", gateway.createCalls)
	}
}

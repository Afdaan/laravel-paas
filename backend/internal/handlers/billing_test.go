package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/backend/internal/services/billing"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
)

func TestDecodeBillingJSONRejectsDuplicateAndUnknownFields(t *testing.T) {
	app := fiber.New()
	app.Post("/", func(c *fiber.Ctx) error {
		var input struct {
			Credits int64 `json:"credits"`
		}
		if err := decodeBillingJSON(c, &input); err != nil {
			return c.SendStatus(http.StatusBadRequest)
		}
		return c.SendStatus(http.StatusNoContent)
	})
	for _, body := range []string{
		`{"credits":100,"credits":200}`,
		`{"credits":100,"unexpected":true}`,
		`{"credits":100}{"credits":200}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("body %s status = %d", body, resp.StatusCode)
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"credits":100}`))
	req.Header.Set("Content-Type", "text/plain")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("non-JSON content type status = %d", resp.StatusCode)
	}
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{\"credits\":\xff}"))
	req.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid UTF-8 status = %d", resp.StatusCode)
	}
	if err := rejectDuplicateJSONKeys([]byte(`[[[[[[[[[0]]]]]]]]]`)); err == nil {
		t.Fatal("overly nested JSON accepted")
	}
}

func TestBillingResponsesExcludeInternalFields(t *testing.T) {
	response := billing.WalletView{
		BalanceCredits: 100,
		LedgerEntries: []billing.WalletLedgerEntryView{{
			Type:          models.WalletLedgerEntryTopup,
			AmountCredits: 100,
			BalanceAfter:  100,
			CreatedAt:     "2026-07-30T00:00:00Z",
		}},
	}
	body, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"wallet_id", "idempotency_key", "created_by", "reference_id", "reference_type"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("wallet response leaked %q: %s", forbidden, body)
		}
	}
}

func TestUpdatePaymentProviderEndpoint(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Setting{}, &models.BillingAuditEvent{}); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		BillingEnabled:      true,
		BillingTopupEnabled: true,
		PakasirEnabled:      true,
		PakasirProjectSlug:  "slug-1",
		PakasirAPIKey:       "key-1",
	}
	catalogService := billing.NewCatalogService(db, cfg)
	handler := NewBillingHandler(catalogService)

	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
	app.Put("/admin/billing/payment-provider", func(c *fiber.Ctx) error {
		c.Locals("user_id", uint(1))
		c.Locals("role", string(models.RoleSuperAdmin))
		return handler.UpdatePaymentProvider(c)
	})

	body := `{"provider":"pakasir","reason":"Enabling Pakasir as primary gateway"}`
	req := httptest.NewRequest(http.MethodPut, "/admin/billing/payment-provider", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "req-test-provider-switch-1")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", resp.StatusCode)
	}
}

package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

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

type fakePakasirGatewayForHandlerTest struct {
	detailStatus string
}

func (f *fakePakasirGatewayForHandlerTest) CreateTransaction(ctx context.Context, orderID string, amount int64, paymentType string) (billing.PakasirCreateResponse, error) {
	return billing.PakasirCreateResponse{PaymentNumber: "https://app.pakasir.com/pay/runara/" + strconv.FormatInt(amount, 10) + "?order_id=" + orderID}, nil
}

func (f *fakePakasirGatewayForHandlerTest) SimulatePayment(ctx context.Context, orderID string, amount int64) error {
	return nil
}

func (f *fakePakasirGatewayForHandlerTest) CancelTransaction(ctx context.Context, orderID string, amount int64) error {
	return nil
}

func (f *fakePakasirGatewayForHandlerTest) GetTransactionDetail(ctx context.Context, orderID string, amount int64) (billing.PakasirTransactionDetail, error) {
	return billing.PakasirTransactionDetail{
		OrderID: orderID,
		Amount:  amount,
		Status:  f.detailStatus,
	}, nil
}

func TestReconcileTopupByRefEndpoint(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Wallet{}, &models.WalletLedgerEntry{}, &models.TopupPackage{}, &models.Topup{}, &models.PaymentEvent{}); err != nil {
		t.Fatal(err)
	}

	user := models.User{Email: "user-ref-endpoint@example.test", Password: "test", Name: "Owner"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	other := models.User{Email: "other-ref-endpoint@example.test", Password: "test", Name: "Other"}
	if err := db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}

	pkg := models.TopupPackage{Currency: models.BillingCurrencyIDR, Credits: 100, AmountMinor: 10000, Version: 1, IsActive: true}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}

	pakasirGw := &fakePakasirGatewayForHandlerTest{detailStatus: "completed"}
	cfg := &config.Config{
		BillingEnabled:      true,
		BillingTopupEnabled: true,
		PakasirEnabled:      true,
		PakasirProjectSlug:  "test-slug",
		PakasirAPIKey:       "test-key",
	}
	topupSvc := billing.NewTopupService(db, billing.NewWalletService(db), cfg, nil, pakasirGw)
	handler := NewBillingHandlerWithTopups(billing.NewCatalogService(db, cfg), topupSvc)

	view, err := topupSvc.Create(context.Background(), user.ID, "idempotency-key-endpoint", billing.TopupInput{PackageID: pkg.ID})
	if err != nil {
		t.Fatal(err)
	}
	var topup models.Topup
	if err := db.First(&topup, view.ID).Error; err != nil {
		t.Fatal(err)
	}

	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
	app.Post("/billing/topups/by-ref/:topupRef/reconcile", func(c *fiber.Ctx) error {
		actingUserID, _ := strconv.ParseUint(c.Get("X-User-ID"), 10, 64)
		c.Locals("user_id", uint(actingUserID))
		return handler.ReconcileTopupByRef(c)
	})

	// 1. Owner can reconcile by ref -> 200 OK
	req := httptest.NewRequest(http.MethodPost, "/billing/topups/by-ref/"+topup.ProviderOrderID+"/reconcile", nil)
	req.Header.Set("X-User-ID", strconv.FormatUint(uint64(user.ID), 10))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	// 2. Another user cannot reconcile that ref -> 404 Not Found
	reqOther := httptest.NewRequest(http.MethodPost, "/billing/topups/by-ref/"+topup.ProviderOrderID+"/reconcile", nil)
	reqOther.Header.Set("X-User-ID", strconv.FormatUint(uint64(other.ID), 10))
	respOther, err := app.Test(reqOther)
	if err != nil {
		t.Fatal(err)
	}
	if respOther.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found for cross-user reconcile, got %d", respOther.StatusCode)
	}

	// 3. Unknown ref -> 404 Not Found
	reqUnknown := httptest.NewRequest(http.MethodPost, "/billing/topups/by-ref/topup-00000000000000000000000000000000/reconcile", nil)
	reqUnknown.Header.Set("X-User-ID", strconv.FormatUint(uint64(user.ID), 10))
	respUnknown, err := app.Test(reqUnknown)
	if err != nil {
		t.Fatal(err)
	}
	if respUnknown.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found for unknown ref, got %d", respUnknown.StatusCode)
	}

	// 4. Malformed ref -> 400 Bad Request
	reqMalformed := httptest.NewRequest(http.MethodPost, "/billing/topups/by-ref/"+url.PathEscape("topup-123;drop table")+"/reconcile", nil)
	reqMalformed.Header.Set("X-User-ID", strconv.FormatUint(uint64(user.ID), 10))
	respMalformed, err := app.Test(reqMalformed)
	if err != nil {
		t.Fatal(err)
	}
	if respMalformed.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for malformed ref, got %d", respMalformed.StatusCode)
	}
}

func TestListOwnInvoicesEndpoint(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Wallet{}, &models.Invoice{}, &models.InvoiceItem{}); err != nil {
		t.Fatal(err)
	}

	user := models.User{Email: "list-invoices@example.com", Password: "p", Name: "Invoice User"}
	db.Create(&user)
	wallet := models.Wallet{UserID: user.ID}
	db.Create(&wallet)

	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	inv1 := models.Invoice{
		UserID:         user.ID,
		WalletID:       wallet.ID,
		InvoiceNumber:  "INV-202608-0001",
		PeriodStart:    now,
		PeriodEnd:      now.AddDate(0, 1, 0),
		TotalCredits:   100,
		Status:         models.InvoiceStatusPaid,
		IdempotencyKey: "h-inv-1",
	}
	db.Create(&inv1)

	catalogSvc := billing.NewCatalogService(db)
	handler := &BillingHandler{
		catalog: catalogSvc,
	}

	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
	app.Get("/billing/invoices", func(c *fiber.Ctx) error {
		actingUserID, _ := strconv.ParseUint(c.Get("X-User-ID"), 10, 64)
		c.Locals("user_id", uint(actingUserID))
		return handler.ListOwnInvoices(c)
	})

	// 1. Normal list -> 200 OK
	req := httptest.NewRequest(http.MethodGet, "/billing/invoices?search=inv-202608&status=paid", nil)
	req.Header.Set("X-User-ID", strconv.FormatUint(uint64(user.ID), 10))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	// 2. Invalid status -> 400 Bad Request
	reqInvalid := httptest.NewRequest(http.MethodGet, "/billing/invoices?status=MALICIOUS_STATUS", nil)
	reqInvalid.Header.Set("X-User-ID", strconv.FormatUint(uint64(user.ID), 10))
	respInvalid, err := app.Test(reqInvalid)
	if err != nil {
		t.Fatal(err)
	}
	if respInvalid.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for invalid status, got %d", respInvalid.StatusCode)
	}

	// 3. Search query > 100 ASCII chars -> 400 Bad Request
	reqLongSearch := httptest.NewRequest(http.MethodGet, "/billing/invoices?search="+strings.Repeat("A", 101), nil)
	reqLongSearch.Header.Set("X-User-ID", strconv.FormatUint(uint64(user.ID), 10))
	respLongSearch, err := app.Test(reqLongSearch)
	if err != nil {
		t.Fatal(err)
	}
	if respLongSearch.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for 101-char search, got %d", respLongSearch.StatusCode)
	}

	// 4. Search query > 100 multibyte runes -> 400 Bad Request
	reqLongRune := httptest.NewRequest(http.MethodGet, "/billing/invoices?search="+strings.Repeat("日", 101), nil)
	reqLongRune.Header.Set("X-User-ID", strconv.FormatUint(uint64(user.ID), 10))
	respLongRune, err := app.Test(reqLongRune)
	if err != nil {
		t.Fatal(err)
	}
	if respLongRune.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for 101-rune search, got %d", respLongRune.StatusCode)
	}

	// 5. Valid 100-rune multibyte search -> 200 OK
	reqValidRune := httptest.NewRequest(http.MethodGet, "/billing/invoices?search="+strings.Repeat("日", 100), nil)
	reqValidRune.Header.Set("X-User-ID", strconv.FormatUint(uint64(user.ID), 10))
	respValidRune, err := app.Test(reqValidRune)
	if err != nil {
		t.Fatal(err)
	}
	if respValidRune.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for 100-rune search, got %d", respValidRune.StatusCode)
	}
}

package billing

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
)

func TestTopupCreateReplaysOneProviderRequest(t *testing.T) {
	db, user, service, gateway := topupServiceFixture(t)
	first, err := service.Create(context.Background(), user.ID, "topup-key", TopupInput{PackageID: 1})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Create(context.Background(), user.ID, "topup-key", TopupInput{PackageID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if first != second || gateway.createCalls != 1 {
		t.Fatalf("first=%#v second=%#v calls=%d", first, second, gateway.createCalls)
	}
	if _, err := service.Create(context.Background(), user.ID, "topup-key", TopupInput{PackageID: 999}); err != ErrTopupIdempotencyConflict {
		t.Fatalf("conflict=%v", err)
	}
	var count int64
	if err := db.Model(&models.Topup{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("topups=%d err=%v", count, err)
	}
}

func TestTopupWebhookCreditsOnceAndReversesRefund(t *testing.T) {
	db, user, service, gateway := topupServiceFixture(t)
	view, err := service.Create(context.Background(), user.ID, "topup-webhook", TopupInput{PackageID: 1})
	if err != nil {
		t.Fatal(err)
	}
	var topup models.Topup
	if err := db.First(&topup, view.ID).Error; err != nil {
		t.Fatal(err)
	}
	paid := signedNotification(topup.ProviderOrderID, "settlement", "accept", "transaction-1")
	if err := service.ProcessNotification(context.Background(), paid); err != nil {
		t.Fatal(err)
	}
	if err := service.ProcessNotification(context.Background(), paid); err != nil {
		t.Fatal(err)
	}
	assertTopupState(t, db, user.ID, view.ID, models.TopupStatusPaid, 100000, 1)
	refund := signedNotification(topup.ProviderOrderID, "refund", "", "transaction-1")
	gatewaySees(gateway, refund)
	if err := service.ProcessNotification(context.Background(), refund); err != nil {
		t.Fatal(err)
	}
	if err := service.ProcessNotification(context.Background(), paid); err != nil {
		t.Fatal(err)
	}
	assertTopupState(t, db, user.ID, view.ID, models.TopupStatusRefunded, 0, 2)
	invalid := paid
	invalid.SignatureKey = "invalid"
	if err := service.ProcessNotification(context.Background(), invalid); err != ErrInvalidPaymentNotification {
		t.Fatalf("signature=%v", err)
	}
}

func TestTopupRefundMovesActiveResourcesToPaymentDueWhenWalletBecomesNegative(t *testing.T) {
	db, user, service, gateway := topupServiceFixture(t)
	now := time.Now().UTC()
	project := models.Project{UserID: user.ID, Name: "Refund debt", GithubURL: "https://github.com/example/refund-debt", Subdomain: "refund-debt", Status: models.StatusRunning}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	connectionLimit := 10
	spec := models.BillableSpec{Type: models.BillableTypeProject, Name: "Refund debt", Slug: "refund-debt", CPUMillicores: 500, MemoryMB: 512, StorageGB: 10, MonthlyCredits: 100, ConnectionLimit: &connectionLimit, Version: 1, IsActive: true}
	if err := db.Create(&spec).Error; err != nil {
		t.Fatal(err)
	}
	resource := models.BillableResource{UserID: user.ID, Type: models.BillableTypeProject, ResourceID: project.ID, SpecID: spec.ID, BillingStatus: models.BillableResourceStatusActive, CurrentPeriodStart: now, NextInvoiceAt: now.AddDate(0, 1, 0), BillingAnchorDay: now.Day()}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}

	view, err := service.Create(context.Background(), user.ID, "topup-refund-debt", TopupInput{PackageID: 1})
	if err != nil {
		t.Fatal(err)
	}
	var topup models.Topup
	if err := db.First(&topup, view.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.ProcessNotification(context.Background(), signedNotification(topup.ProviderOrderID, "settlement", "accept", "refund-debt-paid")); err != nil {
		t.Fatal(err)
	}
	if _, err := service.wallets.Debit(context.Background(), LedgerMutation{UserID: user.ID, EntryType: models.WalletLedgerEntryInvoiceDebit, AmountCredits: topup.Credits, IdempotencyKey: "invoice:refund-debt:debit", ReferenceType: "invoice", ReferenceID: "refund-debt"}); err != nil {
		t.Fatal(err)
	}
	refund := signedNotification(topup.ProviderOrderID, "refund", "", "refund-debt-paid")
	gatewaySees(gateway, refund)
	if err := service.ProcessNotification(context.Background(), refund); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&resource, resource.ID).Error; err != nil {
		t.Fatal(err)
	}
	if resource.BillingStatus != models.BillableResourceStatusPaymentDue {
		t.Fatalf("billing status=%s", resource.BillingStatus)
	}
	var paymentDueInvoice models.Invoice
	if err := db.Where("idempotency_key = ?", fmt.Sprintf("topup:%d:payment-due", topup.ID)).First(&paymentDueInvoice).Error; err != nil {
		t.Fatal(err)
	}
	if paymentDueInvoice.Status != models.InvoiceStatusPaymentDue || paymentDueInvoice.DueAt == nil || paymentDueInvoice.TotalCredits != 0 {
		t.Fatalf("payment-due invoice=%#v", paymentDueInvoice)
	}
	var paymentDueItem models.InvoiceItem
	if err := db.Where("invoice_id = ? AND billable_resource_id = ?", paymentDueInvoice.ID, resource.ID).First(&paymentDueItem).Error; err != nil {
		t.Fatal(err)
	}
	if paymentDueItem.Credits != 0 || paymentDueItem.SpecID != resource.SpecID {
		t.Fatalf("payment-due invoice item=%#v", paymentDueItem)
	}
	recovery, err := service.Create(context.Background(), user.ID, "topup-refund-debt-recovery", TopupInput{PackageID: 1})
	if err != nil {
		t.Fatal(err)
	}
	var recoveryTopup models.Topup
	if err := db.First(&recoveryTopup, recovery.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.ProcessNotification(context.Background(), signedNotification(recoveryTopup.ProviderOrderID, "settlement", "accept", "refund-debt-recovery-paid")); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&resource, resource.ID).Error; err != nil {
		t.Fatal(err)
	}
	if resource.BillingStatus != models.BillableResourceStatusActive {
		t.Fatalf("recovered billing status=%s", resource.BillingStatus)
	}
	if err := db.First(&paymentDueInvoice, paymentDueInvoice.ID).Error; err != nil {
		t.Fatal(err)
	}
	if paymentDueInvoice.Status != models.InvoiceStatusPaid || paymentDueInvoice.PaidAt == nil {
		t.Fatalf("recovered payment-due invoice=%#v", paymentDueInvoice)
	}
}

func TestTopupPartialRefundWebhookResolvesViaStatusCrosscheck(t *testing.T) {
	db, user, service, gateway := topupServiceFixture(t)
	view, err := service.Create(context.Background(), user.ID, "topup-partial-refund", TopupInput{PackageID: 1})
	if err != nil {
		t.Fatal(err)
	}
	var topup models.Topup
	if err := db.First(&topup, view.ID).Error; err != nil {
		t.Fatal(err)
	}
	paid := signedNotification(topup.ProviderOrderID, "settlement", "accept", "transaction-partial")
	if err := service.ProcessNotification(context.Background(), paid); err != nil {
		t.Fatal(err)
	}
	assertTopupState(t, db, user.ID, view.ID, models.TopupStatusPaid, 100000, 1)

	// Midtrans keeps gross_amount as the original transaction amount and
	// supplies the cumulative monetary refund in refund_amount.
	partial := withRefundAmount(signedNotification(topup.ProviderOrderID, "partial_refund", "", "transaction-partial"), "10000.00")
	gateway.status = partial
	gateway.status.SignatureKey = ""
	if err := service.ProcessNotification(context.Background(), partial); err != nil {
		t.Fatal(err)
	}
	assertTopupState(t, db, user.ID, view.ID, models.TopupStatusPartialRefund, 60000, 2)

	secondPartial := withRefundAmount(signedNotification(topup.ProviderOrderID, "partial_refund", "", "transaction-partial"), "15000.00")
	gateway.status = secondPartial
	gateway.status.SignatureKey = ""
	if err := service.ProcessNotification(context.Background(), secondPartial); err != nil {
		t.Fatal(err)
	}
	assertTopupState(t, db, user.ID, view.ID, models.TopupStatusPartialRefund, 40000, 3)

	refunded := signedNotification(topup.ProviderOrderID, "refund", "", "transaction-partial")
	gateway.status = refunded
	gateway.status.SignatureKey = ""
	if err := service.ProcessNotification(context.Background(), refunded); err != nil {
		t.Fatal(err)
	}
	assertTopupState(t, db, user.ID, view.ID, models.TopupStatusRefunded, 0, 4)
}

func TestTopupPartialRefundWebhookFailsClosedWithoutCrosscheck(t *testing.T) {
	db, user, service, _ := topupServiceFixture(t)
	view, err := service.Create(context.Background(), user.ID, "topup-partial-unverified", TopupInput{PackageID: 1})
	if err != nil {
		t.Fatal(err)
	}
	var topup models.Topup
	if err := db.First(&topup, view.ID).Error; err != nil {
		t.Fatal(err)
	}
	paid := signedNotification(topup.ProviderOrderID, "settlement", "accept", "transaction-unverified")
	if err := service.ProcessNotification(context.Background(), paid); err != nil {
		t.Fatal(err)
	}

	// The status API still reports the transaction as settled, so the partial
	// refund webhook cannot be confirmed and must not touch the wallet.
	gateway := service.gateway.(*fakeMidtransGateway)
	gateway.status = signedNotification(topup.ProviderOrderID, "settlement", "accept", "transaction-unverified")
	gateway.status.SignatureKey = ""
	partial := withRefundAmount(signedNotification(topup.ProviderOrderID, "partial_refund", "", "transaction-unverified"), "10000.00")
	if err := service.ProcessNotification(context.Background(), partial); !errors.Is(err, ErrInvalidPaymentNotification) {
		t.Fatalf("expected rejection, got %v", err)
	}
	assertTopupState(t, db, user.ID, view.ID, models.TopupStatusPaid, 100000, 1)
}

func TestTopupRecoverStaleTopupsRecoversAbandonedCheckout(t *testing.T) {
	db, user, service, gateway := topupServiceFixture(t)
	view, err := service.Create(context.Background(), user.ID, "topup-stale", TopupInput{PackageID: 1})
	if err != nil {
		t.Fatal(err)
	}

	// Simulate a process that died after claiming the provider request:
	// state is "creating", no token, and the row is older than the threshold.
	staleAt := time.Now().UTC().Add(-staleTopupRecoveryThreshold - time.Minute)
	if err := db.Model(&models.Topup{}).Where("id = ?", view.ID).
		Updates(map[string]any{"provider_request_state": providerRequestCreating, "provider_payment_token": "", "provider_payment_url": "", "updated_at": staleAt}).Error; err != nil {
		t.Fatal(err)
	}

	gateway.status = signedNotification("", "", "", "") // no transaction recorded yet
	if err := service.RecoverStaleTopups(context.Background(), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	var topup models.Topup
	if err := db.First(&topup, view.ID).Error; err != nil {
		t.Fatal(err)
	}
	if topup.ProviderPaymentToken == "" || topup.ProviderRequestState != providerRequestReady {
		t.Fatalf("stale topup not recovered: %#v", topup)
	}
}

func TestTopupRecoverStaleTopupsAppliesCompletedProviderPayment(t *testing.T) {
	db, user, service, gateway := topupServiceFixture(t)
	view, err := service.Create(context.Background(), user.ID, "topup-stale-paid", TopupInput{PackageID: 1})
	if err != nil {
		t.Fatal(err)
	}
	var created models.Topup
	if err := db.First(&created, view.ID).Error; err != nil {
		t.Fatal(err)
	}

	staleAt := time.Now().UTC().Add(-staleTopupRecoveryThreshold - time.Minute)
	if err := db.Model(&models.Topup{}).Where("id = ?", view.ID).
		Updates(map[string]any{"provider_request_state": providerRequestCreating, "provider_payment_token": "", "provider_payment_url": "", "updated_at": staleAt}).Error; err != nil {
		t.Fatal(err)
	}

	// The user paid while our process was down; the provider knows.
	gateway.status = signedNotification(created.ProviderOrderID, "settlement", "accept", "transaction-stale")
	gateway.status.SignatureKey = ""
	if err := service.RecoverStaleTopups(context.Background(), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	assertTopupState(t, db, user.ID, view.ID, models.TopupStatusPaid, 100000, 1)
}

func TestTopupReconcileUsesVerifiedProviderStatus(t *testing.T) {
	db, user, service, gateway := topupServiceFixture(t)
	view, err := service.Create(context.Background(), user.ID, "topup-reconcile", TopupInput{PackageID: 1})
	if err != nil {
		t.Fatal(err)
	}
	var topup models.Topup
	if err := db.First(&topup, view.ID).Error; err != nil {
		t.Fatal(err)
	}
	gateway.status = signedNotification(topup.ProviderOrderID, "settlement", "accept", "transaction-2")
	view, err = service.Reconcile(context.Background(), user.ID, view.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gateway.statusCalls != 1 || view.Status != models.TopupStatusPaid {
		t.Fatalf("view=%#v statusCalls=%d", view, gateway.statusCalls)
	}
	assertTopupState(t, db, user.ID, view.ID, models.TopupStatusPaid, 100000, 1)
}

func TestTopupWebhookRetriesPaymentDueInvoicesAfterCommit(t *testing.T) {
	db, user, service, _ := topupServiceFixture(t)
	now := time.Now().UTC()
	project := models.Project{UserID: user.ID, Name: "Payment Due", GithubURL: "https://github.com/example/payment-due", Subdomain: "payment-due", Status: models.StatusRunning}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	spec := models.BillableSpec{Type: models.BillableTypeProject, Name: "Project", Slug: "payment-due", CPUMillicores: 500, MemoryMB: 512, StorageGB: 10, MonthlyCredits: 100, Version: 1, IsActive: true}
	if err := db.Create(&spec).Error; err != nil {
		t.Fatal(err)
	}
	resource := models.BillableResource{UserID: user.ID, Type: models.BillableTypeProject, ResourceID: project.ID, SpecID: spec.ID, BillingStatus: models.BillableResourceStatusPaymentDue, CurrentPeriodStart: now.AddDate(0, -1, 0), NextInvoiceAt: now.Add(-time.Second), BillingAnchorDay: now.AddDate(0, -1, 0).Day()}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}

	view, err := service.Create(context.Background(), user.ID, "topup-invoice-retry", TopupInput{PackageID: 1})
	if err != nil {
		t.Fatal(err)
	}
	var topup models.Topup
	if err := db.First(&topup, view.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.ProcessNotification(context.Background(), signedNotification(topup.ProviderOrderID, "settlement", "accept", "invoice-retry")); err != nil {
		t.Fatal(err)
	}

	if err := db.First(&resource, resource.ID).Error; err != nil {
		t.Fatal(err)
	}
	if resource.BillingStatus != models.BillableResourceStatusActive || !resource.NextInvoiceAt.After(now) {
		t.Fatalf("resource=%#v", resource)
	}
	var invoice models.Invoice
	if err := db.First(&invoice).Error; err != nil {
		t.Fatal(err)
	}
	if invoice.Status != models.InvoiceStatusPaid || invoice.TotalCredits != spec.MonthlyCredits {
		t.Fatalf("invoice=%#v", invoice)
	}
	assertTopupState(t, db, user.ID, view.ID, models.TopupStatusPaid, 99900, 2)
}

type fakeMidtransGateway struct {
	mu                       sync.Mutex
	createCalls, statusCalls int
	status                   MidtransNotification
	createStarted            chan struct{}
	createRelease            <-chan struct{}
	createStartedClosed      bool
	createResponse           MidtransPaymentResponse
	createResponseSet        bool
	createErr                error
}

func (g *fakeMidtransGateway) CreatePayment(ctx context.Context, _ MidtransPaymentRequest) (MidtransPaymentResponse, error) {
	g.mu.Lock()
	g.createCalls++
	if g.createStarted != nil && !g.createStartedClosed {
		close(g.createStarted)
		g.createStartedClosed = true
	}
	release := g.createRelease
	response := g.createResponse
	responseSet := g.createResponseSet
	createErr := g.createErr
	g.mu.Unlock()
	if release != nil {
		select {
		case <-release:
		case <-ctx.Done():
			return MidtransPaymentResponse{}, ctx.Err()
		}
	}
	if createErr != nil {
		return MidtransPaymentResponse{}, createErr
	}
	if responseSet {
		return response, nil
	}
	return MidtransPaymentResponse{Token: "token", RedirectURL: "https://app.sandbox.midtrans.com/snap/v2/vtweb/token"}, nil
}
func (g *fakeMidtransGateway) GetTransactionStatus(context.Context, string) (MidtransNotification, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.statusCalls++
	status := g.status
	if status.OrderID != "" && status.SignatureKey == "" {
		// The transaction status API response is authenticated by the service;
		// sign it so validateNotification accepts the cross-check result.
		sum := sha512.Sum512([]byte(status.OrderID + status.StatusCode + status.GrossAmount + "server-key"))
		status.SignatureKey = hex.EncodeToString(sum[:])
	}
	return status, nil
}

func topupServiceFixture(t *testing.T) (*gorm.DB, models.User, *TopupService, *fakeMidtransGateway) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Project{}, &models.ProjectSuspensionTask{}, &models.Wallet{}, &models.WalletLedgerEntry{}, &models.TopupPackage{}, &models.Topup{}, &models.BillableSpec{}, &models.BillableResource{}, &models.Invoice{}, &models.InvoiceItem{}, &models.PaymentEvent{}); err != nil {
		t.Fatal(err)
	}
	user := models.User{Email: fmt.Sprintf("%s@example.test", t.Name()), Password: "test", Name: "Topup"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	pkg := models.TopupPackage{Provider: models.BillingProviderMidtrans, Currency: models.BillingCurrencyIDR, Credits: 100000, AmountMinor: 25000, Version: 1, IsActive: true}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}
	gateway := &fakeMidtransGateway{}
	service := NewTopupService(db, NewWalletService(db), &config.Config{BillingEnabled: true, BillingTopupEnabled: true, MidtransServerKey: "server-key", MidtransMerchantID: "merchant-id"}, gateway)
	return db, user, service, gateway
}

// gatewaySees points the fake gateway's status response at the given webhook
// notification so reversal cross-checks observe the same transaction state the
// provider would report.
func gatewaySees(gateway *fakeMidtransGateway, notification MidtransNotification) {
	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	status := notification
	status.SignatureKey = ""
	gateway.status = status
}

func withRefundAmount(notification MidtransNotification, amount string) MidtransNotification {
	notification.RefundAmount = amount
	return notification
}

func signedNotification(orderID, status, fraud, transactionID string) MidtransNotification {
	n := MidtransNotification{OrderID: orderID, StatusCode: "200", GrossAmount: "25000.00", TransactionStatus: status, FraudStatus: fraud, TransactionID: transactionID, Currency: models.BillingCurrencyIDR, MerchantID: "merchant-id"}
	sum := sha512.Sum512([]byte(n.OrderID + n.StatusCode + n.GrossAmount + "server-key"))
	n.SignatureKey = hex.EncodeToString(sum[:])
	return n
}

func assertTopupState(t *testing.T, db *gorm.DB, userID, topupID uint, wantStatus models.TopupStatus, wantBalance, wantEntries int64) {
	t.Helper()
	var topup models.Topup
	if err := db.First(&topup, topupID).Error; err != nil || topup.Status != wantStatus {
		t.Fatalf("topup=%#v err=%v", topup, err)
	}
	var wallet models.Wallet
	if err := db.Where("user_id = ?", userID).First(&wallet).Error; err != nil || wallet.BalanceCredits != wantBalance {
		t.Fatalf("wallet=%#v err=%v", wallet, err)
	}
	var entries int64
	if err := db.Model(&models.WalletLedgerEntry{}).Where("wallet_id = ?", wallet.ID).Count(&entries).Error; err != nil || entries != wantEntries {
		t.Fatalf("entries=%d err=%v", entries, err)
	}
}

func TestTopupNotificationRejectsInvalidPaidStatusAndMerchant(t *testing.T) {
	_, _, service, _ := topupServiceFixture(t)
	for name, notification := range map[string]MidtransNotification{
		"paid status code": signedNotification("topup-1", "settlement", "accept", "transaction-3"),
		"missing merchant": signedNotification("topup-1", "settlement", "accept", "transaction-4"),
	} {
		t.Run(name, func(t *testing.T) {
			if name == "paid status code" {
				notification.StatusCode = "201"
				sum := sha512.Sum512([]byte(notification.OrderID + notification.StatusCode + notification.GrossAmount + "server-key"))
				notification.SignatureKey = hex.EncodeToString(sum[:])
			} else {
				notification.MerchantID = ""
			}
			if _, err := service.validateNotification(notification); err != ErrInvalidPaymentNotification {
				t.Fatalf("notification error = %v", err)
			}
		})
	}
}

func TestPaymentEventKeyIncludesTransactionStatus(t *testing.T) {
	_, _, service, _ := topupServiceFixture(t)
	capture, err := service.validateNotification(signedNotification("topup-1", "capture", "accept", "transaction-5"))
	if err != nil {
		t.Fatal(err)
	}
	settlement, err := service.validateNotification(signedNotification("topup-1", "settlement", "accept", "transaction-5"))
	if err != nil {
		t.Fatal(err)
	}
	capturePayload, err := json.Marshal(capture)
	if err != nil {
		t.Fatal(err)
	}
	settlementPayload, err := json.Marshal(settlement)
	if err != nil {
		t.Fatal(err)
	}
	if paymentEventKey(capturePayload) == paymentEventKey(settlementPayload) {
		t.Fatal("payment event key collapsed transaction status transition")
	}
}

func TestTopupCreateFailsClosedAfterProviderSuccessStorageFailure(t *testing.T) {
	db, user, service, gateway := topupServiceFixture(t)
	if err := db.Exec(`
		CREATE TRIGGER reject_payment_token
		BEFORE UPDATE OF provider_payment_token ON topups
		BEGIN SELECT RAISE(FAIL, 'payment token write rejected'); END;
	`).Error; err != nil {
		t.Fatal(err)
	}
	_, err := service.Create(context.Background(), user.ID, "topup-recovery", TopupInput{PackageID: 1})
	if err == nil {
		t.Fatal("topup succeeded despite payment token storage failure")
	}
	replayContext, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := service.Create(replayContext, user.ID, "topup-recovery", TopupInput{PackageID: 1}); err != context.DeadlineExceeded {
		t.Fatalf("recovery error = %v", err)
	}
	if gateway.createCalls != 1 {
		t.Fatalf("provider calls = %d, want 1", gateway.createCalls)
	}
}

func TestTopupCreateResetsPendingAfterCanceledProvider(t *testing.T) {
	db, user, service, gateway := topupServiceFixture(t)
	gateway.createStarted = make(chan struct{})
	gateway.createRelease = make(chan struct{})
	requestContext, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := service.Create(requestContext, user.ID, "topup-provider-canceled", TopupInput{PackageID: 1})
		result <- err
	}()
	select {
	case <-gateway.createStarted:
	case <-time.After(time.Second):
		t.Fatal("provider request did not start")
	}
	cancel()
	if err := <-result; err != context.Canceled {
		t.Fatalf("create error=%v", err)
	}

	var topup models.Topup
	if err := db.Where("client_idempotency_key = ?", "topup-provider-canceled").First(&topup).Error; err != nil {
		t.Fatal(err)
	}
	if topup.ProviderRequestState != providerRequestPending {
		t.Fatalf("provider state=%q", topup.ProviderRequestState)
	}
	orderID := topup.ProviderOrderID
	gateway.mu.Lock()
	gateway.createRelease = nil
	gateway.mu.Unlock()
	view, err := service.Create(context.Background(), user.ID, "topup-provider-canceled", TopupInput{PackageID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.First(&topup, topup.ID).Error; err != nil || topup.ProviderOrderID != orderID {
		t.Fatalf("topup=%#v err=%v", topup, err)
	}
	if view.ID != topup.ID || gateway.createCalls != 2 {
		t.Fatalf("view=%#v topup=%#v provider calls=%d", view, topup, gateway.createCalls)
	}
}

func TestTopupCreateResetsPendingAfterInvalidProviderResponse(t *testing.T) {
	db, user, service, gateway := topupServiceFixture(t)
	gateway.createResponse = MidtransPaymentResponse{Token: "invalid"}
	gateway.createResponseSet = true
	if _, err := service.Create(context.Background(), user.ID, "topup-provider-invalid", TopupInput{PackageID: 1}); err != ErrPaymentProvider {
		t.Fatalf("create error=%v", err)
	}

	var topup models.Topup
	if err := db.Where("client_idempotency_key = ?", "topup-provider-invalid").First(&topup).Error; err != nil {
		t.Fatal(err)
	}
	if topup.ProviderRequestState != providerRequestPending {
		t.Fatalf("provider state=%q", topup.ProviderRequestState)
	}
	orderID := topup.ProviderOrderID
	gateway.mu.Lock()
	gateway.createResponseSet = false
	gateway.mu.Unlock()
	view, err := service.Create(context.Background(), user.ID, "topup-provider-invalid", TopupInput{PackageID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.First(&topup, topup.ID).Error; err != nil || topup.ProviderOrderID != orderID {
		t.Fatalf("topup=%#v err=%v", topup, err)
	}
	if view.ID != topup.ID || gateway.createCalls != 2 {
		t.Fatalf("view=%#v topup=%#v provider calls=%d", view, topup, gateway.createCalls)
	}
}

func TestTopupCreateSignalsRecoveryWhenProviderFailureCleanupFails(t *testing.T) {
	db, user, service, gateway := topupServiceFixture(t)
	if err := db.Exec(`
		CREATE TRIGGER reject_provider_request_reset
		BEFORE UPDATE OF provider_request_state ON topups
		WHEN NEW.provider_request_state = 'pending'
		BEGIN SELECT RAISE(FAIL, 'provider request reset rejected'); END;
	`).Error; err != nil {
		t.Fatal(err)
	}
	gateway.createErr = errors.New("provider request failed")
	_, err := service.Create(context.Background(), user.ID, "topup-cleanup-failure", TopupInput{PackageID: 1})
	if !errors.Is(err, ErrTopupRecoveryRequired) {
		t.Fatalf("create error=%v", err)
	}
	var topup models.Topup
	if err := db.Where("client_idempotency_key = ?", "topup-cleanup-failure").First(&topup).Error; err != nil {
		t.Fatal(err)
	}
	if topup.ProviderRequestState != providerRequestCreating {
		t.Fatalf("provider state=%q", topup.ProviderRequestState)
	}
}

func TestTopupCreateWaitsForConcurrentProviderRequest(t *testing.T) {
	_, user, service, gateway := topupServiceFixture(t)
	gateway.createStarted = make(chan struct{})
	release := make(chan struct{})
	gateway.createRelease = release

	type result struct {
		view TopupView
		err  error
	}
	requestContext, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	results := make(chan result, 2)
	go func() {
		view, err := service.Create(requestContext, user.ID, "topup-slow-provider", TopupInput{PackageID: 1})
		results <- result{view: view, err: err}
	}()
	select {
	case <-gateway.createStarted:
	case <-time.After(time.Second):
		t.Fatal("provider request did not start")
	}
	go func() {
		view, err := service.Create(requestContext, user.ID, "topup-slow-provider", TopupInput{PackageID: 1})
		results <- result{view: view, err: err}
	}()
	select {
	case result := <-results:
		t.Fatalf("concurrent replay returned before provider completed: %#v", result)
	case <-time.After(time.Second + 2*paymentRequestWaitInterval):
	}
	close(release)

	first, second := <-results, <-results
	if first.err != nil || second.err != nil || first.view != second.view {
		t.Fatalf("first=%#v second=%#v", first, second)
	}
	if gateway.createCalls != 1 {
		t.Fatalf("provider calls=%d", gateway.createCalls)
	}
}

func TestTopupReconcileReturnsTerminalAfterTokenStorageFailure(t *testing.T) {
	db, user, service, gateway := topupServiceFixture(t)
	if err := db.Exec(`
		CREATE TRIGGER reject_payment_token
		BEFORE UPDATE OF provider_payment_token ON topups
		BEGIN SELECT RAISE(FAIL, 'payment token write rejected'); END;
	`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(context.Background(), user.ID, "topup-terminal-recovery", TopupInput{PackageID: 1}); err == nil {
		t.Fatal("topup succeeded despite payment token storage failure")
	}
	var topup models.Topup
	if err := db.Where("client_idempotency_key = ?", "topup-terminal-recovery").First(&topup).Error; err != nil {
		t.Fatal(err)
	}
	paid := signedNotification(topup.ProviderOrderID, "settlement", "accept", "transaction-terminal-recovery")
	if err := service.ProcessNotification(context.Background(), paid); err != nil {
		t.Fatal(err)
	}
	gateway.status = paid
	view, err := service.Reconcile(context.Background(), user.ID, topup.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Status != models.TopupStatusPaid || view.PaymentToken != "" || gateway.createCalls != 1 {
		t.Fatalf("view=%#v provider calls=%d", view, gateway.createCalls)
	}
	assertTopupState(t, db, user.ID, topup.ID, models.TopupStatusPaid, 100000, 1)
	if err := db.First(&topup, topup.ID).Error; err != nil || topup.ProviderRequestState != providerRequestTerminal {
		t.Fatalf("topup=%#v err=%v", topup, err)
	}
}

func TestTopupReconcileRejectsAnotherUser(t *testing.T) {
	db, user, service, _ := topupServiceFixture(t)
	other := models.User{Email: "other@example.test", Password: "test", Name: "Other"}
	if err := db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	view, err := service.Create(context.Background(), user.ID, "topup-owner", TopupInput{PackageID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Reconcile(context.Background(), other.ID, view.ID); err != ErrTopupNotFound {
		t.Fatalf("cross-user reconciliation error = %v", err)
	}
}

func TestTopupReconcileRecoversPendingCheckoutToken(t *testing.T) {
	db, user, service, gateway := topupServiceFixture(t)
	view, err := service.Create(context.Background(), user.ID, "topup-recover-pending", TopupInput{PackageID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.Topup{}).Where("id = ?", view.ID).Updates(map[string]any{"provider_payment_token": "", "provider_payment_url": "", "provider_request_state": "creating"}).Error; err != nil {
		t.Fatal(err)
	}
	var topup models.Topup
	if err := db.First(&topup, view.ID).Error; err != nil {
		t.Fatal(err)
	}
	gateway.status = signedNotificationWithCode(topup.ProviderOrderID, "201", "pending", "", "transaction-recovery")
	view, err = service.Reconcile(context.Background(), user.ID, view.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.PaymentToken == "" || gateway.createCalls != 2 {
		t.Fatalf("view=%#v provider calls=%d", view, gateway.createCalls)
	}
}

func TestTopupStatusTransitionCodes(t *testing.T) {
	for name, test := range map[string]struct{ code, status, fraud string }{
		"capture accept":     {"200", "capture", "accept"},
		"capture challenge":  {"201", "capture", "challenge"},
		"capture deny":       {"202", "capture", "deny"},
		"pending":            {"201", "pending", ""},
		"failed":             {"202", "cancel", ""},
		"expired":            {"202", "expire", ""},
		"refund":             {"200", "refund", ""},
		"partial refund":     {"200", "partial_refund", ""},
		"chargeback":         {"200", "chargeback", ""},
		"partial chargeback": {"200", "partial_chargeback", ""},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := topupStatusFromNotification(test.code, test.status, test.fraud); err != nil {
				t.Fatal(err)
			}
			if _, err := topupStatusFromNotification("999", test.status, test.fraud); err != ErrInvalidPaymentNotification {
				t.Fatalf("invalid code error=%v", err)
			}
		})
	}
}

func signedNotificationWithCode(orderID, code, status, fraud, transactionID string) MidtransNotification {
	n := MidtransNotification{OrderID: orderID, StatusCode: code, GrossAmount: "25000.00", TransactionStatus: status, FraudStatus: fraud, TransactionID: transactionID, Currency: models.BillingCurrencyIDR, MerchantID: "merchant-id"}
	sum := sha512.Sum512([]byte(n.OrderID + n.StatusCode + n.GrossAmount + "server-key"))
	n.SignatureKey = hex.EncodeToString(sum[:])
	return n
}

func TestTopupWebhookRollsBackLedgerWhenStatusUpdateFails(t *testing.T) {
	db, user, service, _ := topupServiceFixture(t)
	view, err := service.Create(context.Background(), user.ID, "topup-status-rollback", TopupInput{PackageID: 1})
	if err != nil {
		t.Fatal(err)
	}
	var topup models.Topup
	if err := db.First(&topup, view.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TRIGGER reject_paid_status BEFORE UPDATE OF status ON topups WHEN NEW.status = 'paid' BEGIN SELECT RAISE(FAIL, 'status update rejected'); END;`).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.ProcessNotification(context.Background(), signedNotification(topup.ProviderOrderID, "settlement", "accept", "transaction-rollback")); err == nil {
		t.Fatal("webhook succeeded despite topup status trigger")
	}
	assertTopupState(t, db, user.ID, view.ID, models.TopupStatusPending, 0, 0)
	var eventCount int64
	if err := db.Model(&models.PaymentEvent{}).Count(&eventCount).Error; err != nil || eventCount != 0 {
		t.Fatalf("events=%d err=%v", eventCount, err)
	}
}

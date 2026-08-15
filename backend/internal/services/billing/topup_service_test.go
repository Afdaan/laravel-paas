package billing

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/repositories"
	"github.com/laravel-paas/shared/services/setting"
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
	pkg := models.TopupPackage{Currency: models.BillingCurrencyIDR, Credits: 100000, AmountMinor: 25000, Version: 1, IsActive: true}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}
	gateway := &fakeMidtransGateway{}
	service := NewTopupService(db, NewWalletService(db), &config.Config{BillingEnabled: true, BillingTopupEnabled: true, BillingTopupProvider: models.BillingProviderMidtrans, MidtransServerKey: "server-key", MidtransMerchantID: "merchant-id"}, gateway)
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
	if _, err := service.Create(replayContext, user.ID, "topup-recovery", TopupInput{PackageID: 1}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("recovery error = %v, want context.DeadlineExceeded", err)
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

// Providers are paid in major units while the DB stores minor units. IDR makes the two
// identical, which is exactly why a decimal currency would slip through untested.
func TestMajorUnitsConvertsPerCurrency(t *testing.T) {
	for _, tc := range []struct {
		currency    string
		amountMinor int64
		want        int64
		wantErr     bool
	}{
		{models.BillingCurrencyIDR, 100_000, 100_000, false},
		{models.BillingCurrencyUSD, 1000, 10, false},
		{models.BillingCurrencyUSD, 1050, 0, true}, // 10.50 is not whole dollars
		{"EUR", 100, 0, true},
	} {
		got, err := majorUnits(tc.amountMinor, tc.currency)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("majorUnits(%d, %s) = %d, want error", tc.amountMinor, tc.currency, got)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Fatalf("majorUnits(%d, %s) = %d, %v; want %d", tc.amountMinor, tc.currency, got, err, tc.want)
		}
	}
}

func TestCustomTopupValidation(t *testing.T) {
	_, _, svc, _ := topupServiceFixture(t)
	for _, tc := range []struct {
		name    string
		input   TopupInput
		wantErr bool
	}{
		{"package topup still works", TopupInput{PackageID: 1}, false},
		{"custom 50k valid", TopupInput{AmountMinor: 50_000}, false},
		{"custom min boundary", TopupInput{AmountMinor: 10_000}, false},
		{"custom max boundary", TopupInput{AmountMinor: 10_000_000}, false},
		{"both zero rejected", TopupInput{}, true},
		{"below minimum rejected", TopupInput{AmountMinor: 5_000}, true},
		{"above maximum rejected", TopupInput{AmountMinor: 11_000_000}, true},
		{"not divisible by 1000 rejected", TopupInput{AmountMinor: 10_500}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.validateCreate(context.Background(), 1, "key", tc.input, models.BillingProviderMidtrans)
			if tc.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestTopupIdempotencyMatch(t *testing.T) {
	pkgID := uint(42)
	for _, tc := range []struct {
		name  string
		topup models.Topup
		input TopupInput
		want  bool
	}{
		{"package match", models.Topup{TopupPackageID: &pkgID, AmountMinor: 100_000}, TopupInput{PackageID: 42}, true},
		{"package mismatch", models.Topup{TopupPackageID: &pkgID, AmountMinor: 100_000}, TopupInput{PackageID: 99}, false},
		{"package vs custom conflict", models.Topup{TopupPackageID: &pkgID, AmountMinor: 100_000}, TopupInput{AmountMinor: 100_000}, false},
		{"custom match", models.Topup{AmountMinor: 50_000}, TopupInput{AmountMinor: 50_000}, true},
		{"custom amount mismatch", models.Topup{AmountMinor: 50_000}, TopupInput{AmountMinor: 60_000}, false},
		{"custom vs package conflict", models.Topup{AmountMinor: 50_000}, TopupInput{PackageID: 1}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := topupIdempotencyMatch(tc.topup, tc.input); got != tc.want {
				t.Fatalf("topupIdempotencyMatch = %v, want %v", got, tc.want)
			}
		})
	}
}

type fakePakasirGateway struct {
	createCalls    int
	createdOrderID string
	createdAmount  int64
	method         string
	paymentNumber  string
}

func (f *fakePakasirGateway) CreateTransaction(ctx context.Context, orderID string, amountMinor int64, method string) (PakasirCreateResponse, error) {
	f.createCalls++
	f.createdOrderID = orderID
	f.createdAmount = amountMinor
	f.method = method
	paymentNumber := f.paymentNumber
	if paymentNumber == "" {
		paymentNumber = "https://app.pakasir.com/pay/test-project/" + orderID
	}
	return PakasirCreateResponse{
		PaymentNumber: paymentNumber,
		TotalPayment:  amountMinor,
		Fee:           700,
		ExpiredAt:     "2026-08-11 12:00:00",
	}, nil
}

func (f *fakePakasirGateway) SimulatePayment(ctx context.Context, orderID string, amountMinor int64) error {
	return nil
}

func (f *fakePakasirGateway) CancelTransaction(ctx context.Context, orderID string, amountMinor int64) error {
	return nil
}

func (f *fakePakasirGateway) GetTransactionDetail(ctx context.Context, orderID string, amountMinor int64) (PakasirTransactionDetail, error) {
	return PakasirTransactionDetail{
		OrderID:       orderID,
		Amount:        amountMinor,
		Project:       "test-project",
		Status:        "completed",
		PaymentMethod: "qris",
	}, nil
}

func TestTopupServiceWithPakasirProvider(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Wallet{}, &models.WalletLedgerEntry{}, &models.TopupPackage{}, &models.Topup{}, &models.PaymentEvent{}); err != nil {
		t.Fatal(err)
	}
	user := models.User{Email: "pakasir@example.test", Password: "test", Name: "Pakasir User"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	pkg := models.TopupPackage{Currency: models.BillingCurrencyIDR, Credits: 100, AmountMinor: 10000, Version: 1, IsActive: true}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}

	pakasirGw := &fakePakasirGateway{paymentNumber: "00020101021226610016ID.CO.QRIS.WWW"}
	cfg := &config.Config{
		BillingEnabled:       true,
		BillingTopupEnabled:  true,
		BillingTopupProvider: "pakasir",
		PakasirEnabled:       true,
		PakasirProjectSlug:   "test-project",
		PakasirAPIKey:        "test-key",
		FrontendURL:          "https://runara.example",
	}
	service := NewTopupService(db, NewWalletService(db), cfg, nil, pakasirGw)

	view, err := service.Create(context.Background(), user.ID, "client-pakasir-key", TopupInput{PackageID: pkg.ID})
	if err != nil {
		t.Fatalf("unexpected error creating topup: %v", err)
	}
	if view.PaymentURL == "" || view.Credits != 100 {
		t.Fatalf("unexpected topup view: %+v", view)
	}
	paymentURL, err := url.Parse(view.PaymentURL)
	if err != nil {
		t.Fatalf("parse payment URL: %v", err)
	}
	returnURL, err := url.Parse(paymentURL.Query().Get("redirect"))
	if err != nil {
		t.Fatalf("parse return URL: %v", err)
	}
	if returnURL.Path != "/billing" || returnURL.Query().Get("payment_return") != "pakasir" || returnURL.Query().Get("topup_id") != strconv.FormatUint(uint64(view.ID), 10) {
		t.Fatalf("unexpected return URL: %s", returnURL.String())
	}
	if pakasirGw.createdAmount != 10000 {
		t.Fatalf("pakasir gateway did not receive transaction: %+v", pakasirGw)
	}
	if err := service.ProcessPakasirWebhook(context.Background(), pakasirGw.createdOrderID, pakasirGw.createdAmount, "completed"); err != nil {
		t.Fatalf("process pakasir payment: %v", err)
	}
	var wallet models.Wallet
	if err := db.Where("user_id = ?", user.ID).First(&wallet).Error; err != nil {
		t.Fatalf("load credited wallet: %v", err)
	}
	if wallet.BalanceCredits != pkg.Credits {
		t.Fatalf("wallet credits = %d, want %d", wallet.BalanceCredits, pkg.Credits)
	}
	var event models.PaymentEvent
	if err := db.Where("provider_order_id = ?", pakasirGw.createdOrderID).First(&event).Error; err != nil {
		t.Fatalf("load pakasir payment event: %v", err)
	}
	if event.ProcessedAt == nil {
		t.Fatal("pakasir payment event was not marked processed")
	}
}

func TestTopupServiceActiveProviderSwitching(t *testing.T) {
	cfg := &config.Config{
		BillingEnabled:      true,
		BillingTopupEnabled: true,
		PakasirEnabled:      true,
		PakasirProjectSlug:  "test-project",
	}

	service := NewTopupService(nil, nil, cfg, nil, nil)
	prov, err := service.activeProvider(context.Background())
	if err != nil || prov != models.BillingProviderPakasir {
		t.Fatalf("expected default provider pakasir, got %s (err: %v)", prov, err)
	}
}

func TestTopupServicePakasirDisabledRejectsCreation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Wallet{}, &models.WalletLedgerEntry{}, &models.TopupPackage{}, &models.Topup{}); err != nil {
		t.Fatal(err)
	}
	user := models.User{Email: "disabled-pakasir@example.test", Password: "test", Name: "Disabled Pakasir User"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	pkg := models.TopupPackage{Currency: models.BillingCurrencyIDR, Credits: 100, AmountMinor: 10000, Version: 1, IsActive: true}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}

	pakasirGw := &fakePakasirGateway{}
	midtransGw := &fakeMidtransGateway{}
	// Pakasir is selected but disabled via flag, even though Midtrans credentials exist
	cfg := &config.Config{
		BillingEnabled:       true,
		BillingTopupEnabled:  true,
		BillingTopupProvider: "pakasir",
		PakasirEnabled:       false,
		PakasirProjectSlug:   "test-project",
		PakasirAPIKey:        "test-key",
		MidtransServerKey:    "midtrans-key",
		MidtransMerchantID:   "midtrans-merchant",
	}
	service := NewTopupService(db, NewWalletService(db), cfg, midtransGw, pakasirGw)

	_, err = service.Create(context.Background(), user.ID, "client-pakasir-disabled-key", TopupInput{PackageID: pkg.ID})
	if err == nil {
		t.Fatal("expected error when PAKASIR_ENABLED=false, got nil")
	}
	if !errors.Is(err, ErrPaymentProvider) {
		t.Fatalf("expected ErrPaymentProvider, got: %v", err)
	}
	if midtransGw.createCalls != 0 {
		t.Fatalf("expected 0 midtrans calls on fail-closed disabled pakasir, got %d", midtransGw.createCalls)
	}
	if pakasirGw.createCalls != 0 {
		t.Fatalf("expected 0 pakasir calls, got %d", pakasirGw.createCalls)
	}
}

func TestTopupServiceMidtransUnconfiguredFailsClosed(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Wallet{}, &models.WalletLedgerEntry{}, &models.TopupPackage{}, &models.Topup{}); err != nil {
		t.Fatal(err)
	}
	user := models.User{Email: "unconfigured-midtrans@example.test", Password: "test", Name: "Unconfigured Midtrans User"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	pkg := models.TopupPackage{Currency: models.BillingCurrencyIDR, Credits: 100, AmountMinor: 10000, Version: 1, IsActive: true}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}

	pakasirGw := &fakePakasirGateway{}
	midtransGw := &fakeMidtransGateway{}
	// Midtrans is selected but has missing credentials, even though Pakasir is fully enabled
	cfg := &config.Config{
		BillingEnabled:       true,
		BillingTopupEnabled:  true,
		BillingTopupProvider: "midtrans",
		PakasirEnabled:       true,
		PakasirProjectSlug:   "test-project",
		PakasirAPIKey:        "test-key",
	}
	service := NewTopupService(db, NewWalletService(db), cfg, midtransGw, pakasirGw)

	_, err = service.Create(context.Background(), user.ID, "client-midtrans-unconfigured-key", TopupInput{PackageID: pkg.ID})
	if err == nil {
		t.Fatal("expected error when Midtrans unconfigured, got nil")
	}
	if !errors.Is(err, ErrPaymentProvider) {
		t.Fatalf("expected ErrPaymentProvider, got: %v", err)
	}
	if pakasirGw.createCalls != 0 {
		t.Fatalf("expected 0 pakasir calls on fail-closed unconfigured midtrans, got %d", pakasirGw.createCalls)
	}
	if midtransGw.createCalls != 0 {
		t.Fatalf("expected 0 midtrans calls, got %d", midtransGw.createCalls)
	}
}

type failingSettingRepo struct {
	err error
}

func (f *failingSettingRepo) GetByKey(ctx context.Context, key string) (*models.Setting, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return nil, f.err
}

func (f *failingSettingRepo) GetValue(key string, defaultValue string) string {
	return defaultValue
}

func (f *failingSettingRepo) Upsert(key string, value string) error {
	return f.err
}

func (f *failingSettingRepo) ListAll() ([]models.Setting, error) {
	return nil, f.err
}

func TestTopupServiceSettingDBReadFailureFailsClosed(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Wallet{}, &models.WalletLedgerEntry{}, &models.TopupPackage{}, &models.Topup{}); err != nil {
		t.Fatal(err)
	}
	user := models.User{Email: "db-fail@example.test", Password: "test", Name: "DB Fail User"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	pkg := models.TopupPackage{Currency: models.BillingCurrencyIDR, Credits: 100, AmountMinor: 10000, Version: 1, IsActive: true}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}

	pakasirGw := &fakePakasirGateway{}
	midtransGw := &fakeMidtransGateway{}
	cfg := &config.Config{
		BillingEnabled:      true,
		BillingTopupEnabled: true,
		PakasirEnabled:      true,
		PakasirProjectSlug:  "test-project",
		PakasirAPIKey:       "test-key",
	}

	failingRepo := &failingSettingRepo{err: errors.New("database connection timeout")}
	settingSvc := setting.NewSettingService(failingRepo, nil)

	service := NewTopupService(db, NewWalletService(db), cfg, midtransGw, pakasirGw)
	service.SetSettingService(settingSvc)

	_, err = service.Create(context.Background(), user.ID, "client-db-err-key", TopupInput{PackageID: pkg.ID})
	if err == nil {
		t.Fatal("expected error on database failure, got nil")
	}
	if !errors.Is(err, ErrPaymentProvider) {
		t.Fatalf("expected ErrPaymentProvider, got: %v", err)
	}

	// Verify no topup was persisted in the database
	var count int64
	if err := db.Model(&models.Topup{}).Where("client_idempotency_key = ?", "client-db-err-key").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("expected 0 topups created in DB, got %d (err: %v)", count, err)
	}

	// Verify neither gateway was called
	if pakasirGw.createCalls != 0 {
		t.Fatalf("expected 0 pakasir calls on DB read failure, got %d", pakasirGw.createCalls)
	}
	if midtransGw.createCalls != 0 {
		t.Fatalf("expected 0 midtrans calls on DB read failure, got %d", midtransGw.createCalls)
	}
}

func TestTopupServiceInvalidPersistedProviderFailsClosed(t *testing.T) {
	for _, badValue := range []string{"", "   ", "stripe_unsupported", "INVALID_PROVIDER"} {
		t.Run(fmt.Sprintf("value_%q", badValue), func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s_%x?mode=memory&cache=shared", t.Name(), badValue)), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			if err := db.AutoMigrate(&models.User{}, &models.Wallet{}, &models.WalletLedgerEntry{}, &models.TopupPackage{}, &models.Topup{}, &models.Setting{}); err != nil {
				t.Fatal(err)
			}
			user := models.User{Email: "invalid-prov@example.test", Password: "test", Name: "Invalid Provider User"}
			if err := db.Create(&user).Error; err != nil {
				t.Fatal(err)
			}
			pkg := models.TopupPackage{Currency: models.BillingCurrencyIDR, Credits: 100, AmountMinor: 10000, Version: 1, IsActive: true}
			if err := db.Create(&pkg).Error; err != nil {
				t.Fatal(err)
			}

			// Seed invalid/empty provider in settings table
			if err := db.Create(&models.Setting{Key: models.SettingDefaultPaymentProvider, Value: badValue}).Error; err != nil {
				t.Fatal(err)
			}

			pakasirGw := &fakePakasirGateway{}
			midtransGw := &fakeMidtransGateway{}
			cfg := &config.Config{
				BillingEnabled:      true,
				BillingTopupEnabled: true,
				PakasirEnabled:      true,
				PakasirProjectSlug:  "test-project",
				PakasirAPIKey:       "test-key",
			}

			settingRepo := repositories.NewSettingRepository(db)
			settingSvc := setting.NewSettingService(settingRepo, nil)

			service := NewTopupService(db, NewWalletService(db), cfg, midtransGw, pakasirGw)
			service.SetSettingService(settingSvc)

			idempotencyKey := fmt.Sprintf("client-invalid-prov-key-%s", badValue)
			_, err = service.Create(context.Background(), user.ID, idempotencyKey, TopupInput{PackageID: pkg.ID})
			if err == nil {
				t.Fatalf("expected error on invalid persisted provider %q, got nil", badValue)
			}
			if !errors.Is(err, ErrPaymentProvider) {
				t.Fatalf("expected ErrPaymentProvider, got: %v", err)
			}

			// Verify no topup was persisted in the database
			var count int64
			if err := db.Model(&models.Topup{}).Where("client_idempotency_key = ?", idempotencyKey).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("expected 0 topups created in DB, got %d (err: %v)", count, err)
			}

			// Verify neither gateway was called
			if pakasirGw.createCalls != 0 {
				t.Fatalf("expected 0 pakasir calls on invalid persisted provider, got %d", pakasirGw.createCalls)
			}
			if midtransGw.createCalls != 0 {
				t.Fatalf("expected 0 midtrans calls on invalid persisted provider, got %d", midtransGw.createCalls)
			}
		})
	}
}

func TestTopupServiceSettingContextCancellationAndTimeoutFailsClosed(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Wallet{}, &models.WalletLedgerEntry{}, &models.TopupPackage{}, &models.Topup{}); err != nil {
		t.Fatal(err)
	}
	user := models.User{Email: "ctx-cancel@example.test", Password: "test", Name: "Context User"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	pkg := models.TopupPackage{Currency: models.BillingCurrencyIDR, Credits: 100, AmountMinor: 10000, Version: 1, IsActive: true}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}

	pakasirGw := &fakePakasirGateway{}
	midtransGw := &fakeMidtransGateway{}
	cfg := &config.Config{
		BillingEnabled:      true,
		BillingTopupEnabled: true,
		PakasirEnabled:      true,
		PakasirProjectSlug:  "test-project",
		PakasirAPIKey:       "test-key",
	}

	failingRepo := &failingSettingRepo{err: errors.New("timeout")}
	settingSvc := setting.NewSettingService(failingRepo, nil)

	service := NewTopupService(db, NewWalletService(db), cfg, midtransGw, pakasirGw)
	service.SetSettingService(settingSvc)

	// Pre-canceled context
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = service.Create(canceledCtx, user.ID, "client-canceled-key", TopupInput{PackageID: pkg.ID})
	if err == nil {
		t.Fatal("expected error on canceled context, got nil")
	}

	// Verify no topup created
	var count int64
	if err := db.Model(&models.Topup{}).Where("client_idempotency_key = ?", "client-canceled-key").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("expected 0 topups created in DB, got %d (err: %v)", count, err)
	}
	if pakasirGw.createCalls != 0 || midtransGw.createCalls != 0 {
		t.Fatal("expected 0 gateway calls on canceled context")
	}
}

func TestTopupServiceNilContextRejected(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Wallet{}, &models.WalletLedgerEntry{}, &models.TopupPackage{}, &models.Topup{}); err != nil {
		t.Fatal(err)
	}
	user := models.User{Email: "nil-ctx@example.test", Password: "test", Name: "Nil Ctx User"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	service := NewTopupService(db, NewWalletService(db), &config.Config{}, nil, nil)
	var nilCtx context.Context
	_, err = service.Create(nilCtx, user.ID, "client-nil-ctx-key", TopupInput{PackageID: 1})
	if err == nil {
		t.Fatal("expected error on nil context, got nil")
	}
}

func TestTopupServiceSingleProviderResolutionPerCreate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Wallet{}, &models.WalletLedgerEntry{}, &models.TopupPackage{}, &models.Topup{}, &models.Setting{}); err != nil {
		t.Fatal(err)
	}
	user := models.User{Email: "single-res@example.test", Password: "test", Name: "Single Resolution User"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	pkg := models.TopupPackage{Currency: models.BillingCurrencyIDR, Credits: 100, AmountMinor: 10000, Version: 1, IsActive: true}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}

	// Set default payment provider to midtrans
	if err := db.Create(&models.Setting{Key: models.SettingDefaultPaymentProvider, Value: "midtrans"}).Error; err != nil {
		t.Fatal(err)
	}

	pakasirGw := &fakePakasirGateway{}
	midtransGw := &fakeMidtransGateway{}
	cfg := &config.Config{
		BillingEnabled:      true,
		BillingTopupEnabled: true,
		MidtransServerKey:   "midtrans-server-key",
		MidtransMerchantID:  "midtrans-merchant-id",
		PakasirEnabled:      true,
		PakasirProjectSlug:  "test-project",
		PakasirAPIKey:       "test-key",
	}

	settingRepo := repositories.NewSettingRepository(db)
	settingSvc := setting.NewSettingService(settingRepo, nil)

	service := NewTopupService(db, NewWalletService(db), cfg, midtransGw, pakasirGw)
	service.SetSettingService(settingSvc)

	view, err := service.Create(context.Background(), user.ID, "client-single-res-key", TopupInput{PackageID: pkg.ID})
	if err != nil {
		t.Fatalf("unexpected create error: %v", err)
	}
	if view.ID == 0 {
		t.Fatal("expected non-zero topup ID")
	}

	var savedTopup models.Topup
	if err := db.First(&savedTopup, view.ID).Error; err != nil {
		t.Fatalf("failed to load saved topup: %v", err)
	}
	if savedTopup.Provider != models.BillingProviderMidtrans {
		t.Fatalf("expected persisted topup provider midtrans, got %s", savedTopup.Provider)
	}

	if midtransGw.createCalls != 1 {
		t.Fatalf("expected 1 midtrans call, got %d", midtransGw.createCalls)
	}
	if pakasirGw.createCalls != 0 {
		t.Fatalf("expected 0 pakasir calls, got %d", pakasirGw.createCalls)
	}
}

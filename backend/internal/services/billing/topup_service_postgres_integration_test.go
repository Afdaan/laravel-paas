//go:build integration

package billing

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
)

func TestTopupServicePostgresConcurrentWebhookCreditsOnce(t *testing.T) {
	db := walletPostgresTestDB(t)
	user := createPostgresWalletTestUser(t, db)
	credits := int64(800000000 + time.Now().UTC().UnixNano()%99999999)
	pkg := models.TopupPackage{Currency: models.BillingCurrencyIDR, Credits: credits, AmountMinor: credits, Version: 1, IsActive: true}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}
	service := NewTopupService(db, NewWalletService(db), topupIntegrationConfig(), &fakeMidtransGateway{})
	view, err := service.Create(context.Background(), user.ID, fmt.Sprintf("postgres-webhook-%d", credits), TopupInput{PackageID: pkg.ID})
	if err != nil {
		t.Fatal(err)
	}
	var topup models.Topup
	if err := db.First(&topup, view.ID).Error; err != nil {
		t.Fatal(err)
	}
	notification := signedNotificationForAmount(topup.ProviderOrderID, "settlement", "accept", fmt.Sprintf("transaction-%d", credits), credits)
	for _, err := range runConcurrentCalls(t, []func() error{
		func() error { return service.ProcessNotification(context.Background(), notification) },
		func() error { return service.ProcessNotification(context.Background(), notification) },
		func() error { return service.ProcessNotification(context.Background(), notification) },
		func() error { return service.ProcessNotification(context.Background(), notification) },
	}) {
		if err != nil {
			t.Fatalf("concurrent webhook: %v", err)
		}
	}
	assertTopupState(t, db, user.ID, view.ID, models.TopupStatusPaid, credits, 1)
}

func TestTopupServicePostgresRejectsProviderTransactionIDMutationAfterCredit(t *testing.T) {
	db := walletPostgresTestDB(t)
	user := createPostgresWalletTestUser(t, db)
	credits := int64(810000000 + time.Now().UTC().UnixNano()%99999999)
	pkg := models.TopupPackage{Currency: models.BillingCurrencyIDR, Credits: credits, AmountMinor: credits, Version: 1, IsActive: true}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}
	service := NewTopupService(db, NewWalletService(db), topupIntegrationConfig(), &fakeMidtransGateway{})
	view, err := service.Create(context.Background(), user.ID, fmt.Sprintf("postgres-transaction-identity-%d", credits), TopupInput{PackageID: pkg.ID})
	if err != nil {
		t.Fatal(err)
	}
	var topup models.Topup
	if err := db.First(&topup, view.ID).Error; err != nil {
		t.Fatal(err)
	}
	firstTransactionID := fmt.Sprintf("transaction-first-%d", credits)
	if err := service.ProcessNotification(context.Background(), signedNotificationForAmount(topup.ProviderOrderID, "settlement", "accept", firstTransactionID, credits)); err != nil {
		t.Fatalf("process first valid callback: %v", err)
	}
	if err := service.ProcessNotification(context.Background(), signedNotificationForAmount(topup.ProviderOrderID, "settlement", "accept", fmt.Sprintf("transaction-mutated-%d", credits), credits)); !errors.Is(err, ErrInvalidPaymentNotification) {
		t.Fatalf("mutated transaction ID error=%v", err)
	}
	assertTopupState(t, db, user.ID, view.ID, models.TopupStatusPaid, credits, 1)
	if err := db.First(&topup, view.ID).Error; err != nil {
		t.Fatal(err)
	}
	if topup.ProviderTransactionID == nil || *topup.ProviderTransactionID != firstTransactionID {
		t.Fatalf("provider transaction ID=%v, want %q", topup.ProviderTransactionID, firstTransactionID)
	}
}

func TestTopupServicePostgresConcurrentReplayAndWebhook(t *testing.T) {
	db := walletPostgresTestDB(t)
	user := createPostgresWalletTestUser(t, db)
	credits := int64(700000000 + time.Now().UTC().UnixNano()%99999999)
	pkg := models.TopupPackage{Currency: models.BillingCurrencyIDR, Credits: credits, AmountMinor: credits, Version: 1, IsActive: true}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}
	service := NewTopupService(db, NewWalletService(db), topupIntegrationConfig(), &fakeMidtransGateway{})
	key := fmt.Sprintf("postgres-replay-%d", credits)
	view, err := service.Create(context.Background(), user.ID, key, TopupInput{PackageID: pkg.ID})
	if err != nil {
		t.Fatal(err)
	}
	var topup models.Topup
	if err := db.First(&topup, view.ID).Error; err != nil {
		t.Fatal(err)
	}
	notification := signedNotificationForAmount(topup.ProviderOrderID, "settlement", "accept", fmt.Sprintf("transaction-%d", credits), credits)
	for _, err := range runConcurrentCalls(t, []func() error{
		func() error {
			_, err := service.Create(context.Background(), user.ID, key, TopupInput{PackageID: pkg.ID})
			return err
		},
		func() error { return service.ProcessNotification(context.Background(), notification) },
	}) {
		if err != nil {
			t.Fatalf("concurrent replay/webhook: %v", err)
		}
	}
	assertTopupState(t, db, user.ID, view.ID, models.TopupStatusPaid, credits, 1)
}

func topupIntegrationConfig() *config.Config {
	return &config.Config{BillingEnabled: true, BillingTopupEnabled: true, BillingTopupProvider: models.BillingProviderMidtrans, MidtransServerKey: "server-key", MidtransMerchantID: "merchant-id"}
}

func TestTopupServicePostgresConcurrentIdenticalCreate(t *testing.T) {
	db := walletPostgresTestDB(t)
	user := createPostgresWalletTestUser(t, db)
	credits := int64(600000000 + time.Now().UTC().UnixNano()%99999999)
	pkg := models.TopupPackage{Currency: models.BillingCurrencyIDR, Credits: credits, AmountMinor: credits, Version: 1, IsActive: true}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}
	service := NewTopupService(db, NewWalletService(db), topupIntegrationConfig(), &fakeMidtransGateway{})
	key := fmt.Sprintf("postgres-create-%d", credits)
	for _, err := range runConcurrentCalls(t, []func() error{
		func() error {
			_, err := service.Create(context.Background(), user.ID, key, TopupInput{PackageID: pkg.ID})
			return err
		},
		func() error {
			_, err := service.Create(context.Background(), user.ID, key, TopupInput{PackageID: pkg.ID})
			return err
		},
	}) {
		if err != nil {
			t.Fatalf("concurrent create: %v", err)
		}
	}
	var count int64
	if err := db.Model(&models.Topup{}).Where("wallet_id IN (SELECT id FROM wallets WHERE user_id = ?)", user.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("topups=%d err=%v", count, err)
	}
}

func TestTopupServicePostgresConcurrentCreateWaitsForSlowProvider(t *testing.T) {
	db := walletPostgresTestDB(t)
	user := createPostgresWalletTestUser(t, db)
	credits := int64(500000000 + time.Now().UTC().UnixNano()%99999999)
	pkg := models.TopupPackage{Currency: models.BillingCurrencyIDR, Credits: credits, AmountMinor: credits, Version: 1, IsActive: true}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}
	gateway := &fakeMidtransGateway{createStarted: make(chan struct{})}
	release := make(chan struct{})
	gateway.createRelease = release
	service := NewTopupService(db, NewWalletService(db), topupIntegrationConfig(), gateway)
	key := fmt.Sprintf("postgres-slow-provider-%d", credits)

	type result struct {
		view TopupView
		err  error
	}
	requestContext, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	results := make(chan result, 2)
	go func() {
		view, err := service.Create(requestContext, user.ID, key, TopupInput{PackageID: pkg.ID})
		results <- result{view: view, err: err}
	}()
	select {
	case <-gateway.createStarted:
	case <-time.After(time.Second):
		t.Fatal("provider request did not start")
	}
	go func() {
		view, err := service.Create(requestContext, user.ID, key, TopupInput{PackageID: pkg.ID})
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

func TestTopupServicePostgresCompetingPaidAndRefundTransitions(t *testing.T) {
	db := walletPostgresTestDB(t)
	user := createPostgresWalletTestUser(t, db)
	credits := int64(400000000 + time.Now().UTC().UnixNano()%99999999)
	pkg := models.TopupPackage{Currency: models.BillingCurrencyIDR, Credits: credits, AmountMinor: credits, Version: 1, IsActive: true}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}
	gateway := &fakeMidtransGateway{}
	service := NewTopupService(db, NewWalletService(db), topupIntegrationConfig(), gateway)
	view, err := service.Create(context.Background(), user.ID, fmt.Sprintf("postgres-transition-%d", credits), TopupInput{PackageID: pkg.ID})
	if err != nil {
		t.Fatal(err)
	}
	var topup models.Topup
	if err := db.First(&topup, view.ID).Error; err != nil {
		t.Fatal(err)
	}
	transactionID := fmt.Sprintf("transition-%d", credits)
	gateway.status = signedNotificationForAmount(topup.ProviderOrderID, "refund", "", transactionID, credits)
	if err := service.ProcessNotification(context.Background(), signedNotificationForAmount(topup.ProviderOrderID, "settlement", "accept", transactionID, credits)); err != nil {
		t.Fatal(err)
	}
	for _, err := range runConcurrentCalls(t, []func() error{
		func() error {
			return service.ProcessNotification(context.Background(), signedNotificationForAmount(topup.ProviderOrderID, "settlement", "accept", transactionID, credits))
		},
		func() error {
			return service.ProcessNotification(context.Background(), signedNotificationForAmount(topup.ProviderOrderID, "refund", "", transactionID, credits))
		},
	}) {
		if err != nil {
			t.Fatalf("competing transition: %v", err)
		}
	}
	var storedTopup models.Topup
	if err := db.First(&storedTopup, view.ID).Error; err != nil || storedTopup.Status != models.TopupStatusRefunded {
		t.Fatalf("topup=%#v err=%v", storedTopup, err)
	}
	var wallet models.Wallet
	if err := db.Where("user_id = ?", user.ID).First(&wallet).Error; err != nil || wallet.BalanceCredits != 0 {
		t.Fatalf("wallet=%#v err=%v", wallet, err)
	}
	var entryCount int64
	if err := db.Model(&models.WalletLedgerEntry{}).Where("wallet_id = ?", wallet.ID).Count(&entryCount).Error; err != nil || (entryCount != 0 && entryCount != 2) {
		t.Fatalf("entries=%d err=%v", entryCount, err)
	}
	var eventCount int64
	if err := db.Model(&models.PaymentEvent{}).Where("provider_order_id = ?", topup.ProviderOrderID).Count(&eventCount).Error; err != nil || eventCount != 2 {
		t.Fatalf("events=%d err=%v", eventCount, err)
	}
}

func TestTopupServicePostgresRollsBackLedgerWhenStatusUpdateFails(t *testing.T) {
	db := walletPostgresTestDB(t)
	user := createPostgresWalletTestUser(t, db)
	credits := int64(300000000 + time.Now().UTC().UnixNano()%99999999)
	pkg := models.TopupPackage{Currency: models.BillingCurrencyIDR, Credits: credits, AmountMinor: credits, Version: 1, IsActive: true}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}
	service := NewTopupService(db, NewWalletService(db), topupIntegrationConfig(), &fakeMidtransGateway{})
	view, err := service.Create(context.Background(), user.ID, fmt.Sprintf("postgres-rollback-%d", credits), TopupInput{PackageID: pkg.ID})
	if err != nil {
		t.Fatal(err)
	}
	functionName := fmt.Sprintf("reject_paid_status_%d", view.ID)
	triggerName := fmt.Sprintf("reject_paid_status_trigger_%d", view.ID)
	if err := db.Exec(fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'status update rejected'; END; $$`, functionName)).Error; err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = db.Exec(fmt.Sprintf("DROP TRIGGER IF EXISTS %s ON topups", triggerName)).Error
		_ = db.Exec(fmt.Sprintf("DROP FUNCTION IF EXISTS %s()", functionName)).Error
	}()
	if err := db.Exec(fmt.Sprintf("CREATE TRIGGER %s BEFORE UPDATE OF status ON topups FOR EACH ROW WHEN (NEW.status = 'paid') EXECUTE FUNCTION %s()", triggerName, functionName)).Error; err != nil {
		t.Fatal(err)
	}
	var topup models.Topup
	if err := db.First(&topup, view.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.ProcessNotification(context.Background(), signedNotificationForAmount(topup.ProviderOrderID, "settlement", "accept", fmt.Sprintf("rollback-%d", credits), credits)); err == nil {
		t.Fatal("webhook succeeded despite topup status trigger")
	}
	assertTopupState(t, db, user.ID, view.ID, models.TopupStatusPending, 0, 0)
	var eventCount int64
	if err := db.Model(&models.PaymentEvent{}).Where("provider_order_id = ?", topup.ProviderOrderID).Count(&eventCount).Error; err != nil || eventCount != 0 {
		t.Fatalf("events=%d err=%v", eventCount, err)
	}
}

func TestTopupServicePostgresReconcileTerminalAfterTokenStorageFailure(t *testing.T) {
	db := walletPostgresTestDB(t)
	user := createPostgresWalletTestUser(t, db)
	credits := int64(200000000 + time.Now().UTC().UnixNano()%99999999)
	pkg := models.TopupPackage{Currency: models.BillingCurrencyIDR, Credits: credits, AmountMinor: credits, Version: 1, IsActive: true}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}
	gateway := &fakeMidtransGateway{}
	service := NewTopupService(db, NewWalletService(db), topupIntegrationConfig(), gateway)
	key := fmt.Sprintf("postgres-terminal-recovery-%d", credits)
	functionName := fmt.Sprintf("reject_payment_token_%d", credits)
	triggerName := fmt.Sprintf("reject_payment_token_trigger_%d", credits)
	if err := db.Exec(fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'payment token write rejected'; END; $$`, functionName)).Error; err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = db.Exec(fmt.Sprintf("DROP TRIGGER IF EXISTS %s ON topups", triggerName)).Error
		_ = db.Exec(fmt.Sprintf("DROP FUNCTION IF EXISTS %s()", functionName)).Error
	}()
	if err := db.Exec(fmt.Sprintf("CREATE TRIGGER %s BEFORE UPDATE OF provider_payment_token ON topups FOR EACH ROW EXECUTE FUNCTION %s()", triggerName, functionName)).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(context.Background(), user.ID, key, TopupInput{PackageID: pkg.ID}); err == nil {
		t.Fatal("topup succeeded despite payment token storage failure")
	}
	var topup models.Topup
	if err := db.Where("client_idempotency_key = ?", key).First(&topup).Error; err != nil {
		t.Fatal(err)
	}
	paid := signedNotificationForAmount(topup.ProviderOrderID, "settlement", "accept", fmt.Sprintf("terminal-%d", credits), credits)
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
	assertTopupState(t, db, user.ID, topup.ID, models.TopupStatusPaid, credits, 1)
	if err := db.First(&topup, topup.ID).Error; err != nil || topup.ProviderRequestState != providerRequestTerminal {
		t.Fatalf("topup=%#v err=%v", topup, err)
	}
}

func TestTopupServicePostgresReversalCreatesAndSettlesPaymentDueEvidence(t *testing.T) {
	db := walletPostgresTestDB(t)
	user := createPostgresWalletTestUser(t, db)
	credits := int64(300000000 + time.Now().UTC().UnixNano()%99999999)
	pkg := models.TopupPackage{Currency: models.BillingCurrencyIDR, Credits: credits, AmountMinor: credits, Version: 1, IsActive: true}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}
	project := models.Project{UserID: user.ID, Name: "Reversal evidence", GithubURL: "https://github.com/example/reversal-evidence", Subdomain: fmt.Sprintf("reversal-evidence-%d", credits), Status: models.StatusRunning}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	spec := models.BillableSpec{Type: models.BillableTypeProject, Name: "Reversal evidence", Slug: fmt.Sprintf("reversal-evidence-%d", credits), CPUMillicores: 500, MemoryMB: 512, StorageGB: 10, MonthlyCredits: 100, Version: 1, IsActive: true}
	if err := db.Create(&spec).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	resource := models.BillableResource{UserID: user.ID, Type: models.BillableTypeProject, ResourceID: project.ID, SpecID: spec.ID, BillingStatus: models.BillableResourceStatusActive, CurrentPeriodStart: now, NextInvoiceAt: now.AddDate(0, 1, 0), BillingAnchorDay: now.Day()}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}
	gateway := &fakeMidtransGateway{}
	service := NewTopupService(db, NewWalletService(db), topupIntegrationConfig(), gateway)
	view, err := service.Create(context.Background(), user.ID, fmt.Sprintf("postgres-reversal-%d", credits), TopupInput{PackageID: pkg.ID})
	if err != nil {
		t.Fatal(err)
	}
	var topup models.Topup
	if err := db.First(&topup, view.ID).Error; err != nil {
		t.Fatal(err)
	}
	gateway.status = signedNotificationForAmount(topup.ProviderOrderID, "refund", "", fmt.Sprintf("reversal-paid-%d", credits), credits)
	if err := service.ProcessNotification(context.Background(), signedNotificationForAmount(topup.ProviderOrderID, "settlement", "accept", fmt.Sprintf("reversal-paid-%d", credits), credits)); err != nil {
		t.Fatal(err)
	}
	if _, err := service.wallets.Debit(context.Background(), LedgerMutation{UserID: user.ID, EntryType: models.WalletLedgerEntryInvoiceDebit, AmountCredits: credits, IdempotencyKey: fmt.Sprintf("postgres-reversal-debit-%d", credits), ReferenceType: "invoice", ReferenceID: "reversal"}); err != nil {
		t.Fatal(err)
	}
	if err := service.ProcessNotification(context.Background(), signedNotificationForAmount(topup.ProviderOrderID, "refund", "", fmt.Sprintf("reversal-paid-%d", credits), credits)); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&resource, resource.ID).Error; err != nil {
		t.Fatal(err)
	}
	if resource.BillingStatus != models.BillableResourceStatusPaymentDue {
		t.Fatalf("resource billing status=%s", resource.BillingStatus)
	}
	var invoice models.Invoice
	if err := db.Where("idempotency_key = ?", fmt.Sprintf("topup:%s:payment-due", topup.ProviderOrderID)).First(&invoice).Error; err != nil {
		t.Fatal(err)
	}
	if invoice.Status != models.InvoiceStatusPaymentDue || invoice.DueAt == nil || invoice.TotalCredits != 0 {
		t.Fatalf("payment-due invoice=%#v", invoice)
	}
	recovery, err := service.Create(context.Background(), user.ID, fmt.Sprintf("postgres-reversal-recovery-%d", credits), TopupInput{PackageID: pkg.ID})
	if err != nil {
		t.Fatal(err)
	}
	var recoveryTopup models.Topup
	if err := db.First(&recoveryTopup, recovery.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.ProcessNotification(context.Background(), signedNotificationForAmount(recoveryTopup.ProviderOrderID, "settlement", "accept", fmt.Sprintf("reversal-recovery-%d", credits), credits)); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&resource, resource.ID).Error; err != nil {
		t.Fatal(err)
	}
	if resource.BillingStatus != models.BillableResourceStatusActive {
		t.Fatalf("recovered resource billing status=%s", resource.BillingStatus)
	}
	if err := db.First(&invoice, invoice.ID).Error; err != nil {
		t.Fatal(err)
	}
	if invoice.Status != models.InvoiceStatusPaid || invoice.PaidAt == nil {
		t.Fatalf("recovered payment-due invoice=%#v", invoice)
	}
}

func signedNotificationForAmount(orderID, status, fraud, transactionID string, amount int64) MidtransNotification {
	notification := MidtransNotification{
		OrderID:           orderID,
		StatusCode:        "200",
		GrossAmount:       fmt.Sprintf("%d.00", amount),
		TransactionStatus: status,
		FraudStatus:       fraud,
		TransactionID:     transactionID,
		Currency:          models.BillingCurrencyIDR,
		MerchantID:        "merchant-id",
	}
	signature := sha512.Sum512([]byte(notification.OrderID + notification.StatusCode + notification.GrossAmount + "server-key"))
	notification.SignatureKey = hex.EncodeToString(signature[:])
	return notification
}

func TestTopupServicePostgresPartialRefundReversesCreditsProportionallyOnce(t *testing.T) {
	db := walletPostgresTestDB(t)
	user := createPostgresWalletTestUser(t, db)
	amount := (int64(900000000+time.Now().UTC().UnixNano()%99999999) / 2) * 2
	credits := amount
	pkg := models.TopupPackage{Currency: models.BillingCurrencyIDR, Credits: credits, AmountMinor: amount, Version: 1, IsActive: true}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}
	service := NewTopupService(db, NewWalletService(db), topupIntegrationConfig(), &fakeMidtransGateway{})
	view, err := service.Create(context.Background(), user.ID, fmt.Sprintf("postgres-partial-refund-%d", amount), TopupInput{PackageID: pkg.ID})
	if err != nil {
		t.Fatal(err)
	}
	var topup models.Topup
	if err := db.First(&topup, view.ID).Error; err != nil {
		t.Fatal(err)
	}
	transactionID := fmt.Sprintf("partial-refund-%d", amount)
	if err := service.ProcessReconciledNotification(context.Background(), signedNotificationForAmount(topup.ProviderOrderID, "settlement", "accept", transactionID, amount)); err != nil {
		t.Fatal(err)
	}

	partial := signedNotificationForAmount(topup.ProviderOrderID, "partial_refund", "", transactionID, amount)
	partial.RefundAmount = fmt.Sprintf("%d.00", amount/2)
	for _, err := range runConcurrentCalls(t, []func() error{
		func() error { return service.ProcessReconciledNotification(context.Background(), partial) },
		func() error { return service.ProcessReconciledNotification(context.Background(), partial) },
		func() error { return service.ProcessReconciledNotification(context.Background(), partial) },
	}) {
		if err != nil {
			t.Fatalf("partial refund: %v", err)
		}
	}
	assertTopupState(t, db, user.ID, view.ID, models.TopupStatusPartialRefund, credits/2, 2)

	refunded := signedNotificationForAmount(topup.ProviderOrderID, "refund", "", transactionID, amount)
	if err := service.ProcessReconciledNotification(context.Background(), refunded); err != nil {
		t.Fatal(err)
	}
	assertTopupState(t, db, user.ID, view.ID, models.TopupStatusRefunded, 0, 3)
}

func TestTopupServicePostgresReconcileByProviderOrderID(t *testing.T) {
	db := walletPostgresTestDB(t)
	user := createPostgresWalletTestUser(t, db)
	other := createPostgresWalletTestUser(t, db)
	credits := int64(400000000 + time.Now().UTC().UnixNano()%99999999)
	pkg := models.TopupPackage{Currency: models.BillingCurrencyIDR, Credits: credits, AmountMinor: credits, Version: 1, IsActive: true}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}
	gateway := &fakeMidtransGateway{}
	service := NewTopupService(db, NewWalletService(db), topupIntegrationConfig(), gateway)
	view, err := service.Create(context.Background(), user.ID, fmt.Sprintf("postgres-reconcile-ref-%d", credits), TopupInput{PackageID: pkg.ID})
	if err != nil {
		t.Fatal(err)
	}
	var topup models.Topup
	if err := db.First(&topup, view.ID).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := service.ReconcileByProviderOrderID(context.Background(), other.ID, topup.ProviderOrderID); err != ErrTopupNotFound {
		t.Fatalf("cross-user postgres reconciliation = %v, want %v", err, ErrTopupNotFound)
	}

	gateway.status = signedNotificationForAmount(topup.ProviderOrderID, "settlement", "accept", fmt.Sprintf("txn-pg-%d", credits), credits)
	reconciledView, err := service.ReconcileByProviderOrderID(context.Background(), user.ID, topup.ProviderOrderID)
	if err != nil {
		t.Fatalf("owner postgres reconciliation failed: %v", err)
	}
	if reconciledView.Status != models.TopupStatusPaid {
		t.Fatalf("expected paid status, got %s", reconciledView.Status)
	}
	assertTopupState(t, db, user.ID, topup.ID, models.TopupStatusPaid, credits, 1)
}

func TestTopupServicePostgresConcurrentCreateSharesPersistedRandomReference(t *testing.T) {
	db := walletPostgresTestDB(t)
	user := createPostgresWalletTestUser(t, db)
	credits := int64(600000000 + time.Now().UTC().UnixNano()%99999999)
	pkg := models.TopupPackage{Currency: models.BillingCurrencyIDR, Credits: credits, AmountMinor: credits, Version: 1, IsActive: true}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}
	gateway := &fakeMidtransGateway{}
	service := NewTopupService(db, NewWalletService(db), topupIntegrationConfig(), gateway)
	key := fmt.Sprintf("postgres-concurrent-create-%d", credits)

	views := make([]TopupView, 4)
	errs := runConcurrentCalls(t, []func() error{
		func() error { var err error; views[0], err = service.Create(context.Background(), user.ID, key, TopupInput{PackageID: pkg.ID}); return err },
		func() error { var err error; views[1], err = service.Create(context.Background(), user.ID, key, TopupInput{PackageID: pkg.ID}); return err },
		func() error { var err error; views[2], err = service.Create(context.Background(), user.ID, key, TopupInput{PackageID: pkg.ID}); return err },
		func() error { var err error; views[3], err = service.Create(context.Background(), user.ID, key, TopupInput{PackageID: pkg.ID}); return err },
	})
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent create %d failed: %v", i, err)
		}
		if views[i].ID != views[0].ID || views[i].PaymentToken != views[0].PaymentToken || views[i].PaymentURL != views[0].PaymentURL {
			t.Fatalf("view %d mismatch: %+v vs %+v", i, views[i], views[0])
		}
	}
	var count int64
	if err := db.Model(&models.Topup{}).Where("client_idempotency_key = ?", key).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 topup row, got %d", count)
	}
	if gateway.createCalls != 1 {
		t.Fatalf("expected exactly 1 provider create call, got %d", gateway.createCalls)
	}
}

func TestTopupServicePostgresConcurrentPendingCapEnforced(t *testing.T) {
	db := walletPostgresTestDB(t)
	user := createPostgresWalletTestUser(t, db)
	credits := int64(700000000 + time.Now().UTC().UnixNano()%99999999)
	pkg := models.TopupPackage{Currency: models.BillingCurrencyIDR, Credits: credits, AmountMinor: credits, Version: 1, IsActive: true}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}
	gateway := &fakeMidtransGateway{}
	service := NewTopupService(db, NewWalletService(db), topupIntegrationConfig(), gateway)

	numRequests := 15
	funcs := make([]func() error, numRequests)
	for i := 0; i < numRequests; i++ {
		idx := i
		funcs[i] = func() error {
			key := fmt.Sprintf("postgres-concurrent-pending-%d-%d", credits, idx)
			_, err := service.Create(context.Background(), user.ID, key, TopupInput{PackageID: pkg.ID})
			return err
		}
	}

	errs := runConcurrentCalls(t, funcs)
	var successCount int
	var limitExceededCount int
	for _, err := range errs {
		if err == nil {
			successCount++
		} else if errors.Is(err, ErrTopupPendingLimitExceeded) {
			limitExceededCount++
		} else {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	if successCount != MaxPendingTopupsPerWallet {
		t.Fatalf("expected exactly %d successful creates under concurrency, got %d", MaxPendingTopupsPerWallet, successCount)
	}
	if limitExceededCount != numRequests-MaxPendingTopupsPerWallet {
		t.Fatalf("expected %d limit exceeded errors, got %d", numRequests-MaxPendingTopupsPerWallet, limitExceededCount)
	}

	var dbPendingCount int64
	if err := db.Model(&models.Topup{}).
		Where("wallet_id IN (SELECT id FROM wallets WHERE user_id = ?) AND status = ?", user.ID, models.TopupStatusPending).
		Count(&dbPendingCount).Error; err != nil {
		t.Fatal(err)
	}
	if dbPendingCount != int64(MaxPendingTopupsPerWallet) {
		t.Fatalf("expected exactly %d pending rows in DB, got %d", MaxPendingTopupsPerWallet, dbPendingCount)
	}
}

func TestInvoiceServicePostgresConcurrentReversalVsManualSettlement(t *testing.T) {
	db := walletPostgresTestDB(t)
	user := createPostgresWalletTestUser(t, db)
	now := time.Now().UTC()

	walletService := NewWalletService(db)
	wallet, err := walletService.GetOrCreateWallet(context.Background(), user.ID)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Create a unique topup package and topup for credits
	credits := int64(900000000 + time.Now().UTC().UnixNano()%99999999)
	pkg := models.TopupPackage{
		Currency:    models.BillingCurrencyIDR,
		Credits:     credits,
		AmountMinor: credits,
		Version:     1,
		IsActive:    true,
	}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}
	topupService := NewTopupService(db, walletService, topupIntegrationConfig(), &fakeMidtransGateway{})
	view, err := topupService.Create(context.Background(), user.ID, fmt.Sprintf("init-topup-%d", credits), TopupInput{PackageID: pkg.ID})
	if err != nil {
		t.Fatal(err)
	}
	var topup models.Topup
	if err := db.First(&topup, view.ID).Error; err != nil {
		t.Fatal(err)
	}
	transactionID := fmt.Sprintf("txn-%d", credits)
	if err := topupService.ProcessReconciledNotification(context.Background(), signedNotificationForAmount(topup.ProviderOrderID, "settlement", "accept", transactionID, credits)); err != nil {
		t.Fatal(err)
	}

	// 2. Spend (credits - 10) on a debit, leaving wallet balance at exactly 10 credits.
	// When the top-up of `credits` is reversed, wallet balance becomes 10 - credits = negative debt!
	if _, err := walletService.Debit(context.Background(), LedgerMutation{
		UserID:         user.ID,
		EntryType:      models.WalletLedgerEntryInvoiceDebit,
		AmountCredits:  credits - 10,
		IdempotencyKey: fmt.Sprintf("prior-spend-%d", credits),
		ReferenceType:  "invoice_item",
		ReferenceID:    "1",
	}); err != nil {
		t.Fatal(err)
	}

	spec := models.BillableSpec{
		Type:           models.BillableTypeProject,
		Name:           "Starter",
		Slug:           fmt.Sprintf("spec-conc-%d", credits),
		CPUMillicores:  500,
		MemoryMB:       512,
		StorageGB:      10,
		MonthlyCredits: 50,
		Version:        1,
		IsActive:       true,
	}
	if err := db.Create(&spec).Error; err != nil {
		t.Fatal(err)
	}

	project := models.Project{
		UserID:    user.ID,
		Name:      "ConcApp",
		GithubURL: "https://github.com/example/concapp",
		Subdomain: fmt.Sprintf("concapp-%d", credits),
		Status:    models.StatusRunning,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}

	resource := models.BillableResource{
		UserID:                user.ID,
		Type:                  models.BillableTypeProject,
		ResourceID:            project.ID,
		SpecID:                spec.ID,
		BillingStatus:         models.BillableResourceStatusSuspended,
		CurrentPeriodStart:    now.AddDate(0, -1, 0),
		NextInvoiceAt:         now.AddDate(0, 1, 0),
		BillingAnchorDay:      1,
		BillingAnchorMonthEnd: false,
		AutoRenew:             true,
	}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}

	revInvoice := models.Invoice{
		UserID:         user.ID,
		WalletID:       wallet.ID,
		InvoiceNumber:  fmt.Sprintf("INV-CONC-%d", credits),
		PeriodStart:    now.AddDate(0, -1, 0),
		PeriodEnd:      now.AddDate(0, -1, 0).Add(time.Second),
		TotalCredits:   0,
		Status:         models.InvoiceStatusPaymentDue,
		IdempotencyKey: fmt.Sprintf("rev-invoice-%d", credits),
	}
	if err := db.Create(&revInvoice).Error; err != nil {
		t.Fatal(err)
	}

	item := models.InvoiceItem{
		InvoiceID:          revInvoice.ID,
		BillableResourceID: resource.ID,
		SpecID:             spec.ID,
		Description:        "zero credit reversal debt",
		Credits:            0,
	}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}

	invoiceSvc := NewInvoiceService(db, walletService)

	// Concurrently run:
	// 1. Real topup webhook reversal (refunding full credits, pulling wallet balance from 10 to 10 - credits < 0)
	// 2. PayDueResource attempting zero-credit debt settlement
	refundNotification := signedNotificationForAmount(topup.ProviderOrderID, "refund", "", transactionID, credits)
	funcs := []func() error{
		func() error {
			return topupService.ProcessReconciledNotification(context.Background(), refundNotification)
		},
		func() error {
			return invoiceSvc.PayDueResource(context.Background(), user.ID, models.BillableTypeProject, project.ID, now)
		},
	}

	errs := runConcurrentCalls(t, funcs)
	for i, cErr := range errs {
		if cErr != nil {
			if !errors.Is(cErr, ErrInsufficientCredits) && !errors.Is(cErr, ErrResourcePaymentNotDue) {
				t.Fatalf("unexpected non-domain error in concurrent call %d: %v", i, cErr)
			}
		}
	}

	// Check final state consistency:
	var finalWallet models.Wallet
	if err := db.First(&finalWallet, wallet.ID).Error; err != nil {
		t.Fatal(err)
	}
	var finalInv models.Invoice
	if err := db.First(&finalInv, revInvoice.ID).Error; err != nil {
		t.Fatal(err)
	}
	var finalRes models.BillableResource
	if err := db.First(&finalRes, resource.ID).Error; err != nil {
		t.Fatal(err)
	}

	// Invariant 1: Reversal MUST have executed and driven wallet negative
	if finalWallet.BalanceCredits >= 0 {
		t.Fatalf("expected wallet to be negative after reversal, got %d", finalWallet.BalanceCredits)
	}

	// Invariant 2: Under no serialization order can the resource remain active while wallet is negative
	if finalRes.BillingStatus == models.BillableResourceStatusActive {
		t.Fatalf("INVARIANT VIOLATION: Wallet is negative (%d) but resource remained ACTIVE!", finalWallet.BalanceCredits)
	}

	// Invariant 3: The user MUST have an outstanding payment_due invoice for the debt
	var dueInvoices []models.Invoice
	if err := db.Where("user_id = ? AND status = ?", user.ID, models.InvoiceStatusPaymentDue).Find(&dueInvoices).Error; err != nil {
		t.Fatal(err)
	}
	if len(dueInvoices) == 0 {
		t.Fatalf("INVARIANT VIOLATION: Wallet is negative (%d) but no payment_due invoice exists!", finalWallet.BalanceCredits)
	}
}

func TestInvoiceServicePostgresNegativeWalletRejectsZeroCreditPayment(t *testing.T) {
	db := walletPostgresTestDB(t)
	user := createPostgresWalletTestUser(t, db)
	now := time.Now().UTC()
	credits := int64(950000000 + time.Now().UTC().UnixNano()%99999999)

	walletService := NewWalletService(db)
	wallet, err := walletService.GetOrCreateWallet(context.Background(), user.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Set wallet directly in debt (-100 credits) via negative reversal entry
	if err := db.Model(&wallet).Update("balance_credits", -100).Error; err != nil {
		t.Fatal(err)
	}

	spec := models.BillableSpec{
		Type:           models.BillableTypeProject,
		Name:           "Starter",
		Slug:           fmt.Sprintf("spec-neg-%d", credits),
		CPUMillicores:  500,
		MemoryMB:       512,
		StorageGB:      10,
		MonthlyCredits: 50,
		Version:        1,
		IsActive:       true,
	}
	if err := db.Create(&spec).Error; err != nil {
		t.Fatal(err)
	}

	project := models.Project{
		UserID:    user.ID,
		Name:      "NegApp",
		GithubURL: "https://github.com/example/negapp",
		Subdomain: fmt.Sprintf("negapp-%d", credits),
		Status:    models.StatusRunning,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}

	resource := models.BillableResource{
		UserID:                user.ID,
		Type:                  models.BillableTypeProject,
		ResourceID:            project.ID,
		SpecID:                spec.ID,
		BillingStatus:         models.BillableResourceStatusSuspended,
		CurrentPeriodStart:    now.AddDate(0, -1, 0),
		NextInvoiceAt:         now.AddDate(0, 1, 0),
		BillingAnchorDay:      1,
		BillingAnchorMonthEnd: false,
		AutoRenew:             true,
	}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}

	revInvoice := models.Invoice{
		UserID:         user.ID,
		WalletID:       wallet.ID,
		InvoiceNumber:  fmt.Sprintf("INV-NEG-%d", credits),
		PeriodStart:    now.AddDate(0, -1, 0),
		PeriodEnd:      now.AddDate(0, -1, 0).Add(time.Second),
		TotalCredits:   0,
		Status:         models.InvoiceStatusPaymentDue,
		IdempotencyKey: fmt.Sprintf("rev-neg-inv-%d", credits),
	}
	if err := db.Create(&revInvoice).Error; err != nil {
		t.Fatal(err)
	}

	item := models.InvoiceItem{
		InvoiceID:          revInvoice.ID,
		BillableResourceID: resource.ID,
		SpecID:             spec.ID,
		Description:        "zero credit reversal debt",
		Credits:            0,
	}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}

	invoiceSvc := NewInvoiceService(db, walletService)

	// Attempting to pay zero-credit debt while wallet is negative MUST be rejected
	err = invoiceSvc.PayDueResource(context.Background(), user.ID, models.BillableTypeProject, project.ID, now)
	if !errors.Is(err, ErrInsufficientCredits) {
		t.Fatalf("expected ErrInsufficientCredits when wallet is negative, got: %v", err)
	}

	// Verify invoice is NOT paid and resource is NOT active
	var checkInv models.Invoice
	if err := db.First(&checkInv, revInvoice.ID).Error; err != nil {
		t.Fatal(err)
	}
	if checkInv.Status != models.InvoiceStatusPaymentDue {
		t.Fatalf("expected invoice to remain payment_due, got %s", checkInv.Status)
	}

	var checkRes models.BillableResource
	if err := db.First(&checkRes, resource.ID).Error; err != nil {
		t.Fatal(err)
	}
	if checkRes.BillingStatus != models.BillableResourceStatusSuspended {
		t.Fatalf("expected resource to remain suspended, got %s", checkRes.BillingStatus)
	}
}

func TestInvoiceServicePostgresConcurrentSchedulerVsManualSettlement(t *testing.T) {
	db := walletPostgresTestDB(t)
	user := createPostgresWalletTestUser(t, db)
	now := time.Now().UTC()
	credits := int64(850000000 + time.Now().UTC().UnixNano()%99999999)

	walletService := NewWalletService(db)
	wallet, err := walletService.GetOrCreateWallet(context.Background(), user.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Initial balance: 200 credits
	if _, err := walletService.Credit(context.Background(), LedgerMutation{
		UserID:         user.ID,
		EntryType:      models.WalletLedgerEntryTopup,
		AmountCredits:  200,
		IdempotencyKey: fmt.Sprintf("sched-credit-%d", credits),
		ReferenceType:  "test_init",
		ReferenceID:    "1",
	}); err != nil {
		t.Fatal(err)
	}

	spec := models.BillableSpec{
		Type:           models.BillableTypeProject,
		Name:           "Starter",
		Slug:           fmt.Sprintf("spec-sched-%d", credits),
		CPUMillicores:  500,
		MemoryMB:       512,
		StorageGB:      10,
		MonthlyCredits: 50,
		Version:        1,
		IsActive:       true,
	}
	if err := db.Create(&spec).Error; err != nil {
		t.Fatal(err)
	}

	project := models.Project{
		UserID:    user.ID,
		Name:      "SchedApp",
		GithubURL: "https://github.com/example/schedapp",
		Subdomain: fmt.Sprintf("schedapp-%d", credits),
		Status:    models.StatusRunning,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}

	resource := models.BillableResource{
		UserID:                user.ID,
		Type:                  models.BillableTypeProject,
		ResourceID:            project.ID,
		SpecID:                spec.ID,
		BillingStatus:         models.BillableResourceStatusPaymentDue,
		CurrentPeriodStart:    now.AddDate(0, -1, 0),
		NextInvoiceAt:         now.Add(-time.Hour),
		BillingAnchorDay:      1,
		BillingAnchorMonthEnd: false,
		AutoRenew:             true,
	}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatal(err)
	}

	dueInvoice := models.Invoice{
		UserID:         user.ID,
		WalletID:       wallet.ID,
		InvoiceNumber:  fmt.Sprintf("INV-SCHED-%d", credits),
		PeriodStart:    now.AddDate(0, -1, 0),
		PeriodEnd:      now.Add(-time.Hour),
		TotalCredits:   50,
		Status:         models.InvoiceStatusPaymentDue,
		IdempotencyKey: fmt.Sprintf("sched-inv-%d", credits),
	}
	if err := db.Create(&dueInvoice).Error; err != nil {
		t.Fatal(err)
	}
	item := models.InvoiceItem{
		InvoiceID:          dueInvoice.ID,
		BillableResourceID: resource.ID,
		SpecID:             spec.ID,
		Description:        "monthly charge",
		Credits:            50,
	}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}

	invoiceSvc := NewInvoiceService(db, walletService)

	// Concurrently run scheduler charge and manual PayDueResource
	funcs := []func() error{
		func() error {
			return invoiceSvc.chargeDueResource(context.Background(), db, resource.ID, now)
		},
		func() error {
			return invoiceSvc.PayDueResource(context.Background(), user.ID, models.BillableTypeProject, project.ID, now)
		},
	}

	errs := runConcurrentCalls(t, funcs)
	for i, cErr := range errs {
		if cErr != nil && !errors.Is(cErr, ErrResourcePaymentNotDue) {
			t.Fatalf("unexpected error in scheduler vs manual payment call %d: %v", i, cErr)
		}
	}

	var finalRes models.BillableResource
	if err := db.First(&finalRes, resource.ID).Error; err != nil {
		t.Fatal(err)
	}
	if finalRes.BillingStatus != models.BillableResourceStatusActive {
		t.Fatalf("expected resource to be active after concurrent settlement, got %s", finalRes.BillingStatus)
	}
}

func TestTopupServicePostgresConcurrentCreateRetryVsWebhookSettlement(t *testing.T) {
	db := walletPostgresTestDB(t)
	user := createPostgresWalletTestUser(t, db)
	credits := int64(870000000 + time.Now().UTC().UnixNano()%99999999)
	pkg := models.TopupPackage{
		Currency:    models.BillingCurrencyIDR,
		Credits:     credits,
		AmountMinor: credits,
		Version:     1,
		IsActive:    true,
	}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}
	gateway := &fakeMidtransGateway{}
	service := NewTopupService(db, NewWalletService(db), topupIntegrationConfig(), gateway)
	key := fmt.Sprintf("postgres-concurrent-create-webhook-%d", credits)

	view, err := service.Create(context.Background(), user.ID, key, TopupInput{PackageID: pkg.ID})
	if err != nil {
		t.Fatal(err)
	}
	var topup models.Topup
	if err := db.First(&topup, view.ID).Error; err != nil {
		t.Fatal(err)
	}

	notification := signedNotificationForAmount(topup.ProviderOrderID, "settlement", "accept", fmt.Sprintf("tx-settle-%d", credits), credits)
	funcs := []func() error{
		func() error {
			_, err := service.Create(context.Background(), user.ID, key, TopupInput{PackageID: pkg.ID})
			return err
		},
		func() error {
			return service.ProcessReconciledNotification(context.Background(), notification)
		},
	}

	errs := runConcurrentCalls(t, funcs)
	for i, cErr := range errs {
		if cErr != nil {
			t.Fatalf("unexpected error in create retry vs webhook call %d: %v", i, cErr)
		}
	}
}

func TestCatalogServicePostgresInvoiceSearch(t *testing.T) {
	db := walletPostgresTestDB(t)
	user1 := createPostgresWalletTestUser(t, db)
	user2 := createPostgresWalletTestUser(t, db)
	now := time.Now().UTC()

	walletService := NewWalletService(db)
	wallet1, _ := walletService.GetOrCreateWallet(context.Background(), user1.ID)
	wallet2, _ := walletService.GetOrCreateWallet(context.Background(), user2.ID)

	inv1 := models.Invoice{
		UserID:         user1.ID,
		WalletID:       wallet1.ID,
		InvoiceNumber:  fmt.Sprintf("INV-%d-0001", user1.ID),
		PeriodStart:    now,
		PeriodEnd:      now.AddDate(0, 1, 0),
		TotalCredits:   100,
		Status:         models.InvoiceStatusPaid,
		IdempotencyKey: fmt.Sprintf("inv1-%d", user1.ID),
	}
	if err := db.Create(&inv1).Error; err != nil {
		t.Fatal(err)
	}
	inv2 := models.Invoice{
		UserID:         user1.ID,
		WalletID:       wallet1.ID,
		InvoiceNumber:  fmt.Sprintf("INV_SPEC_%d%%TEST_0002", user1.ID),
		PeriodStart:    now.AddDate(0, 1, 0),
		PeriodEnd:      now.AddDate(0, 2, 0),
		TotalCredits:   200,
		Status:         models.InvoiceStatusPaymentDue,
		IdempotencyKey: fmt.Sprintf("inv2-%d", user1.ID),
	}
	if err := db.Create(&inv2).Error; err != nil {
		t.Fatal(err)
	}
	invUser2 := models.Invoice{
		UserID:         user2.ID,
		WalletID:       wallet2.ID,
		InvoiceNumber:  fmt.Sprintf("INV-%d-9999", user2.ID),
		PeriodStart:    now,
		PeriodEnd:      now.AddDate(0, 1, 0),
		TotalCredits:   300,
		Status:         models.InvoiceStatusPaid,
		IdempotencyKey: fmt.Sprintf("inv-u2-%d", user2.ID),
	}
	if err := db.Create(&invUser2).Error; err != nil {
		t.Fatal(err)
	}

	catalog := NewCatalogService(db)

	// 1. Case-insensitive search: lowercase matches on PostgreSQL
	searchLower := strings.ToLower(inv1.InvoiceNumber)
	res, err := catalog.ListUserInvoices(context.Background(), user1.ID, 1, 10, ListInvoicesFilter{
		Search: searchLower,
	})
	if err != nil {
		t.Fatalf("case-insensitive search failed: %v", err)
	}
	if res.Total != 1 || len(res.Data) != 1 || res.Data[0].InvoiceNumber != inv1.InvoiceNumber {
		t.Fatalf("expected 1 result for lowercase search %q, got total=%d", searchLower, res.Total)
	}

	// 2. Literal wildcard search: "%TEST" matches "INV_SPEC_<id>%TEST_0002"
	resWildcard, err := catalog.ListUserInvoices(context.Background(), user1.ID, 1, 10, ListInvoicesFilter{
		Search: "%TEST",
	})
	if err != nil {
		t.Fatalf("literal wildcard search failed: %v", err)
	}
	if resWildcard.Total != 1 || len(resWildcard.Data) != 1 || resWildcard.Data[0].InvoiceNumber != inv2.InvoiceNumber {
		t.Fatalf("expected 1 match for literal %%TEST, got total=%d", resWildcard.Total)
	}

	// 3. Invalid status returns ErrInvalidCatalogInput
	_, errInvalid := catalog.ListUserInvoices(context.Background(), user1.ID, 1, 10, ListInvoicesFilter{
		Status: "INVALID_STATUS_INJECTION",
	})
	if !errors.Is(errInvalid, ErrInvalidCatalogInput) {
		t.Fatalf("expected ErrInvalidCatalogInput for invalid status, got: %v", errInvalid)
	}

	// 4. Max search length bounded (101 ASCII characters returns ErrInvalidCatalogInput)
	_, errLong := catalog.ListUserInvoices(context.Background(), user1.ID, 1, 10, ListInvoicesFilter{
		Search: strings.Repeat("A", 101),
	})
	if !errors.Is(errLong, ErrInvalidCatalogInput) {
		t.Fatalf("expected ErrInvalidCatalogInput for 101 ASCII chars, got: %v", errLong)
	}

	// 5. UTF-8 multi-byte boundary validation (101 Japanese runes returns ErrInvalidCatalogInput)
	_, errLongRune := catalog.ListUserInvoices(context.Background(), user1.ID, 1, 10, ListInvoicesFilter{
		Search: strings.Repeat("日", 101),
	})
	if !errors.Is(errLongRune, ErrInvalidCatalogInput) {
		t.Fatalf("expected ErrInvalidCatalogInput for 101 Japanese runes, got: %v", errLongRune)
	}

	// 6. Valid 100-rune UTF-8 multi-byte search does not crash or cause DB encoding error
	resRune, err := catalog.ListUserInvoices(context.Background(), user1.ID, 1, 10, ListInvoicesFilter{
		Search: strings.Repeat("日", 100),
	})
	if err != nil {
		t.Fatalf("100-rune multibyte search failed with error: %v", err)
	}
	if resRune.Total != 0 {
		t.Fatalf("expected 0 matches for 100-rune query, got %d", resRune.Total)
	}

	// 7. Tenant isolation: user 1 cannot find user 2's invoice
	resTenant, err := catalog.ListUserInvoices(context.Background(), user1.ID, 1, 10, ListInvoicesFilter{
		Search: "9999",
	})
	if err != nil {
		t.Fatalf("tenant isolation search failed: %v", err)
	}
	if resTenant.Total != 0 {
		t.Fatalf("tenant isolation breach: user 1 saw user 2 invoice")
	}
}

func TestTopupServicePostgresCrossWalletReversalIndependence(t *testing.T) {
	db := walletPostgresTestDB(t)
	user1 := createPostgresWalletTestUser(t, db)
	user2 := createPostgresWalletTestUser(t, db)
	credits := int64(980000000 + time.Now().UTC().UnixNano()%99999999)

	pkg := models.TopupPackage{
		Currency:    models.BillingCurrencyIDR,
		Credits:     credits,
		AmountMinor: credits,
		Version:     1,
		IsActive:    true,
	}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}

	walletService := NewWalletService(db)
	wallet1, err := walletService.GetOrCreateWallet(context.Background(), user1.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = walletService.GetOrCreateWallet(context.Background(), user2.ID)
	if err != nil {
		t.Fatal(err)
	}

	gateway := &fakeMidtransGateway{}
	service := NewTopupService(db, walletService, topupIntegrationConfig(), gateway)

	// User 2 creates a topup
	view2, err := service.Create(context.Background(), user2.ID, fmt.Sprintf("pg-user2-topup-%d", credits), TopupInput{PackageID: pkg.ID})
	if err != nil {
		t.Fatal(err)
	}
	var topup2 models.Topup
	if err := db.First(&topup2, view2.ID).Error; err != nil {
		t.Fatal(err)
	}

	// Settle User 2's topup
	txID := fmt.Sprintf("pg-tx-u2-%d", credits)
	if err := service.ProcessNotification(context.Background(), signedNotificationForAmount(topup2.ProviderOrderID, "settlement", "accept", txID, credits)); err != nil {
		t.Fatal(err)
	}

	// Manually inject a stale reversal entry into User 1's wallet with reference_id matching topup2.ID
	refType := "topup"
	refID := strconv.FormatUint(uint64(topup2.ID), 10)
	staleEntry := models.WalletLedgerEntry{
		WalletID:       wallet1.ID,
		Type:           models.WalletLedgerEntryTopupReversal,
		AmountCredits:  -credits,
		BalanceAfter:   -credits,
		ReferenceType:  &refType,
		ReferenceID:    &refID,
		IdempotencyKey: fmt.Sprintf("pg-stale-u1-rev-%d", credits),
	}
	if err := db.Create(&staleEntry).Error; err != nil {
		t.Fatal(err)
	}

	// Reversal for User 2's topup MUST succeed and MUST NOT be blocked by User 1's ledger entry
	gateway.status = signedNotificationForAmount(topup2.ProviderOrderID, "refund", "", txID, credits)
	if err := service.ProcessNotification(context.Background(), signedNotificationForAmount(topup2.ProviderOrderID, "refund", "", txID, credits)); err != nil {
		t.Fatalf("cross-wallet reversal for user 2 failed: %v", err)
	}

	// Verify User 2's wallet balance was deducted
	wallet2After, err := walletService.GetOrCreateWallet(context.Background(), user2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if wallet2After.BalanceCredits != 0 {
		t.Fatalf("expected user 2 wallet balance to be 0 after refund, got %d", wallet2After.BalanceCredits)
	}

	assertTopupState(t, db, user2.ID, topup2.ID, models.TopupStatusRefunded, 0, 2)
}

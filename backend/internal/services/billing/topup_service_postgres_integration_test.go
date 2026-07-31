//go:build integration

package billing

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
)

func TestTopupServicePostgresConcurrentWebhookCreditsOnce(t *testing.T) {
	db := walletPostgresTestDB(t)
	user := createPostgresWalletTestUser(t, db)
	credits := int64(800000000 + time.Now().UTC().UnixNano()%99999999)
	pkg := models.TopupPackage{Provider: models.BillingProviderMidtrans, Currency: models.BillingCurrencyIDR, Credits: credits, AmountMinor: credits, Version: 1, IsActive: true}
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
	notification := signedNotification(topup.ProviderOrderID, "settlement", "accept", fmt.Sprintf("transaction-%d", credits))
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

func TestTopupServicePostgresConcurrentReplayAndWebhook(t *testing.T) {
	db := walletPostgresTestDB(t)
	user := createPostgresWalletTestUser(t, db)
	credits := int64(700000000 + time.Now().UTC().UnixNano()%99999999)
	pkg := models.TopupPackage{Provider: models.BillingProviderMidtrans, Currency: models.BillingCurrencyIDR, Credits: credits, AmountMinor: credits, Version: 1, IsActive: true}
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
	notification := signedNotification(topup.ProviderOrderID, "settlement", "accept", fmt.Sprintf("transaction-%d", credits))
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
	return &config.Config{BillingEnabled: true, BillingTopupEnabled: true, MidtransServerKey: "server-key", MidtransMerchantID: "merchant-id"}
}

func TestTopupServicePostgresConcurrentIdenticalCreate(t *testing.T) {
	db := walletPostgresTestDB(t)
	user := createPostgresWalletTestUser(t, db)
	credits := int64(600000000 + time.Now().UTC().UnixNano()%99999999)
	pkg := models.TopupPackage{Provider: models.BillingProviderMidtrans, Currency: models.BillingCurrencyIDR, Credits: credits, AmountMinor: credits, Version: 1, IsActive: true}
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
	pkg := models.TopupPackage{Provider: models.BillingProviderMidtrans, Currency: models.BillingCurrencyIDR, Credits: credits, AmountMinor: credits, Version: 1, IsActive: true}
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
	pkg := models.TopupPackage{Provider: models.BillingProviderMidtrans, Currency: models.BillingCurrencyIDR, Credits: credits, AmountMinor: credits, Version: 1, IsActive: true}
	if err := db.Create(&pkg).Error; err != nil {
		t.Fatal(err)
	}
	service := NewTopupService(db, NewWalletService(db), topupIntegrationConfig(), &fakeMidtransGateway{})
	view, err := service.Create(context.Background(), user.ID, fmt.Sprintf("postgres-transition-%d", credits), TopupInput{PackageID: pkg.ID})
	if err != nil {
		t.Fatal(err)
	}
	var topup models.Topup
	if err := db.First(&topup, view.ID).Error; err != nil {
		t.Fatal(err)
	}
	transactionID := fmt.Sprintf("transition-%d", credits)
	for _, err := range runConcurrentCalls(t, []func() error{
		func() error {
			return service.ProcessNotification(context.Background(), signedNotification(topup.ProviderOrderID, "settlement", "accept", transactionID))
		},
		func() error {
			return service.ProcessNotification(context.Background(), signedNotification(topup.ProviderOrderID, "refund", "", transactionID))
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
	pkg := models.TopupPackage{Provider: models.BillingProviderMidtrans, Currency: models.BillingCurrencyIDR, Credits: credits, AmountMinor: credits, Version: 1, IsActive: true}
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
	if err := service.ProcessNotification(context.Background(), signedNotification(topup.ProviderOrderID, "settlement", "accept", fmt.Sprintf("rollback-%d", credits))); err == nil {
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
	pkg := models.TopupPackage{Provider: models.BillingProviderMidtrans, Currency: models.BillingCurrencyIDR, Credits: credits, AmountMinor: credits, Version: 1, IsActive: true}
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
	paid := signedNotification(topup.ProviderOrderID, "settlement", "accept", fmt.Sprintf("terminal-%d", credits))
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

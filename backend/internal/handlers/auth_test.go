package handlers

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/go-redis/redismock/v9"
	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/backend/internal/middleware"
	"github.com/laravel-paas/backend/internal/services"
	"github.com/laravel-paas/backend/internal/services/billing"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/repositories"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func TestReturnToAdminDoesNotAuditSuccessWhenRevocationFails(t *testing.T) {
	for _, tc := range []struct {
		name    string
		failure int
	}{
		{name: "impersonated session", failure: 1},
		{name: "admin backup session", failure: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, db, mock := returnToAdminTestApp(t, tc.failure)
			req := httptest.NewRequest(http.MethodPost, "/return", nil)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != http.StatusServiceUnavailable {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("status = %d body = %s", resp.StatusCode, body)
			}
			var succeeded int64
			if err := db.Model(&models.ImpersonationAudit{}).Where("event = ? AND result = ?", "return", "succeeded").Count(&succeeded).Error; err != nil {
				t.Fatal(err)
			}
			if succeeded != 0 {
				t.Fatal("successful return audit written after revocation failure")
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func returnToAdminTestApp(t *testing.T, failure int) (*fiber.App, *gorm.DB, redismock.ClientMock) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:return_to_admin_%d?mode=memory&cache=shared", failure)), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Project{}, &models.ImpersonationAudit{}); err != nil {
		t.Fatal(err)
	}
	admin := models.User{Email: "admin-return@example.test", Password: "test", Name: "Admin", Role: models.RoleAdmin}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatal(err)
	}
	user := models.User{Email: "user-return@example.test", Password: "test", Name: "User", Role: models.RoleUser}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{JWTSecret: "abcdefghijklmnopqrstuvwxyz123456", JWTKeyID: "current", JWTIssuer: "runara", JWTAudience: "runara-api", JWTExpiryHours: 24, CSRFSecret: "abcdefghijklmnopqrstuvwxyz123456"}
	client, mock := redismock.NewClientMock()
	redis := infrastructure.NewRedisServiceWithClient(client)
	auth := services.NewAuthService(repositories.NewUserRepository(db), cfg, redis)
	impersonated, err := auth.IssueSession(&user, admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	backup, err := auth.IssueSession(&admin, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := auth.Verify(backup.Token, models.TokenUseSession); err != nil {
		t.Fatalf("backup token invalid: %v", err)
	}
	mock.Regexp().ExpectExists("auth:revoked:.*").SetVal(0)
	if failure == 1 {
		mock.CustomMatch(func(expected, actual []any) error {
			return nil
		}).ExpectSet("auth:revoked:expected", true, time.Hour).SetErr(errors.New("redis unavailable"))
	} else {
		mock.CustomMatch(func(expected, actual []any) error {
			return nil
		}).ExpectSet("auth:revoked:expected", true, time.Hour).SetVal("OK")
		mock.CustomMatch(func(expected, actual []any) error {
			return nil
		}).ExpectSet("auth:revoked:expected", true, time.Hour).SetErr(errors.New("redis unavailable"))
	}
	handler := NewAuthHandler(auth, cfg, services.NewUserService(repositories.NewUserRepository(db), nil), db)
	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
	app.Post("/return", func(c *fiber.Ctx) error {
		c.Locals("claims", impersonated.Claims)
		c.Request().Header.Set("Cookie", "paas_admin_session="+backup.Token)
		return handler.ReturnToAdmin(c)
	})
	return app, db, mock
}

func TestReauthenticateReplacesBrowserSession(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Project{}); err != nil {
		t.Fatal(err)
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	user := models.User{Email: "reauth@example.test", Password: string(passwordHash), Name: "Reauth", Role: models.RoleSuperAdmin}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{JWTSecret: "abcdefghijklmnopqrstuvwxyz123456", JWTKeyID: "current", JWTIssuer: "runara", JWTAudience: "runara-api", JWTExpiryHours: 24, CSRFSecret: "abcdefghijklmnopqrstuvwxyz123456"}
	client, mock := redismock.NewClientMock()
	auth := services.NewAuthService(repositories.NewUserRepository(db), cfg, infrastructure.NewRedisServiceWithClient(client))
	oldSession, err := auth.IssueSession(&user, 0)
	if err != nil {
		t.Fatal(err)
	}
	mock.CustomMatch(func(expected, actual []any) error { return nil }).ExpectSet("auth:revoked:expected", true, time.Hour).SetVal("OK")
	handler := NewAuthHandler(auth, cfg, nil, db)
	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
	app.Post("/re-auth", func(c *fiber.Ctx) error {
		c.Locals("claims", oldSession.Claims)
		c.Locals("user_id", user.ID)
		c.Locals("token", oldSession.Token)
		return handler.Reauthenticate(c)
	})
	req := httptest.NewRequest(http.MethodPost, "/re-auth", strings.NewReader(`{"password":"correct-password"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", "paas_session="+oldSession.Token)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body = %s", resp.StatusCode, body)
	}
	if !strings.Contains(strings.Join(resp.Header.Values("Set-Cookie"), "\n"), "paas_session=") {
		t.Fatal("reauthentication did not issue a replacement session cookie")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReauthenticateWithInvalidPasswordRejects(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Project{}); err != nil {
		t.Fatal(err)
	}
	passwordHash, _ := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)
	user := models.User{Email: "wrongpass@example.test", Password: string(passwordHash), Name: "Reauth", Role: models.RoleSuperAdmin}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{JWTSecret: "abcdefghijklmnopqrstuvwxyz123456", JWTKeyID: "current", JWTIssuer: "runara", JWTAudience: "runara-api", JWTExpiryHours: 24, CSRFSecret: "abcdefghijklmnopqrstuvwxyz123456"}
	client, mock := redismock.NewClientMock()
	auth := services.NewAuthService(repositories.NewUserRepository(db), cfg, infrastructure.NewRedisServiceWithClient(client))
	oldSession, err := auth.IssueSession(&user, 0)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewAuthHandler(auth, cfg, nil, db)
	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
	app.Post("/re-auth", func(c *fiber.Ctx) error {
		c.Locals("claims", oldSession.Claims)
		c.Locals("user_id", user.ID)
		c.Locals("token", oldSession.Token)
		return handler.Reauthenticate(c)
	})
	req := httptest.NewRequest(http.MethodPost, "/re-auth", strings.NewReader(`{"password":"wrong-password"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", "paas_session="+oldSession.Token)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong password, got %d", resp.StatusCode)
	}
	if strings.Contains(strings.Join(resp.Header.Values("Set-Cookie"), "\n"), "paas_session=") {
		t.Fatal("replacement session cookie issued on wrong password")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReauthenticateImpersonatedSessionBlocked(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Project{}); err != nil {
		t.Fatal(err)
	}
	user := models.User{Email: "impersonated@example.test", Password: "test", Name: "User", Role: models.RoleUser}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{JWTSecret: "abcdefghijklmnopqrstuvwxyz123456", JWTKeyID: "current", JWTIssuer: "runara", JWTAudience: "runara-api", JWTExpiryHours: 24, CSRFSecret: "abcdefghijklmnopqrstuvwxyz123456"}
	client, mock := redismock.NewClientMock()
	auth := services.NewAuthService(repositories.NewUserRepository(db), cfg, infrastructure.NewRedisServiceWithClient(client))
	impersonatedSession, err := auth.IssueSession(&user, 99)
	if err != nil {
		t.Fatal(err)
	}
	handler := NewAuthHandler(auth, cfg, nil, db)
	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
	app.Post("/re-auth", func(c *fiber.Ctx) error {
		c.Locals("claims", impersonatedSession.Claims)
		c.Locals("user_id", user.ID)
		c.Locals("token", impersonatedSession.Token)
		c.Locals("impersonating", true)
		return handler.Reauthenticate(c)
	})
	req := httptest.NewRequest(http.MethodPost, "/re-auth", strings.NewReader(`{"password":"test"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", "paas_session="+impersonatedSession.Token)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for impersonated reauth, got %d", resp.StatusCode)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStaleSessionRecoversAccessToProtectedBillingRouteAfterReauth(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.Project{}, &models.Setting{}, &models.BillingAuditEvent{}, &models.GithubAppInstallation{}); err != nil {
		t.Fatal(err)
	}
	passwordHash, _ := bcrypt.GenerateFromPassword([]byte("super-secret"), bcrypt.MinCost)
	superadmin := models.User{Email: "superadmin-recover@example.test", Password: string(passwordHash), Name: "SuperAdmin", Role: models.RoleSuperAdmin}
	if err := db.Create(&superadmin).Error; err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		JWTSecret:           "abcdefghijklmnopqrstuvwxyz123456",
		JWTKeyID:            "current",
		JWTIssuer:           "runara",
		JWTAudience:         "runara-api",
		JWTExpiryHours:      24,
		CSRFSecret:          "abcdefghijklmnopqrstuvwxyz123456",
		FrontendURL:         "http://example.test",
		BillingEnabled:      true,
		BillingTopupEnabled: true,
		PakasirEnabled:      true,
		PakasirProjectSlug:  "slug-1",
		PakasirAPIKey:       "key-1",
	}

	client, mock := redismock.NewClientMock()
	// Set up Redis expectations:
	// 1. Initial mutation JWTAuth check: verify old session JTI is not revoked
	mock.CustomMatch(func(expected, actual []any) error { return nil }).ExpectExists("auth:revoked:expected").SetVal(0)
	// 2. Re-auth request JWTAuth check: verify old session JTI is not revoked
	mock.CustomMatch(func(expected, actual []any) error { return nil }).ExpectExists("auth:revoked:expected").SetVal(0)
	// 3. Re-auth execution: revoke old session JTI
	mock.CustomMatch(func(expected, actual []any) error { return nil }).ExpectSet("auth:revoked:expected", true, time.Hour).SetVal("OK")
	// 4. Retried mutation JWTAuth check: verify new session JTI is not revoked
	mock.CustomMatch(func(expected, actual []any) error { return nil }).ExpectExists("auth:revoked:expected").SetVal(0)

	auth := services.NewAuthService(repositories.NewUserRepository(db), cfg, infrastructure.NewRedisServiceWithClient(client))
	catalogService := billing.NewCatalogService(db, cfg)
	billingHandler := NewBillingHandler(catalogService)
	authHandler := NewAuthHandler(auth, cfg, nil, db)

	// Create old session (legacy / stale)
	oldSession, err := auth.IssueSession(&superadmin, 0)
	if err != nil {
		t.Fatal(err)
	}

	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
	// Mount full production JWTAuth and CSRF validation middleware
	protected := app.Group("", middleware.JWTAuth(cfg, auth, auth, nil))
	protected.Post("/re-auth", authHandler.Reauthenticate)
	protected.Put("/admin/billing/payment-provider",
		middleware.RequireSuperAdmin(),
		middleware.RequireNoBillingImpersonation(),
		middleware.RequireRecentBillingAuthentication(cfg),
		billingHandler.UpdatePaymentProvider,
	)

	// Step 1: Initial mutation with stale/legacy session passes JWTAuth and CSRF,
	// but is blocked by RequireRecentBillingAuthentication (403 RECENT_AUTH_REQUIRED)
	req1 := httptest.NewRequest(http.MethodPut, "/admin/billing/payment-provider", strings.NewReader(`{"provider":"pakasir","reason":"Initial config"}`))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Origin", "http://example.test")
	req1.Header.Set("X-CSRF-Token", oldSession.CSRFToken)
	req1.Header.Set("X-Request-ID", "req-test-reauth-1")
	req1.Header.Set("Cookie", fmt.Sprintf("paas_session=%s; paas_csrf=%s", oldSession.Token, oldSession.CSRFToken))
	resp1, err := app.Test(req1)
	if err != nil {
		t.Fatal(err)
	}
	if resp1.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(resp1.Body)
		t.Fatalf("expected 403 for initial stale session, got %d body = %s", resp1.StatusCode, body)
	}

	// Step 2: Re-authenticate with superadmin password via POST /re-auth -> 204 No Content and replacement cookies issued
	req2 := httptest.NewRequest(http.MethodPost, "/re-auth", strings.NewReader(`{"password":"super-secret"}`))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Origin", "http://example.test")
	req2.Header.Set("X-CSRF-Token", oldSession.CSRFToken)
	req2.Header.Set("X-Request-ID", "req-test-reauth-2")
	req2.Header.Set("Cookie", fmt.Sprintf("paas_session=%s; paas_csrf=%s", oldSession.Token, oldSession.CSRFToken))
	resp2, err := app.Test(req2)
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp2.Body)
		t.Fatalf("expected 204 for reauth, got %d body = %s", resp2.StatusCode, body)
	}

	// Extract new session and CSRF tokens from cookies
	var newSessionToken, newCSRFToken string
	for _, cookie := range resp2.Cookies() {
		switch cookie.Name {
		case "paas_session":
			newSessionToken = cookie.Value
		case "paas_csrf":
			newCSRFToken = cookie.Value
		}
	}
	if newSessionToken == "" || newSessionToken == oldSession.Token {
		t.Fatalf("expected replacement session token, got %q", newSessionToken)
	}
	if newCSRFToken == "" || newCSRFToken == oldSession.CSRFToken {
		t.Fatalf("expected replacement CSRF token, got %q", newCSRFToken)
	}

	// Step 3: Retry original mutation with replacement session, replacement CSRF cookie, and replacement X-CSRF-Token header -> 200 OK!
	req3 := httptest.NewRequest(http.MethodPut, "/admin/billing/payment-provider", strings.NewReader(`{"provider":"pakasir","reason":"Initial config"}`))
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("Origin", "http://example.test")
	req3.Header.Set("X-CSRF-Token", newCSRFToken)
	req3.Header.Set("X-Request-ID", "req-test-reauth-3")
	req3.Header.Set("Cookie", fmt.Sprintf("paas_session=%s; paas_csrf=%s", newSessionToken, newCSRFToken))
	resp3, err := app.Test(req3)
	if err != nil {
		t.Fatal(err)
	}
	if resp3.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp3.Body)
		t.Fatalf("expected 200 for retried request with replacement session and CSRF, got %d body = %s", resp3.StatusCode, body)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

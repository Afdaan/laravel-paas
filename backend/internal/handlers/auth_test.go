package handlers

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/go-redis/redismock/v9"
	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/backend/internal/services"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/repositories"
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

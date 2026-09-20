package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/repositories"
	"github.com/laravel-paas/shared/services/setting"
	"gorm.io/gorm"
)

func TestSettingHandlerRejectsDirectPaymentProviderMutation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Setting{}); err != nil {
		t.Fatal(err)
	}

	settingRepo := repositories.NewSettingRepository(db)
	settingService := setting.NewSettingService(settingRepo, nil)
	handler := NewSettingHandler(settingService, &config.Config{})

	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler})
	app.Put("/admin/settings", handler.Update)

	body := `{"settings":{"default_payment_provider":"midtrans"}}`
	req := httptest.NewRequest(http.MethodPut, "/admin/settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400 Bad Request, got %d", resp.StatusCode)
	}
}

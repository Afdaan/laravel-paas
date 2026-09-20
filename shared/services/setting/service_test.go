package setting

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/go-redis/redismock/v9"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/repositories"
	"gorm.io/gorm"
)

func TestSettingServiceBypassesCacheForDefaultPaymentProvider(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Setting{}); err != nil {
		t.Fatal(err)
	}

	repo := repositories.NewSettingRepository(db)
	client, mock := redismock.NewClientMock()
	redisSvc := infrastructure.NewRedisServiceWithClient(client)
	service := NewSettingService(repo, redisSvc)

	// Insert setting in DB
	if err := repo.Upsert(models.SettingDefaultPaymentProvider, "midtrans"); err != nil {
		t.Fatal(err)
	}

	// SettingDefaultPaymentProvider should query DB directly and not call Redis GetCache/SetCache
	val := service.Get(models.SettingDefaultPaymentProvider, "pakasir")
	if val != "midtrans" {
		t.Fatalf("expected midtrans from DB, got %s", val)
	}

	// Normal setting (e.g. SettingAdminIdleTimeout) should query Redis then DB then cache to Redis
	mock.ExpectGet("setting:" + models.SettingAdminIdleTimeout).RedisNil()
	mock.ExpectSet("setting:"+models.SettingAdminIdleTimeout, []byte("\"30\""), 24*time.Hour).SetVal("OK")

	valNormal := service.Get(models.SettingAdminIdleTimeout, "30")
	if valNormal != "30" {
		t.Fatalf("expected 30, got %s", valNormal)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet redis expectations: %v", err)
	}
}

func TestSettingServiceInvalidateReturnsErrorOnRedisFailure(t *testing.T) {
	client, mock := redismock.NewClientMock()
	redisSvc := infrastructure.NewRedisServiceWithClient(client)
	service := NewSettingService(nil, redisSvc)

	mock.ExpectDel("setting:test_key").SetErr(errors.New("redis connection refused"))

	err := service.Invalidate("test_key")
	if err == nil {
		t.Fatal("expected error on redis failure, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet redis expectations: %v", err)
	}
}

func TestSettingServiceInvalidateNoOpForUncachedSettings(t *testing.T) {
	client, mock := redismock.NewClientMock()
	redisSvc := infrastructure.NewRedisServiceWithClient(client)
	service := NewSettingService(nil, redisSvc)

	// No expectations set on Redis. Invalidate on SettingDefaultPaymentProvider must be a no-op
	if err := service.Invalidate(models.SettingDefaultPaymentProvider); err != nil {
		t.Fatalf("expected nil on uncached setting invalidation, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet redis expectations: %v", err)
	}
}

func TestSettingServiceGetUncached(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Setting{}); err != nil {
		t.Fatal(err)
	}

	repo := repositories.NewSettingRepository(db)
	service := NewSettingService(repo, nil)

	// 1. Missing setting returns found=false, err=nil
	val, found, err := service.GetUncached(context.Background(), models.SettingDefaultPaymentProvider)
	if err != nil || found || val != "" {
		t.Fatalf("expected found=false err=nil, got val=%q found=%t err=%v", val, found, err)
	}

	// 2. Existing setting returns val, found=true, err=nil
	if err := repo.Upsert(models.SettingDefaultPaymentProvider, "midtrans"); err != nil {
		t.Fatal(err)
	}
	val, found, err = service.GetUncached(context.Background(), models.SettingDefaultPaymentProvider)
	if err != nil || !found || val != "midtrans" {
		t.Fatalf("expected midtrans found=true, got val=%q found=%t err=%v", val, found, err)
	}

	// 3. Canceled context returns context error
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = service.GetUncached(canceledCtx, models.SettingDefaultPaymentProvider)
	if err == nil {
		t.Fatal("expected error on canceled context, got nil")
	}
}

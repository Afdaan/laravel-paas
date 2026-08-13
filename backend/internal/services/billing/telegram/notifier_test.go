package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
)

func TestTelegramNotifierFormatMessage(t *testing.T) {
	paidAt := time.Date(2026, 8, 13, 13, 44, 0, 0, time.UTC)
	msg := NotificationMessage{
		OrderID:        "order-topup-123",
		UserEmail:      "test@example.com",
		UserID:         42,
		AmountMinor:    50000,
		Currency:       "IDR",
		Credits:        50000,
		BalanceBefore:  100000,
		BalanceAfter:   150000,
		HasBalanceInfo: true,
		Provider:       "pakasir",
		Status:         models.TopupStatusPaid,
		PaidAt:         &paidAt,
	}

	text := formatTelegramMessage(msg)
	if text == "" {
		t.Fatal("expected formatted message, got empty string")
	}

	if !containsSubstring(text, "TOP-UP SUCCESSFUL") || !containsSubstring(text, "test@example.com") || !containsSubstring(text, "Rp 50.000") || !containsSubstring(text, "Pakasir") || !containsSubstring(text, "100000 ➔ <b>150000 Credits</b>") {
		t.Fatalf("unexpected formatted text: %s", text)
	}
}

func TestTelegramNotifierSendMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/bottest-token/sendMessage" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		var payload map[string]interface{}
		json.NewDecoder(r.Body).Decode(&payload)

		if payload["chat_id"] != "123456789" || payload["parse_mode"] != "HTML" {
			t.Errorf("unexpected payload: %v", payload)
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
	}))
	defer server.Close()

	client := NewClient(&config.Config{
		TelegramBotPaymentEnabled: true,
		TelegramBotPaymentToken:   "test-token",
		TelegramBotPaymentChatID:  "123456789",
	})
	client.baseURL = server.URL

	err := client.sendMessage(context.Background(), "<b>Test</b>")
	if err != nil {
		t.Fatalf("unexpected error sending message: %v", err)
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || (len(s) > 0 && searchSubstring(s, sub)))
}

func searchSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

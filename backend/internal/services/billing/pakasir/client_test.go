package pakasir

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
)

func TestPakasirClientCreateTransaction(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/transactioncreate/qris" {
			t.Errorf("expected path /transactioncreate/qris, got %s", r.URL.Path)
		}

		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request body: %v", err)
			return
		}

		if req["project"] != "test-project" || req["api_key"] != "test-key" || req["order_id"] != "order-123" {
			t.Errorf("unexpected request body: %v", req)
		}

		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(CreateResponse{
			PaymentNumber: "00020126360014ID.LINKAJA.WWW01189360091100030005020210510408544453033605802ID5911PAAS TOPUP6007JAKARTA61051211062070703A0163041234",
			TotalPayment:  50000,
			Fee:           700,
			ExpiredAt:     "2026-08-11 12:00:00",
		}); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client := NewClient(&config.Config{
		PakasirProjectSlug: "test-project",
		PakasirAPIKey:      "test-key",
	})
	client.baseURL = server.URL

	res, err := client.CreateTransaction(context.Background(), "order-123", 50000, "qris")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.TotalPayment != 50000 || res.PaymentNumber == "" {
		t.Fatalf("unexpected response: %+v", res)
	}
}

func TestPakasirClientGetTransactionDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/transactiondetail" {
			t.Errorf("expected path /transactiondetail, got %s", r.URL.Path)
		}

		q := r.URL.Query()
		if q.Get("project") != "test-project" || q.Get("order_id") != "order-123" || q.Get("amount") != "50000" {
			t.Errorf("unexpected query string: %v", q)
		}

		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(DetailResponse{
			Transaction: TransactionDetail{
				Amount:        50000,
				OrderID:       "order-123",
				Project:       "test-project",
				Status:        "completed",
				PaymentMethod: "qris",
				CompletedAt:   "2026-08-10 10:00:00",
			},
		}); err != nil {
			t.Errorf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	client := NewClient(&config.Config{
		PakasirProjectSlug: "test-project",
		PakasirAPIKey:      "test-key",
	})
	client.baseURL = server.URL

	detail, err := client.GetTransactionDetail(context.Background(), "order-123", 50000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if detail.Status != "completed" || detail.Amount != 50000 {
		t.Fatalf("unexpected detail: %+v", detail)
	}

	status, err := StatusToTopupStatus(detail.Status)
	if err != nil || status != models.TopupStatusPaid {
		t.Fatalf("status mapping error or mismatch: %v, status: %s", err, status)
	}
}

func TestPakasirStatusMappingVariations(t *testing.T) {
	tests := []struct {
		input    string
		expected models.TopupStatus
		wantErr  bool
	}{
		{"completed", models.TopupStatusPaid, false},
		{"paid", models.TopupStatusPaid, false},
		{"settlement", models.TopupStatusPaid, false},
		{"success", models.TopupStatusPaid, false},
		{"COMPLETED", models.TopupStatusPaid, false},
		{" pending ", models.TopupStatusPending, false},
		{"failed", models.TopupStatusFailed, false},
		{"cancelled", models.TopupStatusFailed, false},
		{"canceled", models.TopupStatusFailed, false},
		{"expired", models.TopupStatusExpired, false},
		{"unknown_status", "", true},
	}

	for _, tt := range tests {
		got, err := StatusToTopupStatus(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("StatusToTopupStatus(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if got != tt.expected {
			t.Errorf("StatusToTopupStatus(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func TestPakasirClientGetTransactionDetailError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(&config.Config{
		PakasirProjectSlug: "test-project",
		PakasirAPIKey:      "test-key",
	})
	client.baseURL = server.URL

	_, err := client.GetTransactionDetail(context.Background(), "order-123", 50000)
	if err == nil {
		t.Fatal("expected error for HTTP 500 from Pakasir, got nil")
	}
}

func TestPakasirClientCreateTransactionFailClosed(t *testing.T) {
	// Unconfigured client must fail closed
	client := NewClient(nil)
	_, err := client.CreateTransaction(context.Background(), "order-123", 50000, "qris")
	if err == nil {
		t.Fatal("expected error for unconfigured client, got nil")
	}

	// Server error must fail closed
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	configuredClient := NewClient(&config.Config{
		PakasirProjectSlug: "test-project",
		PakasirAPIKey:      "test-key",
	})
	configuredClient.baseURL = server.URL

	_, err = configuredClient.CreateTransaction(context.Background(), "order-123", 50000, "qris")
	if err == nil {
		t.Fatal("expected error for HTTP 502 from Pakasir, got nil")
	}
}

package billing

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
		json.NewDecoder(r.Body).Decode(&req)

		if req["project"] != "test-project" || req["api_key"] != "test-key" || req["order_id"] != "order-123" {
			t.Errorf("unexpected request body: %v", req)
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(PakasirCreateResponse{
			PaymentNumber: "00020126360014ID.LINKAJA.WWW01189360091100030005020210510408544453033605802ID5911PAAS TOPUP6007JAKARTA61051211062070703A0163041234",
			TotalPayment:  50000,
			Fee:           700,
			ExpiredAt:     "2026-08-11 12:00:00",
		})
	}))
	defer server.Close()

	client := NewPakasirClient(&config.Config{
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
		json.NewEncoder(w).Encode(PakasirDetailResponse{
			Transaction: PakasirTransactionDetail{
				Amount:        50000,
				OrderID:       "order-123",
				Project:       "test-project",
				Status:        "completed",
				PaymentMethod: "qris",
				CompletedAt:   "2026-08-10 10:00:00",
			},
		})
	}))
	defer server.Close()

	client := NewPakasirClient(&config.Config{
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

	status, err := pakAsirStatusToTopupStatus(detail.Status)
	if err != nil || status != models.TopupStatusPaid {
		t.Fatalf("status mapping error or mismatch: %v, status: %s", err, status)
	}
}

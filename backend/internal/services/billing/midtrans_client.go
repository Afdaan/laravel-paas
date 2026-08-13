// ===========================================
// Midtrans Payment Gateway Client
// ===========================================
// Handles communication with Midtrans Snap & Status API
// ===========================================
package billing

import (
	"bytes"
	"context"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
)

const (
	midtransSandboxSnapURL    = "https://app.sandbox.midtrans.com/snap/v1/transactions"
	midtransProductionSnapURL = "https://app.midtrans.com/snap/v1/transactions"
	midtransSandboxAPIURL     = "https://api.sandbox.midtrans.com"
	midtransProductionAPIURL  = "https://api.midtrans.com"
	midtransRequestTimeout    = 10 * time.Second
)

type MidtransPaymentRequest struct {
	OrderID        string
	AmountMinor    int64
	Currency       string
	IdempotencyKey string
	FinishURL      string
}

type MidtransPaymentResponse struct {
	Token       string
	RedirectURL string
}

type MidtransNotification struct {
	OrderID           string `json:"order_id"`
	StatusCode        string `json:"status_code"`
	GrossAmount       string `json:"gross_amount"`
	SignatureKey      string `json:"signature_key"`
	TransactionStatus string `json:"transaction_status"`
	FraudStatus       string `json:"fraud_status"`
	TransactionID     string `json:"transaction_id"`
	RefundAmount      string `json:"refund_amount"`
	Currency          string `json:"currency"`
	MerchantID        string `json:"merchant_id"`
}

type MidtransGateway interface {
	CreatePayment(context.Context, MidtransPaymentRequest) (MidtransPaymentResponse, error)
	GetTransactionStatus(context.Context, string) (MidtransNotification, error)
}

type MidtransClient struct {
	serverKey  string
	httpClient *http.Client
	snapURL    string
	apiURL     string
}

func NewMidtransClient(cfg *config.Config) *MidtransClient {
	snapURL := midtransSandboxSnapURL
	apiURL := midtransSandboxAPIURL
	if cfg != nil && cfg.MidtransProduction {
		snapURL = midtransProductionSnapURL
		apiURL = midtransProductionAPIURL
	}
	serverKey := ""
	if cfg != nil {
		serverKey = cfg.MidtransServerKey
	}
	return &MidtransClient{
		serverKey:  serverKey,
		httpClient: &http.Client{Timeout: midtransRequestTimeout},
		snapURL:    snapURL,
		apiURL:     apiURL,
	}
}

func (c *MidtransClient) CreatePayment(ctx context.Context, payment MidtransPaymentRequest) (MidtransPaymentResponse, error) {
	if c == nil || c.httpClient == nil || c.serverKey == "" || payment.OrderID == "" || payment.AmountMinor <= 0 || payment.Currency != models.BillingCurrencyIDR {
		return MidtransPaymentResponse{}, ErrPaymentProvider
	}
	grossAmount, err := majorUnits(payment.AmountMinor, payment.Currency)
	if err != nil {
		return MidtransPaymentResponse{}, ErrPaymentProvider
	}
	type snapCallbacks struct {
		Finish string `json:"finish,omitempty"`
	}
	body, err := json.Marshal(struct {
		TransactionDetails struct {
			OrderID     string `json:"order_id"`
			GrossAmount int64  `json:"gross_amount"`
		} `json:"transaction_details"`
		Callbacks *snapCallbacks `json:"callbacks,omitempty"`
	}{TransactionDetails: struct {
		OrderID     string `json:"order_id"`
		GrossAmount int64  `json:"gross_amount"`
	}{OrderID: payment.OrderID, GrossAmount: grossAmount}, Callbacks: func() *snapCallbacks {
		if payment.FinishURL != "" {
			return &snapCallbacks{Finish: payment.FinishURL}
		}
		return nil
	}()})
	if err != nil {
		return MidtransPaymentResponse{}, fmt.Errorf("marshal Midtrans payment request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.snapURL, bytes.NewReader(body))
	if err != nil {
		return MidtransPaymentResponse{}, fmt.Errorf("create Midtrans payment request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-Idempotency-Key", payment.IdempotencyKey)
	request.SetBasicAuth(c.serverKey, "")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return MidtransPaymentResponse{}, fmt.Errorf("%w: create payment request", ErrPaymentProvider)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return MidtransPaymentResponse{}, fmt.Errorf("%w: create payment status %d", ErrPaymentProvider, response.StatusCode)
	}
	var parsed struct {
		Token       string `json:"token"`
		RedirectURL string `json:"redirect_url"`
	}
	if err := decodeProviderJSON(io.LimitReader(response.Body, 8*1024), &parsed); err != nil {
		return MidtransPaymentResponse{}, fmt.Errorf("%w: decode payment response", ErrPaymentProvider)
	}
	return MidtransPaymentResponse{Token: parsed.Token, RedirectURL: parsed.RedirectURL}, nil
}

func (c *MidtransClient) GetTransactionStatus(ctx context.Context, orderID string) (MidtransNotification, error) {
	if c == nil || c.httpClient == nil || c.serverKey == "" || orderID == "" {
		return MidtransNotification{}, ErrPaymentProvider
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.apiURL, "/")+"/v2/"+url.PathEscape(orderID)+"/status", nil)
	if err != nil {
		return MidtransNotification{}, fmt.Errorf("create Midtrans status request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.SetBasicAuth(c.serverKey, "")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return MidtransNotification{}, fmt.Errorf("%w: get transaction status", ErrPaymentProvider)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return MidtransNotification{}, fmt.Errorf("%w: transaction status %d", ErrPaymentProvider, response.StatusCode)
	}
	var notification MidtransNotification
	if err := decodeProviderJSON(io.LimitReader(response.Body, 8*1024), &notification); err != nil {
		return MidtransNotification{}, fmt.Errorf("%w: decode transaction status", ErrPaymentProvider)
	}
	return notification, nil
}

func validMidtransSignature(notification MidtransNotification, serverKey string) bool {
	sum := sha512.Sum512([]byte(notification.OrderID + notification.StatusCode + notification.GrossAmount + serverKey))
	expected := hex.EncodeToString(sum[:])
	return len(notification.SignatureKey) == len(expected) && subtle.ConstantTimeCompare([]byte(strings.ToLower(notification.SignatureKey)), []byte(expected)) == 1
}

func topupStatusFromNotification(statusCode, transactionStatus, fraudStatus string) (models.TopupStatus, error) {
	switch transactionStatus {
	case "capture":
		switch fraudStatus {
		case "accept":
			if statusCode == "200" {
				return models.TopupStatusPaid, nil
			}
		case "challenge":
			if statusCode == "201" {
				return models.TopupStatusPending, nil
			}
		case "deny":
			if statusCode == "202" {
				return models.TopupStatusFailed, nil
			}
		}
	case "settlement":
		if statusCode == "200" && (fraudStatus == "" || fraudStatus == "accept") {
			return models.TopupStatusPaid, nil
		}
	case "pending":
		if statusCode == "201" && (fraudStatus == "" || fraudStatus == "accept" || fraudStatus == "challenge") {
			return models.TopupStatusPending, nil
		}
	case "deny", "cancel":
		if statusCode == "202" {
			return models.TopupStatusFailed, nil
		}
	case "expire":
		if statusCode == "202" {
			return models.TopupStatusExpired, nil
		}
	case "refund":
		if statusCode == "200" {
			return models.TopupStatusRefunded, nil
		}
	case "partial_refund":
		if statusCode == "200" {
			return models.TopupStatusPartialRefund, nil
		}
	case "chargeback":
		if statusCode == "200" {
			return models.TopupStatusChargeback, nil
		}
	case "partial_chargeback":
		if statusCode == "200" {
			return models.TopupStatusPartialChargeback, nil
		}
	}
	return "", ErrInvalidPaymentNotification
}

package billing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
)

const (
	pakasirAPIBaseURL = "https://app.pakasir.com/api"
	pakasirTimeout   = 10 * time.Second
)

type PakasirCreateResponse struct {
	PaymentNumber string `json:"payment_number"`
	TotalPayment  int64  `json:"total_payment"`
	Fee           int64  `json:"fee"`
	ExpiredAt     string `json:"expired_at"`
}

type PakasirTransactionDetail struct {
	Amount        int64  `json:"amount"`
	OrderID       string `json:"order_id"`
	Project       string `json:"project"`
	Status        string `json:"status"`
	PaymentMethod string `json:"payment_method"`
	CompletedAt   string `json:"completed_at"`
}

type PakasirDetailResponse struct {
	Transaction PakasirTransactionDetail `json:"transaction"`
}

type PakasirGateway interface {
	CreateTransaction(ctx context.Context, orderID string, amountMinor int64, method string) (PakasirCreateResponse, error)
	GetTransactionDetail(ctx context.Context, orderID string, amountMinor int64) (PakasirTransactionDetail, error)
}

type PakasirClient struct {
	projectSlug string
	apiKey      string
	httpClient  *http.Client
	baseURL     string
}

func NewPakasirClient(cfg *config.Config) *PakasirClient {
	slug, key := "", ""
	if cfg != nil {
		slug = cfg.PakasirProjectSlug
		key = cfg.PakasirAPIKey
	}
	return &PakasirClient{
		projectSlug: slug,
		apiKey:      key,
		httpClient:  &http.Client{Timeout: pakasirTimeout},
		baseURL:     pakasirAPIBaseURL,
	}
}

func (c *PakasirClient) CreateTransaction(ctx context.Context, orderID string, amountMinor int64, method string) (PakasirCreateResponse, error) {
	if c.projectSlug == "" || c.apiKey == "" {
		return PakasirCreateResponse{}, fmt.Errorf("%w: payment gateway credentials incomplete", ErrPaymentProvider)
	}
	if method == "" {
		method = "qris"
	}
	endpoint := fmt.Sprintf("%s/transactioncreate/%s", c.baseURL, method)

	bodyData := map[string]interface{}{
		"project":  c.projectSlug,
		"order_id": orderID,
		"amount":   amountMinor,
		"api_key":  c.apiKey,
	}

	jsonBytes, err := json.Marshal(bodyData)
	if err != nil {
		return PakasirCreateResponse{}, fmt.Errorf("marshal pakasir request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return PakasirCreateResponse{}, fmt.Errorf("create pakasir request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return PakasirCreateResponse{}, fmt.Errorf("%w: execute pakasir transaction create: %v", ErrPaymentProvider, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return PakasirCreateResponse{}, fmt.Errorf("read pakasir create response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errObj struct {
			Message string `json:"message"`
			Error   string `json:"error"`
		}
		_ = json.Unmarshal(respBody, &errObj)
		msg := errObj.Message
		if msg == "" {
			msg = errObj.Error
		}
		if msg == "" {
			msg = string(respBody)
		}
		return PakasirCreateResponse{}, fmt.Errorf("%w: pakasir transaction create status %d: %s", ErrPaymentProvider, resp.StatusCode, msg)
	}

	// Direct unmarshal
	var res PakasirCreateResponse
	if err := json.Unmarshal(respBody, &res); err == nil && res.PaymentNumber != "" {
		return res, nil
	}

	// Nested wrapper check ({ "payment": { ... } } or { "data": { ... } })
	var nestedWrapper struct {
		Payment PakasirCreateResponse `json:"payment"`
		Data    PakasirCreateResponse `json:"data"`
	}
	if err := json.Unmarshal(respBody, &nestedWrapper); err == nil {
		if nestedWrapper.Payment.PaymentNumber != "" {
			return nestedWrapper.Payment, nil
		}
		if nestedWrapper.Data.PaymentNumber != "" {
			return nestedWrapper.Data, nil
		}
	}

	// Fallback message check if Pakasir returned error JSON with 200 status
	var rawMap map[string]interface{}
	if err := json.Unmarshal(respBody, &rawMap); err == nil {
		if msg, ok := rawMap["message"].(string); ok && msg != "" {
			return PakasirCreateResponse{}, fmt.Errorf("%w: pakasir API error: %s", ErrPaymentProvider, msg)
		}
	}

	return res, nil
}

func (c *PakasirClient) GetTransactionDetail(ctx context.Context, orderID string, amountMinor int64) (PakasirTransactionDetail, error) {
	if c.projectSlug == "" || c.apiKey == "" {
		return PakasirTransactionDetail{}, fmt.Errorf("%w: payment gateway credentials incomplete", ErrPaymentProvider)
	}
	u, err := url.Parse(fmt.Sprintf("%s/transactiondetail", c.baseURL))
	if err != nil {
		return PakasirTransactionDetail{}, fmt.Errorf("parse pakasir detail url: %w", err)
	}

	q := u.Query()
	q.Set("project", c.projectSlug)
	q.Set("amount", strconv.FormatInt(amountMinor, 10))
	q.Set("order_id", orderID)
	q.Set("api_key", c.apiKey)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return PakasirTransactionDetail{}, fmt.Errorf("create pakasir detail request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return PakasirTransactionDetail{}, fmt.Errorf("%w: execute pakasir transaction detail: %v", ErrPaymentProvider, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return PakasirTransactionDetail{}, fmt.Errorf("read pakasir detail response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return PakasirTransactionDetail{}, fmt.Errorf("%w: pakasir transaction detail status %d", ErrPaymentProvider, resp.StatusCode)
	}

	var res PakasirDetailResponse
	if err := json.Unmarshal(respBody, &res); err != nil {
		return PakasirTransactionDetail{}, fmt.Errorf("unmarshal pakasir detail response: %w", err)
	}

	return res.Transaction, nil
}

func pakAsirStatusToTopupStatus(status string) (models.TopupStatus, error) {
	switch status {
	case "completed", "paid":
		return models.TopupStatusPaid, nil
	case "pending":
		return models.TopupStatusPending, nil
	case "failed", "cancelled", "expired":
		return models.TopupStatusFailed, nil
	default:
		return "", fmt.Errorf("unknown pakasir status %q", status)
	}
}

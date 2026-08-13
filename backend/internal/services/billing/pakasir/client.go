// ===========================================
// Pakasir Gateway Integration Package
// ===========================================
package pakasir

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
)

var (
	ErrPaymentProvider = errors.New("payment provider unavailable")
)

const (
	pakasirAPIBaseURL = "https://app.pakasir.com/api"
	pakasirTimeout   = 10 * time.Second
)

type CreateResponse struct {
	PaymentNumber string `json:"payment_number"`
	TotalPayment  int64  `json:"total_payment"`
	Fee           int64  `json:"fee"`
	ExpiredAt     string `json:"expired_at"`
}

type TransactionDetail struct {
	Amount        int64  `json:"amount"`
	OrderID       string `json:"order_id"`
	Project       string `json:"project"`
	Status        string `json:"status"`
	PaymentMethod string `json:"payment_method"`
	CompletedAt   string `json:"completed_at"`
}

type DetailResponse struct {
	Transaction TransactionDetail `json:"transaction"`
}

type Gateway interface {
	CreateTransaction(ctx context.Context, orderID string, amountMinor int64, method string) (CreateResponse, error)
	GetTransactionDetail(ctx context.Context, orderID string, amountMinor int64) (TransactionDetail, error)
	SimulatePayment(ctx context.Context, orderID string, amountMinor int64) error
	CancelTransaction(ctx context.Context, orderID string, amountMinor int64) error
}

type Client struct {
	projectSlug string
	apiKey      string
	httpClient  *http.Client
	baseURL     string
}

func NewClient(cfg *config.Config) *Client {
	slug, key := "", ""
	if cfg != nil {
		slug = cfg.PakasirProjectSlug
		key = cfg.PakasirAPIKey
	}
	return &Client{
		projectSlug: slug,
		apiKey:      key,
		httpClient:  &http.Client{Timeout: pakasirTimeout},
		baseURL:     pakasirAPIBaseURL,
	}
}

func (c *Client) CreateTransaction(ctx context.Context, orderID string, amountMinor int64, method string) (CreateResponse, error) {
	if method == "" {
		method = "qris"
	}

	slug := ""
	if c != nil {
		slug = c.projectSlug
	}
	if slug == "" {
		slug = "runara"
	}
	fallbackURL := fmt.Sprintf("https://app.pakasir.com/pay/%s/%d?order_id=%s", slug, amountMinor, orderID)

	if c == nil || c.apiKey == "" || c.projectSlug == "" {
		slog.Warn("Pakasir credentials not configured, falling back to direct payment URL", "project_slug", slug, "order_id", orderID)
		return CreateResponse{
			PaymentNumber: fallbackURL,
			TotalPayment:  amountMinor,
		}, nil
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
		return CreateResponse{PaymentNumber: fallbackURL, TotalPayment: amountMinor}, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return CreateResponse{PaymentNumber: fallbackURL, TotalPayment: amountMinor}, nil
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		slog.Warn("Pakasir API request failed, falling back to direct payment URL", "error", err)
		return CreateResponse{PaymentNumber: fallbackURL, TotalPayment: amountMinor}, nil
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK {
		slog.Warn("Pakasir API returned non-OK status, falling back to direct payment URL", "status", resp.StatusCode)
		return CreateResponse{PaymentNumber: fallbackURL, TotalPayment: amountMinor}, nil
	}

	var res CreateResponse
	if err := json.Unmarshal(respBody, &res); err == nil && res.PaymentNumber != "" {
		return res, nil
	}

	var nestedWrapper struct {
		Payment CreateResponse `json:"payment"`
		Data    CreateResponse `json:"data"`
	}
	if err := json.Unmarshal(respBody, &nestedWrapper); err == nil {
		if nestedWrapper.Payment.PaymentNumber != "" {
			return nestedWrapper.Payment, nil
		}
		if nestedWrapper.Data.PaymentNumber != "" {
			return nestedWrapper.Data, nil
		}
	}

	return CreateResponse{PaymentNumber: fallbackURL, TotalPayment: amountMinor}, nil
}

func (c *Client) GetTransactionDetail(ctx context.Context, orderID string, amountMinor int64) (TransactionDetail, error) {
	if c.projectSlug == "" || c.apiKey == "" {
		return TransactionDetail{}, fmt.Errorf("%w: payment gateway credentials incomplete", ErrPaymentProvider)
	}
	u, err := url.Parse(fmt.Sprintf("%s/transactiondetail", c.baseURL))
	if err != nil {
		return TransactionDetail{}, fmt.Errorf("%w: parse pakasir detail url: %v", ErrPaymentProvider, err)
	}

	q := u.Query()
	q.Set("project", c.projectSlug)
	q.Set("amount", strconv.FormatInt(amountMinor, 10))
	q.Set("order_id", orderID)
	q.Set("api_key", c.apiKey)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return TransactionDetail{}, fmt.Errorf("%w: create pakasir detail request: %v", ErrPaymentProvider, err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return TransactionDetail{}, fmt.Errorf("%w: execute pakasir transaction detail: %v", ErrPaymentProvider, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return TransactionDetail{}, fmt.Errorf("%w: read pakasir detail response: %v", ErrPaymentProvider, err)
	}

	if resp.StatusCode != http.StatusOK {
		return TransactionDetail{}, fmt.Errorf("%w: pakasir transaction detail status %d", ErrPaymentProvider, resp.StatusCode)
	}

	var res DetailResponse
	if err := json.Unmarshal(respBody, &res); err != nil {
		return TransactionDetail{}, fmt.Errorf("%w: unmarshal pakasir detail response: %v", ErrPaymentProvider, err)
	}

	return res.Transaction, nil
}

func (c *Client) SimulatePayment(ctx context.Context, orderID string, amountMinor int64) error {
	if c == nil || c.projectSlug == "" || c.apiKey == "" {
		return fmt.Errorf("%w: pakasir credentials incomplete for simulation", ErrPaymentProvider)
	}
	endpoint := fmt.Sprintf("%s/paymentsimulation", c.baseURL)
	bodyData := map[string]interface{}{
		"project":  c.projectSlug,
		"order_id": orderID,
		"amount":   amountMinor,
		"api_key":  c.apiKey,
	}

	jsonBytes, err := json.Marshal(bodyData)
	if err != nil {
		return fmt.Errorf("marshal pakasir simulation request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return fmt.Errorf("create pakasir simulation request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute pakasir simulation request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("pakasir payment simulation status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (c *Client) CancelTransaction(ctx context.Context, orderID string, amountMinor int64) error {
	if c == nil || c.projectSlug == "" || c.apiKey == "" {
		return fmt.Errorf("%w: pakasir credentials incomplete for cancellation", ErrPaymentProvider)
	}
	endpoint := fmt.Sprintf("%s/transactioncancel", c.baseURL)
	bodyData := map[string]interface{}{
		"project":  c.projectSlug,
		"order_id": orderID,
		"amount":   amountMinor,
		"api_key":  c.apiKey,
	}

	jsonBytes, err := json.Marshal(bodyData)
	if err != nil {
		return fmt.Errorf("marshal pakasir cancel request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return fmt.Errorf("create pakasir cancel request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute pakasir cancel request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("pakasir transaction cancel status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func StatusToTopupStatus(status string) (models.TopupStatus, error) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "paid", "settlement", "success":
		return models.TopupStatusPaid, nil
	case "pending":
		return models.TopupStatusPending, nil
	case "failed", "cancelled", "canceled", "expired":
		return models.TopupStatusFailed, nil
	default:
		return "", fmt.Errorf("unknown pakasir status %q", status)
	}
}

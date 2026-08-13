// ===========================================
// Telegram Payment Notification Service
// ===========================================
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
)

type NotificationMessage struct {
	OrderID     string
	UserEmail   string
	UserID      uint
	AmountMinor int64
	Currency    string
	Credits     int64
	Provider    string
	Status      models.TopupStatus
	PaidAt      *time.Time
}

type Client struct {
	enabled    bool
	botToken   string
	chatID     string
	topicID    int64
	httpClient *http.Client
	baseURL    string
}

func NewClient(cfg *config.Config) *Client {
	if cfg == nil {
		return &Client{enabled: false}
	}
	token := strings.TrimSpace(cfg.TelegramBotPaymentToken)
	chatID := strings.TrimSpace(cfg.TelegramBotPaymentChatID)
	enabled := cfg.TelegramBotPaymentEnabled && token != "" && chatID != ""

	return &Client{
		enabled:    enabled,
		botToken:   token,
		chatID:     chatID,
		topicID:    cfg.TelegramBotPaymentTopicID,
		httpClient: &http.Client{Timeout: 8 * time.Second},
		baseURL:    "https://api.telegram.org",
	}
}

func (c *Client) SendTopupNotification(msg NotificationMessage) {
	if c == nil || !c.enabled || c.botToken == "" || c.chatID == "" {
		return
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("Recovered panic in Telegram payment notifier", "panic", r)
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		text := formatTelegramMessage(msg)
		if text == "" {
			return
		}

		if err := c.sendMessage(ctx, text); err != nil {
			slog.Warn("Failed to send Telegram payment notification", "order_id", msg.OrderID, "error", err)
		}
	}()
}

func (c *Client) sendMessage(ctx context.Context, text string) error {
	apiURL := fmt.Sprintf("%s/bot%s/sendMessage", c.baseURL, c.botToken)
	payload := map[string]interface{}{
		"chat_id":                  c.chatID,
		"text":                     text,
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
	}

	if c.topicID > 0 {
		payload["message_thread_id"] = c.topicID
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal telegram payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return fmt.Errorf("create telegram request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute telegram request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API returned status %d", resp.StatusCode)
	}

	return nil
}

func formatTelegramMessage(msg NotificationMessage) string {
	providerName := strings.Title(strings.ToLower(msg.Provider))
	if providerName == "" {
		providerName = "Payment Gateway"
	}

	amountFormatted := formatCurrency(msg.AmountMinor, msg.Currency)

	timeStr := time.Now().Format("02 Jan 2006 15:04:05 MST")
	if msg.PaidAt != nil {
		timeStr = msg.PaidAt.Local().Format("02 Jan 2006 15:04:05 MST")
	}

	userStr := msg.UserEmail
	if userStr == "" {
		userStr = fmt.Sprintf("ID %d", msg.UserID)
	} else {
		userStr = fmt.Sprintf("%s (ID: %d)", escapeHTML(userStr), msg.UserID)
	}

	switch msg.Status {
	case models.TopupStatusPaid:
		return fmt.Sprintf(
			"<b>🎉 TOP-UP SUCCESSFUL</b>\n\n"+
				"👤 <b>User:</b> %s\n"+
				"🆔 <b>Order ID:</b> <code>%s</code>\n"+
				"💰 <b>Amount:</b> %s\n"+
				"💎 <b>Wallet Credit:</b> +%d Credits\n"+
				"🏦 <b>Provider:</b> %s\n"+
				"🕒 <b>Time:</b> %s",
			userStr, escapeHTML(msg.OrderID),
			amountFormatted, msg.Credits, providerName, timeStr,
		)
	case models.TopupStatusFailed, models.TopupStatusExpired:
		return fmt.Sprintf(
			"<b>⚠️ TOP-UP %s</b>\n\n"+
				"👤 <b>User:</b> %s\n"+
				"🆔 <b>Order ID:</b> <code>%s</code>\n"+
				"💰 <b>Amount:</b> %s\n"+
				"🏦 <b>Provider:</b> %s\n"+
				"🕒 <b>Time:</b> %s",
			strings.ToUpper(string(msg.Status)), userStr,
			escapeHTML(msg.OrderID), amountFormatted, providerName, timeStr,
		)
	case models.TopupStatusRefunded, models.TopupStatusPartialRefund, models.TopupStatusChargeback, models.TopupStatusPartialChargeback:
		return fmt.Sprintf(
			"<b>🔴 TOP-UP REVERSAL (%s)</b>\n\n"+
				"👤 <b>User:</b> %s\n"+
				"🆔 <b>Order ID:</b> <code>%s</code>\n"+
				"💰 <b>Amount:</b> %s\n"+
				"💎 <b>Reversal:</b> -%d Credits\n"+
				"🏦 <b>Provider:</b> %s\n"+
				"🕒 <b>Time:</b> %s",
			strings.ToUpper(string(msg.Status)), userStr,
			escapeHTML(msg.OrderID), amountFormatted, msg.Credits, providerName, timeStr,
		)
	default:
		return ""
	}
}

func formatCurrency(amount int64, currency string) string {
	if currency == models.BillingCurrencyIDR || currency == "" {
		return fmt.Sprintf("Rp %s", formatNumberWithCommas(amount))
	}
	return fmt.Sprintf("%s %.2f", currency, float64(amount)/100.0)
}

func formatNumberWithCommas(n int64) string {
	in := strconv.FormatInt(n, 10)
	out := make([]byte, len(in)+(len(in)-1)/3)
	for i, j, k := len(in)-1, len(out)-1, 0; i >= 0; i, j, k = i-1, j-1, k+1 {
		if k > 0 && k%3 == 0 {
			out[j] = '.'
			j--
		}
		out[j] = in[i]
	}
	return string(out)
}

func escapeHTML(s string) string {
	r := strings.NewReplacer("<", "&lt;", ">", "&gt;", "&", "&amp;")
	return r.Replace(s)
}

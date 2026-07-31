package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"strconv"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/backend/internal/middleware"
	"github.com/laravel-paas/backend/internal/services/billing"
	"github.com/laravel-paas/shared/apperr"
)

type BillingHandler struct {
	catalog *billing.CatalogService
	topups  *billing.TopupService
}

func NewBillingHandler(catalog *billing.CatalogService) *BillingHandler {
	return &BillingHandler{catalog: catalog}
}

func NewBillingHandlerWithTopups(catalog *billing.CatalogService, topups *billing.TopupService) *BillingHandler {
	return &BillingHandler{catalog: catalog, topups: topups}
}

func (h *BillingHandler) ListActiveCatalog(c *fiber.Ctx) error {
	catalog, err := h.catalog.ListActive(c.UserContext())
	if err != nil {
		return err
	}
	return c.JSON(catalog)
}

func (h *BillingHandler) ListCatalog(c *fiber.Ctx) error {
	adminCatalog, err := h.catalog.ListAll(c.UserContext())
	if err != nil {
		return err
	}
	return c.JSON(adminCatalog)
}

func (h *BillingHandler) GetWallet(c *fiber.Ctx) error {
	userID, err := strconv.ParseUint(c.Params("userID"), 10, 64)
	if err != nil || userID == 0 {
		return apperr.NewBadRequest("Invalid user ID")
	}
	view, err := h.catalog.GetWalletView(c.UserContext(), uint(userID))
	if errors.Is(err, billing.ErrWalletNotFound) {
		return apperr.ErrNotFound
	}
	if err != nil {
		return err
	}
	return c.JSON(view)
}

func (h *BillingHandler) CreateTopup(c *fiber.Ctx) error {
	var input billing.TopupInput
	if err := decodeBillingJSON(c, &input); err != nil {
		return apperr.NewBadRequest("Invalid top-up request")
	}
	userID, _ := c.Locals("user_id").(uint)
	view, err := h.topupService().Create(c.UserContext(), userID, c.Get("Idempotency-Key"), input)
	if err != nil {
		return mapTopupError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(view)
}

func (h *BillingHandler) ReconcileTopup(c *fiber.Ctx) error {
	topupID, err := strconv.ParseUint(c.Params("topupID"), 10, 64)
	if err != nil || topupID == 0 {
		return apperr.NewBadRequest("Invalid top-up ID")
	}
	userID, _ := c.Locals("user_id").(uint)
	view, err := h.topupService().Reconcile(c.UserContext(), userID, uint(topupID))
	if err != nil {
		return mapTopupError(err)
	}
	return c.JSON(view)
}

func (h *BillingHandler) MidtransWebhook(c *fiber.Ctx) error {
	var notification billing.MidtransNotification
	if err := decodeBillingWebhookJSON(c, &notification); err != nil {
		return apperr.New(401, "PAYMENT_NOTIFICATION_REJECTED", "Invalid payment notification")
	}
	if err := h.topupService().ProcessNotification(c.UserContext(), notification); err != nil {
		return mapTopupError(err)
	}
	return c.SendStatus(fiber.StatusOK)
}

func (h *BillingHandler) CreateBillableSpec(c *fiber.Ctx) error {
	var input billing.BillableSpecInput
	if err := decodeBillingJSON(c, &input); err != nil {
		return apperr.NewBadRequest("Invalid billing specification")
	}
	actorUserID, _ := c.Locals("user_id").(uint)
	created, err := h.catalog.CreateBillableSpec(c.UserContext(), billingAuditContext(c, actorUserID, input.Reason), input)
	if errors.Is(err, billing.ErrInvalidCatalogInput) {
		return apperr.NewBadRequest("Invalid billing specification")
	}
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(created)
}

func (h *BillingHandler) CreateTopupPackage(c *fiber.Ctx) error {
	var input billing.TopupPackageInput
	if err := decodeBillingJSON(c, &input); err != nil {
		return apperr.NewBadRequest("Invalid top-up package")
	}
	actorUserID, _ := c.Locals("user_id").(uint)
	created, err := h.catalog.CreateTopupPackage(c.UserContext(), billingAuditContext(c, actorUserID, input.Reason), input)
	if errors.Is(err, billing.ErrInvalidCatalogInput) {
		return apperr.NewBadRequest("Invalid top-up package")
	}
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(created)
}

func decodeBillingJSON(c *fiber.Ctx, target any) error {
	return decodeBillingJSONFields(c, target, true)
}

func decodeBillingWebhookJSON(c *fiber.Ctx, target any) error {
	return decodeBillingJSONFields(c, target, false)
}

func decodeBillingJSONFields(c *fiber.Ctx, target any, rejectUnknownFields bool) error {
	body := c.Body()
	mediaType, _, err := mime.ParseMediaType(c.Get(fiber.HeaderContentType))
	if err != nil || mediaType != fiber.MIMEApplicationJSON || !utf8.Valid(body) {
		return errors.New("invalid billing JSON content type or encoding")
	}
	if err := rejectDuplicateJSONKeys(body); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	if rejectUnknownFields {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("multiple JSON values")
	}
	return nil
}

func (h *BillingHandler) topupService() *billing.TopupService {
	if h.topups != nil {
		return h.topups
	}
	return billing.NewTopupService(nil, nil, nil, nil)
}

func mapTopupError(err error) error {
	switch {
	case errors.Is(err, billing.ErrTopupDisabled):
		return apperr.ErrNotFound
	case errors.Is(err, billing.ErrInvalidTopupInput):
		return apperr.NewBadRequest("Invalid top-up request")
	case errors.Is(err, billing.ErrTopupIdempotencyConflict):
		return apperr.New(409, "TOPUP_IDEMPOTENCY_CONFLICT", "The idempotency key belongs to a different top-up request")
	case errors.Is(err, billing.ErrTopupNotFound):
		return apperr.ErrNotFound
	case errors.Is(err, billing.ErrTopupRecoveryRequired):
		return apperr.New(409, "TOPUP_RECOVERY_REQUIRED", "Payment status is being reconciled before checkout can continue")
	case errors.Is(err, billing.ErrInvalidPaymentNotification):
		return apperr.New(401, "PAYMENT_NOTIFICATION_REJECTED", "Invalid payment notification")
	case errors.Is(err, billing.ErrPaymentProvider):
		return apperr.New(502, "PAYMENT_PROVIDER_UNAVAILABLE", "Payment provider is unavailable")
	default:
		return err
	}
}

func rejectDuplicateJSONKeys(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := scanJSONValue(decoder, 0); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return errors.New("multiple JSON values")
	}
	return nil
}

func scanJSONValue(decoder *json.Decoder, depth int) error {
	if depth > 8 {
		return errors.New("JSON nesting exceeds limit")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return nil
	}
	switch delimiter {
	case '{':
		keys := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("invalid JSON object key")
			}
			if _, exists := keys[key]; exists {
				return errors.New("duplicate JSON object key")
			}
			keys[key] = struct{}{}
			if err := scanJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return errors.New("invalid JSON object")
		}
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return errors.New("invalid JSON array")
		}
	default:
		return errors.New("invalid JSON delimiter")
	}
	return nil
}

func billingAuditContext(c *fiber.Ctx, actorUserID uint, reason string) billing.AuditContext {
	effectiveUserID, _ := c.Locals("user_id").(uint)
	actorRole, _ := c.Locals("role").(string)
	return billing.AuditContext{
		ActorUserID:     actorUserID,
		EffectiveUserID: effectiveUserID,
		ActorRole:       actorRole,
		SourceIP:        c.IP(),
		Reason:          reason,
		RequestID:       middleware.RequestID(c),
	}
}

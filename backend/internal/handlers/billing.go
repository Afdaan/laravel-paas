package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/backend/internal/middleware"
	"github.com/laravel-paas/backend/internal/services/billing"
	"github.com/laravel-paas/shared/apperr"
	"github.com/laravel-paas/shared/models"
)

type BillingHandler struct {
	catalog        *billing.CatalogService
	topups         *billing.TopupService
	suspensions    *billing.SuspensionService
	billingProfile *billing.BillingProfileService
}

func NewBillingHandler(catalog *billing.CatalogService) *BillingHandler {
	return &BillingHandler{catalog: catalog}
}

func NewBillingHandlerWithTopups(catalog *billing.CatalogService, topups *billing.TopupService, suspensions ...*billing.SuspensionService) *BillingHandler {
	var suspensionService *billing.SuspensionService
	if len(suspensions) > 0 {
		suspensionService = suspensions[0]
	}
	return &BillingHandler{catalog: catalog, topups: topups, suspensions: suspensionService, billingProfile: billing.NewBillingProfileService(nil)}
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

func (h *BillingHandler) ListWallets(c *fiber.Ctx) error {
	page, limit, err := billingCollectionPagination(c)
	if err != nil {
		return err
	}
	view, err := h.catalog.ListAdminWallets(c.UserContext(), page, limit)
	if err != nil {
		return mapCatalogError(err)
	}
	return c.JSON(view)
}

func (h *BillingHandler) ListInvoices(c *fiber.Ctx) error {
	page, limit, err := billingCollectionPagination(c)
	if err != nil {
		return err
	}
	view, err := h.catalog.ListAdminInvoices(c.UserContext(), page, limit)
	if err != nil {
		return mapCatalogError(err)
	}
	return c.JSON(view)
}

func (h *BillingHandler) ListTopups(c *fiber.Ctx) error {
	page, limit, err := billingCollectionPagination(c)
	if err != nil {
		return err
	}
	view, err := h.catalog.ListAdminTopups(c.UserContext(), page, limit)
	if err != nil {
		return mapCatalogError(err)
	}
	return c.JSON(view)
}

func (h *BillingHandler) ListSuspensions(c *fiber.Ctx) error {
	if h.suspensions == nil {
		return apperr.New(503, "BILLING_UNAVAILABLE", "Billing service is unavailable")
	}
	views, err := h.suspensions.SuspensionViews(c.UserContext(), time.Now().UTC())
	if err != nil {
		return err
	}
	return c.JSON(views)
}

func (h *BillingHandler) GetOwnPaymentDueStatus(c *fiber.Ctx) error {
	if h.suspensions == nil {
		return apperr.New(503, "BILLING_UNAVAILABLE", "Billing service is unavailable")
	}
	userID, _ := c.Locals("user_id").(uint)
	views, err := h.suspensions.SuspensionViewsForUser(c.UserContext(), userID, time.Now().UTC())
	if err != nil {
		return err
	}
	response := make([]ownPaymentDueView, 0, len(views))
	for _, view := range views {
		response = append(response, ownPaymentDueView{
			ResourceID:     view.ResourceID,
			ResourceType:   view.ResourceType,
			Status:         view.Status,
			OldestDueAt:    view.OldestDueAt,
			PaymentDueDays: view.PaymentDueDays,
		})
	}
	return c.JSON(response)
}

func (h *BillingHandler) GetOwnBillingOverview(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(uint)
	overview, err := h.catalog.GetOwnBillingOverview(c.UserContext(), userID)
	if err != nil {
		return err
	}
	return c.JSON(overview)
}

type ownPaymentDueView struct {
	ResourceID     uint                          `json:"resource_id"`
	ResourceType   models.BillableType           `json:"resource_type"`
	Status         models.BillableResourceStatus `json:"status"`
	OldestDueAt    *time.Time                    `json:"oldest_due_at,omitempty"`
	PaymentDueDays int                           `json:"payment_due_days"`
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

func (h *BillingHandler) UpdateAutoRenew(c *fiber.Ctx) error {
	if h.catalog == nil {
		return apperr.New(503, "BILLING_UNAVAILABLE", "Billing catalog service is unavailable")
	}
	userID, _ := c.Locals("user_id").(uint)
	var input billing.UpdateAutoRenewInput
	if err := decodeBillingJSON(c, &input); err != nil {
		return apperr.NewBadRequest(err.Error())
	}
	if err := h.catalog.UpdateAutoRenew(c.UserContext(), userID, input); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
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

func (h *BillingHandler) UpdateTopupPackage(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil || id == 0 {
		return apperr.NewBadRequest("Invalid top-up package ID")
	}
	var input billing.TopupPackageUpdateInput
	if err := decodeBillingJSON(c, &input); err != nil {
		return apperr.NewBadRequest("Invalid top-up package update")
	}
	actorUserID, _ := c.Locals("user_id").(uint)
	updated, err := h.catalog.UpdateTopupPackage(c.UserContext(), billingAuditContext(c, actorUserID, input.Reason), uint(id), input)
	if errors.Is(err, billing.ErrInvalidCatalogInput) {
		return apperr.NewBadRequest("Invalid top-up package update")
	}
	if errors.Is(err, apperr.ErrNotFound) {
		return apperr.ErrNotFound
	}
	if err != nil {
		return err
	}
	return c.JSON(updated)
}

func (h *BillingHandler) AdjustWalletCredits(c *fiber.Ctx) error {
	userID, err := strconv.ParseUint(c.Params("userID"), 10, 64)
	if err != nil || userID == 0 {
		return apperr.NewBadRequest("Invalid user ID")
	}
	var input billing.WalletCreditAdjustmentInput
	if err := decodeBillingJSON(c, &input); err != nil {
		return apperr.NewBadRequest("Invalid credit adjustment")
	}
	idempotencyKey := c.Get("Idempotency-Key")
	if idempotencyKey == "" {
		return apperr.NewBadRequest("Idempotency-Key header is required")
	}
	actorUserID, _ := c.Locals("user_id").(uint)
	view, err := h.catalog.AdjustWalletCredits(c.UserContext(), billingAuditContext(c, actorUserID, input.Reason), uint(userID), idempotencyKey, input)
	if errors.Is(err, billing.ErrInvalidCatalogInput) {
		return apperr.NewBadRequest("Invalid credit adjustment")
	}
	if errors.Is(err, apperr.ErrNotFound) {
		return apperr.ErrNotFound
	}
	if err != nil {
		return err
	}
	return c.JSON(view)
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

func billingCollectionPagination(c *fiber.Ctx) (int, int, error) {
	page, err := strconv.Atoi(c.Query("page", "1"))
	if err != nil || page < 1 {
		return 0, 0, apperr.NewBadRequest("Invalid billing collection page")
	}
	limit, err := strconv.Atoi(c.Query("limit", "25"))
	if err != nil || limit < 1 || limit > 100 {
		return 0, 0, apperr.NewBadRequest("Invalid billing collection limit")
	}
	return page, limit, nil
}

func mapCatalogError(err error) error {
	if errors.Is(err, billing.ErrInvalidCatalogInput) {
		return apperr.NewBadRequest("Invalid billing collection query")
	}
	return err
}

func mapTopupError(err error) error {
	switch {
	case errors.Is(err, billing.ErrTopupDisabled):
		return apperr.New(400, "BILLING_TOPUP_DISABLED", "Top-up payment processing is currently disabled in system configuration")
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
		slog.Error("Payment provider failure", "error", err)
		return apperr.New(502, "PAYMENT_PROVIDER_UNAVAILABLE", "Payment provider is currently unavailable. Please try again later or contact support.")
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


func (h *BillingHandler) SetBillingProfileService(service *billing.BillingProfileService) {
	h.billingProfile = service
}

func (h *BillingHandler) GetBillingProfile(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(uint)
	if h.billingProfile == nil {
		return apperr.New(503, "BILLING_UNAVAILABLE", "Billing profile service unavailable")
	}
	profile, err := h.billingProfile.GetProfile(c.UserContext(), userID)
	if err != nil {
		return err
	}
	return c.JSON(profile)
}

func (h *BillingHandler) UpdateBillingProfile(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(uint)
	var input billing.UpdateBillingProfileInput
	if err := decodeBillingJSON(c, &input); err != nil {
		return apperr.NewBadRequest("Invalid billing profile data")
	}
	if h.billingProfile == nil {
		return apperr.New(503, "BILLING_UNAVAILABLE", "Billing profile service unavailable")
	}
	profile, err := h.billingProfile.UpsertProfile(c.UserContext(), userID, input)
	if errors.Is(err, billing.ErrInvalidBillingEmail) {
		return apperr.NewBadRequest("Invalid email format")
	}
	if err != nil {
		return err
	}
	return c.JSON(profile)
}

func (h *BillingHandler) GetUserBillingProfileAdmin(c *fiber.Ctx) error {
	userID, err := strconv.ParseUint(c.Params("userID"), 10, 64)
	if err != nil || userID == 0 {
		return apperr.NewBadRequest("Invalid user ID")
	}
	if h.billingProfile == nil {
		return apperr.New(503, "BILLING_UNAVAILABLE", "Billing profile service unavailable")
	}
	profile, err := h.billingProfile.GetProfile(c.UserContext(), uint(userID))
	if err != nil {
		return err
	}
	return c.JSON(profile)
}

type PakasirWebhookPayload struct {
	Amount        int64  `json:"amount"`
	OrderID       string `json:"order_id"`
	Project       string `json:"project"`
	Status        string `json:"status"`
	PaymentMethod string `json:"payment_method"`
	CompletedAt   string `json:"completed_at"`
}

func (h *BillingHandler) PakasirWebhook(c *fiber.Ctx) error {
	var payload PakasirWebhookPayload
	if err := decodeBillingWebhookJSON(c, &payload); err != nil {
		return apperr.NewBadRequest("Invalid webhook payload")
	}

	if payload.OrderID == "" || payload.Amount <= 0 {
		return apperr.NewBadRequest("Missing required webhook parameters")
	}

	err := h.topupService().ProcessPakasirWebhook(c.UserContext(), payload.OrderID, payload.Amount, payload.Status)
	if err != nil {
		return mapTopupError(err)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "ok"})
}

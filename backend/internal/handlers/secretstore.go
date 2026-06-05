package handlers

import (
	"fmt"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/backend/internal/services"
	"github.com/laravel-paas/shared/apperr"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/pkg/utils"
	"gorm.io/gorm"
)

type SecretStoreHandler struct {
	db                 *gorm.DB
	cfg                *config.Config
	secretStoreService *services.SecretStoreService
}

func NewSecretStoreHandler(db *gorm.DB, cfg *config.Config, secretStoreService *services.SecretStoreService) *SecretStoreHandler {
	return &SecretStoreHandler{
		db:                 db,
		cfg:                cfg,
		secretStoreService: secretStoreService,
	}
}

func (h *SecretStoreHandler) List(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)
	stores, err := h.secretStoreService.ListSecretStores(userID)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": stores})
}

func (h *SecretStoreHandler) Get(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return apperr.New(400, "INVALID_ID", "Invalid SecretStore ID")
	}

	store, err := h.secretStoreService.GetSecretStore(userID, uint(id))
	if err != nil {
		return apperr.New(404, "NOT_FOUND", "SecretStore not found")
	}

	// Mask values by default for security, values can only be read via specific reveal endpoint.
	type MaskedItem struct {
		ID        uint   `json:"id"`
		Key       string `json:"key"`
		Version   int    `json:"version"`
		UpdatedAt string `json:"updated_at"`
	}

	var maskedItems []MaskedItem
	for _, item := range store.Items {
		maskedItems = append(maskedItems, MaskedItem{
			ID:        item.ID,
			Key:       item.Key,
			Version:   item.LatestSnapshotVersion,
			UpdatedAt: item.UpdatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	return c.JSON(fiber.Map{
		"data": fiber.Map{
			"id":          store.ID,
			"name":        store.Name,
			"description": store.Description,
			"is_disabled": store.IsDisabled,
			"items":       maskedItems,
			"bindings":    store.Bindings,
		},
	})
}

func (h *SecretStoreHandler) Create(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := c.BodyParser(&req); err != nil {
		return apperr.New(400, "BAD_REQUEST", "Invalid request body")
	}

	if req.Name == "" {
		return apperr.New(400, "VALIDATION_FAILED", "Name is required")
	}

	store, err := h.secretStoreService.CreateSecretStore(userID, req.Name, req.Description, c.IP(), c.Get("User-Agent"))
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": store})
}

func (h *SecretStoreHandler) Update(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return apperr.New(400, "INVALID_ID", "Invalid SecretStore ID")
	}

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := c.BodyParser(&req); err != nil {
		return apperr.New(400, "BAD_REQUEST", "Invalid request body")
	}

	store, err := h.secretStoreService.UpdateSecretStore(userID, uint(id), req.Name, req.Description, c.IP(), c.Get("User-Agent"))
	if err != nil {
		return err
	}

	return c.JSON(fiber.Map{"data": store})
}

func (h *SecretStoreHandler) Delete(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return apperr.New(400, "INVALID_ID", "Invalid SecretStore ID")
	}

	if err := h.secretStoreService.DeleteSecretStore(userID, uint(id), c.IP(), c.Get("User-Agent")); err != nil {
		return err
	}

	return c.JSON(fiber.Map{"message": "SecretStore deleted successfully"})
}

func (h *SecretStoreHandler) SetSecret(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return apperr.New(400, "INVALID_ID", "Invalid SecretStore ID")
	}

	var req struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := c.BodyParser(&req); err != nil {
		return apperr.New(400, "BAD_REQUEST", "Invalid request body")
	}

	if req.Key == "" {
		return apperr.New(400, "VALIDATION_FAILED", "Secret key cannot be empty")
	}

	item, err := h.secretStoreService.SetSecretValue(userID, uint(id), req.Key, req.Value, c.IP(), c.Get("User-Agent"))
	if err != nil {
		return err
	}

	return c.JSON(fiber.Map{"data": item})
}

func (h *SecretStoreHandler) RevealSecret(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return apperr.New(400, "INVALID_ID", "Invalid SecretStore ID")
	}

	itemID, errItem := strconv.ParseUint(c.Params("itemID"), 10, 32)
	if errItem != nil {
		return apperr.New(400, "INVALID_ITEM_ID", "Invalid SecretStore Item ID")
	}

	store, err := h.secretStoreService.GetSecretStore(userID, uint(id))
	if err != nil {
		return apperr.New(404, "NOT_FOUND", "SecretStore not found")
	}

	var targetItem *models.SecretStoreItem
	for i := range store.Items {
		if store.Items[i].ID == uint(itemID) {
			targetItem = &store.Items[i]
			break
		}
	}

	if targetItem == nil {
		return apperr.New(404, "ITEM_NOT_FOUND", "SecretStore Item not found")
	}

	var latestVal *models.SecretStoreItemValue
	for i := range targetItem.Values {
		val := &targetItem.Values[i]
		if val.Version == targetItem.LatestSnapshotVersion {
			latestVal = val
			break
		}
	}

	if latestVal == nil {
		return apperr.New(404, "VALUE_NOT_FOUND", "No values set for this secret key")
	}

	stretchedKey := utils.DeriveKey(h.cfg.CredentialEncryptionKey)
	legacyKey := utils.DeriveKeyLegacy(h.cfg.CredentialEncryptionKey)
	decryptedVal, errDec := utils.Decrypt(latestVal.EncryptedValue, stretchedKey, legacyKey)
	if errDec != nil {
		return apperr.New(500, "DECRYPTION_FAILED", "Failed to decrypt secret value")
	}

	// Write audited activity log entry for tracking.
	h.secretStoreService.LogActivity(userID, &store.ID, &targetItem.ID, nil, "reveal_value", "Revealed plaintext value of secret key: "+targetItem.Key, c.IP(), c.Get("User-Agent"))

	return c.JSON(fiber.Map{"value": decryptedVal})
}

func (h *SecretStoreHandler) Bind(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return apperr.New(400, "INVALID_ID", "Invalid SecretStore ID")
	}

	var req struct {
		ProjectUID  string `json:"project_uid"`
		Environment string `json:"environment"`
	}
	if err := c.BodyParser(&req); err != nil {
		return apperr.New(400, "BAD_REQUEST", "Invalid request body")
	}

	var project models.Project
	if err := h.db.Where("uid = ? AND user_id = ?", req.ProjectUID, userID).First(&project).Error; err != nil {
		return apperr.New(404, "PROJECT_NOT_FOUND", "Project not found")
	}

	binding, err := h.secretStoreService.BindSecretStore(userID, uint(id), project.ID, req.Environment, c.IP(), c.Get("User-Agent"))
	if err != nil {
		return err
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": binding})
}

func (h *SecretStoreHandler) Unbind(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return apperr.New(400, "INVALID_ID", "Invalid SecretStore ID")
	}

	bindingID, err := strconv.ParseUint(c.Params("bindingID"), 10, 32)
	if err != nil {
		return apperr.New(400, "INVALID_BINDING_ID", "Invalid Binding ID")
	}

	if err := h.secretStoreService.UnbindSecretStore(userID, uint(id), uint(bindingID), c.IP(), c.Get("User-Agent")); err != nil {
		return err
	}

	return c.JSON(fiber.Map{"message": "SecretStore unbound successfully"})
}

func (h *SecretStoreHandler) Export(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return apperr.New(400, "INVALID_ID", "Invalid SecretStore ID")
	}

	store, err := h.secretStoreService.GetSecretStore(userID, uint(id))
	if err != nil {
		return apperr.New(404, "NOT_FOUND", "SecretStore not found")
	}

	stretchedKey := utils.DeriveKey(h.cfg.CredentialEncryptionKey)
	legacyKey := utils.DeriveKeyLegacy(h.cfg.CredentialEncryptionKey)
	secretsMap := make(map[string]string)

	for _, item := range store.Items {
		var latestVal *models.SecretStoreItemValue
		for i := range item.Values {
			val := &item.Values[i]
			if val.Version == item.LatestSnapshotVersion {
				latestVal = val
				break
			}
		}

		if latestVal != nil {
			decrypted, err := utils.Decrypt(latestVal.EncryptedValue, stretchedKey, legacyKey)
			if err == nil {
				secretsMap[item.Key] = decrypted
			}
		}
	}

	h.secretStoreService.LogActivity(userID, &store.ID, nil, nil, "export_secrets", "Exported secret store backup data", c.IP(), c.Get("User-Agent"))

	return c.JSON(fiber.Map{"secrets": secretsMap})
}

func (h *SecretStoreHandler) Import(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return apperr.New(400, "INVALID_ID", "Invalid SecretStore ID")
	}

	var req struct {
		Secrets map[string]string `json:"secrets"`
	}
	if err := c.BodyParser(&req); err != nil {
		return apperr.New(400, "BAD_REQUEST", "Invalid request body")
	}

	storeID := uint(id)
	for k, v := range req.Secrets {
		if k == "" {
			continue
		}
		if _, err := h.secretStoreService.SetSecretValueNoPropagate(userID, storeID, k, v, c.IP(), c.Get("User-Agent")); err != nil {
			return err
		}
	}

	h.secretStoreService.LogActivity(userID, &storeID, nil, nil, "import_secrets", "Imported secrets into store container", c.IP(), c.Get("User-Agent"))

	// Propagate updates exactly once after bulk import
	go h.secretStoreService.PropagateSecretStoreUpdates(storeID)

	return c.JSON(fiber.Map{"message": "Secrets imported successfully"})
}

func (h *SecretStoreHandler) AdminListAll(c *fiber.Ctx) error {
	var stores []models.SecretStore
	if err := h.db.Preload("User").Preload("Items").Preload("Bindings").Find(&stores).Error; err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": stores})
}

func (h *SecretStoreHandler) AdminDisable(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return apperr.New(400, "INVALID_ID", "Invalid SecretStore ID")
	}

	var req struct {
		Disable bool `json:"disable"`
	}
	if err := c.BodyParser(&req); err != nil {
		return apperr.New(400, "BAD_REQUEST", "Invalid request body")
	}

	var store models.SecretStore
	if err := h.db.First(&store, id).Error; err != nil {
		return apperr.New(404, "NOT_FOUND", "SecretStore not found")
	}

	store.IsDisabled = req.Disable
	if err := h.db.Save(&store).Error; err != nil {
		return err
	}

	adminUserID := c.Locals("user_id").(uint)
	action := "enable_secretstore"
	if req.Disable {
		action = "disable_secretstore"
	}
	h.secretStoreService.LogActivity(adminUserID, &store.ID, nil, nil, action, fmt.Sprintf("Admin toggled disable state to %t", req.Disable), c.IP(), c.Get("User-Agent"))

	return c.JSON(fiber.Map{"message": "SecretStore disabled status updated"})
}

func (h *SecretStoreHandler) AdminListLogs(c *fiber.Ctx) error {
	var logs []models.SecretStoreActivityLog
	if err := h.db.Preload("User").Preload("SecretStore").Preload("Project").Order("created_at DESC").Limit(100).Find(&logs).Error; err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": logs})
}



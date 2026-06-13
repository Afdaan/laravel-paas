package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

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

const maxSecretStoreImportBytes = 1024 * 1024

type secretStoreImportEntry struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Value string `json:"value"`
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

	var targetVal *models.SecretStoreItemValue
	versionStr := c.Query("version")
	if versionStr != "" {
		if version, err := strconv.Atoi(versionStr); err == nil {
			for i := range targetItem.Values {
				if targetItem.Values[i].Version == version {
					targetVal = &targetItem.Values[i]
					break
				}
			}
		}
	}

	if targetVal == nil {
		for i := range targetItem.Values {
			val := &targetItem.Values[i]
			if val.Version == targetItem.LatestSnapshotVersion {
				targetVal = val
				break
			}
		}
	}

	if targetVal == nil {
		return apperr.New(404, "VALUE_NOT_FOUND", "No values set for this secret key")
	}

	stretchedKey := utils.DeriveKey(h.cfg.CredentialEncryptionKey)
	legacyKey := utils.DeriveKeyLegacy(h.cfg.CredentialEncryptionKey)
	decryptedVal, errDec := utils.Decrypt(targetVal.EncryptedValue, stretchedKey, legacyKey)
	if errDec != nil {
		return apperr.New(500, "DECRYPTION_FAILED", "Failed to decrypt secret value")
	}

	// Write audited activity log entry for tracking.
	details := "Revealed plaintext value of secret key: " + targetItem.Key
	if versionStr != "" {
		details = fmt.Sprintf("Revealed plaintext value of secret key %s version %s", targetItem.Key, versionStr)
	}
	h.secretStoreService.LogActivity(userID, &store.ID, &targetItem.ID, nil, "reveal_value", details, c.IP(), c.Get("User-Agent"))

	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(fiber.Map{"data": fiber.Map{"value": decryptedVal}})
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

		if latestVal == nil {
			return apperr.New(500, "SECRET_EXPORT_INCOMPLETE", "Unable to export SecretStore backup because one or more secrets have no active value")
		}

		decrypted, err := utils.Decrypt(latestVal.EncryptedValue, stretchedKey, legacyKey)
		if err != nil {
			return apperr.New(500, "DECRYPTION_FAILED", "Failed to decrypt secret value")
		}
		secretsMap[item.Key] = normalizeSecretStoreValue(item.Key, decrypted)
	}

	h.secretStoreService.LogActivity(userID, &store.ID, nil, nil, "export_secrets", "Exported secret store backup data", c.IP(), c.Get("User-Agent"))

	c.Set(fiber.HeaderCacheControl, "no-store")
	c.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSONCharsetUTF8)
	return c.JSON(secretsMap)
}

func (h *SecretStoreHandler) Import(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return apperr.New(400, "INVALID_ID", "Invalid SecretStore ID")
	}

	rawBody := c.BodyRaw()
	if len(rawBody) == 0 {
		return apperr.New(400, "BAD_REQUEST", "Invalid request body")
	}
	if len(rawBody) > maxSecretStoreImportBytes {
		return apperr.New(413, "PAYLOAD_TOO_LARGE", "SecretStore import payload is too large")
	}

	secrets, err := parseSecretStoreImportPayload(rawBody)
	if err != nil {
		return apperr.New(400, "BAD_REQUEST", "Invalid request body")
	}
	if len(secrets) == 0 {
		return apperr.New(400, "VALIDATION_FAILED", "SecretStore import contains no secrets")
	}

	storeID := uint(id)
	if err := h.db.Transaction(func(tx *gorm.DB) error {
		if _, err := h.secretStoreService.GetSecretStoreTx(tx, userID, storeID); err != nil {
			return err
		}

		for key, value := range secrets {
			if _, err := h.secretStoreService.SetSecretValueNoPropagateTx(tx, userID, storeID, key, value, c.IP(), c.Get("User-Agent")); err != nil {
				return err
			}
		}

		h.secretStoreService.LogActivityTx(tx, userID, &storeID, nil, nil, "import_secrets", "Imported secrets into store container", c.IP(), c.Get("User-Agent"))
		return nil
	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperr.New(404, "NOT_FOUND", "SecretStore not found")
		}
		return err
	}

	// Propagate updates exactly once after bulk import
	utils.SafeGo(func() {
		h.secretStoreService.PropagateSecretStoreUpdates(storeID)
	})

	return c.JSON(fiber.Map{"message": "Secrets imported successfully"})
}

func parseSecretStoreImportPayload(raw []byte) (map[string]string, error) {
	secrets, err := parseSecretStoreImportPayloadStrict(raw)
	if err == nil {
		return secrets, nil
	}

	sanitized := escapeJSONControlCharactersInStrings(raw)
	if string(sanitized) == string(raw) {
		return nil, err
	}

	return parseSecretStoreImportPayloadStrict(sanitized)
}

func parseSecretStoreImportPayloadStrict(raw []byte) (map[string]string, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err == nil {
		for _, field := range []string{"secrets", "variables", "env", "items"} {
			if nested, ok := object[field]; ok {
				return parseSecretStoreSecretsNode(nested)
			}
		}
	}

	return parseSecretStoreSecretsNode(raw)
}

func parseSecretStoreSecretsNode(raw []byte) (map[string]string, error) {
	var rawSecrets map[string]string
	if err := json.Unmarshal(raw, &rawSecrets); err == nil {
		return normalizeSecretStoreSecrets(rawSecrets)
	}

	var entries []secretStoreImportEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, err
	}

	secrets := make(map[string]string, len(entries))
	for _, entry := range entries {
		key := entry.Key
		if key == "" {
			key = entry.Name
		}
		if err := addNormalizedSecret(secrets, key, entry.Value); err != nil {
			return nil, err
		}
	}
	return secrets, nil
}

func normalizeSecretStoreSecrets(input map[string]string) (map[string]string, error) {
	secrets := make(map[string]string, len(input))
	for key, value := range input {
		if err := addNormalizedSecret(secrets, key, value); err != nil {
			return nil, err
		}
	}
	return secrets, nil
}

func addNormalizedSecret(secrets map[string]string, key string, value string) error {
	normalizedKey := strings.TrimSpace(key)
	if normalizedKey == "" {
		return fmt.Errorf("secret key cannot be empty")
	}
	if _, exists := secrets[normalizedKey]; exists {
		return fmt.Errorf("duplicate secret key after normalization")
	}
	secrets[normalizedKey] = normalizeSecretStoreValue(normalizedKey, value)
	return nil
}

func normalizeSecretStoreValue(key string, value string) string {
	if !isSingleLineSecretValueKey(key) {
		return value
	}

	return strings.NewReplacer("\r", "", "\n", "").Replace(value)
}

func isSingleLineSecretValueKey(key string) bool {
	normalizedKey := strings.ToUpper(strings.TrimSpace(key))
	return normalizedKey == "DATABASE_URL" ||
		normalizedKey == "REDIS_URL" ||
		normalizedKey == "SENTRY_DSN" ||
		strings.HasSuffix(normalizedKey, "_URL") ||
		strings.HasSuffix(normalizedKey, "_DSN")
}

func escapeJSONControlCharactersInStrings(raw []byte) []byte {
	escaped := make([]byte, 0, len(raw))
	inString := false
	escaping := false

	for _, b := range raw {
		if !inString {
			escaped = append(escaped, b)
			if b == '"' {
				inString = true
			}
			continue
		}

		if escaping {
			escaped = append(escaped, b)
			escaping = false
			continue
		}

		switch b {
		case '\\':
			escaped = append(escaped, b)
			escaping = true
		case '"':
			escaped = append(escaped, b)
			inString = false
		case '\n':
			escaped = append(escaped, '\\', 'n')
		case '\r':
			escaped = append(escaped, '\\', 'r')
		case '\t':
			escaped = append(escaped, '\\', 't')
		default:
			if b < 0x20 {
				escaped = append(escaped, []byte(fmt.Sprintf("\\u%04x", b))...)
				continue
			}
			escaped = append(escaped, b)
		}
	}

	return escaped
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

// CreateItem creates a new secret item in the store (aliased to SetSecret)
func (h *SecretStoreHandler) CreateItem(c *fiber.Ctx) error {
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

	// Verify ownership of the secret store
	store, err := h.secretStoreService.GetSecretStore(userID, uint(id))
	if err != nil {
		return apperr.New(404, "NOT_FOUND", "SecretStore not found")
	}

	// Check for key collision dynamically to enforce standard RESTful creation rules
	var count int64
	if err := h.db.Model(&models.SecretStoreItem{}).Where("secret_store_id = ? AND key = ?", store.ID, req.Key).Count(&count).Error; err == nil && count > 0 {
		return apperr.New(409, "KEY_COLLISION", "Secret key already exists in this store")
	}

	item, err := h.secretStoreService.SetSecretValue(userID, store.ID, req.Key, req.Value, c.IP(), c.Get("User-Agent"))
	if err != nil {
		return err
	}

	return c.JSON(fiber.Map{"data": item})
}

// UpdateItem updates an existing secret item's value by creating a new version
func (h *SecretStoreHandler) UpdateItem(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return apperr.New(400, "INVALID_ID", "Invalid SecretStore ID")
	}

	itemID, err := strconv.ParseUint(c.Params("itemID"), 10, 32)
	if err != nil {
		return apperr.New(400, "INVALID_ITEM_ID", "Invalid SecretStore Item ID")
	}

	var req struct {
		Value string `json:"value"`
	}
	if err := c.BodyParser(&req); err != nil {
		return apperr.New(400, "BAD_REQUEST", "Invalid request body")
	}

	// Verify ownership of the secret store
	store, err := h.secretStoreService.GetSecretStore(userID, uint(id))
	if err != nil {
		return apperr.New(404, "NOT_FOUND", "SecretStore not found")
	}

	// Resolve target item to verify key
	var item models.SecretStoreItem
	if err := h.db.Where("id = ? AND secret_store_id = ?", itemID, store.ID).First(&item).Error; err != nil {
		return apperr.New(404, "ITEM_NOT_FOUND", "SecretStore Item not found")
	}

	// Update the secret value using SetSecretValue (handles versioning, encryption, audit log and propagation)
	updatedItem, err := h.secretStoreService.SetSecretValue(userID, store.ID, item.Key, req.Value, c.IP(), c.Get("User-Agent"))
	if err != nil {
		return err
	}

	return c.JSON(fiber.Map{"data": updatedItem})
}

// DeleteItem soft-deletes a secret item and propagates updates to bound projects
func (h *SecretStoreHandler) DeleteItem(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return apperr.New(400, "INVALID_ID", "Invalid SecretStore ID")
	}

	itemID, err := strconv.ParseUint(c.Params("itemID"), 10, 32)
	if err != nil {
		return apperr.New(400, "INVALID_ITEM_ID", "Invalid SecretStore Item ID")
	}

	// Verify ownership of the secret store
	store, err := h.secretStoreService.GetSecretStore(userID, uint(id))
	if err != nil {
		return apperr.New(404, "NOT_FOUND", "SecretStore not found")
	}

	// Retrieve target item
	var item models.SecretStoreItem
	if err := h.db.Where("id = ? AND secret_store_id = ?", itemID, store.ID).First(&item).Error; err != nil {
		return apperr.New(404, "ITEM_NOT_FOUND", "SecretStore Item not found")
	}

	// Perform GORM soft delete
	if err := h.db.Delete(&item).Error; err != nil {
		return err
	}

	// Log activity
	h.secretStoreService.LogActivity(
		userID,
		&store.ID,
		&item.ID,
		nil,
		"delete_secret_item",
		fmt.Sprintf("Deleted secret key: %s", item.Key),
		c.IP(),
		c.Get("User-Agent"),
	)

	// Propagate updates asynchronously using safe panic recovery wrapper
	utils.SafeGo(func() {
		h.secretStoreService.PropagateSecretStoreUpdates(store.ID)
	})

	return c.JSON(fiber.Map{"message": "SecretStore Item deleted successfully"})
}

// History returns version history details for a secret item
func (h *SecretStoreHandler) History(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return apperr.New(400, "INVALID_ID", "Invalid SecretStore ID")
	}

	itemID, err := strconv.ParseUint(c.Params("itemID"), 10, 32)
	if err != nil {
		return apperr.New(400, "INVALID_ITEM_ID", "Invalid SecretStore Item ID")
	}

	// Verify ownership of the secret store
	store, err := h.secretStoreService.GetSecretStore(userID, uint(id))
	if err != nil {
		return apperr.New(404, "NOT_FOUND", "SecretStore not found")
	}

	// Retrieve target item
	var item models.SecretStoreItem
	if err := h.db.Where("id = ? AND secret_store_id = ?", itemID, store.ID).First(&item).Error; err != nil {
		return apperr.New(404, "ITEM_NOT_FOUND", "SecretStore Item not found")
	}

	// Retrieve history values ordered by version desc
	var values []models.SecretStoreItemValue
	if err := h.db.Where("secret_store_item_id = ?", item.ID).Order("version DESC").Find(&values).Error; err != nil {
		return err
	}

	// Format response array of {id, version, created_at}
	type HistoryItemResponse struct {
		ID        uint      `json:"id"`
		Version   int       `json:"version"`
		CreatedAt time.Time `json:"created_at"`
	}

	data := make([]HistoryItemResponse, len(values))
	for i, val := range values {
		data[i] = HistoryItemResponse{
			ID:        val.ID,
			Version:   val.Version,
			CreatedAt: val.CreatedAt,
		}
	}

	return c.JSON(fiber.Map{"data": data})
}

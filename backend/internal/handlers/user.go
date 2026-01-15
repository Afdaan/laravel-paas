// ===========================================
// User Handler
// ===========================================
// Handles user CRUD and Excel import
// ===========================================
package handlers

import (
	"fmt"
	"math/rand"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/backend/internal/models"
	"github.com/xuri/excelize/v2"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// UserHandler handles user management endpoints
type UserHandler struct {
	db *gorm.DB
}

// NewUserHandler creates a new user handler
func NewUserHandler(db *gorm.DB) *UserHandler {
	return &UserHandler{db: db}
}

// CreateUserRequest represents user creation payload
type CreateUserRequest struct {
	Email    string      `json:"email"`
	Name     string      `json:"name"`
	Role     models.Role `json:"role"`
	Password string      `json:"password,omitempty"` // Optional, will be generated if empty
}

// List returns paginated users
func (h *UserHandler) List(c *fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "10"))
	role := c.Query("role", "")
	search := c.Query("search", "")

	offset := (page - 1) * limit

	var users []models.User
	var total int64

	query := h.db.Model(&models.User{})

	// Filter by role if specified
	if role != "" {
		query = query.Where("role = ?", role)
	}

	// Search by name or email
	if search != "" {
		query = query.Where("name LIKE ? OR email LIKE ?", "%"+search+"%", "%"+search+"%")
	}

	// Get total count
	query.Count(&total)

	// Get paginated results
	if err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&users).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to fetch users",
		})
	}

	return c.JSON(fiber.Map{
		"data":  users,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// Get returns a single user by ID
func (h *UserHandler) Get(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid user ID",
		})
	}

	var user models.User
	if err := h.db.Preload("Projects").First(&user, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	return c.JSON(user)
}

// Create creates a new user
func (h *UserHandler) Create(c *fiber.Ctx) error {
	var req CreateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate required fields
	if req.Email == "" || req.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Email and name are required",
		})
	}

	// Check if email exists
	var existing models.User
	if h.db.Where("email = ?", req.Email).First(&existing).Error == nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{
			"error": "Email already exists",
		})
	}

	// Generate random password if not provided
	password := req.Password
	if password == "" {
		password = generateRandomPassword(12)
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to hash password",
		})
	}

	// Set default role
	role := req.Role
	if role == "" {
		role = models.RoleStudent
	}

	// Only superadmin can create admins
	currentRole := c.Locals("role").(string)
	if role == models.RoleAdmin && currentRole != string(models.RoleSuperAdmin) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Only superadmin can create admin users",
		})
	}

	// Get creator ID
	creatorID := c.Locals("user_id").(uint)

	user := models.User{
		Email:     req.Email,
		Password:  string(hashedPassword),
		Name:      req.Name,
		Role:      role,
		CreatedBy: &creatorID,
	}

	if err := h.db.Create(&user).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to create user",
		})
	}

	// Return user with plain password (only on creation)
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"user":     user,
		"password": password, // Show generated password
	})
}

// Update modifies an existing user
func (h *UserHandler) Update(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid user ID",
		})
	}

	var user models.User
	if err := h.db.First(&user, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	var req CreateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Update fields
	if req.Name != "" {
		user.Name = req.Name
	}
	if req.Email != "" && req.Email != user.Email {
		var existing models.User
		if h.db.Where("email = ? AND id != ?", req.Email, id).First(&existing).Error == nil {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "Email already exists",
			})
		}
		user.Email = req.Email
	}
	if req.Password != "" {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to hash password",
			})
		}
		user.Password = string(hashedPassword)
	}

	if err := h.db.Save(&user).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update user",
		})
	}

	return c.JSON(user)
}

// Delete removes a user
func (h *UserHandler) Delete(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid user ID",
		})
	}

	var user models.User
	if err := h.db.First(&user, id).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found",
		})
	}

	// Prevent deleting superadmin
	if user.Role == models.RoleSuperAdmin {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Cannot delete superadmin",
		})
	}

	// Soft delete user and their projects
	if err := h.db.Delete(&user).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to delete user",
		})
	}

	return c.JSON(fiber.Map{
		"message": "User deleted successfully",
	})
}

// ImportExcel imports users from Excel file
func (h *UserHandler) ImportExcel(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "File is required",
		})
	}

	// Open uploaded file
	src, err := file.Open()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to open file",
		})
	}
	defer src.Close()

	// Parse Excel file
	xlsx, err := excelize.OpenReader(src)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid Excel file",
		})
	}
	defer xlsx.Close()

	// Get first sheet
	sheetName := xlsx.GetSheetName(0)
	rows, err := xlsx.GetRows(sheetName)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Failed to read Excel file",
		})
	}

	// Expected format: Name, Email (with header row)
	if len(rows) < 2 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Excel file must have at least one data row",
		})
	}

	creatorID := c.Locals("user_id").(uint)
	var created []fiber.Map
	var errors []string

	// Process rows (skip header)
	for i, row := range rows[1:] {
		if len(row) < 2 {
			errors = append(errors, fmt.Sprintf("Row %d: insufficient columns", i+2))
			continue
		}

		name := row[0]
		email := row[1]

		if name == "" || email == "" {
			errors = append(errors, fmt.Sprintf("Row %d: name and email are required", i+2))
			continue
		}

		// Check if email exists
		var existing models.User
		if h.db.Where("email = ?", email).First(&existing).Error == nil {
			errors = append(errors, fmt.Sprintf("Row %d: email %s already exists", i+2, email))
			continue
		}

		// Generate password
		password := generateRandomPassword(12)
		hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

		user := models.User{
			Email:     email,
			Name:      name,
			Password:  string(hashedPassword),
			Role:      models.RoleStudent,
			CreatedBy: &creatorID,
		}

		if err := h.db.Create(&user).Error; err != nil {
			errors = append(errors, fmt.Sprintf("Row %d: failed to create user", i+2))
			continue
		}

		created = append(created, fiber.Map{
			"id":       user.ID,
			"name":     user.Name,
			"email":    user.Email,
			"password": password,
		})
	}

	return c.JSON(fiber.Map{
		"created": created,
		"errors":  errors,
		"total":   len(created),
	})
}

// generateRandomPassword creates a random password
func generateRandomPassword(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	password := make([]byte, length)
	for i := range password {
		password[i] = charset[r.Intn(len(charset))]
	}
	return string(password)
}

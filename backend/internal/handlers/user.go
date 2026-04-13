// ===========================================
// User Handler
// ===========================================
// Handles user CRUD and Excel import
// ===========================================
package handlers

import (
	"fmt"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/backend/internal/models"
	"github.com/laravel-paas/backend/internal/services"
	"github.com/xuri/excelize/v2"
)

// UserHandler handles user management endpoints
type UserHandler struct {
	userService *services.UserService
}

// NewUserHandler creates a new user handler
func NewUserHandler(userService *services.UserService) *UserHandler {
	return &UserHandler{userService: userService}
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

	users, total, err := h.userService.GetAllUsers(page, limit, role, search)
	if err != nil {
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

	user, err := h.userService.GetUserByID(uint(id))
	if err != nil {
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

	creatorID := c.Locals("user_id").(uint)
	user, plainPassword, err := h.userService.CreateUser(req.Name, req.Email, req.Password, role, &creatorID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"user":     user,
		"password": plainPassword,
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

	var req CreateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	user, err := h.userService.UpdateUser(uint(id), req.Name, req.Email, req.Password)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
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

	if err := h.userService.DeleteUser(uint(id)); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
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

		user, plainPassword, err := h.userService.CreateUser(name, email, "", models.RoleStudent, &creatorID)
		if err != nil {
			errors = append(errors, fmt.Sprintf("Row %d: %s", i+2, err.Error()))
			continue
		}

		created = append(created, fiber.Map{
			"id":       user.ID,
			"name":     user.Name,
			"email":    user.Email,
			"password": plainPassword,
		})
	}

	return c.JSON(fiber.Map{
		"created": created,
		"errors":  errors,
		"total":   len(created),
	})
}


package project

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"
	"strings"
	"unsafe"

	"github.com/glebarez/sqlite"
	"github.com/go-redis/redismock/v9"
	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/backend/internal/services"
	projectServicePkg "github.com/laravel-paas/backend/internal/services/project"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/repositories"
	"github.com/laravel-paas/shared/services/setting"
	"gorm.io/gorm"
)

// Helper to set unexported fields via reflection
func setUnexportedField(field reflect.Value, value interface{}) {
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(reflect.ValueOf(value))
}

// Setup a clean in-memory database and project handler with a unique database name
func setupTestApp(t *testing.T, dbName string) (*fiber.App, *gorm.DB, redismock.ClientMock) {
	// 1. Initialize SQLite in-memory DB with unique name to isolate state between tests
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", dbName)), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}

	// 2. Auto-migrate tables
	err = db.AutoMigrate(
		&models.User{},
		&models.Project{},
		&models.DatabaseInstance{},
		&models.SecretStore{},
		&models.SecretStoreItem{},
		&models.SecretStoreItemValue{},
		&models.SecretStoreBinding{},
		&models.Setting{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	// Create test user
	user := models.User{
		Email: "test@example.com",
		Role:  models.RoleUser,
	}
	db.Create(&user)

	// 3. Configure mock RedisService
	redisClient, mock := redismock.NewClientMock()
	redisService := &infrastructure.RedisService{}
	val := reflect.ValueOf(redisService).Elem()
	setUnexportedField(val.FieldByName("client"), redisClient)
	setUnexportedField(val.FieldByName("ctx"), context.Background())

	// 4. Create handler dependencies
	cfg := &config.Config{
		JWTSecret:               "test-secret-key-1234567890-test-secret-key-1234567890",
		UIDSalt:                 "test-salt",
		CredentialEncryptionKey: "test-key-32-chars-long-123456789", // 32 bytes
	}

	projectRepo := repositories.NewProjectRepository(db)
	userRepo := repositories.NewUserRepository(db)
	settingRepo := repositories.NewSettingRepository(db)

	settingService := setting.NewSettingService(settingRepo, redisService)
	projectService := projectServicePkg.NewProjectService(
		cfg,
		projectRepo,
		settingService,
		nil, // dockerService
		nil, // storageService
		nil, // mysqlService
		redisService,
		nil, // transitionManager
	)
	userService := services.NewUserService(userRepo, projectService)
	secretStoreService := services.NewSecretStoreService(db, cfg, redisService)

	projectHandler := NewProjectHandler(
		cfg,
		db,
		redisService,
		projectService,
		userService,
		nil, // dockerService
		secretStoreService,
	)

	// 5. Initialize Fiber App and register local route for testing
	app := fiber.New()
	app.Post("/projects", func(c *fiber.Ctx) error {
		c.Locals("user_id", user.ID)
		c.Locals("role", string(user.Role))
		return projectHandler.Create(c)
	})

	return app, db, mock
}

func TestCreateProject_InvalidDatabaseOption(t *testing.T) {
	app, _, _ := setupTestApp(t, "invalid_option_db")

	reqPayload := CreateProjectRequest{
		Name:           "Test Project",
		GithubURL:      "https://github.com/test/repo",
		Branch:         "main",
		DatabaseOption: "invalid_option",
	}
	body, _ := json.Marshal(reqPayload)

	req := httptest.NewRequest("POST", "/projects", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("failed to run request: %v", err)
	}

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, resp.StatusCode)
	}

	var respJSON map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&respJSON)
	expectedErr := "Invalid database_option. Must be one of: none, sqlite, new, existing, external"
	if respJSON["error"] != expectedErr {
		t.Errorf("expected error %q, got %q", expectedErr, respJSON["error"])
	}
}

func TestCreateProject_RollbackOnAttachFailure(t *testing.T) {
	app, db, mock := setupTestApp(t, "rollback_attach_db")

	// Create an existing database owned by the user (who has ID 1)
	existingDb := models.DatabaseInstance{
		UserID: 1,
		Engine: "mysql",
		Name:   "shared_db",
	}
	db.Create(&existingDb)

	// Create another project that already has this database name to trigger unique index violation on save
	existingProject := models.Project{
		UserID:       1,
		Name:         "Other Project",
		GithubURL:    "https://github.com/test/other",
		Subdomain:    "other",
		DatabaseName: &existingDb.Name,
	}
	db.Create(&existingProject)

	// Mock settings expectations as valid JSON strings
	mock.ExpectGet("setting:max_projects_per_user").SetVal("\"3\"")
	mock.ExpectGet("setting:project_expiry_days").SetVal("\"0\"")

	reqPayload := CreateProjectRequest{
		Name:                "New Project",
		GithubURL:           "https://github.com/test/new",
		Branch:              "main",
		DatabaseOption:      "existing",
		ExistingDatabaseUID: existingDb.UID,
	}
	body, _ := json.Marshal(reqPayload)

	req := httptest.NewRequest("POST", "/projects", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("failed to run request: %v", err)
	}

	if resp.StatusCode == http.StatusCreated {
		t.Errorf("expected failure, but got 201 Created")
	}

	// Verify project was NOT created
	var count int64
	db.Model(&models.Project{}).Where("name = ?", "New Project").Count(&count)
	if count > 0 {
		t.Errorf("expected project to not be created, but found %d projects", count)
	}

	// Verify database ProjectID was NOT updated
	var updatedDb models.DatabaseInstance
	db.First(&updatedDb, existingDb.ID)
	if updatedDb.ProjectID != nil {
		t.Errorf("expected database project_id to remain nil, but got %v", *updatedDb.ProjectID)
	}

	// Verify mock expectations
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("redis mock expectations were not met: %v", err)
	}
}

func TestCreateProject_RollbackOnSecretStoreFailure(t *testing.T) {
	app, db, mock := setupTestApp(t, "rollback_secretstore_db")

	// Inject a GORM callback to force creation failure for secret_stores table
	_ = db.Callback().Create().Before("gorm:create").Register("fail_secret_stores", func(tx *gorm.DB) {
		if tx.Statement.Table == "secret_stores" {
			_ = tx.AddError(errors.New("mocked secret store creation failure"))
		}
	})
	defer func() { _ = db.Callback().Create().Remove("fail_secret_stores") }()

	// Mock settings expectations as valid JSON strings
	mock.ExpectGet("setting:max_projects_per_user").SetVal("\"3\"")
	mock.ExpectGet("setting:project_expiry_days").SetVal("\"0\"")

	reqPayload := CreateProjectRequest{
		Name:           "SQLite Project",
		GithubURL:      "https://github.com/test/sqlite-repo",
		Branch:         "main",
		DatabaseOption: "sqlite",
	}
	body, _ := json.Marshal(reqPayload)

	req := httptest.NewRequest("POST", "/projects", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("failed to run request: %v", err)
	}

	if resp.StatusCode == http.StatusCreated {
		t.Errorf("expected failure, but got 201 Created")
	}

	// Verify project was NOT created
	var count int64
	db.Model(&models.Project{}).Where("name = ?", "SQLite Project").Count(&count)
	if count > 0 {
		t.Errorf("expected project to not be created, but found %d projects", count)
	}

	// Verify mock expectations
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("redis mock expectations were not met: %v", err)
	}
}

func TestCreateProject_ConcurrentAttach(t *testing.T) {
	app, db, mock := setupTestApp(t, "concurrent_db")

	// Create an existing database owned by the user (who has ID 1)
	existingDb := models.DatabaseInstance{
		UserID: 1,
		Engine: "mysql",
		Name:   "concurrent_db",
	}
	db.Create(&existingDb)

	// Disable order matching because of concurrent requests interleaving Redis calls
	mock.MatchExpectationsInOrder(false)

	// Mock settings expectations for the successful request as valid JSON strings
	mock.ExpectGet("setting:max_projects_per_user").SetVal("\"3\"")
	mock.ExpectGet("setting:project_expiry_days").SetVal("\"0\"")

	// Mock queue expectations for the single successful enqueue
	mock.ExpectLRange("deployment:queue", 0, -1).RedisNil()
	mock.ExpectZRange("deployment:delayed_queue", 0, -1).RedisNil()
	mock.Regexp().ExpectRPush("deployment:queue", ".*").SetVal(1)
	mock.ExpectHIncrBy("deployment:stats", "enqueued", 1).SetVal(1)
	mock.ExpectLLen("deployment:queue").SetVal(1)

	// Mock settings expectations checked at the end of successful create (PopulateURL)
	mock.ExpectGet("setting:project_domain").SetVal("\"localhost\"")

	var wg sync.WaitGroup
	wg.Add(2)

	results := make(chan int, 2)
	errorsChan := make(chan error, 2)

	runRequest := func(projName, subdomain string, delay time.Duration) {
		defer wg.Done()

		if delay > 0 {
			time.Sleep(delay)
		}

		reqPayload := CreateProjectRequest{
			Name:                projName,
			GithubURL:           "https://github.com/test/repo",
			Branch:              "main",
			DatabaseOption:      "existing",
			ExistingDatabaseUID: existingDb.UID,
		}
		body, _ := json.Marshal(reqPayload)

		req := httptest.NewRequest("POST", "/projects", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req, 10000) // Increase timeout to 10s for concurrency test
		if err != nil {
			errorsChan <- err
			return
		}
		results <- resp.StatusCode
	}

	// Fire both with a tiny delay to ensure SQLite locks serialize properly
	go runRequest("Project 1", "proj1", 0)
	go runRequest("Project 2", "proj2", 50*time.Millisecond)

	wg.Wait()
	close(results)
	close(errorsChan)

	for err := range errorsChan {
		t.Fatalf("concurrent request failed: %v", err)
	}

	statusCodes := []int{}
	for status := range results {
		statusCodes = append(statusCodes, status)
	}

	if len(statusCodes) != 2 {
		t.Fatalf("expected 2 status codes, got %d", len(statusCodes))
	}

	// One should succeed (201) and one should fail (400)
	successCount := 0
	failureCount := 0
	for _, status := range statusCodes {
		if status == http.StatusCreated {
			successCount++
		} else if status == http.StatusBadRequest {
			failureCount++
		}
	}

	if successCount != 1 || failureCount != 1 {
		t.Errorf("expected exactly 1 success (201) and 1 failure (400), got success=%d failure=%d", successCount, failureCount)
	}

	// Verify database is attached to exactly one project
	var checkDb models.DatabaseInstance
	db.First(&checkDb, existingDb.ID)
	if checkDb.ProjectID == nil {
		t.Errorf("expected database to be attached to a project, but project_id is nil")
	}

	var attachedProject models.Project
	if err := db.First(&attachedProject, *checkDb.ProjectID).Error; err != nil {
		t.Fatalf("failed to load attached project: %v", err)
	}
	if attachedProject.DatabaseOption != "existing" {
		t.Errorf("expected attached project database_option existing, got %q", attachedProject.DatabaseOption)
	}

	// Verify mock expectations
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("redis mock expectations were not met: %v", err)
	}
}

func TestCreateProject_DatabaseValidation(t *testing.T) {
	t.Run("uppercase database name", func(t *testing.T) {
		app, _, mock := setupTestApp(t, "db_val_upper_name")
		mock.ExpectGet("setting:max_projects_per_user").SetVal("\"3\"")
		mock.ExpectGet("setting:project_expiry_days").SetVal("\"0\"")

		reqPayload := CreateProjectRequest{
			Name:           "Test Project",
			GithubURL:      "https://github.com/test/repo",
			Branch:         "main",
			DatabaseOption: "new",
			DatabaseName:   "UPPERCASE_DB",
			DatabaseEngine: "mysql",
		}
		body, _ := json.Marshal(reqPayload)

		req := httptest.NewRequest("POST", "/projects", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("failed to run request: %v", err)
		}

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, resp.StatusCode)
		}
	})

	t.Run("uppercase database username", func(t *testing.T) {
		app, _, mock := setupTestApp(t, "db_val_upper_user")
		mock.ExpectGet("setting:max_projects_per_user").SetVal("\"3\"")
		mock.ExpectGet("setting:project_expiry_days").SetVal("\"0\"")

		reqPayload := CreateProjectRequest{
			Name:             "Test Project",
			GithubURL:        "https://github.com/test/repo",
			Branch:           "main",
			DatabaseOption:   "new",
			DatabaseName:     "lowercase_db",
			DatabaseUsername: "UPPERCASE_USER",
			DatabaseEngine:   "mysql",
		}
		body, _ := json.Marshal(reqPayload)

		req := httptest.NewRequest("POST", "/projects", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("failed to run request: %v", err)
		}

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, resp.StatusCode)
		}
	})

	t.Run("weak database password", func(t *testing.T) {
		app, _, mock := setupTestApp(t, "db_val_weak_pass")
		mock.ExpectGet("setting:max_projects_per_user").SetVal("\"3\"")
		mock.ExpectGet("setting:project_expiry_days").SetVal("\"0\"")

		reqPayload := CreateProjectRequest{
			Name:             "Test Project",
			GithubURL:        "https://github.com/test/repo",
			Branch:           "main",
			DatabaseOption:   "new",
			DatabaseName:     "lowercase_db",
			DatabaseUsername: "lowercase_user",
			DatabasePassword: "weak",
			DatabaseEngine:   "mysql",
		}
		body, _ := json.Marshal(reqPayload)

		req := httptest.NewRequest("POST", "/projects", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("failed to run request: %v", err)
		}

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, resp.StatusCode)
		}
	})

	t.Run("distinct name and username persistence", func(t *testing.T) {
		app, db, mock := setupTestApp(t, "db_val_distinct")
		mock.ExpectGet("setting:max_projects_per_user").SetVal("\"3\"")
		mock.ExpectGet("setting:project_expiry_days").SetVal("\"0\"")
		mock.ExpectLRange("deployment:queue", 0, -1).RedisNil()
		mock.ExpectZRange("deployment:delayed_queue", 0, -1).RedisNil()
		mock.Regexp().ExpectRPush("deployment:queue", ".*").SetVal(1)
		mock.ExpectHIncrBy("deployment:stats", "enqueued", 1).SetVal(1)
		mock.ExpectLLen("deployment:queue").SetVal(1)
		mock.ExpectGet("setting:project_domain").SetVal("\"localhost\"")

		reqPayload := CreateProjectRequest{
			Name:             "distinctproj",
			GithubURL:        "https://github.com/test/repo",
			Branch:           "main",
			DatabaseOption:   "new",
			DatabaseName:     "customdb",
			DatabaseUsername: "customuser",
			DatabasePassword: "SecurePassword123",
			DatabaseEngine:   "mysql",
		}
		body, _ := json.Marshal(reqPayload)

		req := httptest.NewRequest("POST", "/projects", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("failed to run request: %v", err)
		}

		if resp.StatusCode != http.StatusCreated {
			var respJSON map[string]string
			_ = json.NewDecoder(resp.Body).Decode(&respJSON)
			t.Fatalf("expected status 201, got %d. Error: %s", resp.StatusCode, respJSON["error"])
		}

		var createdProject models.Project
		if err := db.Preload("DatabaseInstance").Where("name = ?", "distinctproj").First(&createdProject).Error; err != nil {
			t.Fatalf("failed to find created project: %v", err)
		}

		if createdProject.DatabaseInstance == nil {
			t.Fatalf("expected DatabaseInstance to be created and associated, got nil")
		}

		// DatabaseInstance Name gets the suffix appended, but Username should match exactly what was requested (or be distinct)
		if createdProject.DatabaseInstance.Username != "customuser" {
			t.Errorf("expected DatabaseInstance.Username to be 'customuser', got %q", createdProject.DatabaseInstance.Username)
		}
		if createdProject.DatabaseInstance.Password != "SecurePassword123" {
			t.Errorf("expected DatabaseInstance.Password to be 'SecurePassword123', got %q", createdProject.DatabaseInstance.Password)
		}
	})

	t.Run("spaced database username trimming", func(t *testing.T) {
		app, db, mock := setupTestApp(t, "db_val_spaced_user")
		mock.ExpectGet("setting:max_projects_per_user").SetVal("\"3\"")
		mock.ExpectGet("setting:project_expiry_days").SetVal("\"0\"")
		mock.ExpectLRange("deployment:queue", 0, -1).RedisNil()
		mock.ExpectZRange("deployment:delayed_queue", 0, -1).RedisNil()
		mock.Regexp().ExpectRPush("deployment:queue", ".*").SetVal(1)
		mock.ExpectHIncrBy("deployment:stats", "enqueued", 1).SetVal(1)
		mock.ExpectLLen("deployment:queue").SetVal(1)
		mock.ExpectGet("setting:project_domain").SetVal("\"localhost\"")

		reqPayload := CreateProjectRequest{
			Name:             "spacedproj",
			GithubURL:        "https://github.com/test/repo",
			Branch:           "main",
			DatabaseOption:   "new",
			DatabaseName:     "customdb",
			DatabaseUsername: " customuser ",
			DatabasePassword: "SecurePassword123",
			DatabaseEngine:   "mysql",
		}
		body, _ := json.Marshal(reqPayload)

		req := httptest.NewRequest("POST", "/projects", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("failed to run request: %v", err)
		}

		if resp.StatusCode != http.StatusCreated {
			var respJSON map[string]string
			_ = json.NewDecoder(resp.Body).Decode(&respJSON)
			t.Fatalf("expected status 201, got %d. Error: %s", resp.StatusCode, respJSON["error"])
		}

		var createdProject models.Project
		if err := db.Preload("DatabaseInstance").Where("name = ?", "spacedproj").First(&createdProject).Error; err != nil {
			t.Fatalf("failed to find created project: %v", err)
		}

		if createdProject.DatabaseInstance.Username != "customuser" {
			t.Errorf("expected DatabaseInstance.Username to be trimmed to 'customuser', got %q", createdProject.DatabaseInstance.Username)
		}
	})

	t.Run("long project name username generation", func(t *testing.T) {
		app, db, mock := setupTestApp(t, "db_val_long_name")
		mock.ExpectGet("setting:max_projects_per_user").SetVal("\"3\"")
		mock.ExpectGet("setting:project_expiry_days").SetVal("\"0\"")
		mock.ExpectLRange("deployment:queue", 0, -1).RedisNil()
		mock.ExpectZRange("deployment:delayed_queue", 0, -1).RedisNil()
		mock.Regexp().ExpectRPush("deployment:queue", ".*").SetVal(1)
		mock.ExpectHIncrBy("deployment:stats", "enqueued", 1).SetVal(1)
		mock.ExpectLLen("deployment:queue").SetVal(1)
		mock.ExpectGet("setting:project_domain").SetVal("\"localhost\"")

		reqPayload := CreateProjectRequest{
			Name:           "this-is-a-very-long-project-name-to-test-generated-defaults-and-suffix-preservation",
			GithubURL:      "https://github.com/test/repo",
			Branch:         "main",
			DatabaseOption: "new",
			DatabaseEngine: "mysql",
		}
		body, _ := json.Marshal(reqPayload)

		req := httptest.NewRequest("POST", "/projects", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("failed to run request: %v", err)
		}

		if resp.StatusCode != http.StatusCreated {
			var respJSON map[string]string
			_ = json.NewDecoder(resp.Body).Decode(&respJSON)
			t.Fatalf("expected status 201, got %d. Error: %s", resp.StatusCode, respJSON["error"])
		}

		var createdProject models.Project
		if err := db.Preload("DatabaseInstance").Where("name = ?", reqPayload.Name).First(&createdProject).Error; err != nil {
			t.Fatalf("failed to find created project: %v", err)
		}

		// Extract suffix from subdomain
		parts := strings.Split(createdProject.Subdomain, "-")
		suffix := parts[len(parts)-1]

		username := createdProject.DatabaseInstance.Username
		if !strings.HasSuffix(username, suffix) {
			t.Errorf("expected generated username %q to end with suffix %q", username, suffix)
		}
		if len(username) > 32 {
			t.Errorf("expected generated username length to be <= 32, got %d (value: %q)", len(username), username)
		}
		if !strings.HasPrefix(username, "dbuser_") {
			t.Errorf("expected generated username %q to start with 'dbuser_'", username)
		}
	})
}

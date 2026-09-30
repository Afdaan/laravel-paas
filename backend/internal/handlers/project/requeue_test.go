package project

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	githubhandlers "github.com/laravel-paas/backend/internal/handlers"
	"github.com/laravel-paas/backend/internal/services"
	projectservice "github.com/laravel-paas/backend/internal/services/project"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/repositories"
	"github.com/laravel-paas/shared/services/deployment"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type adminRequeuePublishHook struct {
	onPublish func(string)
}

func (hook adminRequeuePublishHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func extractJobFromCmd(cmd redis.Cmder) *infrastructure.DeploymentJob {
	for _, arg := range cmd.Args() {
		var job infrastructure.DeploymentJob
		if err := json.Unmarshal([]byte(fmt.Sprint(arg)), &job); err == nil && job.JobID != "" {
			return &job
		}
	}
	return nil
}

func (hook adminRequeuePublishHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (hook adminRequeuePublishHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		isPublish := (cmd.Name() == "evalsha" || cmd.Name() == "eval") && strings.Contains(fmt.Sprint(cmd.Args()), "deployment:stats")
		err := next(ctx, cmd)
		if err == nil && isPublish {
			if job := extractJobFromCmd(cmd); job != nil && hook.onPublish != nil {
				hook.onPublish(job.JobID)
			}
		}
		return err
	}
}

type adminRequeueFaultHook struct {
	beforePublish func(string)
	afterReserve  func()
	onPublish     func(string)
	injectErr     error
	injectOnCmd   func(cmd redis.Cmder) bool
	skipServer    bool
	inProgress    *bool
}

func (hook adminRequeueFaultHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (hook adminRequeueFaultHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (hook adminRequeueFaultHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if hook.inProgress != nil && *hook.inProgress {
			return next(ctx, cmd)
		}
		isPublish := (cmd.Name() == "evalsha" || cmd.Name() == "eval") && strings.Contains(fmt.Sprint(cmd.Args()), "deployment:stats")
		isReserve := (cmd.Name() == "evalsha" || cmd.Name() == "eval") && strings.Contains(fmt.Sprint(cmd.Args()), "deployment:processing_queue") && strings.Contains(fmt.Sprint(cmd.Args()), "deployment:lock:")
		if hook.injectOnCmd != nil && hook.injectOnCmd(cmd) {
			if hook.skipServer {
				return hook.injectErr
			}
			_ = next(ctx, cmd)
			return hook.injectErr
		}
		if isPublish {
			if hook.beforePublish != nil {
				if job := extractJobFromCmd(cmd); job != nil {
					if hook.inProgress != nil {
						*hook.inProgress = true
					}
					hook.beforePublish(job.JobID)
					if hook.inProgress != nil {
						*hook.inProgress = false
					}
				}
			}
			if hook.skipServer {
				return hook.injectErr
			}
			if hook.inProgress != nil {
				*hook.inProgress = true
				defer func() { *hook.inProgress = false }()
			}
			err := next(ctx, cmd)
			if err == nil {
				if job := extractJobFromCmd(cmd); job != nil && hook.onPublish != nil {
					hook.onPublish(job.JobID)
				}
				if hook.injectErr != nil {
					return hook.injectErr
				}
			}
			return err
		}
		err := next(ctx, cmd)
		if err == nil && isReserve && hook.afterReserve != nil {
			if hook.inProgress != nil {
				*hook.inProgress = true
				defer func() { *hook.inProgress = false }()
			}
			hook.afterReserve()
		}
		return err
	}
}

func setupRequeueTest(t *testing.T, client *redis.Client) (*ProjectHandler, models.Project, models.User, *gorm.DB, deployment.TransitionManager, *infrastructure.RedisService) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+fmt.Sprintf("%s-%d", t.Name(), time.Now().UnixNano())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&models.User{},
		&models.Project{},
		&models.CustomDomain{},
		&models.DatabaseInstance{},
		&models.DeploymentEvent{},
		&models.SecretStore{},
		&models.SecretStoreItem{},
		&models.SecretStoreItemValue{},
		&models.SecretStoreBinding{},
		&models.SecretStoreActivityLog{},
		&models.AuditLog{},
	); err != nil {
		t.Fatal(err)
	}
	user := models.User{Email: fmt.Sprintf("admin-%d@example.com", time.Now().UnixNano()), Role: models.RoleAdmin}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	project := models.Project{
		ID: uint(time.Now().UnixNano() % 1000000000), UID: fmt.Sprintf("req-%d", time.Now().UnixNano()), UserID: user.ID,
		Name: "admin-requeue", Subdomain: fmt.Sprintf("sub-%d", time.Now().UnixNano()), DeploymentStatus: models.DepStatusBuilding,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	redisService := infrastructure.NewRedisServiceWithClient(client)
	manager := deployment.NewTransitionManager(db, nil)
	repo := repositories.NewProjectRepository(db)
	cfg := &config.Config{
		JWTSecret:               "test-secret-key-1234567890-test-secret-key-1234567890",
		CredentialEncryptionKey: "test-key-32-chars-long-123456789",
	}
	service := projectservice.NewProjectService(cfg, repo, nil, nil, nil, nil, redisService, manager)
	secretStoreService := services.NewSecretStoreService(db, cfg, redisService)
	handler := &ProjectHandler{
		cfg:                cfg,
		db:                 db,
		redisService:       redisService,
		projectService:     service,
		secretStoreService: secretStoreService,
	}
	return handler, project, user, db, manager, redisService
}

func completeAndAcknowledgePublishedJob(t *testing.T, redisService *infrastructure.RedisService, manager deployment.TransitionManager, projectID uint, jobID string) {
	t.Helper()
	job, err := redisService.DequeueDeployment(time.Second)
	if err != nil || job == nil || job.JobID != jobID {
		t.Fatalf("dequeue published job %s: job=%v err=%v", jobID, job, err)
	}
	for _, status := range []models.DeploymentStatus{models.DepStatusPreparing, models.DepStatusCompleted} {
		if _, err := manager.TransitionState(context.Background(), projectID, jobID, status, 100, "worker_update", "worker completed job"); err != nil {
			t.Fatalf("transition published job to %s: %v", status, err)
		}
	}
	if err := redisService.AcknowledgeDeployment(job); err != nil {
		t.Fatal(err)
	}
}

func publishAdminOverride(t *testing.T, redisService *infrastructure.RedisService, manager deployment.TransitionManager, projectID, userID uint, jobID string) {
	t.Helper()
	token, err := redisService.ReserveAdminRequeue(projectID)
	if err != nil || token == "" {
		t.Fatalf("admin reserve: token=%q err=%v", token, err)
	}
	if _, err := manager.RequeueDeployment(context.Background(), projectID, jobID, "Admin override"); err != nil {
		t.Fatal(err)
	}
	job := &infrastructure.DeploymentJob{ProjectID: projectID, UserID: userID, JobID: jobID, Type: "redeploy", EnqueuedAt: time.Now()}
	if err := redisService.EnqueueReplacingDeploymentJob(job, token); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = redisService.RemoveDeploymentJob(jobID) })
}

func TestAdminRequeueDoesNotRewindImmediatePickup(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	t.Cleanup(func() { _ = client.Close() })

	handler, project, user, db, manager, redisService := setupRequeueTest(t, client)

	var publishedJobID string
	client.AddHook(adminRequeuePublishHook{onPublish: func(jobID string) {
		publishedJobID = jobID
		var queued models.Project
		if err := db.First(&queued, project.ID).Error; err != nil {
			t.Fatal(err)
		}
		if queued.Status != models.StatusQueued || queued.DeploymentStatus != models.DepStatusQueued || queued.DeploymentJobID == nil || *queued.DeploymentJobID != jobID {
			t.Fatalf("admin published job before queued identity: project_status=%s deployment_status=%s job=%v", queued.Status, queued.DeploymentStatus, queued.DeploymentJobID)
		}
		if _, err := manager.TransitionState(context.Background(), project.ID, jobID, models.DepStatusPreparing, 10, "picked_up", "worker picked up job"); err != nil {
			t.Fatal(err)
		}
	}})
	app := fiber.New()
	app.Post("/requeue/:id", func(c *fiber.Ctx) error {
		c.Locals("user_id", user.ID)
		c.Locals("role", string(user.Role))
		return handler.RequeueJob(c)
	})
	response, err := app.Test(httptest.NewRequest("POST", "/requeue/"+project.UID, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != fiber.StatusOK {
		t.Fatalf("admin requeue status = %d", response.StatusCode)
	}
	var current models.Project
	if err := db.First(&current, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	if publishedJobID == "" || current.DeploymentStatus != models.DepStatusPreparing || current.DeploymentStartedAt == nil {
		t.Fatalf("admin requeue rewound pickup: published=%q status=%s started=%v", publishedJobID, current.DeploymentStatus, current.DeploymentStartedAt)
	}
	if err := redisService.RemoveDeploymentJob(publishedJobID); err != nil {
		t.Fatal(err)
	}
}

func TestAdminRequeueSucceedsWhenPublishSucceedsWithClientErrorAndPickup(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	t.Cleanup(func() { _ = client.Close() })

	handler, project, user, db, manager, redisService := setupRequeueTest(t, client)

	var publishedJobID string
	client.AddHook(adminRequeueFaultHook{
		injectErr: fmt.Errorf("connection reset by peer after eval"),
		onPublish: func(jobID string) {
			publishedJobID = jobID
			// Simulate immediate worker pickup
			if _, err := manager.TransitionState(context.Background(), project.ID, jobID, models.DepStatusPreparing, 10, "picked_up", "worker picked up job"); err != nil {
				t.Fatal(err)
			}
		},
	})

	app := fiber.New()
	app.Post("/requeue/:id", func(c *fiber.Ctx) error {
		c.Locals("user_id", user.ID)
		c.Locals("role", string(user.Role))
		return handler.RequeueJob(c)
	})

	response, err := app.Test(httptest.NewRequest("POST", "/requeue/"+project.UID, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	if response.StatusCode != fiber.StatusOK {
		t.Fatalf("admin requeue expected 200 OK on recovered publish, got %d", response.StatusCode)
	}

	var current models.Project
	if err := db.First(&current, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	// Verify deployment state was NOT overwritten to failed; worker's preparing state is preserved!
	if current.DeploymentStatus != models.DepStatusPreparing {
		t.Fatalf("expected deployment status to stay preparing, got %s", current.DeploymentStatus)
	}
	if current.DeploymentJobID == nil || *current.DeploymentJobID != publishedJobID {
		t.Fatalf("expected job ID %s, got %v", publishedJobID, current.DeploymentJobID)
	}

	if err := redisService.RemoveDeploymentJob(publishedJobID); err != nil {
		t.Fatal(err)
	}
}

func TestAdminRequeueMarksFailedOnGenuineEnqueueFailure(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	t.Cleanup(func() { _ = client.Close() })

	handler, project, user, db, _, _ := setupRequeueTest(t, client)

	client.AddHook(adminRequeueFaultHook{
		skipServer: true,
		injectErr:  fmt.Errorf("redis network unreachable"),
	})

	app := fiber.New()
	app.Post("/requeue/:id", func(c *fiber.Ctx) error {
		c.Locals("user_id", user.ID)
		c.Locals("role", string(user.Role))
		return handler.RequeueJob(c)
	})

	response, err := app.Test(httptest.NewRequest("POST", "/requeue/"+project.UID, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	if response.StatusCode != fiber.StatusInternalServerError {
		t.Fatalf("admin requeue expected 500 on genuine failure, got %d", response.StatusCode)
	}

	var current models.Project
	if err := db.First(&current, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	// Verify genuine failure marks failed
	if current.DeploymentStatus != models.DepStatusFailed {
		t.Fatalf("expected deployment status to be failed, got %s", current.DeploymentStatus)
	}
	if current.Status != models.StatusFailed {
		t.Fatalf("expected project status to be failed, got %s", current.Status)
	}
}

func TestRunningProjectSurvivesFailedPublish(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	for _, source := range []string{"redeploy", "admin", "webhook"} {
		for _, failure := range []string{"rejected", "expired_token"} {
			t.Run(source+"/"+failure, func(t *testing.T) {
				client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
				t.Cleanup(func() { _ = client.Close() })
				if err := client.FlushDB(context.Background()).Err(); err != nil {
					t.Fatal(err)
				}
				handler, project, user, db, _, redisService := setupRequeueTest(t, client)
				handler.cfg.ProjectsPath = t.TempDir()
				containerID := "serving-container"
				if err := db.Model(&project).Updates(map[string]interface{}{
					"status": models.StatusRunning, "container_id": containerID,
					"deployment_status": models.DepStatusCompleted,
					"github_repo_owner": "owner", "github_repo_name": "repo", "branch": "main",
					"github_installation_id": int64(0),
				}).Error; err != nil {
					t.Fatal(err)
				}
				inProgress := false
				hook := adminRequeueFaultHook{inProgress: &inProgress}
				if failure == "rejected" {
					hook.skipServer = true
					hook.injectErr = errors.New("publish rejected")
				} else {
					hook.beforePublish = func(string) {
						if err := redisService.ForceReleaseDeploymentLock(project.ID, "test expired token"); err != nil {
							t.Fatal(err)
						}
					}
				}
				client.AddHook(hook)
				app := fiber.New()
				var request *http.Request
				switch source {
				case "redeploy":
					app.Post("/projects/:id/redeploy", func(c *fiber.Ctx) error {
						c.Locals("user_id", user.ID)
						c.Locals("role", string(user.Role))
						return handler.Redeploy(c)
					})
					request = httptest.NewRequest("POST", "/projects/"+project.UID+"/redeploy", nil)
				case "admin":
					app.Post("/projects/:id/requeue", func(c *fiber.Ctx) error {
						c.Locals("user_id", user.ID)
						c.Locals("role", string(user.Role))
						return handler.RequeueJob(c)
					})
					request = httptest.NewRequest("POST", "/projects/"+project.UID+"/requeue", nil)
				case "webhook":
					handler.cfg.GithubAppWebhookSecret = "test-webhook-secret"
					webhook := githubhandlers.NewGithubAppHandler(db, handler.cfg, infrastructure.NewGithubService(handler.cfg, redisService), redisService, handler.projectService)
					app.Post("/webhook", webhook.Webhook)
					payload := []byte(`{"ref":"refs/heads/main","repository":{"name":"repo","owner":{"login":"owner"}},"head_commit":{"id":"2222222222222222222222222222222222222222"},"installation":{"id":0}}`)
					signature := hmac.New(sha256.New, []byte(handler.cfg.GithubAppWebhookSecret))
					_, _ = signature.Write(payload)
					request = httptest.NewRequest("POST", "/webhook", bytes.NewReader(payload))
					request.Header.Set("X-GitHub-Event", "push")
					request.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(signature.Sum(nil)))
				}
				response, err := app.Test(request, 5000)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				if source == "webhook" && response.StatusCode != fiber.StatusOK {
					t.Fatalf("webhook status=%d", response.StatusCode)
				}
				if source != "webhook" && response.StatusCode < fiber.StatusBadRequest {
					t.Fatalf("publish failure returned success: %d", response.StatusCode)
				}
				var current models.Project
				if err := db.First(&current, project.ID).Error; err != nil {
					t.Fatal(err)
				}
				if current.Status != models.StatusRunning || current.DeploymentStatus != models.DepStatusFailed || current.ContainerID == nil || *current.ContainerID != containerID {
					t.Fatalf("publish failure changed serving runtime: %+v", current)
				}
				running, err := repositories.NewProjectRepository(db).GetRunningWithContainers()
				if err != nil || len(running) != 1 || running[0].ID != project.ID {
					t.Fatalf("watchdog lost serving container: %+v %v", running, err)
				}
			})
		}
	}
}

func TestAdminRequeuePreservesQueuedWhenReconciliationFails(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	t.Cleanup(func() { _ = client.Close() })

	handler, project, user, db, _, _ := setupRequeueTest(t, client)

	client.AddHook(adminRequeueFaultHook{
		injectOnCmd: func(cmd redis.Cmder) bool {
			// Fail both the enqueue eval and any subsequent check script
			return cmd.Name() == "evalsha" || cmd.Name() == "eval"
		},
		skipServer: true,
		injectErr:  fmt.Errorf("simulated total redis partition"),
	})

	app := fiber.New()
	app.Post("/requeue/:id", func(c *fiber.Ctx) error {
		c.Locals("user_id", user.ID)
		c.Locals("role", string(user.Role))
		return handler.RequeueJob(c)
	})

	response, err := app.Test(httptest.NewRequest("POST", "/requeue/"+project.UID, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	if response.StatusCode != fiber.StatusServiceUnavailable {
		t.Fatalf("admin requeue expected 503 on unverified check, got %d", response.StatusCode)
	}

	var current models.Project
	if err := db.First(&current, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	// Verify state is PRESERVED as queued, NOT failed
	if current.DeploymentStatus != models.DepStatusQueued {
		t.Fatalf("expected deployment status to remain queued, got %s", current.DeploymentStatus)
	}
	if current.Status != models.StatusQueued {
		t.Fatalf("expected project status to remain queued, got %s", current.Status)
	}
}

func TestAdminRequeueInterleavedReplacesDoNotOverwriteNewerJob(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	t.Cleanup(func() { _ = client.Close() })

	handler, project, user, db, manager, redisService := setupRequeueTest(t, client)

	jobBID := fmt.Sprintf("job-B-%d", time.Now().UnixNano())

	// When Admin A's publish hook fires:
	// 1. Admin B concurrently intervenes: updates DB to queued with Job B, and enqueues Job B into Redis.
	// 2. Admin A's publish returns a transport error.
	var jobAID string
	inProgress := false
	client.AddHook(adminRequeueFaultHook{
		inProgress: &inProgress,
		onPublish: func(publishedAID string) {
			jobAID = publishedAID

			// Concurrent Admin B operation:
			// Directly transition DB to queued with job B
			if _, err := manager.TransitionState(context.Background(), project.ID, jobBID, models.DepStatusQueued, 0, "admin_b_requeue", "Admin B requeue"); err != nil {
				t.Fatalf("admin B transition: %v", err)
			}
			jobB := &infrastructure.DeploymentJob{
				ProjectID: project.ID, UserID: user.ID, Type: "redeploy", JobID: jobBID, EnqueuedAt: time.Now(),
			}
			// EnqueueReplacingDeploymentJob will replace Job A with Job B in Redis queue!
			if err := redisService.EnqueueReplacingDeploymentJob(jobB); err != nil {
				t.Fatalf("admin B enqueue: %v", err)
			}
		},
		injectErr: fmt.Errorf("transport timeout on admin A publish"),
	})

	app := fiber.New()
	app.Post("/requeue/:id", func(c *fiber.Ctx) error {
		c.Locals("user_id", user.ID)
		c.Locals("role", string(user.Role))
		return handler.RequeueJob(c)
	})

	// Call Admin A's requeue
	response, err := app.Test(httptest.NewRequest("POST", "/requeue/"+project.UID, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	var current models.Project
	if err := db.First(&current, project.ID).Error; err != nil {
		t.Fatal(err)
	}

	// Job A failed its enqueue, but Job B took over ownership before Admin A's compensation ran.
	// CRITICAL ASSERTION: Admin A's stale compensation must NOT have overwritten Job B to failed!
	if current.DeploymentJobID == nil || *current.DeploymentJobID != jobBID {
		t.Fatalf("expected project to remain owned by Job B (%s), but got %v", jobBID, current.DeploymentJobID)
	}
	if current.DeploymentStatus == models.DepStatusFailed {
		t.Fatalf("Admin A's stale compensation overwrote Job B to failed!")
	}
	if current.DeploymentStatus != models.DepStatusQueued {
		t.Fatalf("expected deployment status to be queued for Job B, got %s", current.DeploymentStatus)
	}
	if current.Status == models.StatusFailed {
		t.Fatalf("expected project status not to be failed, got %s", current.Status)
	}

	// Verify Job B is indeed in Redis
	hasJobB, err := redisService.HasDeploymentJob(jobBID)
	if err != nil || !hasJobB {
		t.Fatalf("Job B should be present in Redis: has=%v err=%v", hasJobB, err)
	}
	// Verify Job A is not in Redis
	hasJobA, _ := redisService.HasDeploymentJob(jobAID)
	if hasJobA {
		t.Fatalf("Job A should have been replaced in Redis")
	}

	_ = redisService.RemoveDeploymentJob(jobBID)
}

func TestAdminRequeueInterleavedTwoSuccessfulPublishes(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	t.Cleanup(func() { _ = client.Close() })

	handler, project, user, db, manager, redisService := setupRequeueTest(t, client)

	jobBID := fmt.Sprintf("job-B-%d", time.Now().UnixNano())

	// Interleaving:
	// When Admin A's publish hook is about to execute:
	// 1. Admin B concurrently intervenes and executes a full requeue (reserve lock, DB update, Redis publish).
	// 2. Admin A's publish then proceeds to Redis with NO injected transport error.
	// 3. Fencing check in Redis must reject Admin A because Admin B superseded the lock token!
	var jobAID string
	inProgress := false
	client.AddHook(adminRequeueFaultHook{
		inProgress: &inProgress,
		onPublish: func(publishedAID string) {
			jobAID = publishedAID

			// Concurrent Admin B operation:
			tokenB, err := redisService.ReserveAdminRequeue(project.ID)
			if err != nil {
				t.Fatalf("admin B reserve: %v", err)
			}
			if _, err := manager.RequeueDeployment(context.Background(), project.ID, jobBID, "Admin B requeue"); err != nil {
				t.Fatalf("admin B requeue DB: %v", err)
			}
			jobB := &infrastructure.DeploymentJob{
				ProjectID: project.ID, UserID: user.ID, Type: "redeploy", JobID: jobBID, EnqueuedAt: time.Now(),
			}
			if err := redisService.EnqueueReplacingDeploymentJob(jobB, tokenB); err != nil {
				t.Fatalf("admin B enqueue: %v", err)
			}
		},
		// NO transport error injected! Both publishes attempt to succeed.
	})

	app := fiber.New()
	app.Post("/requeue/:id", func(c *fiber.Ctx) error {
		c.Locals("user_id", user.ID)
		c.Locals("role", string(user.Role))
		return handler.RequeueJob(c)
	})

	response, err := app.Test(httptest.NewRequest("POST", "/requeue/"+project.UID, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	if response.StatusCode != fiber.StatusOK {
		t.Fatalf("admin requeue expected 200, got %d", response.StatusCode)
	}

	var current models.Project
	if err := db.First(&current, project.ID).Error; err != nil {
		t.Fatal(err)
	}

	// CRITICAL ASSERTION: DB and Redis MUST agree on Job B!
	// Admin A's later publish must NOT have replaced Job B in Redis!
	if current.DeploymentJobID == nil || *current.DeploymentJobID != jobBID {
		t.Fatalf("expected project in DB to remain owned by Job B (%s), but got %v", jobBID, current.DeploymentJobID)
	}
	if current.DeploymentStatus != models.DepStatusQueued {
		t.Fatalf("expected deployment status to be queued for Job B, got %s", current.DeploymentStatus)
	}
	if current.Status != models.StatusQueued {
		t.Fatalf("expected project status to be queued for Job B, got %s", current.Status)
	}

	// Verify Job B is in Redis
	hasJobB, err := redisService.HasDeploymentJob(jobBID)
	if err != nil || !hasJobB {
		t.Fatalf("Job B should be present in Redis: has=%v err=%v", hasJobB, err)
	}
	// Verify Job A is NOT in Redis
	hasJobA, _ := redisService.HasDeploymentJob(jobAID)
	if hasJobA {
		t.Fatalf("Job A should NOT be present in Redis because its publish was rejected as stale owner!")
	}

	_ = redisService.RemoveDeploymentJob(jobBID)
}

func TestAdminRequeueVersusOwnerRedeployInterleaving(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	t.Cleanup(func() { _ = client.Close() })

	handler, project, user, db, _, redisService := setupRequeueTest(t, client)

	app := fiber.New()
	app.Post("/requeue/:id", func(c *fiber.Ctx) error {
		c.Locals("user_id", user.ID)
		c.Locals("role", "admin")
		return handler.RequeueJob(c)
	})
	app.Post("/redeploy/:id", func(c *fiber.Ctx) error {
		c.Locals("user_id", user.ID)
		c.Locals("role", "user")
		return handler.Redeploy(c)
	})

	var userRedeployStatusCode int
	var userRedeployBody string
	var adminJobID string

	client.AddHook(adminRequeueFaultHook{
		onPublish: func(publishedJobID string) {
			adminJobID = publishedJobID

			// Concurrent user redeploy attempt while Admin holds lock
			req := httptest.NewRequest("POST", "/redeploy/"+project.UID, nil)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("concurrent user redeploy request: %v", err)
			}
			defer resp.Body.Close()
			userRedeployStatusCode = resp.StatusCode
			buf := new(bytes.Buffer)
			_, _ = buf.ReadFrom(resp.Body)
			userRedeployBody = buf.String()
		},
	})

	adminResp, err := app.Test(httptest.NewRequest("POST", "/requeue/"+project.UID, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer adminResp.Body.Close()

	if adminResp.StatusCode != fiber.StatusOK {
		t.Fatalf("admin requeue expected 200, got %d", adminResp.StatusCode)
	}

	// User redeploy should have detected lock/queue in progress
	if !strings.Contains(userRedeployBody, "already in queue") && !strings.Contains(userRedeployBody, "in progress") {
		t.Fatalf("user redeploy should indicate in progress, got status=%d body=%s", userRedeployStatusCode, userRedeployBody)
	}

	// Verify DB state
	var current models.Project
	if err := db.First(&current, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.DeploymentJobID == nil || *current.DeploymentJobID != adminJobID {
		t.Fatalf("expected DB job ID to be %s, got %v", adminJobID, current.DeploymentJobID)
	}
	if current.DeploymentStatus != models.DepStatusQueued {
		t.Fatalf("expected deployment status queued, got %s", current.DeploymentStatus)
	}

	// Verify Redis state
	hasAdminJob, err := redisService.HasDeploymentJob(adminJobID)
	if err != nil || !hasAdminJob {
		t.Fatalf("expected admin job %s in Redis, got %v", adminJobID, hasAdminJob)
	}

	_ = redisService.RemoveDeploymentJob(adminJobID)
}

func TestAdminRequeueFailsWhenCancelledBeforePublish(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	t.Cleanup(func() { _ = client.Close() })

	handler, project, user, db, _, redisService := setupRequeueTest(t, client)

	app := fiber.New()
	app.Post("/requeue/:id", func(c *fiber.Ctx) error {
		c.Locals("user_id", user.ID)
		c.Locals("role", "admin")
		return handler.RequeueJob(c)
	})
	app.Post("/cancel/:id", func(c *fiber.Ctx) error {
		c.Locals("user_id", user.ID)
		c.Locals("role", "admin")
		return handler.CancelQueueJob(c)
	})

	var adminJobID string
	client.AddHook(adminRequeueFaultHook{
		beforePublish: func(publishedJobID string) {
			adminJobID = publishedJobID

			cancelResp, err := app.Test(httptest.NewRequest("POST", "/cancel/"+project.UID, nil))
			if err != nil {
				t.Fatalf("cancel request: %v", err)
			}
			_ = cancelResp.Body.Close()
		},
	})

	response, err := app.Test(httptest.NewRequest("POST", "/requeue/"+project.UID, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	if response.StatusCode != fiber.StatusConflict {
		t.Fatalf("expected 409 Conflict when cancelled before publish, got %d", response.StatusCode)
	}

	var current models.Project
	if err := db.First(&current, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.DeploymentStatus != models.DepStatusCancelled {
		t.Fatalf("expected deployment status Cancelled, got %s", current.DeploymentStatus)
	}

	hasJob, _ := redisService.HasDeploymentJob(adminJobID)
	if hasJob {
		t.Fatalf("job %s must not exist in Redis", adminJobID)
	}
}

func TestAdminRequeueFailsWhenTokenExpiresBeforePublish(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	t.Cleanup(func() { _ = client.Close() })

	handler, project, user, db, _, redisService := setupRequeueTest(t, client)

	app := fiber.New()
	app.Post("/requeue/:id", func(c *fiber.Ctx) error {
		c.Locals("user_id", user.ID)
		c.Locals("role", "admin")
		return handler.RequeueJob(c)
	})

	var adminJobID string
	client.AddHook(adminRequeueFaultHook{
		beforePublish: func(publishedJobID string) {
			adminJobID = publishedJobID
			lockKey := fmt.Sprintf("deployment:lock:%d", project.ID)
			_ = client.Del(context.Background(), lockKey).Err()
		},
	})

	response, err := app.Test(httptest.NewRequest("POST", "/requeue/"+project.UID, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	if response.StatusCode != fiber.StatusConflict {
		t.Fatalf("expected 409 Conflict when token expired before publish, got %d", response.StatusCode)
	}

	var current models.Project
	if err := db.First(&current, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.DeploymentStatus != models.DepStatusFailed {
		t.Fatalf("expected deployment status Failed after expiry compensation, got %s", current.DeploymentStatus)
	}

	hasJob, _ := redisService.HasDeploymentJob(adminJobID)
	if hasJob {
		t.Fatalf("job %s must not exist in Redis", adminJobID)
	}
}

func TestGuardedDeploymentPreventsConcurrentStomp(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	t.Cleanup(func() { _ = client.Close() })

	handler, project, _, db, _, redisService := setupRequeueTest(t, client)
	ctx := context.Background()

	// 1. Normal guarded deployment queues successfully
	jobID1, err := handler.projectService.EnqueueGuardedDeployment(ctx, project.ID, project.UserID, "start", "Starting container")
	if err != nil {
		t.Fatalf("first guarded deployment should succeed: %v", err)
	}
	t.Cleanup(func() { _ = redisService.RemoveDeploymentJob(jobID1) })

	// Verify project in DB was set to queued with jobID1
	var current models.Project
	if err := db.First(&current, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.DeploymentStatus != models.DepStatusQueued || current.DeploymentJobID == nil || *current.DeploymentJobID != jobID1 {
		t.Fatalf("expected project to be queued with %s, got dep_status=%s job=%v", jobID1, current.DeploymentStatus, current.DeploymentJobID)
	}

	// 2. Second concurrent guarded deployment while first is still queued/locked MUST be rejected
	_, err2 := handler.projectService.EnqueueGuardedDeployment(ctx, project.ID, project.UserID, "restart", "Restarting container")
	if err2 == nil {
		t.Fatal("expected second concurrent guarded deployment to be rejected")
	}

	// Verify DB is STILL jobID1 and NOT overwritten
	if err := db.First(&current, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.DeploymentJobID == nil || *current.DeploymentJobID != jobID1 {
		t.Fatalf("DB must remain owned by %s, got %v", jobID1, current.DeploymentJobID)
	}
}

func TestGuardedDeploymentAdminOverrideBeforeDBStage(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	for _, action := range []struct {
		name string
		run  func(*projectservice.ProjectService, *models.Project) error
	}{
		{"stop", (*projectservice.ProjectService).StopProject},
		{"start", (*projectservice.ProjectService).StartProject},
		{"restart", (*projectservice.ProjectService).RestartProject},
	} {
		t.Run(action.name, func(t *testing.T) {
			client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
			t.Cleanup(func() { _ = client.Close() })
			handler, project, _, db, manager, redisService := setupRequeueTest(t, client)
			if err := client.FlushDB(context.Background()).Err(); err != nil {
				t.Fatal(err)
			}
			project.Status = models.StatusRunning
			if err := db.Model(&project).Update("status", project.Status).Error; err != nil {
				t.Fatal(err)
			}
			adminJobID := "admin-before-stage-" + action.name
			inProgress := false
			client.AddHook(adminRequeueFaultHook{
				inProgress: &inProgress,
				afterReserve: func() {
					publishAdminOverride(t, redisService, manager, project.ID, project.UserID, adminJobID)
				},
			})
			if err := action.run(handler.projectService, &project); err == nil {
				t.Fatal("expected superseded producer to fail")
			}
			var current models.Project
			if err := db.First(&current, project.ID).Error; err != nil {
				t.Fatal(err)
			}
			if current.DeploymentJobID == nil || *current.DeploymentJobID != adminJobID || current.DeploymentStatus != models.DepStatusQueued {
				t.Fatalf("admin DB job overwritten: status=%s job=%v", current.DeploymentStatus, current.DeploymentJobID)
			}
			job, err := redisService.DequeueDeployment(time.Second)
			if err != nil || job == nil || job.JobID != adminJobID {
				t.Fatalf("admin job not pickable: job=%v err=%v", job, err)
			}
			if err := redisService.AcknowledgeDeployment(job); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCreateAndRedeployAdminOverrideBeforeDBStage(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	for _, operation := range []string{"create", "redeploy"} {
		t.Run(operation, func(t *testing.T) {
			client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
			t.Cleanup(func() { _ = client.Close() })
			handler, project, user, db, manager, redisService := setupRequeueTest(t, client)
			if err := client.FlushDB(context.Background()).Err(); err != nil {
				t.Fatal(err)
			}
			adminJobID := "admin-before-" + operation
			inProgress := false
			client.AddHook(adminRequeueFaultHook{
				inProgress: &inProgress,
				afterReserve: func() {
					if operation == "create" {
						var created models.Project
						if err := db.Where("name = ?", "create-before-stage").Find(&created).Error; err != nil {
							t.Fatal(err)
						}
						if created.ID == 0 {
							return
						}
						project = created
					}
					publishAdminOverride(t, redisService, manager, project.ID, user.ID, adminJobID)
				},
			})
			app := fiber.New()
			app.Post("/projects", func(c *fiber.Ctx) error {
				c.Locals("user_id", user.ID)
				c.Locals("role", "admin")
				return handler.Create(c)
			})
			app.Post("/redeploy/:id", func(c *fiber.Ctx) error {
				c.Locals("user_id", user.ID)
				c.Locals("role", "admin")
				return handler.Redeploy(c)
			})
			var request *http.Request
			if operation == "create" {
				request = httptest.NewRequest("POST", "/projects", bytes.NewBufferString(`{"name":"create-before-stage","github_url":"https://github.com/test/repo","database_option":"none"}`))
				request.Header.Set("Content-Type", "application/json")
			} else {
				request = httptest.NewRequest("POST", "/redeploy/"+project.UID, nil)
			}
			response, err := app.Test(request, 5000)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if operation == "create" {
				var result struct {
					Project models.Project `json:"project"`
				}
				if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
					t.Fatal(err)
				}
				if response.StatusCode != fiber.StatusCreated || result.Project.DeploymentJobID == nil || *result.Project.DeploymentJobID != adminJobID {
					t.Fatalf("create reported stale job: status=%d job=%v", response.StatusCode, result.Project.DeploymentJobID)
				}
			}
			var current models.Project
			if err := db.First(&current, project.ID).Error; err != nil {
				t.Fatal(err)
			}
			if current.DeploymentJobID == nil || *current.DeploymentJobID != adminJobID || current.DeploymentStatus != models.DepStatusQueued {
				t.Fatalf("admin DB job overwritten: status=%s job=%v response=%d", current.DeploymentStatus, current.DeploymentJobID, response.StatusCode)
			}
			job, err := redisService.DequeueDeployment(time.Second)
			if err != nil || job == nil || job.JobID != adminJobID {
				t.Fatalf("admin job not pickable: job=%v err=%v", job, err)
			}
			if err := redisService.AcknowledgeDeployment(job); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestUpdateEnvPublishErrorAfterWorkerAckSucceeds(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	t.Cleanup(func() { _ = client.Close() })
	handler, project, user, db, manager, redisService := setupRequeueTest(t, client)
	if err := client.FlushDB(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	inProgress := false
	client.AddHook(adminRequeueFaultHook{
		inProgress: &inProgress,
		injectErr:  fmt.Errorf("response lost after env publish"),
		onPublish: func(jobID string) {
			completeAndAcknowledgePublishedJob(t, redisService, manager, project.ID, jobID)
		},
	})
	app := fiber.New()
	app.Put("/projects/:id/env", func(c *fiber.Ctx) error {
		c.Locals("user_id", user.ID)
		c.Locals("role", string(user.Role))
		return handler.UpdateEnv(c)
	})
	request := httptest.NewRequest("PUT", "/projects/"+project.UID+"/env", bytes.NewBufferString(`{"content":"APP_ENV=production"}`))
	request.Header.Set("Content-Type", "application/json")
	response, err := app.Test(request, 5000)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != fiber.StatusOK {
		t.Fatalf("completed env job reported failure: %d", response.StatusCode)
	}
	var current models.Project
	if err := db.First(&current, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.DeploymentStatus != models.DepStatusCompleted || current.DeploymentJobID == nil {
		t.Fatalf("env job not completed: status=%s job=%v", current.DeploymentStatus, current.DeploymentJobID)
	}
	if queued, err := redisService.HasDeploymentJob(*current.DeploymentJobID); err != nil || queued {
		t.Fatalf("ACKed env job still queued: queued=%v err=%v", queued, err)
	}
}

func TestRollbackAdmissionPreservesRunningCommit(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	const runningHash = "1111111111111111111111111111111111111111"
	const targetHash = "2222222222222222222222222222222222222222"
	for _, failure := range []string{"busy", "database", "publish", "instant busy"} {
		t.Run(failure, func(t *testing.T) {
			client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
			t.Cleanup(func() { _ = client.Close() })
			handler, project, user, db, _, redisService := setupRequeueTest(t, client)
			if err := client.FlushDB(context.Background()).Err(); err != nil {
				t.Fatal(err)
			}
			containerID := "running-image-container"
			if err := db.Model(&project).Updates(map[string]interface{}{"last_commit_hash": runningHash, "container_id": containerID, "status": models.StatusRunning}).Error; err != nil {
				t.Fatal(err)
			}
			if failure == "instant busy" {
				if err := exec.Command("docker", "image", "inspect", "redis:alpine").Run(); err != nil {
					t.Skip("Docker test image unavailable")
				}
				tag := "paas-" + project.Subdomain + ":" + targetHash
				if err := exec.Command("docker", "tag", "redis:alpine", tag).Run(); err != nil {
					t.Skip("Docker image tagging unavailable")
				}
				t.Cleanup(func() { _ = exec.Command("docker", "rmi", tag).Run() })
			}
			if failure == "busy" || failure == "instant busy" {
				if _, err := handler.projectService.EnqueueGuardedDeployment(context.Background(), project.ID, user.ID, "redeploy", "Running job"); err != nil {
					t.Fatal(err)
				}
			} else if failure == "database" {
				failDB := false
				client.AddHook(adminRequeueFaultHook{afterReserve: func() { failDB = true }})
				if err := db.Callback().Update().Before("gorm:update").Register("fail_rollback_stage", func(tx *gorm.DB) {
					if failDB && tx.Statement.Table == "projects" {
						tx.AddError(errors.New("private database connection detail"))
					}
				}); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = db.Callback().Update().Remove("fail_rollback_stage") })
			} else {
				client.AddHook(adminRequeueFaultHook{skipServer: true, injectErr: errors.New("private redis address detail")})
			}
			app := fiber.New()
			app.Post("/rollback/:id", func(c *fiber.Ctx) error {
				c.Locals("user_id", user.ID)
				c.Locals("role", string(user.Role))
				return handler.Rollback(c)
			})
			request := httptest.NewRequest("POST", "/rollback/"+project.UID, bytes.NewBufferString(`{"commit_sha":"`+targetHash+`"}`))
			request.Header.Set("Content-Type", "application/json")
			response, err := app.Test(request, 5000)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			wantStatus := fiber.StatusServiceUnavailable
			if failure == "busy" || failure == "instant busy" {
				wantStatus = fiber.StatusConflict
			}
			if response.StatusCode != wantStatus {
				t.Fatalf("status=%d want=%d", response.StatusCode, wantStatus)
			}
			var body fiber.Map
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(fmt.Sprint(body["error"]), "private") {
				t.Fatalf("internal error leaked: %v", body)
			}
			var current models.Project
			if err := db.First(&current, project.ID).Error; err != nil {
				t.Fatal(err)
			}
			if current.LastCommitHash != runningHash || current.ContainerID == nil || *current.ContainerID != containerID {
				t.Fatalf("running image metadata changed: hash=%s container=%v", current.LastCommitHash, current.ContainerID)
			}
			if failure == "busy" || failure == "instant busy" {
				job, err := redisService.DequeueDeployment(time.Second)
				if err != nil || job == nil || job.TargetCommitHash != "" || job.PreviousCommitHash != runningHash {
					t.Fatalf("active job altered: job=%v err=%v", job, err)
				}
				_ = redisService.AcknowledgeDeployment(job)
			} else if queued, err := redisService.GetQueueLength(); err != nil || queued != 0 {
				t.Fatalf("rejected rollback reached queue: len=%d err=%v", queued, err)
			}
		})
	}
}

func TestRollbackQueueCarriesTargetWithoutReplacingRunningImage(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	const runningHash = "1111111111111111111111111111111111111111"
	const targetHash = "2222222222222222222222222222222222222222"
	for _, instant := range []bool{false, true} {
		name := "fallback"
		if instant {
			name = "instant"
		}
		t.Run(name, func(t *testing.T) {
			client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
			t.Cleanup(func() { _ = client.Close() })
			handler, project, user, db, _, redisService := setupRequeueTest(t, client)
			handler.cfg.ProjectsPath = t.TempDir()
			buildLogPath := filepath.Join(project.GetProjectPath(handler.cfg.ProjectsPath), "build.log")
			if err := os.MkdirAll(filepath.Dir(buildLogPath), 0755); err != nil {
				t.Fatal(err)
			}
			if err := client.FlushDB(context.Background()).Err(); err != nil {
				t.Fatal(err)
			}
			published := false
			client.AddHook(adminRequeuePublishHook{onPublish: func(string) {
				published = true
				if err := os.WriteFile(buildLogPath, []byte("worker pickup wrote log\n"), 0644); err != nil {
					t.Fatal(err)
				}
			}})
			if err := db.Model(&project).Update("last_commit_hash", runningHash).Error; err != nil {
				t.Fatal(err)
			}
			if instant {
				if err := exec.Command("docker", "image", "inspect", "redis:alpine").Run(); err != nil {
					t.Skip("Docker test image unavailable")
				}
				tag := "paas-" + project.Subdomain + ":" + targetHash
				if err := exec.Command("docker", "tag", "redis:alpine", tag).Run(); err != nil {
					t.Skip("Docker image tagging unavailable")
				}
				t.Cleanup(func() { _ = exec.Command("docker", "rmi", tag).Run() })
			}
			app := fiber.New()
			app.Post("/rollback/:id", func(c *fiber.Ctx) error {
				c.Locals("user_id", user.ID)
				c.Locals("role", string(user.Role))
				return handler.Rollback(c)
			})
			request := httptest.NewRequest("POST", "/rollback/"+project.UID, bytes.NewBufferString(`{"commit_sha":"`+targetHash+`"}`))
			request.Header.Set("Content-Type", "application/json")
			response, err := app.Test(request, 5000)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != fiber.StatusOK {
				t.Fatalf("rollback status=%d", response.StatusCode)
			}
			if !published {
				t.Fatal("rollback was not published")
			}
			if content, err := os.ReadFile(buildLogPath); err != nil || string(content) != "worker pickup wrote log\n" {
				t.Fatalf("rollback erased log after pickup: %q, %v", content, err)
			}
			job, err := redisService.DequeueDeployment(time.Second)
			if err != nil || job == nil || job.TargetCommitHash != targetHash || job.PreviousCommitHash != runningHash {
				t.Fatalf("rollback job lost commit snapshot: job=%v err=%v", job, err)
			}
			if instant && job.Type != "rollback" || !instant && job.Type != "redeploy" {
				t.Fatalf("wrong rollback job type: %s", job.Type)
			}
			_ = redisService.AcknowledgeDeployment(job)
			var current models.Project
			if err := db.First(&current, project.ID).Error; err != nil {
				t.Fatal(err)
			}
			if current.LastCommitHash != runningHash {
				t.Fatalf("rollback admission changed running commit: %s", current.LastCommitHash)
			}
		})
	}
}

func TestWebhookAdminOverridePreservesRunningCommit(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	t.Cleanup(func() { _ = client.Close() })
	handler, project, _, db, manager, redisService := setupRequeueTest(t, client)
	if err := client.FlushDB(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	const runningHash = "1111111111111111111111111111111111111111"
	if err := db.Model(&project).Updates(map[string]interface{}{"last_commit_hash": runningHash, "github_installation_id": int64(0), "github_repo_owner": "owner", "github_repo_name": "repo", "branch": "main"}).Error; err != nil {
		t.Fatal(err)
	}
	inProgress := false
	client.AddHook(adminRequeueFaultHook{
		inProgress: &inProgress,
		afterReserve: func() {
			publishAdminOverride(t, redisService, manager, project.ID, project.UserID, "admin-webhook-winner")
		},
	})
	handler.cfg.GithubAppWebhookSecret = "test-webhook-secret"
	webhook := githubhandlers.NewGithubAppHandler(db, handler.cfg, infrastructure.NewGithubService(handler.cfg, redisService), redisService, handler.projectService)
	app := fiber.New()
	app.Post("/webhook", webhook.Webhook)
	payload := []byte(`{"ref":"refs/heads/main","repository":{"name":"repo","owner":{"login":"owner"}},"head_commit":{"id":"2222222222222222222222222222222222222222"},"installation":{"id":0}}`)
	signature := hmac.New(sha256.New, []byte(handler.cfg.GithubAppWebhookSecret))
	_, _ = signature.Write(payload)
	request := httptest.NewRequest("POST", "/webhook", bytes.NewReader(payload))
	request.Header.Set("X-GitHub-Event", "push")
	request.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(signature.Sum(nil)))
	response, err := app.Test(request, 5000)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != fiber.StatusOK {
		t.Fatalf("webhook status=%d", response.StatusCode)
	}
	var current models.Project
	if err := db.First(&current, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.LastCommitHash != runningHash || current.DeploymentJobID == nil || *current.DeploymentJobID != "admin-webhook-winner" {
		t.Fatalf("webhook changed active image/job: hash=%s job=%v", current.LastCommitHash, current.DeploymentJobID)
	}
	job, err := redisService.DequeueDeployment(time.Second)
	if err != nil || job == nil || job.JobID != "admin-webhook-winner" {
		t.Fatalf("admin job lost: job=%v err=%v", job, err)
	}
	_ = redisService.AcknowledgeDeployment(job)
}

func TestGuardedDeploymentImmediateWorkerPickup(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	t.Cleanup(func() { _ = client.Close() })

	handler, project, _, db, _, redisService := setupRequeueTest(t, client)
	ctx := context.Background()

	jobID, err := handler.projectService.EnqueueGuardedDeployment(ctx, project.ID, project.UserID, "redeploy", "Redeploying")
	if err != nil {
		t.Fatalf("guarded deployment failed: %v", err)
	}

	// Worker dequeues immediately
	job, err := redisService.DequeueDeployment(time.Second)
	if err != nil || job == nil {
		t.Fatalf("expected to dequeue job: %v", err)
	}
	t.Cleanup(func() { _ = redisService.AcknowledgeDeployment(job) })

	if job.JobID != jobID {
		t.Fatalf("expected dequeued job ID %s, got %s", jobID, job.JobID)
	}

	// Worker transitions to preparing
	updated, transErr := handler.projectService.TransitionDeploymentState(ctx, project.ID, job.JobID, models.DepStatusPreparing, 10, "preparing", "Starting...")
	if transErr != nil {
		t.Fatalf("worker transition to preparing must succeed: %v", transErr)
	}
	if updated.DeploymentStatus != models.DepStatusPreparing {
		t.Fatalf("expected preparing, got %s", updated.DeploymentStatus)
	}

	// Verify DB state
	var current models.Project
	if err := db.First(&current, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.DeploymentStatus != models.DepStatusPreparing {
		t.Fatalf("expected DB status preparing, got %s", current.DeploymentStatus)
	}
	if current.DeploymentJobID == nil || *current.DeploymentJobID != jobID {
		t.Fatalf("expected DB job ID %s, got %v", jobID, current.DeploymentJobID)
	}
}

func TestUpdateEnvSetsPendingMarkerWhenDeploymentBusy(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	t.Cleanup(func() { _ = client.Close() })

	handler, project, user, _, _, redisService := setupRequeueTest(t, client)

	// Simulate active deployment holding the admission/deployment lock
	lockToken, err := redisService.ReserveDeployment(project.ID)
	if err != nil || lockToken == "" {
		t.Fatalf("failed to reserve deployment lock: %v", err)
	}
	t.Cleanup(func() { _ = redisService.ReleaseDeploymentLock(project.ID, lockToken) })

	app := fiber.New()
	app.Put("/projects/:id/env", func(c *fiber.Ctx) error {
		c.Locals("user_id", user.ID)
		c.Locals("role", string(user.Role))
		return handler.UpdateEnv(c)
	})

	envPayload := UpdateEnvRequest{
		Content: "APP_ENV=production\nAPP_DEBUG=false\nNEW_KEY=secret_val",
	}
	body, _ := json.Marshal(envPayload)
	req := httptest.NewRequest("PUT", "/projects/"+project.UID+"/env", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	// Verify pending env refresh marker was set in Redis
	hasPending, err := redisService.HasPendingEnvRefresh(project.ID)
	if err != nil || !hasPending {
		t.Fatalf("expected HasPendingEnvRefresh to be true when deployment is busy, got hasPending=%v err=%v", hasPending, err)
	}

	// Simulate worker clearing marker when done
	cleared, err := redisService.ClearPendingEnvRefresh(project.ID)
	if err != nil || !cleared {
		t.Fatalf("failed to clear pending env refresh: cleared=%v err=%v", cleared, err)
	}
	hasPendingAfter, _ := redisService.HasPendingEnvRefresh(project.ID)
	if hasPendingAfter {
		t.Fatalf("expected pending marker to be cleared")
	}
}

func TestCreateProjectFencesAgainstConcurrentAdminRequeue(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	t.Cleanup(func() { _ = client.Close() })

	_, project, _, _, _, redisService := setupRequeueTest(t, client)

	// Admin reserves the deployment lock first
	adminToken, err := redisService.ReserveDeployment(project.ID)
	if err != nil || adminToken == "" {
		t.Fatalf("admin failed to reserve deployment: %v", err)
	}
	t.Cleanup(func() { _ = redisService.ReleaseDeploymentLock(project.ID, adminToken) })

	adminJob := &infrastructure.DeploymentJob{
		ProjectID:  project.ID,
		UserID:     project.UserID,
		Type:       "admin_requeue",
		JobID:      "admin-job-123",
		EnqueuedAt: time.Now(),
	}
	if err := redisService.EnqueueReplacingDeploymentJob(adminJob, adminToken); err != nil {
		t.Fatalf("admin publish failed: %v", err)
	}
	t.Cleanup(func() { _ = redisService.RemoveDeploymentJob("admin-job-123") })

	// Initial create with a stale token or no token trying to overwrite admin job
	staleCreateJob := &infrastructure.DeploymentJob{
		ProjectID:  project.ID,
		UserID:     project.UserID,
		Type:       "deploy",
		JobID:      "stale-create-job-456",
		EnqueuedAt: time.Now(),
	}
	createToken := "stale-or-wrong-token"
	err = redisService.EnqueueReplacingDeploymentJob(staleCreateJob, createToken)
	if err == nil {
		t.Fatal("expected EnqueueReplacingDeploymentJob to fail with stale owner error")
	}
	if !infrastructure.IsStaleOwnerError(err) {
		t.Fatalf("expected IsStaleOwnerError to be true, got %v", err)
	}

	// Verify admin job is STILL in Redis and was NOT replaced
	hasAdminJob, _ := redisService.HasDeploymentJob("admin-job-123")
	if !hasAdminJob {
		t.Fatal("admin job was stomped in Redis!")
	}
	hasCreateJob, _ := redisService.HasDeploymentJob("stale-create-job-456")
	if hasCreateJob {
		t.Fatal("stale create job was erroneously enqueued into Redis!")
	}
}

func TestRuntimeActionInterleaving_DoesNotOverwriteNewerOrTerminalStatus(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	t.Cleanup(func() { _ = client.Close() })

	handler, project, _, db, manager, _ := setupRequeueTest(t, client)
	_ = client.FlushDB(context.Background()).Err()

	// Set project initially to Running
	project.Status = models.StatusRunning
	_ = db.Model(&project).Update("status", models.StatusRunning)

	inProgress := false
	hook := adminRequeueFaultHook{
		inProgress: &inProgress,
		onPublish: func(jobID string) {
			// Simulate that worker quickly finishes the stop job and marks it completed with StatusStopped,
			// OR simulates a new concurrent admin requeue B that marks the project Running with job B!
			_, _ = manager.RequeueDeployment(context.Background(), project.ID, "job-B-requeue", "Concurrent admin requeue B", models.StatusRunning)
		},
	}
	client.AddHook(hook)

	// Handler A executes StopProject
	err := handler.projectService.StopProject(&project)
	if err != nil {
		t.Fatalf("StopProject failed: %v", err)
	}

	// Verify that project status in DB is STILL StatusRunning (from job B), and NOT overwritten with StatusStopped!
	var fresh models.Project
	if err := db.First(&fresh, project.ID).Error; err != nil {
		t.Fatalf("failed to reload project: %v", err)
	}
	if fresh.Status != models.StatusRunning {
		t.Fatalf("expected project status to remain %s from concurrent job B, but got %s", models.StatusRunning, fresh.Status)
	}
	if fresh.DeploymentJobID == nil || *fresh.DeploymentJobID != "job-B-requeue" {
		t.Fatalf("expected deployment job ID to be job-B-requeue, got %v", fresh.DeploymentJobID)
	}
}

func TestUpdateEnv_RedisReserveOrMarkerFailure_DoesNotPromiseApplication(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}

	t.Run("Reserve error returns 503 and saves secret without promising application", func(t *testing.T) {
		client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
		t.Cleanup(func() { _ = client.Close() })

		handler, project, user, db, _, _ := setupRequeueTest(t, client)
		_ = client.FlushDB(context.Background()).Err()

		inProgress := false
		hook := adminRequeueFaultHook{
			inProgress: &inProgress,
			injectOnCmd: func(cmd redis.Cmder) bool {
				return (cmd.Name() == "evalsha" || cmd.Name() == "eval") && strings.Contains(fmt.Sprint(cmd.Args()), "deployment:processing_queue")
			},
			injectErr:  fmt.Errorf("simulated redis network down"),
			skipServer: true,
		}
		client.AddHook(hook)

		app := fiber.New()
		app.Put("/projects/:id/env", func(c *fiber.Ctx) error {
			c.Locals("user_id", user.ID)
			c.Locals("role", "admin")
			return handler.UpdateEnv(c)
		})

		body := bytes.NewBufferString(`{"content":"FOO=bar\nNEW_KEY=val123"}`)
		req := httptest.NewRequest("PUT", "/projects/"+project.UID+"/env", body)
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req, 5000)
		if err != nil {
			t.Fatalf("UpdateEnv request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusServiceUnavailable {
			t.Fatalf("expected HTTP 503 Service Unavailable, got %d", resp.StatusCode)
		}

		var respData map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&respData)
		msg, _ := respData["message"].(string)
		if strings.Contains(strings.ToLower(msg), "propagating") || strings.Contains(strings.ToLower(msg), "will take effect") {
			t.Fatalf("response promised application when reserve failed: %q", msg)
		}
		if !strings.Contains(strings.ToLower(msg), "restart") && !strings.Contains(strings.ToLower(msg), "retry") {
			t.Fatalf("response must provide manual restart or retry instruction: %q", msg)
		}

		// Verify secret was saved to DB
		var binding models.SecretStoreBinding
		if err := db.Where("project_id = ?", project.ID).First(&binding).Error; err != nil {
			t.Fatalf("secret store binding should exist: %v", err)
		}
		var item models.SecretStoreItem
		if err := db.Where("secret_store_id = ? AND key = ?", binding.SecretStoreID, "NEW_KEY").First(&item).Error; err != nil {
			t.Fatalf("new secret item must be persisted in database despite Redis error: %v", err)
		}
	})

	t.Run("Marker set error when busy returns 503 and saves secret without promising application", func(t *testing.T) {
		client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
		t.Cleanup(func() { _ = client.Close() })

		handler, project, user, db, _, redisService := setupRequeueTest(t, client)
		_ = client.FlushDB(context.Background()).Err()

		token, err := redisService.ReserveDeployment(project.ID)
		if err != nil || token == "" {
			t.Fatalf("failed to reserve deployment: %v", err)
		}
		t.Cleanup(func() { _ = redisService.ReleaseDeploymentLock(project.ID, token) })

		inProgress := false
		hook := adminRequeueFaultHook{
			inProgress: &inProgress,
			injectOnCmd: func(cmd redis.Cmder) bool {
				return (cmd.Name() == "evalsha" || cmd.Name() == "eval") && strings.Contains(fmt.Sprint(cmd.Args()), "project:pending_env_refresh")
			},
			injectErr:  fmt.Errorf("simulated redis error on pending marker set"),
			skipServer: true,
		}
		client.AddHook(hook)

		app := fiber.New()
		app.Put("/projects/:id/env", func(c *fiber.Ctx) error {
			c.Locals("user_id", user.ID)
			c.Locals("role", "admin")
			return handler.UpdateEnv(c)
		})

		body := bytes.NewBufferString(`{"content":"FOO=bar\nANOTHER_KEY=val456"}`)
		req := httptest.NewRequest("PUT", "/projects/"+project.UID+"/env", body)
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req, 5000)
		if err != nil {
			t.Fatalf("UpdateEnv request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusServiceUnavailable {
			t.Fatalf("expected HTTP 503 Service Unavailable, got %d", resp.StatusCode)
		}

		var respData map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&respData)
		msg, _ := respData["message"].(string)
		if strings.Contains(strings.ToLower(msg), "will take effect") {
			t.Fatalf("response promised application when marker failed: %q", msg)
		}
		if !strings.Contains(strings.ToLower(msg), "restart") {
			t.Fatalf("response must provide manual restart instruction: %q", msg)
		}

		// Verify secret was saved to DB
		var binding models.SecretStoreBinding
		if err := db.Where("project_id = ?", project.ID).First(&binding).Error; err != nil {
			t.Fatalf("secret store binding should exist: %v", err)
		}
		var item models.SecretStoreItem
		if err := db.Where("secret_store_id = ? AND key = ?", binding.SecretStoreID, "ANOTHER_KEY").First(&item).Error; err != nil {
			t.Fatalf("new secret item must be persisted in database despite Redis marker error: %v", err)
		}
	})
}

func TestCreateProject_FailsClosedOnReserveOrDBError_AndHandlesAmbiguousPublish(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}

	t.Run("Reserve error fails closed without publishing", func(t *testing.T) {
		client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
		t.Cleanup(func() { _ = client.Close() })

		handler, _, user, db, _, redisService := setupRequeueTest(t, client)
		_ = client.FlushDB(context.Background()).Err()

		inProgress := false
		hook := adminRequeueFaultHook{
			inProgress: &inProgress,
			injectOnCmd: func(cmd redis.Cmder) bool {
				return (cmd.Name() == "evalsha" || cmd.Name() == "eval") && strings.Contains(fmt.Sprint(cmd.Args()), "deployment:processing_queue")
			},
			injectErr:  fmt.Errorf("simulated redis reserve failure"),
			skipServer: true,
		}
		client.AddHook(hook)

		app := fiber.New()
		app.Post("/projects", func(c *fiber.Ctx) error {
			c.Locals("user_id", user.ID)
			c.Locals("role", "admin")
			return handler.Create(c)
		})

		payload := `{"name":"fail-reserve-proj","github_url":"https://github.com/test/repo","branch":"main","database_option":"none"}`
		req := httptest.NewRequest("POST", "/projects", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req, 5000)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		// Verify that NO jobs are in Redis queue
		qLen, _ := redisService.GetQueueLength()
		if qLen != 0 {
			t.Fatalf("expected queue length 0, got %d", qLen)
		}

		// Verify project is saved in DB and deployment status is failed
		var created models.Project
		if err := db.Where("name = ?", "fail-reserve-proj").First(&created).Error; err != nil {
			t.Fatalf("project should be saved in DB: %v", err)
		}
		if created.DeploymentStatus != models.DepStatusFailed {
			t.Fatalf("expected deployment status failed, got %s", created.DeploymentStatus)
		}
	})

	t.Run("Ambiguous publish with confirmed job in queue returns queued status", func(t *testing.T) {
		client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
		t.Cleanup(func() { _ = client.Close() })

		handler, _, user, db, _, redisService := setupRequeueTest(t, client)
		_ = client.FlushDB(context.Background()).Err()

		inProgress := false
		hook := adminRequeueFaultHook{
			inProgress: &inProgress,
			injectErr:  fmt.Errorf("network reset after write"),
		}
		client.AddHook(hook)

		app := fiber.New()
		app.Post("/projects", func(c *fiber.Ctx) error {
			c.Locals("user_id", user.ID)
			c.Locals("role", "admin")
			return handler.Create(c)
		})

		payload := `{"name":"ambiguous-proj","github_url":"https://github.com/test/repo","branch":"main","database_option":"none"}`
		req := httptest.NewRequest("POST", "/projects", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req, 5000)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != fiber.StatusCreated {
			t.Fatalf("expected 201 Created, got %d", resp.StatusCode)
		}

		var respData map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&respData)
		if warn, ok := respData["warning"]; ok && warn != nil {
			t.Fatalf("expected no warning for confirmed published job, got warning: %v", warn)
		}

		var created models.Project
		if err := db.Where("name = ?", "ambiguous-proj").First(&created).Error; err != nil {
			t.Fatalf("project should be saved in DB: %v", err)
		}
		if created.DeploymentStatus != models.DepStatusQueued {
			t.Fatalf("expected deployment status Queued, got %s", created.DeploymentStatus)
		}
		if created.DeploymentJobID == nil || *created.DeploymentJobID == "" {
			t.Fatalf("expected non-empty deployment job ID")
		}

		inQueue, err := redisService.HasDeploymentJob(*created.DeploymentJobID)
		if err != nil || !inQueue {
			t.Fatalf("expected job to be in Redis queue: inQueue=%v err=%v", inQueue, err)
		}
	})
}

func TestCreateProjectPublishErrorAfterWorkerAckReturnsCompleted(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	t.Cleanup(func() { _ = client.Close() })
	handler, _, user, db, manager, redisService := setupRequeueTest(t, client)
	if err := client.FlushDB(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	inProgress := false
	client.AddHook(adminRequeueFaultHook{
		inProgress: &inProgress,
		injectErr:  fmt.Errorf("response lost after publish"),
		onPublish: func(jobID string) {
			var created models.Project
			if err := db.Where("name = ?", "ack-before-reconcile").First(&created).Error; err != nil {
				t.Fatal(err)
			}
			completeAndAcknowledgePublishedJob(t, redisService, manager, created.ID, jobID)
		},
	})
	app := fiber.New()
	app.Post("/projects", func(c *fiber.Ctx) error {
		c.Locals("user_id", user.ID)
		c.Locals("role", "admin")
		return handler.Create(c)
	})
	request := httptest.NewRequest("POST", "/projects", bytes.NewBufferString(`{"name":"ack-before-reconcile","github_url":"https://github.com/test/repo","branch":"main","database_option":"none"}`))
	request.Header.Set("Content-Type", "application/json")
	response, err := app.Test(request, 5000)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != fiber.StatusCreated {
		t.Fatalf("expected 201, got %d", response.StatusCode)
	}
	var result struct {
		Project models.Project `json:"project"`
		Warning string         `json:"warning"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Project.DeploymentStatus != models.DepStatusCompleted || result.Project.DeploymentJobID == nil || result.Warning != "" {
		t.Fatalf("completed job reported incorrectly: status=%s job=%v warning=%q", result.Project.DeploymentStatus, result.Project.DeploymentJobID, result.Warning)
	}
	if queued, err := redisService.HasDeploymentJob(*result.Project.DeploymentJobID); err != nil || queued {
		t.Fatalf("ACKed job still queued: queued=%v err=%v", queued, err)
	}
}

func TestRuntimeActionPublishErrorAfterWorkerAckSucceeds(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	for _, action := range []struct {
		name    string
		initial models.ProjectStatus
		result  models.ProjectStatus
		run     func(*projectservice.ProjectService, *models.Project) error
	}{
		{"stop", models.StatusRunning, models.StatusStopped, (*projectservice.ProjectService).StopProject},
		{"start", models.StatusStopped, models.StatusRunning, (*projectservice.ProjectService).StartProject},
		{"restart", models.StatusRunning, models.StatusRunning, (*projectservice.ProjectService).RestartProject},
	} {
		t.Run(action.name, func(t *testing.T) {
			client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
			t.Cleanup(func() { _ = client.Close() })
			handler, project, _, db, manager, redisService := setupRequeueTest(t, client)
			if err := client.FlushDB(context.Background()).Err(); err != nil {
				t.Fatal(err)
			}
			if err := db.Model(&project).Update("status", action.initial).Error; err != nil {
				t.Fatal(err)
			}
			project.Status = action.initial
			inProgress := false
			client.AddHook(adminRequeueFaultHook{
				inProgress: &inProgress,
				injectErr:  fmt.Errorf("response lost after publish"),
				onPublish: func(jobID string) {
					completeAndAcknowledgePublishedJob(t, redisService, manager, project.ID, jobID)
					if err := db.Model(&project).Update("status", action.result).Error; err != nil {
						t.Fatal(err)
					}
				},
			})
			if err := action.run(handler.projectService, &project); err != nil {
				t.Fatalf("completed action returned publish failure: %v", err)
			}
			var current models.Project
			if err := db.First(&current, project.ID).Error; err != nil {
				t.Fatal(err)
			}
			if current.Status != action.result || current.DeploymentStatus != models.DepStatusCompleted || current.DeploymentJobID == nil {
				t.Fatalf("completed action changed: status=%s deployment=%s job=%v", current.Status, current.DeploymentStatus, current.DeploymentJobID)
			}
			if queued, err := redisService.HasDeploymentJob(*current.DeploymentJobID); err != nil || queued {
				t.Fatalf("ACKed job still queued: queued=%v err=%v", queued, err)
			}
		})
	}
}

func TestRuntimeActionPublishFailureRestoresPriorStatus(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	actions := []struct {
		name    string
		initial models.ProjectStatus
		run     func(*projectservice.ProjectService, *models.Project) error
	}{
		{"stop", models.StatusRunning, (*projectservice.ProjectService).StopProject},
		{"start", models.StatusStopped, (*projectservice.ProjectService).StartProject},
		{"restart", models.StatusRunning, (*projectservice.ProjectService).RestartProject},
	}
	for _, action := range actions {
		for _, failure := range []string{"publish rejected", "stale token"} {
			t.Run(action.name+"/"+failure, func(t *testing.T) {
				client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
				t.Cleanup(func() { _ = client.Close() })
				handler, project, _, db, _, redisService := setupRequeueTest(t, client)
				if err := db.Model(&project).Update("status", action.initial).Error; err != nil {
					t.Fatal(err)
				}
				project.Status = action.initial
				inProgress := false
				hook := adminRequeueFaultHook{inProgress: &inProgress}
				if failure == "publish rejected" {
					hook.skipServer = true
					hook.injectErr = fmt.Errorf("simulated publish failure")
				} else {
					hook.beforePublish = func(string) {
						if err := redisService.ForceReleaseDeploymentLock(project.ID, "test expired token"); err != nil {
							t.Fatal(err)
						}
					}
				}
				client.AddHook(hook)
				if err := action.run(handler.projectService, &project); err == nil {
					t.Fatal("expected enqueue failure")
				}
				var current models.Project
				if err := db.First(&current, project.ID).Error; err != nil {
					t.Fatal(err)
				}
				if current.Status != action.initial || current.DeploymentStatus != models.DepStatusFailed {
					t.Fatalf("runtime status not restored: status=%s want=%s deployment=%s", current.Status, action.initial, current.DeploymentStatus)
				}
				if current.DeploymentJobID == nil {
					t.Fatal("missing failed job identity")
				}
				if queued, err := redisService.HasDeploymentJob(*current.DeploymentJobID); err != nil || queued {
					t.Fatalf("failed job remained queued: queued=%v err=%v", queued, err)
				}
			})
		}
	}
}

func TestRuntimeActionFailureDoesNotRestoreOverSuccessor(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	t.Cleanup(func() { _ = client.Close() })
	handler, project, _, db, manager, _ := setupRequeueTest(t, client)
	if err := db.Model(&project).Update("status", models.StatusRunning).Error; err != nil {
		t.Fatal(err)
	}
	inProgress := false
	client.AddHook(adminRequeueFaultHook{
		inProgress: &inProgress,
		beforePublish: func(string) {
			if _, err := manager.RequeueDeployment(context.Background(), project.ID, "successor-job", "new owner", models.StatusStarting); err != nil {
				t.Fatal(err)
			}
		},
		skipServer: true,
		injectErr:  fmt.Errorf("old job publish rejected"),
	})
	if err := handler.projectService.StopProject(&project); err == nil {
		t.Fatal("expected old job publish error")
	}
	var current models.Project
	if err := db.First(&current, project.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.Status != models.StatusStarting || current.DeploymentStatus != models.DepStatusQueued || current.DeploymentJobID == nil || *current.DeploymentJobID != "successor-job" {
		t.Fatalf("old job overwrote successor: status=%s deployment=%s job=%v", current.Status, current.DeploymentStatus, current.DeploymentJobID)
	}
}

package workers

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/laravel-paas/shared/infrastructure"
	"github.com/redis/go-redis/v9"
)

func TestProjectResourceLockSerializesCloneBuildAndStart(t *testing.T) {
	for _, heldAt := range []string{"clone", "build", "before_start"} {
		t.Run(heldAt, func(t *testing.T) {
			projectsPath := t.TempDir()
			lockA, err := acquireProjectResourceLock(context.Background(), projectsPath, 42)
			if err != nil {
				t.Fatal(err)
			}
			defer lockA.Close()
			source := filepath.Join(projectsPath, "checkout")
			image := filepath.Join(projectsPath, "latest")
			if err := os.WriteFile(source, []byte("A"), 0600); err != nil {
				t.Fatal(err)
			}
			if heldAt != "clone" {
				if err := os.WriteFile(image, []byte("A"), 0600); err != nil {
					t.Fatal(err)
				}
			}

			started := make(chan struct{})
			acquired := make(chan *os.File, 1)
			errCh := make(chan error, 1)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go func() {
				close(started)
				lockB, lockErr := acquireProjectResourceLock(ctx, projectsPath, 42)
				if lockErr != nil {
					errCh <- lockErr
					return
				}
				acquired <- lockB
			}()
			<-started
			select {
			case lockB := <-acquired:
				lockB.Close()
				t.Fatal("B changed shared source/image before A drained")
			case err := <-errCh:
				t.Fatal(err)
			case <-time.After(250 * time.Millisecond):
			}
			if heldAt == "clone" {
				if err := os.WriteFile(image, []byte("A"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := lockA.Close(); err != nil {
				t.Fatal(err)
			}
			var lockB *os.File
			select {
			case lockB = <-acquired:
			case err := <-errCh:
				t.Fatal(err)
			case <-time.After(2 * time.Second):
				t.Fatal("B did not resume after A drained")
			}
			defer lockB.Close()
			if err := os.WriteFile(source, []byte("B"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(image, []byte("B"), 0600); err != nil {
				t.Fatal(err)
			}
			built, err := os.ReadFile(image)
			if err != nil || string(built) != "B" {
				t.Fatalf("B started wrong image: %q, %v", built, err)
			}
		})
	}
}

func TestProjectResourceLockWaitIsCancelable(t *testing.T) {
	projectsPath := t.TempDir()
	lockA, err := acquireProjectResourceLock(context.Background(), projectsPath, 42)
	if err != nil {
		t.Fatal(err)
	}
	defer lockA.Close()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		lockB, err := acquireProjectResourceLock(ctx, projectsPath, 42)
		if lockB != nil {
			lockB.Close()
		}
		result <- err
	}()
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("resource waiter ignored cancellation")
	}
}

func TestAdminOverrideWaitsForProjectResources(t *testing.T) {
	addr := os.Getenv("REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set REDIS_TEST_ADDR to an isolated Redis instance")
	}
	client := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	t.Cleanup(func() { _ = client.Close() })
	if err := client.FlushDB(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	service := infrastructure.NewRedisServiceWithClient(client)
	const projectID = uint(42)
	lockA, err := service.AcquireDeploymentLock(projectID, "A", 2*time.Minute)
	if err != nil || lockA == "" {
		t.Fatalf("claim A: %q %v", lockA, err)
	}
	projectsPath := t.TempDir()
	physicalA, err := acquireProjectResourceLock(context.Background(), projectsPath, projectID)
	if err != nil {
		t.Fatal(err)
	}
	defer physicalA.Close()

	override, err := service.ReserveAdminRequeue(projectID)
	if err != nil {
		t.Fatal(err)
	}
	jobB := &infrastructure.DeploymentJob{ProjectID: projectID, UserID: 1, Type: "redeploy", JobID: "B", EnqueuedAt: time.Now()}
	if err := service.EnqueueReplacingDeploymentJob(jobB, override); err != nil {
		t.Fatal(err)
	}
	claimed, err := service.DequeueDeployment(time.Second)
	if err != nil || claimed == nil || claimed.JobID != "B" {
		t.Fatalf("claim B: %v %v", claimed, err)
	}
	defer service.AcknowledgeDeployment(claimed)
	lockB, err := service.AcquireDeploymentLock(projectID, "B", 2*time.Minute)
	if err != nil || lockB == "" {
		t.Fatalf("override did not let B claim Redis: %q %v", lockB, err)
	}
	defer service.ReleaseDeploymentLock(projectID, lockB)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	acquired := make(chan *os.File, 1)
	errCh := make(chan error, 1)
	go func() {
		physicalB, lockErr := acquireProjectResourceLock(ctx, projectsPath, projectID)
		if lockErr != nil {
			errCh <- lockErr
			return
		}
		acquired <- physicalB
	}()
	select {
	case physicalB := <-acquired:
		physicalB.Close()
		t.Fatal("B used shared Git/image resources while A still building")
	case err := <-errCh:
		t.Fatal(err)
	case <-time.After(250 * time.Millisecond):
	}
	if err := physicalA.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case physicalB := <-acquired:
		defer physicalB.Close()
		owner, err := service.GetLockMetadata(projectID)
		if err != nil || owner == nil || owner.Token != lockB {
			t.Fatalf("B lost ownership after A drained: %v %v", owner, err)
		}
	case err := <-errCh:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal("B did not resume after A drained")
	}
}

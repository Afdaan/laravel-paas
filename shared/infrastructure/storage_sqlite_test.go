package infrastructure

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
)

func TestEnsurePersistentPath_SQLiteCreatesFile(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.Config{
		DataPath:         tmpDir,
		HostDataPath:     tmpDir,
		ProjectsPath:     filepath.Join(tmpDir, "projects"),
		HostProjectsPath: filepath.Join(tmpDir, "projects"),
	}

	// nil DB causes GetUserDirName to return "user-1"
	svc := NewStorageService(cfg, nil)

	project := &models.Project{
		UserID:         1,
		Subdomain:      "test-app",
		DatabaseOption: "sqlite",
	}

	svc.EnsurePersistentPath(project)

	// Verify sqlite directory exists
	sqliteDir := filepath.Join(tmpDir, "user-1", "test-app", "storage", "sqlite")
	info, err := os.Stat(sqliteDir)
	if err != nil {
		t.Fatalf("sqlite dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("sqlite path is not a directory")
	}

	// Verify database.sqlite file exists
	sqliteFile := filepath.Join(sqliteDir, "database.sqlite")
	fInfo, err := os.Stat(sqliteFile)
	if err != nil {
		t.Fatalf("database.sqlite not created: %v", err)
	}
	if fInfo.IsDir() {
		t.Fatal("database.sqlite should be a file, not a directory")
	}

	// Verify permissions
	if fInfo.Mode().Perm() != 0664 {
		t.Errorf("expected file perms 0664, got %o", fInfo.Mode().Perm())
	}
}

func TestEnsurePersistentPath_NonSQLiteSkipsFile(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.Config{
		DataPath:         tmpDir,
		HostDataPath:     tmpDir,
		ProjectsPath:     filepath.Join(tmpDir, "projects"),
		HostProjectsPath: filepath.Join(tmpDir, "projects"),
	}

	svc := NewStorageService(cfg, nil)

	project := &models.Project{
		UserID:         1,
		Subdomain:      "mysql-app",
		DatabaseOption: "new",
	}

	svc.EnsurePersistentPath(project)

	sqliteDir := filepath.Join(tmpDir, "user-1", "mysql-app", "storage", "sqlite")
	if _, err := os.Stat(sqliteDir); err == nil {
		t.Fatal("sqlite dir should not exist for non-sqlite project")
	}
}

func TestEnsurePersistentPath_SQLiteIdempotent(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.Config{
		DataPath:         tmpDir,
		HostDataPath:     tmpDir,
		ProjectsPath:     filepath.Join(tmpDir, "projects"),
		HostProjectsPath: filepath.Join(tmpDir, "projects"),
	}

	svc := NewStorageService(cfg, nil)

	project := &models.Project{
		UserID:         1,
		Subdomain:      "idempotent-app",
		DatabaseOption: "sqlite",
	}

	// Call twice - must not error
	svc.EnsurePersistentPath(project)
	svc.EnsurePersistentPath(project)

	sqliteFile := filepath.Join(tmpDir, "user-1", "idempotent-app", "storage", "sqlite", "database.sqlite")
	if _, err := os.Stat(sqliteFile); err != nil {
		t.Fatalf("database.sqlite missing after second call: %v", err)
	}
}

func TestPrepareSQLiteHostFile_Success(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.Config{
		DataPath:         tmpDir,
		HostDataPath:     tmpDir,
		ProjectsPath:     filepath.Join(tmpDir, "projects"),
		HostProjectsPath: filepath.Join(tmpDir, "projects"),
	}

	svc := NewStorageService(cfg, nil)

	project := &models.Project{
		UserID:         1,
		Subdomain:      "sqlite-ok",
		DatabaseOption: "sqlite",
	}

	// Pre-create storage path (normally EnsurePersistentPath does this)
	storagePath := filepath.Join(tmpDir, "user-1", "sqlite-ok", "storage")
	if err := os.MkdirAll(storagePath, 0777); err != nil {
		t.Fatal(err)
	}

	err := svc.PrepareSQLiteHostFile(project)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	sqliteFile := filepath.Join(storagePath, "sqlite", "database.sqlite")
	fInfo, err := os.Stat(sqliteFile)
	if err != nil {
		t.Fatalf("sqlite file not created: %v", err)
	}
	if fInfo.IsDir() {
		t.Fatal("sqlite file should not be a directory")
	}
	if fInfo.Mode().Perm() != 0664 {
		t.Errorf("expected 0664, got %o", fInfo.Mode().Perm())
	}
}

func TestPrepareSQLiteHostFile_NonSQLiteNoop(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.Config{
		DataPath:         tmpDir,
		HostDataPath:     tmpDir,
		ProjectsPath:     filepath.Join(tmpDir, "projects"),
		HostProjectsPath: filepath.Join(tmpDir, "projects"),
	}

	svc := NewStorageService(cfg, nil)

	project := &models.Project{
		UserID:         1,
		Subdomain:      "mysql-app",
		DatabaseOption: "new",
	}

	err := svc.PrepareSQLiteHostFile(project)
	if err != nil {
		t.Fatalf("non-sqlite project should return nil, got: %v", err)
	}
}

func TestPrepareSQLiteHostFile_FailsOnUnwritableParent(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.Config{
		DataPath:         tmpDir,
		HostDataPath:     tmpDir,
		ProjectsPath:     filepath.Join(tmpDir, "projects"),
		HostProjectsPath: filepath.Join(tmpDir, "projects"),
	}

	svc := NewStorageService(cfg, nil)

	project := &models.Project{
		UserID:         1,
		Subdomain:      "broken-app",
		DatabaseOption: "sqlite",
	}

	// Create storage path as a read-only directory to block sqlite dir creation
	storagePath := filepath.Join(tmpDir, "user-1", "broken-app", "storage")
	if err := os.MkdirAll(storagePath, 0777); err != nil {
		t.Fatal(err)
	}
	// Make it unwritable
	if err := os.Chmod(storagePath, 0555); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(storagePath, 0755) }() // cleanup

	err := svc.PrepareSQLiteHostFile(project)
	if err == nil {
		t.Fatal("expected error when parent is unwritable, got nil")
	}
}

func TestPrepareSQLiteHostFile_FailsWhenPathIsFile(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.Config{
		DataPath:         tmpDir,
		HostDataPath:     tmpDir,
		ProjectsPath:     filepath.Join(tmpDir, "projects"),
		HostProjectsPath: filepath.Join(tmpDir, "projects"),
	}

	svc := NewStorageService(cfg, nil)

	project := &models.Project{
		UserID:         1,
		Subdomain:      "conflict-app",
		DatabaseOption: "sqlite",
	}

	// Create a file at the path where the sqlite directory should be
	storagePath := filepath.Join(tmpDir, "user-1", "conflict-app", "storage")
	if err := os.MkdirAll(storagePath, 0777); err != nil {
		t.Fatal(err)
	}
	// Create a file named "sqlite" which conflicts with the directory
	conflictPath := filepath.Join(storagePath, "sqlite")
	if err := os.WriteFile(conflictPath, []byte("conflict"), 0644); err != nil {
		t.Fatal(err)
	}

	err := svc.PrepareSQLiteHostFile(project)
	if err == nil {
		t.Fatal("expected error when sqlite path conflicts with existing file, got nil")
	}
}

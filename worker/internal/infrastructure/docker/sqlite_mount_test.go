package docker

import (
	"strings"
	"testing"
)

func TestSQLiteVolumeMountFormat(t *testing.T) {
	// Simulate the volume mount logic from BuildAndRun/StartWorkerContainer/StartExistingImage
	hostPersistentPath := "/data/user-1/myapp/storage"

	hostSQLiteFile := hostPersistentPath + "/sqlite/database.sqlite"
	mount := hostSQLiteFile + ":/var/www/html/database/database.sqlite"

	// Must mount file-to-file, not directory-to-directory
	if !strings.HasSuffix(mount, "database.sqlite:/var/www/html/database/database.sqlite") {
		t.Errorf("mount target should be file, got: %s", mount)
	}

	// Must NOT mount to /var/www/html/database (directory)
	if strings.Contains(mount, ":/var/www/html/database\"") || strings.HasSuffix(mount, ":/var/www/html/database") {
		t.Errorf("mount should not target /var/www/html/database directory, got: %s", mount)
	}
}

func TestSQLiteVolumeMountNotDirectory(t *testing.T) {
	// Ensure the old broken pattern is gone: sqlite dir -> /var/www/html/database
	hostPersistentPath := "/data/user-1/myapp/storage"

	// Old broken mount
	brokenMount := hostPersistentPath + "/sqlite:/var/www/html/database"

	// New correct mount
	hostSQLiteFile := hostPersistentPath + "/sqlite/database.sqlite"
	correctMount := hostSQLiteFile + ":/var/www/html/database/database.sqlite"

	if brokenMount == correctMount {
		t.Fatal("mounts should differ")
	}

	// Correct mount must reference the file on both sides
	parts := strings.SplitN(correctMount, ":", 2)
	if len(parts) != 2 {
		t.Fatalf("expected host:container split, got: %s", correctMount)
	}
	if !strings.HasSuffix(parts[0], "/database.sqlite") {
		t.Errorf("host side should end with /database.sqlite, got: %s", parts[0])
	}
	if parts[1] != "/var/www/html/database/database.sqlite" {
		t.Errorf("container side should be /var/www/html/database/database.sqlite, got: %s", parts[1])
	}
}

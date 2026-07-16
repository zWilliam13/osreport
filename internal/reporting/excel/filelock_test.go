package excel

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAcquireFileLock_SecondCallFailsWhileFirstHeld(t *testing.T) {
	path := filepath.Join(t.TempDir(), "informe.xlsx")

	release, err := AcquireFileLock(path)
	if err != nil {
		t.Fatalf("first AcquireFileLock() error = %v", err)
	}

	if _, err := AcquireFileLock(path); err == nil {
		t.Fatal("second AcquireFileLock() while the first is held: expected an error, got nil")
	}

	release()

	release2, err := AcquireFileLock(path)
	if err != nil {
		t.Fatalf("AcquireFileLock() after release: unexpected error = %v", err)
	}
	release2()
}

func TestAcquireFileLock_ReleaseRemovesLockFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "informe.xlsx")
	lockPath := path + ".lock"

	release, err := AcquireFileLock(path)
	if err != nil {
		t.Fatalf("AcquireFileLock() error = %v", err)
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock file not created: %v", err)
	}

	release()

	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Errorf("lock file still exists after release: %v", err)
	}
}

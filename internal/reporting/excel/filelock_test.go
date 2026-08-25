package excel

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

func TestAcquireFileLock_RecoversFromStaleLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "informe.xlsx")
	lockPath := path + ".lock"

	// Simulate a lock left behind by a killed process: create it, then
	// backdate its mtime past staleLockAge — no live holder to release it.
	if err := os.WriteFile(lockPath, nil, 0644); err != nil {
		t.Fatalf("seed stale lock file: %v", err)
	}
	old := time.Now().Add(-staleLockAge - time.Minute)
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatalf("backdate lock file: %v", err)
	}

	release, err := AcquireFileLock(path)
	if err != nil {
		t.Fatalf("AcquireFileLock() with a stale lock present: expected auto-recovery, got error = %v", err)
	}
	release()
}

func TestAcquireFileLock_DoesNotStealFreshLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "informe.xlsx")
	lockPath := path + ".lock"

	if err := os.WriteFile(lockPath, nil, 0644); err != nil {
		t.Fatalf("seed fresh lock file: %v", err)
	}
	// Not backdated — this is what a live, in-progress run's lock looks like.

	if _, err := AcquireFileLock(path); err == nil {
		t.Fatal("AcquireFileLock() with a fresh lock present: expected an error, got nil (stole a live lock)")
	}
}

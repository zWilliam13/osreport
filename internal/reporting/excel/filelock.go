package excel

import (
	"errors"
	"fmt"
	"os"
)

// AcquireFileLock creates path+".lock" exclusively (O_CREATE|O_EXCL) as a
// simple cross-process advisory lock. An in-process mutex only protects
// against overlap within a single osreport.exe run — it does nothing if
// a scheduled batch run, a manual double-click of run-report.bat, and
// `osreport serve`'s own background refresh all happen to target the
// same --output path at once. Two such processes both calling excelize
// SaveAs on the same file at the same time can corrupt it, exactly like
// the in-process race this guards against elsewhere.
//
// If the lock file already exists, this fails fast with a clear message
// instead of letting a second process silently race the first one's
// write. Release removes the lock file; a lock left behind by a crashed
// process must be deleted by hand before the next run — this trades a
// (rare, visible) stuck-lock failure mode for never silently corrupting
// a report.
func AcquireFileLock(path string) (release func(), err error) {
	lockPath := path + ".lock"
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("%s is locked by another osreport run (found %s) — wait for it to finish, or delete the lock file if a previous run crashed without cleaning up", path, lockPath)
		}
		return nil, fmt.Errorf("create lock file %s: %w", lockPath, err)
	}
	_ = f.Close()
	return func() { _ = os.Remove(lockPath) }, nil
}

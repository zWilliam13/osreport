package excel

import (
	"errors"
	"fmt"
	"os"
	"time"
)

// staleLockAge is how old a lock file must be before AcquireFileLock treats
// it as abandoned (its creator crashed/was killed without running its
// deferred release) rather than a live run still in progress. A
// well-behaved holder never keeps the lock past its own --timeout (5
// minutes by default) plus the small surrounding I/O — the deferred release
// fires on a clean finish, an error, or a recovered panic. The only way a
// lock outlives that is a hard kill that skips every Go defer, which is
// exactly the real scenario this guards against (confirmed once already: a
// shell timeout that SIGKILLs an in-flight run leaves the lock file
// behind). 15 minutes gives several times headroom over any real run seen
// in practice, so a crash recovers on its own well within the same working
// session instead of silently blocking every refresh until someone notices
// and deletes the file by hand.
const staleLockAge = 15 * time.Minute

// AcquireFileLock creates path+".lock" exclusively (O_CREATE|O_EXCL) as a
// simple cross-process advisory lock. An in-process mutex only protects
// against overlap within a single osreport.exe run — it does nothing if
// a scheduled batch run, a manual double-click of run-report.bat, and
// `osreport serve`'s own background refresh all happen to target the
// same --output path at once. Two such processes both calling excelize
// SaveAs on the same file at the same time can corrupt it, exactly like
// the in-process race this guards against elsewhere.
//
// If the lock file already exists but is older than staleLockAge, it's
// removed and creation retried once — see staleLockAge for why that age is
// safe to treat as abandoned rather than a live run. A lock still within
// that age fails fast with a clear message instead of letting a second
// process silently race the first one's write.
func AcquireFileLock(path string) (release func(), err error) {
	lockPath := path + ".lock"
	f, err := createLockFile(lockPath)
	if err != nil && errors.Is(err, os.ErrExist) && removeIfStale(lockPath) {
		f, err = createLockFile(lockPath)
	}
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("%s is locked by another osreport run (found %s) — wait for it to finish, or delete the lock file if a previous run crashed without cleaning up", path, lockPath)
		}
		return nil, fmt.Errorf("create lock file %s: %w", lockPath, err)
	}
	_ = f.Close()
	return func() { _ = os.Remove(lockPath) }, nil
}

func createLockFile(lockPath string) (*os.File, error) {
	return os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
}

// removeIfStale deletes lockPath and reports true if it was older than
// staleLockAge. Any error stating it (e.g. another process already cleaned
// it up) is treated as "not stale" — the safe default is to fall through to
// the normal locked-file error rather than guess.
func removeIfStale(lockPath string) bool {
	info, err := os.Stat(lockPath)
	if err != nil || time.Since(info.ModTime()) < staleLockAge {
		return false
	}
	return os.Remove(lockPath) == nil
}

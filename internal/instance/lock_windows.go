package instance

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// tryLock takes an exclusive lock on the file without waiting. The system drops it when the
// process ends, however that happens, so a crash leaves nothing to clean up.
func tryLock(file *os.File) (bool, error) {
	var whole windows.Overlapped
	err := windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &whole)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_IO_PENDING) {
		return false, nil
	}
	return err == nil, err
}

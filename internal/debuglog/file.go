package debuglog

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

const (
	// FileName is the opt-in log file, under the directory given to OpenRotating.
	FileName = "airpods-tray.log"
	// maxFileBytes and keptFiles bound the log file at about 4 MB on disk.
	maxFileBytes = 1 << 20
	keptFiles    = 3
)

// Rotating is a log file that moves aside when it grows too big, keeping a few older ones.
type Rotating struct {
	mu   sync.Mutex
	path string
	file *os.File
	size int64
}

// OpenRotating opens the log file in dir, private to the user.
func OpenRotating(dir string) (*Rotating, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	r := &Rotating{path: filepath.Join(dir, FileName)}
	if err := r.open(); err != nil {
		return nil, err
	}
	// The lines carry only the time of day; this says which day they start on.
	if _, err := fmt.Fprintf(r, "--- started %s\n", time.Now().Format(time.RFC3339)); err != nil {
		r.Close()
		return nil, err
	}
	return r, nil
}

func (r *Rotating) open() error {
	file, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return err
	}
	r.file, r.size = file, info.Size()
	return nil
}

func (r *Rotating) older(n int) string { return r.path + "." + strconv.Itoa(n) }

// rotate moves the current file aside. If that fails the log keeps going in the same file.
func (r *Rotating) rotate() error {
	if err := r.file.Close(); err != nil {
		return err
	}
	os.Remove(r.older(keptFiles))
	for n := keptFiles - 1; n >= 1; n-- {
		os.Rename(r.older(n), r.older(n+1))
	}
	os.Rename(r.path, r.older(1))
	return r.open()
}

// Write implements io.Writer. A line that would pass the limit starts the next file.
func (r *Rotating) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return 0, os.ErrClosed
	}
	if r.size > 0 && r.size+int64(len(p)) > maxFileBytes {
		if err := r.rotate(); err != nil {
			r.file = nil
			return 0, err
		}
	}
	n, err := r.file.Write(p)
	r.size += int64(n)
	return n, err
}

// Close closes the file; later writes fail.
func (r *Rotating) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return nil
	}
	err := r.file.Close()
	r.file = nil
	return err
}

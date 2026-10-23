package store

import (
	"fmt"
	"os"
	"path/filepath"
)

// AcquireRuntimeLock reserves a canonical local path for one server lifetime.
// Never remove the lock file: another process may already have it open.
func AcquireRuntimeLock(path string) (*os.File, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return nil, err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return nil, err
	}
	abs = filepath.Join(parent, filepath.Base(abs))
	file, err := os.OpenFile(abs, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := lockRuntimeFile(file); err != nil {
		file.Close()
		return nil, fmt.Errorf("runtime ownership unavailable for %s; another server may be using this data: %w", abs, err)
	}
	return file, nil
}

func DatabaseRuntimeLockPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	canonical, err := filepath.EvalSymlinks(abs)
	if err == nil {
		abs = canonical
	} else if !os.IsNotExist(err) {
		return "", err
	}
	return abs + ".server.lock", nil
}

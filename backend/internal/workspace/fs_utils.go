package workspace

import "os"

// Workspace and trash moves must be atomic. A failed rename (permission,
// cross-device, etc.) must never fall back to a partial recursive copy/delete.
func movePathWithFallback(src, dst string) error {
	if _, err := os.Lstat(dst); err == nil {
		return ErrPathExists
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Rename(src, dst)
}

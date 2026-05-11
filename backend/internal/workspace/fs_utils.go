package workspace

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

func movePathWithFallback(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	} else {
		if copyErr := copyPath(src, dst); copyErr != nil {
			return fmt.Errorf("%w; fallback copy failed: %v", err, copyErr)
		}
		if removeErr := os.RemoveAll(src); removeErr != nil {
			_ = os.RemoveAll(dst)
			return fmt.Errorf("%w; fallback remove failed: %v", err, removeErr)
		}
		return nil
	}
}

func copyPath(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return copyFile(src, dst, info.Mode())
	}
	if err := os.MkdirAll(dst, info.Mode()); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcChild := filepath.Join(src, entry.Name())
		dstChild := filepath.Join(dst, entry.Name())
		if err := copyPath(srcChild, dstChild); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string, mode fs.FileMode) error {
	raw, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(dst, raw, mode); err != nil {
		return err
	}
	return nil
}

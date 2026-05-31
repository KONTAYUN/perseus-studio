package skills

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func unzipSafe(raw []byte, dest string) error {
	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return fmt.Errorf("invalid zip: %w", err)
	}
	if err := validateZipEntryPaths(reader.File, dest); err != nil {
		return err
	}
	total := int64(0)
	for _, file := range reader.File {
		name, isDir, err := safeZipEntryName(file)
		if err != nil {
			return err
		}
		target := filepath.Clean(filepath.Join(dest, filepath.FromSlash(name)))
		if !isWithin(dest, target) {
			return fmt.Errorf("zip 包含路径穿越风险")
		}
		if isDir {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("zip 包含冲突路径：%s", filepath.ToSlash(name))
			}
			continue
		}
		total += int64(file.UncompressedSize64)
		if total > maxExtractedBytes {
			return fmt.Errorf("zip 解压后内容过大")
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("zip 包含冲突路径：%s", filepath.ToSlash(name))
		}
		src, err := file.Open()
		if err != nil {
			return err
		}
		dst, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			_ = src.Close()
			if os.IsExist(err) {
				return fmt.Errorf("zip 包含重复路径：%s", filepath.ToSlash(name))
			}
			return err
		}
		_, copyErr := io.Copy(dst, io.LimitReader(src, maxExtractedBytes+1))
		closeErr := dst.Close()
		_ = src.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func validateZipEntryPaths(files []*zip.File, dest string) error {
	entryIsDir := map[string]bool{}
	filePaths := map[string]bool{}
	dirPaths := map[string]bool{}

	for _, file := range files {
		name, isDir, err := safeZipEntryName(file)
		if err != nil {
			return err
		}
		target := filepath.Clean(filepath.Join(dest, filepath.FromSlash(name)))
		if !isWithin(dest, target) {
			return fmt.Errorf("zip 包含路径穿越风险")
		}
		if previousIsDir, exists := entryIsDir[name]; exists {
			if previousIsDir != isDir {
				return fmt.Errorf("zip 包含冲突路径：%s", filepath.ToSlash(name))
			}
			return fmt.Errorf("zip 包含重复路径：%s", filepath.ToSlash(name))
		}
		entryIsDir[name] = isDir

		if isDir {
			if filePaths[name] {
				return fmt.Errorf("zip 包含冲突路径：%s", filepath.ToSlash(name))
			}
			dirPaths[name] = true
			continue
		}
		if dirPaths[name] {
			return fmt.Errorf("zip 包含冲突路径：%s", filepath.ToSlash(name))
		}
		for parent := path.Dir(name); parent != "." && parent != "/"; parent = path.Dir(parent) {
			if filePaths[parent] {
				return fmt.Errorf("zip 包含冲突路径：%s", filepath.ToSlash(parent))
			}
			dirPaths[parent] = true
		}
		filePaths[name] = true
	}
	return nil
}

func safeZipEntryName(file *zip.File) (string, bool, error) {
	name := strings.ReplaceAll(file.Name, "\\", "/")
	if strings.HasPrefix(name, "/") || filepath.IsAbs(name) || hasUnsafePathElement(name) {
		return "", false, fmt.Errorf("zip 包含不安全路径")
	}
	cleanName := path.Clean(name)
	if cleanName == "." {
		return "", false, fmt.Errorf("zip 包含不安全路径")
	}
	return cleanName, file.FileInfo().IsDir() || strings.HasSuffix(name, "/"), nil
}

func hasUnsafePathElement(name string) bool {
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

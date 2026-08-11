package maintenance

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func DefaultBackupPath(databasePath string, now time.Time) string {
	return filepath.Join(filepath.Dir(filepath.Clean(databasePath)), "backups", fmt.Sprintf("perseus-backup-%s.db", now.UTC().Format("20060102T150405Z")))
}

func DefaultArchivePath(databasePath string, now time.Time) string {
	return filepath.Join(filepath.Dir(filepath.Clean(databasePath)), "backups", fmt.Sprintf("perseus-archive-%s.jsonl.gz", now.UTC().Format("20060102T150405Z")))
}

func ValidateMaintenanceOutputPath(databasePath, outputPath string) (string, error) {
	if strings.TrimSpace(outputPath) == "" {
		return "", fmt.Errorf("maintenance output path is required")
	}
	databasePath, err := filepath.Abs(filepath.Clean(databasePath))
	if err != nil {
		return "", err
	}
	outputPath, err = filepath.Abs(filepath.Clean(strings.TrimSpace(outputPath)))
	if err != nil {
		return "", err
	}
	for _, reserved := range []string{databasePath, databasePath + "-wal", databasePath + "-shm", databasePath + "-journal"} {
		if samePath(outputPath, reserved) {
			return "", fmt.Errorf("maintenance output must not replace the live database or its journal files")
		}
	}
	allowedRoots := []string{filepath.Dir(databasePath)}
	for _, value := range filepath.SplitList(os.Getenv("PERSEUS_MAINTENANCE_OUTPUT_ROOTS")) {
		if root := strings.TrimSpace(value); root != "" {
			absolute, absErr := filepath.Abs(filepath.Clean(root))
			if absErr != nil {
				return "", absErr
			}
			allowedRoots = append(allowedRoots, absolute)
		}
	}
	for _, root := range allowedRoots {
		relative, relErr := filepath.Rel(root, outputPath)
		if relErr == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return outputPath, nil
		}
	}
	return "", fmt.Errorf("maintenance output %s is outside persistent data roots", outputPath)
}

func samePath(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if strings.EqualFold(left, right) {
		return true
	}
	leftInfo, leftErr := os.Stat(left)
	rightInfo, rightErr := os.Stat(right)
	return leftErr == nil && rightErr == nil && os.SameFile(leftInfo, rightInfo)
}

package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type commandSnapshotEntry struct {
	Type    string
	Size    int64
	ModTime time.Time
	Hash    string
}

type commandWorkspaceSnapshot map[string]commandSnapshotEntry

func (s *Service) commandWorkspaceSnapshot() (commandWorkspaceSnapshot, error) {
	snapshot := commandWorkspaceSnapshot{}
	err := filepath.WalkDir(s.rootAbs, func(absPath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if absPath == s.rootAbs {
			return nil
		}
		rel, err := filepath.Rel(s.rootAbs, absPath)
		if err != nil {
			return err
		}
		relPath := filepath.ToSlash(rel)
		if shouldSkipSearchPath(relPath, entry.IsDir()) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		item := commandSnapshotEntry{
			Type:    "file",
			Size:    info.Size(),
			ModTime: info.ModTime().UTC(),
		}
		if entry.IsDir() {
			item.Type = "folder"
			item.Size = 0
		} else {
			item.Hash = hashFileForCommandSnapshot(absPath)
		}
		snapshot[relPath] = item
		return nil
	})
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

func hashFileForCommandSnapshot(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func diffCommandWorkspaceSnapshots(before, after commandWorkspaceSnapshot) []string {
	if len(before) == 0 && len(after) == 0 {
		return nil
	}
	changed := map[string]bool{}
	for path, afterItem := range after {
		beforeItem, existed := before[path]
		if !existed {
			changed[path] = true
			continue
		}
		if afterItem.Type != beforeItem.Type {
			changed[path] = true
			continue
		}
		if afterItem.Type == "folder" {
			continue
		}
		if afterItem.Size != beforeItem.Size || afterItem.Hash != beforeItem.Hash || !afterItem.ModTime.Equal(beforeItem.ModTime) {
			changed[path] = true
		}
	}
	for path := range before {
		if _, exists := after[path]; !exists {
			changed[path] = true
		}
	}
	return sortedCommandChangedPaths(changed)
}

func sortedCommandChangedPaths(changed map[string]bool) []string {
	if len(changed) == 0 {
		return nil
	}
	paths := make([]string, 0, len(changed))
	for path := range changed {
		path = strings.TrimSpace(path)
		if path != "" {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths
}

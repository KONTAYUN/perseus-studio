package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
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
	return s.commandWorkspaceSnapshotContext(context.Background())
}

func (s *Service) commandWorkspaceSnapshotContext(ctx context.Context) (commandWorkspaceSnapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	const maxFiles = 10000
	const maxBytes int64 = 256 << 20
	var totalBytes int64
	snapshot := commandWorkspaceSnapshot{}
	err := filepath.WalkDir(s.rootAbs, func(absPath string, entry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if len(snapshot) >= maxFiles {
			return fmt.Errorf("command observation exceeds file budget (%d)", maxFiles)
		}
		if walkErr != nil {
			return walkErr
		}
		if absPath == s.rootAbs {
			return nil
		}
		if entry.Type()&(os.ModeSymlink|os.ModeIrregular) != 0 || (!entry.IsDir() && !entry.Type().IsRegular()) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
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
			totalBytes += info.Size()
			if totalBytes > maxBytes {
				return fmt.Errorf("command observation exceeds byte budget (%d)", maxBytes)
			}
			item.Hash, err = hashFileForCommandSnapshot(ctx, absPath, maxBytes-totalBytes+info.Size())
			if err != nil {
				return err
			}
		}
		snapshot[relPath] = item
		return nil
	})
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

func hashFileForCommandSnapshot(ctx context.Context, path string, limit int64) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	reader := &commandSnapshotReader{ctx: ctx, reader: io.LimitReader(file, limit+1)}
	n, err := io.Copy(hash, reader)
	if err != nil {
		return "", err
	}
	if n > limit {
		return "", fmt.Errorf("command observation file grew beyond byte budget")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
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

type commandSnapshotReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *commandSnapshotReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

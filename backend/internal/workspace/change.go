package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
)

// FileChange describes final filesystem state, independently of the tool or
// provider that produced it. Hash is the expected content of Path after commit.
type FileChange struct {
	Operation  string `json:"operation"`
	Path       string `json:"path"`
	OldPath    string `json:"old_path,omitempty"`
	TrashPath  string `json:"trash_path,omitempty"`
	BeforeHash string `json:"before_hash,omitempty"`
	Hash       string `json:"hash,omitempty"`
	VersionID  string `json:"version_id,omitempty"`
}

func (s *Service) VerifyChanges(changes []FileChange) error {
	if len(changes) == 0 {
		return fmt.Errorf("change set is empty")
	}
	seen := map[string]bool{}
	for _, change := range changes {
		if change.Path == "" || seen[change.Path] {
			return fmt.Errorf("missing or duplicate change path %q", change.Path)
		}
		seen[change.Path] = true
		switch change.Operation {
		case "create", "update", "move":
			if change.Hash == "" {
				return fmt.Errorf("missing expected hash for %s", change.Path)
			}
			read, err := s.ReadFile(change.Path)
			if err != nil {
				return fmt.Errorf("verify %s: %w", change.Path, err)
			}
			if read.Hash != change.Hash {
				return fmt.Errorf("file hash mismatch after write: %s", change.Path)
			}
			if change.Operation == "move" {
				if change.OldPath == "" {
					return fmt.Errorf("move is missing old path")
				}
				if err := s.verifyMissingPath(change.OldPath); err != nil {
					return err
				}
			}
		case "delete":
			if err := s.verifyMissingPath(change.Path); err != nil {
				return err
			}
			if change.TrashPath == "" || change.BeforeHash == "" {
				return fmt.Errorf("delete is missing recovery metadata: %s", change.Path)
			}
			clean, abs, err := s.sanitizePath(change.TrashPath)
			if err != nil || !isAnyTrashPath(clean) {
				return fmt.Errorf("invalid trash recovery path: %s", change.Path)
			}
			raw, err := os.ReadFile(abs)
			if err != nil || hashBytes(raw) != change.BeforeHash {
				return fmt.Errorf("deleted file is not intact in trash: %s", change.Path)
			}
		default:
			return fmt.Errorf("unsupported change operation %q", change.Operation)
		}
	}
	return nil
}

// Roll back only an unchanged file created by this operation. Never delete a
// user's newer content, and never use a recursive delete for a created file.
func (s *Service) RemoveCreatedFileIfUnchanged(ctx context.Context, path, expectedHash string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	clean, abs, err := s.sanitizePath(path)
	if err != nil {
		return err
	}
	if isAnyTrashPath(clean) {
		return trashManagedOnlyError()
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		return err
	}
	if hashBytes(raw) != expectedHash {
		return ErrFileConflict
	}
	return os.Remove(abs)
}

func (s *Service) verifyMissingPath(path string) error {
	_, abs, err := s.sanitizePath(path)
	if err != nil {
		return err
	}
	_, err = os.Lstat(abs)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("path still exists after change: %s", path)
}

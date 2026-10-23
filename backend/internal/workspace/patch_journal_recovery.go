package workspace

import (
	"context"
	"errors"
	"os"
)

func (s *Service) recoverPatchEntry(ctx context.Context, e PatchJournalEntry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	current, err := s.ReadFile(e.Path)
	absent := errors.Is(err, os.ErrNotExist)
	if err != nil && !absent {
		return err
	}
	switch e.Operation {
	case "create":
		if absent {
			return nil
		}
		if current.Hash != e.AfterHash {
			return ErrFileConflict
		}
		return s.RemoveCreatedFileIfUnchanged(ctx, e.Path, e.AfterHash)
	case "update":
		if absent {
			return ErrFileConflict
		}
		if current.Hash == e.BeforeHash {
			return nil
		}
		if current.Hash != e.AfterHash {
			return ErrFileConflict
		}
		_, err = s.WriteFileIfUnchanged(ctx, e.Path, e.Before, "patch_crash_recovery", &e.AfterHash)
		return err
	case "move":
		destination, destErr := s.ReadFile(e.NewPath)
		destAbsent := errors.Is(destErr, os.ErrNotExist)
		if destErr != nil && !destAbsent {
			return destErr
		}
		if destAbsent && !absent {
			return nil
		} // Move not performed, or already undone.
		if !absent || destAbsent || destination.Hash != e.AfterHash {
			return ErrFileConflict
		}
		_, err = s.RenameIfUnchanged(ctx, e.NewPath, e.Path, e.AfterHash)
		return err
	case "delete":
		if !absent {
			if current.Hash == e.BeforeHash {
				return nil
			}
			return ErrFileConflict
		}
		_, err = s.CreateTextFile(ctx, e.Path, e.Before, "patch_crash_recovery")
		return err
	}
	return ErrInvalidPath
}

package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (s *Service) Mkdir(path string) (map[string]string, error) {
	cleanPath, absPath, err := s.sanitizePath(path)
	if err != nil {
		return nil, err
	}
	if isAnyTrashPath(cleanPath) {
		return nil, trashManagedOnlyError()
	}
	if err := os.MkdirAll(absPath, 0o755); err != nil {
		return nil, err
	}
	return map[string]string{"path": cleanPath}, nil
}

func (s *Service) MkdirNew(path string) (PathChangeResult, error) {
	cleanPath, absPath, err := s.sanitizePath(path)
	if err != nil {
		return PathChangeResult{}, err
	}
	if isAnyTrashPath(cleanPath) {
		return PathChangeResult{}, trashManagedOnlyError()
	}
	if _, err := os.Stat(absPath); err == nil {
		return PathChangeResult{}, fmt.Errorf("path already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return PathChangeResult{}, err
	}
	if err := os.MkdirAll(absPath, 0o755); err != nil {
		return PathChangeResult{}, err
	}
	return PathChangeResult{
		Path:         cleanPath,
		ChangedPaths: []string{cleanPath},
	}, nil
}

func (s *Service) RenameWithNewName(oldPath, newName string) (PathChangeResult, error) {
	cleanOld, _, err := s.sanitizePath(oldPath)
	if err != nil {
		return PathChangeResult{}, err
	}
	if isAnyTrashPath(cleanOld) {
		return PathChangeResult{}, trashManagedOnlyError()
	}
	cleanName, err := sanitizeLeafName(newName)
	if err != nil {
		return PathChangeResult{}, err
	}

	parent := getParentPath(cleanOld)
	cleanNew := cleanName
	if parent != "" {
		cleanNew = parent + "/" + cleanName
	}
	if isAnyTrashPath(cleanNew) {
		return PathChangeResult{}, trashManagedOnlyError()
	}
	if cleanNew == cleanOld {
		return PathChangeResult{}, fmt.Errorf("new name must be different")
	}
	return s.renameValidated(cleanOld, cleanNew)
}

func (s *Service) MovePath(oldPath, newParentPath string) (PathChangeResult, error) {
	cleanOld, absOld, err := s.sanitizePath(oldPath)
	if err != nil {
		return PathChangeResult{}, err
	}
	if isAnyTrashPath(cleanOld) {
		return PathChangeResult{}, trashManagedOnlyError()
	}

	cleanParent, absParent, err := s.sanitizeOptionalPath(newParentPath)
	if err != nil {
		return PathChangeResult{}, err
	}
	if isAnyTrashPath(cleanParent) {
		return PathChangeResult{}, trashManagedOnlyError()
	}
	parentInfo, err := os.Stat(absParent)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return PathChangeResult{}, fmt.Errorf("target parent folder does not exist")
		}
		return PathChangeResult{}, err
	}
	if !parentInfo.IsDir() {
		return PathChangeResult{}, fmt.Errorf("target parent path is not a folder")
	}
	sourceInfo, err := os.Stat(absOld)
	if err != nil {
		return PathChangeResult{}, err
	}
	if sourceInfo.IsDir() && cleanParent != "" && (cleanParent == cleanOld || strings.HasPrefix(cleanParent, cleanOld+"/")) {
		return PathChangeResult{}, ErrMoveIntoSelf
	}

	baseName := filepath.Base(cleanOld)
	cleanNew := baseName
	if cleanParent != "" {
		cleanNew = cleanParent + "/" + baseName
	}
	if cleanNew == cleanOld {
		return PathChangeResult{}, fmt.Errorf("source and destination are the same")
	}
	return s.renameValidated(cleanOld, cleanNew)
}

func (s *Service) CopyFile(ctx context.Context, sourcePath, destinationPath string) (PathChangeResult, error) {
	cleanSource, absSource, err := s.sanitizePath(sourcePath)
	if err != nil {
		return PathChangeResult{}, err
	}
	if isAnyTrashPath(cleanSource) {
		return PathChangeResult{}, trashManagedOnlyError()
	}
	sourceInfo, err := os.Stat(absSource)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return PathChangeResult{}, fmt.Errorf("source file does not exist")
		}
		return PathChangeResult{}, err
	}
	if sourceInfo.IsDir() {
		return PathChangeResult{}, fmt.Errorf("copy only supports files")
	}

	cleanDestination, absDestination, err := s.sanitizePath(destinationPath)
	if err != nil {
		return PathChangeResult{}, err
	}
	if isAnyTrashPath(cleanDestination) {
		return PathChangeResult{}, trashManagedOnlyError()
	}
	if cleanSource == cleanDestination {
		return PathChangeResult{}, fmt.Errorf("source and destination are the same")
	}
	if _, err := os.Stat(absDestination); err == nil {
		return PathChangeResult{}, fmt.Errorf("target path already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return PathChangeResult{}, err
	}

	read, err := s.ReadFile(cleanSource)
	if err != nil {
		return PathChangeResult{}, err
	}
	content := read.Content
	if isHTMLNotePath(cleanSource) && isHTMLNotePath(cleanDestination) {
		content = rewriteMovedHTMLNoteAssetRefs(content, cleanSource, cleanDestination)
	}
	write, err := s.CreateBytes(ctx, cleanDestination, []byte(content))
	if err != nil {
		return PathChangeResult{}, err
	}
	return PathChangeResult{
		Path:         write.Path,
		OldPath:      cleanSource,
		NewPath:      write.Path,
		ChangedPaths: []string{cleanSource, write.Path},
		Hash:         write.Hash,
		Size:         write.Size,
	}, nil
}

func (s *Service) Rename(oldPath, newPath string) (map[string]string, error) {
	cleanOld, _, err := s.sanitizePath(oldPath)
	if err != nil {
		return nil, err
	}
	cleanNew, _, err := s.sanitizePath(newPath)
	if err != nil {
		return nil, err
	}
	if isAnyTrashPath(cleanOld) || isAnyTrashPath(cleanNew) {
		return nil, trashManagedOnlyError()
	}
	result, err := s.renameValidated(cleanOld, cleanNew)
	if err != nil {
		return nil, err
	}
	return map[string]string{"old_path": result.OldPath, "new_path": result.NewPath}, nil
}

func (s *Service) Delete(path string) (map[string]string, error) {
	cleanPath, absPath, err := s.sanitizePath(path)
	if err != nil {
		return nil, err
	}
	if isAnyTrashPath(cleanPath) {
		return nil, trashManagedOnlyError()
	}
	if err := os.RemoveAll(absPath); err != nil {
		return nil, err
	}
	if err := s.removeOrderMetadataForPath(cleanPath); err != nil {
		return nil, err
	}
	return map[string]string{"path": cleanPath}, nil
}

func (s *Service) renameValidated(cleanOld, cleanNew string) (PathChangeResult, error) {
	if isAnyTrashPath(cleanOld) || isAnyTrashPath(cleanNew) {
		return PathChangeResult{}, trashManagedOnlyError()
	}
	_, absOld, err := s.sanitizePath(cleanOld)
	if err != nil {
		return PathChangeResult{}, err
	}
	_, absNew, err := s.sanitizePath(cleanNew)
	if err != nil {
		return PathChangeResult{}, err
	}
	if _, err := os.Stat(absNew); err == nil {
		return PathChangeResult{}, fmt.Errorf("target path already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return PathChangeResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(absNew), 0o755); err != nil {
		return PathChangeResult{}, err
	}
	if err := os.Rename(absOld, absNew); err != nil {
		return PathChangeResult{}, err
	}
	if isHTMLNotePath(cleanOld) && isHTMLNotePath(cleanNew) {
		raw, err := os.ReadFile(absNew)
		if err != nil {
			return PathChangeResult{}, err
		}
		next := rewriteMovedHTMLNoteAssetRefs(string(raw), cleanOld, cleanNew)
		if next != string(raw) {
			if err := writeAtomic(absNew, []byte(next)); err != nil {
				return PathChangeResult{}, err
			}
		}
		if err := s.rewriteMovedHTMLNoteVersions(cleanOld, cleanNew); err != nil {
			return PathChangeResult{}, err
		}
	}
	if err := s.moveOrderMetadata(cleanOld, cleanNew); err != nil {
		return PathChangeResult{}, err
	}
	return PathChangeResult{
		Path:         cleanNew,
		OldPath:      cleanOld,
		NewPath:      cleanNew,
		ChangedPaths: []string{cleanOld, cleanNew},
	}, nil
}

package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"webide-backend/internal/failure"
)

func (s *Service) WriteFile(ctx context.Context, path, content, source string) (FileWriteResult, error) {
	return s.WriteBytes(ctx, path, []byte(content), source)
}

func (s *Service) WriteBytes(ctx context.Context, path string, content []byte, source string) (FileWriteResult, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.writeBytes(ctx, path, content, source)
}

// WriteFileIfUnchanged compares and writes under the same lock as agent writes.
// A nil hash preserves the API's unconditional-write behavior for older clients.
func (s *Service) WriteFileIfUnchanged(ctx context.Context, path, content, source string, expectedHash *string) (FileWriteResult, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if expectedHash != nil {
		if err := s.checkFileHash(path, *expectedHash); err != nil {
			return FileWriteResult{}, err
		}
	}
	return s.writeBytes(ctx, path, []byte(content), source)
}

func (s *Service) writeBytes(ctx context.Context, path string, content []byte, source string) (FileWriteResult, error) {
	if err := ctx.Err(); err != nil {
		return FileWriteResult{}, err
	}
	if int64(len(content)) > maxFileReadBytes {
		return FileWriteResult{}, fmt.Errorf("文件超过写入上限 64 MiB")
	}
	cleanPath, absPath, err := s.sanitizePath(path)
	if err != nil {
		return FileWriteResult{}, err
	}
	if isAnyTrashPath(cleanPath) {
		return FileWriteResult{}, trashManagedOnlyError()
	}

	if source == "" {
		source = "manual_write"
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return FileWriteResult{}, err
	}

	var versionID string
	if info, statErr := os.Stat(absPath); statErr == nil && !info.IsDir() {
		oldContent, err := readFileBounded(absPath, maxFileReadBytes)
		if err != nil {
			return FileWriteResult{}, err
		}
		versionID, err = s.createFileVersion(ctx, cleanPath, oldContent, source)
		if err != nil {
			return FileWriteResult{}, err
		}
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return FileWriteResult{}, statErr
	}

	if err := ctx.Err(); err != nil {
		return FileWriteResult{}, err
	}
	if err := writeAtomic(absPath, content); err != nil {
		return FileWriteResult{}, err
	}

	return FileWriteResult{
		Path:         cleanPath,
		Hash:         hashBytes(content),
		VersionID:    versionID,
		Size:         int64(len(content)),
		ChangedPaths: []string{cleanPath},
	}, nil
}

func (s *Service) CreateFile(ctx context.Context, path, content string) (FileWriteResult, error) {
	return s.CreateBytes(ctx, path, []byte(content))
}

func (s *Service) CreateTextFile(ctx context.Context, path, content, source string) (FileWriteResult, error) {
	if source == "" {
		source = "ai_create"
	}
	return s.createBytes(ctx, path, []byte(content), source)
}

func (s *Service) CreateBytes(ctx context.Context, path string, content []byte) (FileWriteResult, error) {
	return s.createBytes(ctx, path, content, "create")
}

func (s *Service) createBytes(ctx context.Context, path string, content []byte, source string) (FileWriteResult, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	cleanPath, absPath, err := s.sanitizePath(path)
	if err != nil {
		return FileWriteResult{}, err
	}
	if isAnyTrashPath(cleanPath) {
		return FileWriteResult{}, trashManagedOnlyError()
	}
	if _, err := os.Stat(absPath); err == nil {
		return FileWriteResult{}, ErrPathExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return FileWriteResult{}, err
	}
	return s.writeBytes(ctx, cleanPath, content, source)
}

func (s *Service) WriteExistingTextFile(ctx context.Context, path, content, source string) (FileWriteResult, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	cleanPath, absPath, err := s.sanitizePath(path)
	if err != nil {
		return FileWriteResult{}, err
	}
	if isAnyTrashPath(cleanPath) {
		return FileWriteResult{}, trashManagedOnlyError()
	}
	info, err := os.Stat(absPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return FileWriteResult{}, fmt.Errorf("file does not exist: %w", err)
		}
		return FileWriteResult{}, err
	}
	if info.IsDir() {
		return FileWriteResult{}, fmt.Errorf("path is a directory")
	}
	if !isTextPath(cleanPath) {
		return FileWriteResult{}, fmt.Errorf("only text files can be written")
	}
	if source == "" {
		source = "ai_write"
	}
	return s.writeBytes(ctx, cleanPath, []byte(content), source)
}

func (s *Service) AppendTextFile(ctx context.Context, path, content, source string) (FileWriteResult, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	cleanPath, absPath, err := s.sanitizePath(path)
	if err != nil {
		return FileWriteResult{}, err
	}
	if isAnyTrashPath(cleanPath) {
		return FileWriteResult{}, trashManagedOnlyError()
	}
	info, err := os.Stat(absPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return FileWriteResult{}, fmt.Errorf("file does not exist: %w", err)
		}
		return FileWriteResult{}, err
	}
	if info.IsDir() {
		return FileWriteResult{}, fmt.Errorf("path is a directory")
	}
	if !isTextPath(cleanPath) {
		return FileWriteResult{}, fmt.Errorf("only text files can be written")
	}
	raw, err := readFileBounded(absPath, maxTextReadBytes)
	if err != nil {
		return FileWriteResult{}, err
	}
	if source == "" {
		source = "ai_append"
	}
	next := make([]byte, 0, len(raw)+len(content))
	next = append(next, raw...)
	next = append(next, []byte(content)...)
	return s.writeBytes(ctx, cleanPath, next, source)
}

func (s *Service) EditTextFile(ctx context.Context, path, oldText, newText, source string, replaceAll bool) (FileWriteResult, int, error) {
	result, err := s.EditTextFileSmart(ctx, path, oldText, newText, source, replaceAll)
	return result.WriteResult, result.Replacements, err
}

func (s *Service) EditTextFileSmart(ctx context.Context, path, oldText, newText, source string, replaceAll bool) (TextEditResult, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	cleanPath, absPath, err := s.sanitizePath(path)
	if err != nil {
		return TextEditResult{}, err
	}
	if isAnyTrashPath(cleanPath) {
		return TextEditResult{}, trashManagedOnlyError()
	}
	info, err := os.Stat(absPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return TextEditResult{}, fmt.Errorf("file does not exist: %w", err)
		}
		return TextEditResult{}, err
	}
	if info.IsDir() {
		return TextEditResult{}, fmt.Errorf("path is a directory")
	}
	if !isTextPath(cleanPath) {
		return TextEditResult{}, fmt.Errorf("only text files can be edited")
	}
	if oldText == "" {
		return TextEditResult{}, fmt.Errorf("old_text is required")
	}

	raw, err := readFileBounded(absPath, maxTextReadBytes)
	if err != nil {
		return TextEditResult{}, err
	}
	content := string(raw)
	editPlan, err := planTextReplacement(content, oldText, newText, replaceAll)
	if err != nil {
		return TextEditResult{}, err
	}
	if source == "" {
		source = "ai_edit"
	}
	writeResult, err := s.writeBytes(ctx, cleanPath, []byte(editPlan.content), source)
	if err != nil {
		return TextEditResult{}, err
	}
	return TextEditResult{
		WriteResult:  writeResult,
		Replacements: editPlan.replacements,
		MatchKind:    editPlan.matchKind,
	}, nil
}

func (s *Service) ReplaceTextFileLines(ctx context.Context, path string, startLine, endLine int, newText, source string) (LineReplaceResult, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	cleanPath, absPath, err := s.sanitizePath(path)
	if err != nil {
		return LineReplaceResult{}, err
	}
	if isAnyTrashPath(cleanPath) {
		return LineReplaceResult{}, trashManagedOnlyError()
	}
	info, err := os.Stat(absPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return LineReplaceResult{}, fmt.Errorf("file does not exist: %w", err)
		}
		return LineReplaceResult{}, err
	}
	if info.IsDir() {
		return LineReplaceResult{}, fmt.Errorf("path is a directory")
	}
	if !isTextPath(cleanPath) {
		return LineReplaceResult{}, fmt.Errorf("only text files can be edited")
	}
	raw, err := readFileBounded(absPath, maxTextReadBytes)
	if err != nil {
		return LineReplaceResult{}, err
	}
	content := string(raw)
	lines := strings.SplitAfter(content, "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}
	totalBefore := countTextLines(content)
	if startLine <= 0 {
		return LineReplaceResult{}, fmt.Errorf("invalid tool arguments: start_line must be a positive integer")
	}
	if endLine <= 0 {
		return LineReplaceResult{}, fmt.Errorf("invalid tool arguments: end_line must be a positive integer")
	}
	if startLine > endLine {
		return LineReplaceResult{}, fmt.Errorf("invalid tool arguments: start_line must be less than or equal to end_line")
	}
	if startLine > len(lines) {
		return LineReplaceResult{}, failure.New("workspace_line_range_invalid", fmt.Errorf("line range starts after end of file").Error(), nil)
	}
	if endLine > len(lines) {
		return LineReplaceResult{}, failure.New("workspace_line_range_invalid", fmt.Errorf("line range ends after end of file").Error(), nil)
	}
	nextContent := strings.Join(lines[:startLine-1], "") + newText + strings.Join(lines[endLine:], "")
	if source == "" {
		source = "ai_line_replace"
	}
	writeResult, err := s.writeBytes(ctx, cleanPath, []byte(nextContent), source)
	if err != nil {
		return LineReplaceResult{}, err
	}
	return LineReplaceResult{
		WriteResult:      writeResult,
		StartLine:        startLine,
		EndLine:          endLine,
		InsertedLines:    countTextLines(newText),
		TotalLinesBefore: totalBefore,
		TotalLinesAfter:  countTextLines(nextContent),
	}, nil
}

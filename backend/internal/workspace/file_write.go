package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (s *Service) WriteFile(ctx context.Context, path, content, source string) (FileWriteResult, error) {
	return s.WriteBytes(ctx, path, []byte(content), source)
}

func (s *Service) WriteBytes(ctx context.Context, path string, content []byte, source string) (FileWriteResult, error) {
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
	if isHTMLNotePath(cleanPath) {
		content = []byte(SanitizeHTMLFragment(string(content)))
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return FileWriteResult{}, err
	}

	var versionID string
	if info, statErr := os.Stat(absPath); statErr == nil && !info.IsDir() {
		oldContent, err := os.ReadFile(absPath)
		if err != nil {
			return FileWriteResult{}, err
		}
		if isTextPath(cleanPath) && hashBytes(oldContent) != hashBytes(content) {
			versionID, err = s.createFileVersion(ctx, cleanPath, oldContent, source)
			if err != nil {
				return FileWriteResult{}, err
			}
		}
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return FileWriteResult{}, statErr
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
	return s.CreateFile(ctx, path, content)
}

func (s *Service) CreateBytes(ctx context.Context, path string, content []byte) (FileWriteResult, error) {
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
	return s.WriteBytes(ctx, cleanPath, content, "create")
}

func (s *Service) WriteExistingTextFile(ctx context.Context, path, content, source string) (FileWriteResult, error) {
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
			return FileWriteResult{}, ErrPathNotFound
		}
		return FileWriteResult{}, err
	}
	if info.IsDir() {
		return FileWriteResult{}, ErrPathIsDirectory
	}
	if !isTextPath(cleanPath) {
		return FileWriteResult{}, ErrUnsupportedFile
	}
	if source == "" {
		source = "ai_write"
	}
	return s.WriteBytes(ctx, cleanPath, []byte(content), source)
}

func (s *Service) AppendTextFile(ctx context.Context, path, content, source string) (FileWriteResult, error) {
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
			return FileWriteResult{}, ErrPathNotFound
		}
		return FileWriteResult{}, err
	}
	if info.IsDir() {
		return FileWriteResult{}, ErrPathIsDirectory
	}
	if !isTextPath(cleanPath) {
		return FileWriteResult{}, ErrUnsupportedFile
	}
	raw, err := os.ReadFile(absPath)
	if err != nil {
		return FileWriteResult{}, err
	}
	if source == "" {
		source = "ai_append"
	}
	next := make([]byte, 0, len(raw)+len(content))
	next = append(next, raw...)
	next = append(next, []byte(content)...)
	return s.WriteBytes(ctx, cleanPath, next, source)
}

func (s *Service) EditTextFile(ctx context.Context, path, oldText, newText, source string, replaceAll bool) (FileWriteResult, int, error) {
	result, err := s.EditTextFileSmart(ctx, path, oldText, newText, source, replaceAll)
	return result.WriteResult, result.Replacements, err
}

func (s *Service) EditTextFileSmart(ctx context.Context, path, oldText, newText, source string, replaceAll bool) (TextEditResult, error) {
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
			return TextEditResult{}, ErrPathNotFound
		}
		return TextEditResult{}, err
	}
	if info.IsDir() {
		return TextEditResult{}, ErrPathIsDirectory
	}
	if !isTextPath(cleanPath) {
		return TextEditResult{}, ErrUnsupportedFile
	}
	if oldText == "" {
		return TextEditResult{}, fmt.Errorf("old_text is required")
	}
	if looksLikeOmittedText(newText) {
		return TextEditResult{}, fmt.Errorf("new_text appears to contain an omission placeholder; provide the complete replacement text")
	}
	raw, err := os.ReadFile(absPath)
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
	writeResult, err := s.WriteBytes(ctx, cleanPath, []byte(editPlan.content), source)
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
			return LineReplaceResult{}, ErrPathNotFound
		}
		return LineReplaceResult{}, err
	}
	if info.IsDir() {
		return LineReplaceResult{}, ErrPathIsDirectory
	}
	if !isTextPath(cleanPath) {
		return LineReplaceResult{}, ErrUnsupportedFile
	}
	raw, err := os.ReadFile(absPath)
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
		return LineReplaceResult{}, fmt.Errorf("%w: start_line must be a positive integer", ErrInvalidLineRange)
	}
	if endLine <= 0 {
		return LineReplaceResult{}, fmt.Errorf("%w: end_line must be a positive integer", ErrInvalidLineRange)
	}
	if startLine > endLine {
		return LineReplaceResult{}, fmt.Errorf("%w: start_line must be less than or equal to end_line", ErrInvalidLineRange)
	}
	if startLine > len(lines) {
		return LineReplaceResult{}, fmt.Errorf("%w: range starts after end of file", ErrInvalidLineRange)
	}
	if endLine > len(lines) {
		return LineReplaceResult{}, fmt.Errorf("%w: range ends after end of file", ErrInvalidLineRange)
	}
	nextContent := strings.Join(lines[:startLine-1], "") + newText + strings.Join(lines[endLine:], "")
	if source == "" {
		source = "ai_line_replace"
	}
	writeResult, err := s.WriteBytes(ctx, cleanPath, []byte(nextContent), source)
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

package workspace

import (
	"fmt"
	"os"
)

func (s *Service) ReadFile(path string) (FileReadResult, error) {
	cleanPath, absPath, err := s.sanitizePath(path)
	if err != nil {
		return FileReadResult{}, err
	}
	if isAnyTrashPath(cleanPath) {
		return FileReadResult{}, trashManagedOnlyError()
	}
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return FileReadResult{}, fmt.Errorf("file does not exist")
		}
		return FileReadResult{}, err
	}
	if info.IsDir() {
		return FileReadResult{}, fmt.Errorf("path is a directory")
	}
	content, err := os.ReadFile(absPath)
	if err != nil {
		return FileReadResult{}, err
	}
	return FileReadResult{
		Path:       cleanPath,
		Content:    string(content),
		Size:       int64(len(content)),
		Hash:       hashBytes(content),
		ModifiedAt: info.ModTime().UTC(),
	}, nil
}

func (s *Service) ReadTextFile(path string, maxChars int) (TextReadResult, error) {
	return s.ReadTextFileRange(path, 0, 0, maxChars)
}

func (s *Service) ReadTextFileRange(path string, startLine, endLine, maxChars int) (TextReadResult, error) {
	if maxChars <= 0 {
		maxChars = 20000
	}
	cleanPath, absPath, err := s.sanitizePath(path)
	if err != nil {
		return TextReadResult{}, err
	}
	if isAnyTrashPath(cleanPath) {
		return TextReadResult{}, trashManagedOnlyError()
	}
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return TextReadResult{}, fmt.Errorf("file does not exist")
		}
		return TextReadResult{}, err
	}
	if info.IsDir() {
		return TextReadResult{}, fmt.Errorf("path is a directory")
	}
	if !isTextPath(cleanPath) {
		return TextReadResult{}, fmt.Errorf("unsupported file type")
	}
	content, err := os.ReadFile(absPath)
	if err != nil {
		return TextReadResult{}, err
	}
	text := string(content)
	totalLines := countTextLines(text)
	if startLine > 0 || endLine > 0 {
		text, startLine, endLine = sliceTextLines(text, startLine, endLine)
	} else {
		startLine = 1
		endLine = totalLines
	}
	runes := []rune(text)
	truncated := false
	if len(runes) > maxChars {
		text = string(runes[:maxChars])
		truncated = true
	}
	return TextReadResult{Path: cleanPath, Content: text, Truncated: truncated, StartLine: startLine, EndLine: endLine, TotalLines: totalLines}, nil
}

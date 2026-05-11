package workspace

import (
	"fmt"
	"os"
	"path/filepath"
)

func (s *Service) Tree() ([]TreeNode, error) {
	if err := s.migrateLegacyTrashDir(); err != nil {
		return nil, err
	}
	return s.readDir(s.rootAbs, "")
}

func (s *Service) List(path string, maxDepth, maxItems int) ([]TreeNode, bool, error) {
	if maxDepth <= 0 {
		maxDepth = 3
	}
	if maxItems <= 0 {
		maxItems = 200
	}
	if err := s.migrateLegacyTrashDir(); err != nil {
		return nil, false, err
	}
	cleanPath, absPath, err := s.sanitizeOptionalPath(path)
	if err != nil {
		return nil, false, err
	}
	if isAnyTrashPath(cleanPath) {
		return nil, false, trashManagedOnlyError()
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, false, err
	}
	if !info.IsDir() {
		return nil, false, fmt.Errorf("path is not a folder")
	}
	count := 0
	nodes, limited, err := s.readDirLimited(absPath, cleanPath, maxDepth, maxItems, &count)
	return nodes, limited, err
}

func (s *Service) readDir(absDir, relDir string) ([]TreeNode, error) {
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return nil, err
	}
	nodes := make([]TreeNode, 0, len(entries))
	for _, entry := range entries {
		if relDir == "" && (entry.Name() == TrashDirName || entry.Name() == LegacyTrashDirName) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		relPath := entry.Name()
		if relDir != "" {
			relPath = relDir + "/" + entry.Name()
		}
		node := TreeNode{
			Name:       entry.Name(),
			Path:       filepath.ToSlash(relPath),
			ModifiedAt: info.ModTime().UTC(),
		}
		if entry.IsDir() {
			node.Type = "folder"
			children, err := s.readDir(filepath.Join(absDir, entry.Name()), relPath)
			if err != nil {
				return nil, err
			}
			node.Children = children
		} else {
			node.Type = "file"
			node.Size = info.Size()
		}
		nodes = append(nodes, node)
	}
	if err := s.sortNodesForParent(relDir, nodes); err != nil {
		return nil, err
	}
	return nodes, nil
}

func (s *Service) readDirLimited(absDir, relDir string, remainingDepth, maxItems int, count *int) ([]TreeNode, bool, error) {
	if *count >= maxItems {
		return nil, true, nil
	}
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return nil, false, err
	}
	nodes := make([]TreeNode, 0, len(entries))
	limited := false
	for _, entry := range entries {
		if relDir == "" && (entry.Name() == TrashDirName || entry.Name() == LegacyTrashDirName) {
			continue
		}
		if *count >= maxItems {
			limited = true
			break
		}
		info, err := entry.Info()
		if err != nil {
			return nil, false, err
		}
		relPath := entry.Name()
		if relDir != "" {
			relPath = relDir + "/" + entry.Name()
		}
		node := TreeNode{
			Name:       entry.Name(),
			Path:       filepath.ToSlash(relPath),
			ModifiedAt: info.ModTime().UTC(),
		}
		*count = *count + 1
		if entry.IsDir() {
			node.Type = "folder"
			if remainingDepth > 1 {
				children, childLimited, err := s.readDirLimited(filepath.Join(absDir, entry.Name()), relPath, remainingDepth-1, maxItems, count)
				if err != nil {
					return nil, false, err
				}
				node.Children = children
				if childLimited {
					limited = true
				}
			}
		} else {
			node.Type = "file"
			node.Size = info.Size()
		}
		nodes = append(nodes, node)
	}
	if err := s.sortNodesForParent(relDir, nodes); err != nil {
		return nil, false, err
	}
	return nodes, limited, nil
}

func (s *Service) readDirShallow(absDir, relDir string) ([]TreeNode, error) {
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return nil, err
	}
	nodes := make([]TreeNode, 0, len(entries))
	for _, entry := range entries {
		if relDir == "" && (entry.Name() == TrashDirName || entry.Name() == LegacyTrashDirName) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		relPath := entry.Name()
		if relDir != "" {
			relPath = relDir + "/" + entry.Name()
		}
		node := TreeNode{
			Name:       entry.Name(),
			Path:       filepath.ToSlash(relPath),
			ModifiedAt: info.ModTime().UTC(),
		}
		if entry.IsDir() {
			node.Type = "folder"
		} else {
			node.Type = "file"
			node.Size = info.Size()
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

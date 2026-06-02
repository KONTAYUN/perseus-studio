package workspace

import "os"

func (s *Service) StatPath(path string) (TreeNode, error) {
	cleanPath, absPath, err := s.sanitizePath(path)
	if err != nil {
		return TreeNode{}, err
	}
	if isAnyTrashPath(cleanPath) {
		return TreeNode{}, trashManagedOnlyError()
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return TreeNode{}, err
	}
	node := TreeNode{
		Name:       pathBaseName(cleanPath),
		Path:       cleanPath,
		ModifiedAt: info.ModTime().UTC(),
	}
	if info.IsDir() {
		node.Type = "folder"
	} else {
		node.Type = "file"
		node.Size = info.Size()
	}
	return node, nil
}

func pathBaseName(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}

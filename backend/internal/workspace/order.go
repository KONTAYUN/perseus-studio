package workspace

import (
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

func (s *Service) ReorderPath(path, targetParentPath, previousPath, nextPath string) (PathChangeResult, error) {
	cleanOld, absOld, err := s.sanitizePath(path)
	if err != nil {
		return PathChangeResult{}, err
	}
	if isAnyTrashPath(cleanOld) {
		return PathChangeResult{}, trashManagedOnlyError()
	}

	cleanParent, absParent, err := s.sanitizeOptionalPath(targetParentPath)
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

	cleanPrevious, err := s.validateReorderSibling(previousPath, cleanParent)
	if err != nil {
		return PathChangeResult{}, err
	}
	cleanNext, err := s.validateReorderSibling(nextPath, cleanParent)
	if err != nil {
		return PathChangeResult{}, err
	}

	baseName := filepath.Base(cleanOld)
	cleanNew := baseName
	if cleanParent != "" {
		cleanNew = cleanParent + "/" + baseName
	}

	result := PathChangeResult{
		Path:         cleanNew,
		OldPath:      cleanOld,
		NewPath:      cleanNew,
		ChangedPaths: []string{cleanOld},
	}
	if cleanNew != cleanOld {
		result, err = s.renameValidated(cleanOld, cleanNew)
		if err != nil {
			return PathChangeResult{}, err
		}
	}

	previousName := reorderSiblingName(cleanPrevious, cleanOld, cleanNew)
	nextName := reorderSiblingName(cleanNext, cleanOld, cleanNew)
	if err := s.rewriteSiblingOrder(cleanParent, filepath.Base(cleanNew), previousName, nextName); err != nil {
		return PathChangeResult{}, err
	}

	if cleanNew == cleanOld {
		result.ChangedPaths = []string{cleanOld}
	}
	return result, nil
}

func (s *Service) validateReorderSibling(path, targetParent string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", nil
	}
	cleanPath, absPath, err := s.sanitizePath(path)
	if err != nil {
		return "", err
	}
	if isAnyTrashPath(cleanPath) {
		return "", trashManagedOnlyError()
	}
	if getParentPath(cleanPath) != targetParent {
		return "", ErrInvalidPath
	}
	if _, err := os.Stat(absPath); err != nil {
		return "", err
	}
	return cleanPath, nil
}

func reorderSiblingName(path, oldPath, newPath string) string {
	if path == "" || path == oldPath || path == newPath {
		return ""
	}
	return filepath.Base(path)
}

func (s *Service) rewriteSiblingOrder(parentPath, movingName, previousName, nextName string) error {
	_, absParent, err := s.sanitizeOptionalPath(parentPath)
	if err != nil {
		return err
	}
	nodes, err := s.readDirShallow(absParent, parentPath)
	if err != nil {
		return err
	}
	if err := s.sortNodesForParent(parentPath, nodes); err != nil {
		return err
	}

	names := make([]string, 0, len(nodes))
	foundMoving := false
	for _, node := range nodes {
		if node.Name == movingName {
			foundMoving = true
			continue
		}
		names = append(names, node.Name)
	}
	if !foundMoving {
		return fs.ErrNotExist
	}

	insertIndex := len(names)
	if nextName != "" {
		index := indexOfString(names, nextName)
		if index < 0 {
			return ErrInvalidPath
		}
		insertIndex = index
	} else if previousName != "" {
		index := indexOfString(names, previousName)
		if index < 0 {
			return ErrInvalidPath
		}
		insertIndex = index + 1
	}

	names = append(names, "")
	copy(names[insertIndex+1:], names[insertIndex:])
	names[insertIndex] = movingName
	return s.replaceOrderForParent(parentPath, names)
}

func (s *Service) sortNodesForParent(parentPath string, nodes []TreeNode) error {
	order, hasOrder, err := s.loadOrderForParent(parentPath)
	if err != nil {
		return err
	}
	sort.Slice(nodes, func(i, j int) bool {
		if hasOrder {
			leftPosition, leftOrdered := order[nodes[i].Name]
			rightPosition, rightOrdered := order[nodes[j].Name]
			if leftOrdered != rightOrdered {
				return leftOrdered
			}
			if leftOrdered && rightOrdered && leftPosition != rightPosition {
				return leftPosition < rightPosition
			}
		}
		return defaultTreeNodeLess(nodes[i], nodes[j])
	})
	return nil
}

func defaultTreeNodeLess(left, right TreeNode) bool {
	if left.Type != right.Type {
		return left.Type == "folder"
	}
	return left.Name < right.Name
}

func (s *Service) loadOrderForParent(parentPath string) (map[string]int, bool, error) {
	rows, err := s.db.Query(`
SELECT child_name, position
FROM workspace_item_orders
WHERE user_id = ? AND parent_path = ?
`, s.userID, parentPath)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()

	order := map[string]int{}
	for rows.Next() {
		var name string
		var position int
		if err := rows.Scan(&name, &position); err != nil {
			return nil, false, err
		}
		order[name] = position
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	return order, len(order) > 0, nil
}

func (s *Service) replaceOrderForParent(parentPath string, childNames []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
DELETE FROM workspace_item_orders
WHERE user_id = ? AND parent_path = ?
`, s.userID, parentPath); err != nil {
		return err
	}

	now := time.Now().UTC()
	stmt, err := tx.Prepare(`
INSERT INTO workspace_item_orders (user_id, parent_path, child_name, position, updated_at)
VALUES (?, ?, ?, ?, ?)
`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for index, name := range childNames {
		if _, err := stmt.Exec(s.userID, parentPath, name, index+1, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Service) moveOrderMetadata(oldPath, newPath string) error {
	oldParent := getParentPath(oldPath)
	newParent := getParentPath(newPath)
	oldName := filepath.Base(oldPath)
	newName := filepath.Base(newPath)

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var position sql.NullInt64
	err = tx.QueryRow(`
SELECT position
FROM workspace_item_orders
WHERE user_id = ? AND parent_path = ? AND child_name = ?
`, s.userID, oldParent, oldName).Scan(&position)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	if _, err := tx.Exec(`
DELETE FROM workspace_item_orders
WHERE user_id = ? AND parent_path = ? AND child_name = ?
`, s.userID, oldParent, oldName); err != nil {
		return err
	}
	if _, err := tx.Exec(`
DELETE FROM workspace_item_orders
WHERE user_id = ? AND parent_path = ? AND child_name = ?
`, s.userID, newParent, newName); err != nil {
		return err
	}
	if oldParent == newParent && position.Valid {
		if _, err := tx.Exec(`
INSERT INTO workspace_item_orders (user_id, parent_path, child_name, position, updated_at)
VALUES (?, ?, ?, ?, ?)
`, s.userID, newParent, newName, position.Int64, time.Now().UTC()); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(`
DELETE FROM workspace_item_orders
WHERE user_id = ? AND (parent_path = ? OR parent_path LIKE ?)
`, s.userID, newPath, newPath+"/%"); err != nil {
		return err
	}
	if _, err := tx.Exec(`
UPDATE workspace_item_orders
SET parent_path = ?
WHERE user_id = ? AND parent_path = ?
`, newPath, s.userID, oldPath); err != nil {
		return err
	}
	if _, err := tx.Exec(`
UPDATE workspace_item_orders
SET parent_path = ? || substr(parent_path, ?)
WHERE user_id = ? AND parent_path LIKE ?
`, newPath, utf8.RuneCountInString(oldPath)+1, s.userID, oldPath+"/%"); err != nil {
		return err
	}

	if _, err := tx.Exec(`
UPDATE file_versions
SET path = ?
WHERE user_id = ? AND path = ?
`, newPath, s.userID, oldPath); err != nil {
		return err
	}
	if _, err := tx.Exec(`
UPDATE file_versions
SET path = ? || substr(path, ?)
WHERE user_id = ? AND path LIKE ?
`, newPath, utf8.RuneCountInString(oldPath)+1, s.userID, oldPath+"/%"); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Service) removeOrderMetadataForPath(path string) error {
	parent := getParentPath(path)
	name := filepath.Base(path)
	_, err := s.db.Exec(`
DELETE FROM workspace_item_orders
WHERE user_id = ?
  AND (
    (parent_path = ? AND child_name = ?)
    OR parent_path = ?
    OR parent_path LIKE ?
  )
`, s.userID, parent, name, path, path+"/%")
	return err
}

func indexOfString(values []string, target string) int {
	for index, value := range values {
		if value == target {
			return index
		}
	}
	return -1
}

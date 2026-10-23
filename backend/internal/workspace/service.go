package workspace

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"webide-backend/internal/store"
)

var (
	ErrInvalidPath      = errors.New("invalid path")
	ErrPathEscape       = errors.New("path escapes workspace")
	ErrPathExists       = errors.New("path already exists")
	ErrTrashManagedOnly = errors.New("回收站只能通过回收站面板管理")
	ErrMoveIntoSelf     = errors.New("不能移动到自身文件夹内部")
	ErrFileConflict     = errors.New("文件已被其他操作修改，请重新加载或确认保留本地修改")
)

const (
	TrashDirName       = "回收站"
	LegacyTrashDirName = ".trash"
)

type Service struct {
	writeMu sync.Mutex
	patchMu sync.Mutex
	db      *sql.DB
	root    string
	rootAbs string
	userID  string
}

func NewService(db *sql.DB, root, userID string) (*Service, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &Service{
		db:      db,
		root:    root,
		rootAbs: filepath.Clean(absRoot),
		userID:  userID,
	}, nil
}

func (s *Service) Migrate(ctx context.Context) error {
	tables := `
CREATE TABLE IF NOT EXISTS workspace_patch_journals (
 id TEXT PRIMARY KEY, user_id TEXT NOT NULL, state TEXT NOT NULL, plan_json TEXT NOT NULL, created_at DATETIME NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_workspace_patch_pending ON workspace_patch_journals(user_id) WHERE state='pending';
CREATE TABLE IF NOT EXISTS file_versions (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL DEFAULT '',
  path TEXT NOT NULL,
  content_text TEXT,
  size_bytes INTEGER NOT NULL,
  content_hash TEXT NOT NULL,
  source TEXT NOT NULL,
  created_at DATETIME NOT NULL
);
CREATE TABLE IF NOT EXISTS trash_items (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL DEFAULT '',
  original_path TEXT NOT NULL,
  trash_path TEXT NOT NULL,
  type TEXT NOT NULL,
  size_bytes INTEGER NOT NULL DEFAULT 0,
  deleted_at DATETIME NOT NULL
);
CREATE TABLE IF NOT EXISTS workspace_item_orders (
  user_id TEXT NOT NULL DEFAULT '',
  parent_path TEXT NOT NULL DEFAULT '',
  child_name TEXT NOT NULL,
  position INTEGER NOT NULL,
  updated_at DATETIME NOT NULL,
  PRIMARY KEY (user_id, parent_path, child_name)
);
CREATE TABLE IF NOT EXISTS workspace_change_reverts (
  user_id TEXT NOT NULL,
  operation_id TEXT NOT NULL,
  state TEXT NOT NULL,
  result_json TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL,
  PRIMARY KEY (user_id, operation_id)
);
`
	if _, err := s.db.ExecContext(ctx, tables); err != nil {
		return err
	}
	if err := store.AddColumnIfNotExists(ctx, s.db, "file_versions", "user_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := store.AddColumnIfNotExists(ctx, s.db, "trash_items", "user_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := store.AddColumnIfNotExists(ctx, s.db, "trash_items", "size_bytes", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	indexes := `
CREATE INDEX IF NOT EXISTS idx_file_versions_path_created_at ON file_versions(path, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_file_versions_user_id ON file_versions(user_id);
CREATE INDEX IF NOT EXISTS idx_trash_items_user_id ON trash_items(user_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_trash_items_user_path ON trash_items(user_id, trash_path);
CREATE INDEX IF NOT EXISTS idx_workspace_item_orders_parent ON workspace_item_orders(user_id, parent_path, position);
`
	_, err := s.db.ExecContext(ctx, indexes)
	return err
}

func (s *Service) EnsureWorkspace() error {
	if err := os.MkdirAll(s.rootAbs, 0o755); err != nil {
		return err
	}
	return s.migrateLegacyTrashDir()
}

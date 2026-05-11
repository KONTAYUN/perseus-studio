package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Skill struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	SourcePath  string    `json:"source_path"`
	InstallPath string    `json:"install_path"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type SkillInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     *bool  `json:"enabled"`
}

type UpsertSkillInput struct {
	ID          string
	UserID      string
	Name        string
	Description string
	SourcePath  string
	InstallPath string
	Enabled     bool
}

type SkillStore struct {
	db *sql.DB
}

func NewSkillStore(db *sql.DB) *SkillStore {
	return &SkillStore{db: db}
}

func (s *SkillStore) Migrate(ctx context.Context) error {
	tables := `
CREATE TABLE IF NOT EXISTS skills (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  source_path TEXT NOT NULL DEFAULT '',
  install_path TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);
`
	if _, err := s.db.ExecContext(ctx, tables); err != nil {
		return err
	}
	if err := AddColumnIfNotExists(ctx, s.db, "skills", "user_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	indexes := `
CREATE INDEX IF NOT EXISTS idx_skills_enabled_updated_at ON skills(enabled, updated_at);
CREATE INDEX IF NOT EXISTS idx_skills_name_source_path ON skills(name, source_path);
CREATE INDEX IF NOT EXISTS idx_skills_user_id ON skills(user_id);
`
	_, err := s.db.ExecContext(ctx, indexes)
	return err
}

func (s *SkillStore) ListSkills(ctx context.Context, userID string) ([]Skill, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, user_id, name, description, source_path, install_path, enabled, created_at, updated_at
FROM skills
WHERE user_id = ?
ORDER BY updated_at DESC
`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Skill
	for rows.Next() {
		item, err := scanSkill(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *SkillStore) ListEnabledSkills(ctx context.Context, userID string) ([]Skill, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, user_id, name, description, source_path, install_path, enabled, created_at, updated_at
FROM skills
WHERE enabled = 1 AND user_id = ?
ORDER BY updated_at DESC
`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Skill
	for rows.Next() {
		item, err := scanSkill(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *SkillStore) GetSkill(ctx context.Context, userID, id string) (Skill, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, user_id, name, description, source_path, install_path, enabled, created_at, updated_at
FROM skills
WHERE id = ? AND user_id = ?
`, id, userID)
	return scanSkill(row)
}

func (s *SkillStore) FindSkillByNameSource(ctx context.Context, userID, name, sourcePath string) (Skill, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, user_id, name, description, source_path, install_path, enabled, created_at, updated_at
FROM skills
WHERE lower(name) = lower(?) AND source_path = ? AND user_id = ?
ORDER BY updated_at DESC
LIMIT 1
`, strings.TrimSpace(name), strings.TrimSpace(sourcePath), userID)
	return scanSkill(row)
}

func (s *SkillStore) UpsertSkill(ctx context.Context, input UpsertSkillInput) (Skill, error) {
	if strings.TrimSpace(input.ID) == "" {
		return Skill{}, fmt.Errorf("id is required")
	}
	if strings.TrimSpace(input.Name) == "" {
		return Skill{}, fmt.Errorf("name is required")
	}
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO skills (id, user_id, name, description, source_path, install_path, enabled, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  name = excluded.name,
  description = excluded.description,
  source_path = excluded.source_path,
  install_path = excluded.install_path,
  enabled = excluded.enabled,
  updated_at = excluded.updated_at
WHERE skills.user_id = excluded.user_id
`, input.ID, input.UserID, strings.TrimSpace(input.Name), strings.TrimSpace(input.Description), strings.TrimSpace(input.SourcePath), strings.TrimSpace(input.InstallPath), boolToInt(input.Enabled), now, now)
	if err != nil {
		return Skill{}, err
	}
	return s.GetSkill(ctx, input.UserID, input.ID)
}

func (s *SkillStore) UpdateSkill(ctx context.Context, userID, id string, input SkillInput) (Skill, error) {
	existing, err := s.GetSkill(ctx, userID, id)
	if err != nil {
		return Skill{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = existing.Name
	}
	description := strings.TrimSpace(input.Description)
	enabled := existing.Enabled
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
UPDATE skills
SET name = ?, description = ?, enabled = ?, updated_at = ?
WHERE id = ? AND user_id = ?
`, name, description, boolToInt(enabled), now, id, userID)
	if err != nil {
		return Skill{}, err
	}
	return s.GetSkill(ctx, userID, id)
}

func (s *SkillStore) DeleteSkill(ctx context.Context, userID, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM skills WHERE id = ? AND user_id = ?`, id, userID)
	return err
}

func scanSkill(s scanner) (Skill, error) {
	var item Skill
	var enabled int
	err := s.Scan(&item.ID, &item.UserID, &item.Name, &item.Description, &item.SourcePath, &item.InstallPath, &enabled, &item.CreatedAt, &item.UpdatedAt)
	item.Enabled = enabled == 1
	return item, err
}

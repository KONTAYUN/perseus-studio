package skills

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"webide-backend/internal/store"
)

const (
	maxZipBytes       = 25 << 20
	maxExtractedBytes = 80 << 20
	maxSkillReadRunes = 20000
	pendingDirName    = ".pending"
	skillMarkdownName = "SKILL.md"
)

type Service struct {
	root  string
	store *store.SkillStore
}

type Candidate struct {
	ID          string `json:"id"`
	SourcePath  string `json:"source_path"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type UploadResult struct {
	Installed  []store.Skill `json:"installed,omitempty"`
	Candidates []Candidate   `json:"candidates,omitempty"`
	UploadID   string        `json:"upload_id,omitempty"`
	Message    string        `json:"message"`
}

type ReadResult struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
}

func NewService(root string, skillStore *store.SkillStore) *Service {
	return &Service{root: filepath.Clean(root), store: skillStore}
}

func (s *Service) Ensure(ctx context.Context, userID string) error {
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(s.root, pendingDirName), 0o755); err != nil {
		return err
	}
	return s.Sync(ctx, userID)
}

func (s *Service) Sync(ctx context.Context, userID string) error {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == pendingDirName {
			continue
		}
		installPath := filepath.Join(s.root, entry.Name())
		content, err := os.ReadFile(filepath.Join(installPath, skillMarkdownName))
		if err != nil {
			continue
		}
		name, description := parseSkillMetadata(string(content), entry.Name())
		visibility := store.ResourceVisibilityPrivate
		enabled := true
		if existing, err := s.store.GetSkill(ctx, userID, entry.Name()); err == nil {
			visibility = existing.Visibility
			enabled = existing.Enabled
		}
		if _, err := s.store.UpsertSkill(ctx, store.UpsertSkillInput{
			ID:          entry.Name(),
			UserID:      userID,
			Name:        name,
			Description: description,
			Visibility:  visibility,
			SourcePath:  "",
			InstallPath: installPath,
			Enabled:     enabled,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) List(ctx context.Context, userID string) ([]store.Skill, error) {
	return s.store.ListSkills(ctx, userID)
}

func (s *Service) ListForUser(ctx context.Context, userID, role string) ([]store.Skill, error) {
	return s.store.ListSkillsForUser(ctx, userID, role)
}

func (s *Service) ListEnabled(ctx context.Context, userID string) ([]store.Skill, error) {
	return s.store.ListEnabledSkills(ctx, userID)
}

func (s *Service) ListEnabledForUser(ctx context.Context, userID, role string) ([]store.Skill, error) {
	return s.store.ListEnabledSkillsForUser(ctx, userID, role)
}

func (s *Service) Get(ctx context.Context, userID, id string) (store.Skill, error) {
	return s.store.GetSkill(ctx, userID, normalizeSkillID(id))
}

func (s *Service) GetForUser(ctx context.Context, userID, role, id string) (store.Skill, error) {
	return s.store.GetSkillForUser(ctx, userID, role, normalizeSkillID(id))
}

func (s *Service) Update(ctx context.Context, userID, id string, input store.SkillInput) (store.Skill, error) {
	return s.store.UpdateSkill(ctx, userID, normalizeSkillID(id), input)
}

func (s *Service) Delete(ctx context.Context, userID, id string) error {
	id = normalizeSkillID(id)
	item, err := s.store.GetSkill(ctx, userID, id)
	if err != nil {
		return err
	}
	if !s.isInstalledPath(item.InstallPath) {
		return fmt.Errorf("invalid skill install path")
	}
	if err := os.RemoveAll(item.InstallPath); err != nil {
		return err
	}
	return s.store.DeleteSkill(ctx, userID, id)
}

func (s *Service) DeleteAny(ctx context.Context, id string) error {
	id = normalizeSkillID(id)
	item, err := s.store.GetSkillAny(ctx, id)
	if err != nil {
		return err
	}
	if !s.isInstalledPath(item.InstallPath) {
		return fmt.Errorf("invalid skill install path")
	}
	if err := os.RemoveAll(item.InstallPath); err != nil {
		return err
	}
	return s.store.DeleteSkill(ctx, item.UserID, id)
}

func (s *Service) ReadSkill(ctx context.Context, userID, id string, enabledOnly bool) (ReadResult, error) {
	item, err := s.skillForRead(ctx, userID, "admin", id)
	if err != nil {
		if err == sql.ErrNoRows {
			return ReadResult{}, fmt.Errorf("skill not found")
		}
		return ReadResult{}, err
	}
	if enabledOnly && !item.Enabled {
		return ReadResult{}, fmt.Errorf("skill is disabled")
	}
	if !s.isInstalledPath(item.InstallPath) {
		return ReadResult{}, fmt.Errorf("invalid skill install path")
	}
	raw, err := os.ReadFile(filepath.Join(item.InstallPath, skillMarkdownName))
	if err != nil {
		return ReadResult{}, err
	}
	content, truncated := truncateRunes(string(raw), maxSkillReadRunes)
	return ReadResult{ID: item.ID, Content: content, Truncated: truncated}, nil
}

func (s *Service) ReadSkillForUser(ctx context.Context, userID, role, id string, enabledOnly bool) (ReadResult, error) {
	item, err := s.skillForRead(ctx, userID, role, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return ReadResult{}, fmt.Errorf("skill not found")
		}
		return ReadResult{}, err
	}
	if enabledOnly && !item.Enabled {
		return ReadResult{}, fmt.Errorf("skill is disabled")
	}
	if !s.isInstalledPath(item.InstallPath) {
		return ReadResult{}, fmt.Errorf("invalid skill install path")
	}
	raw, err := os.ReadFile(filepath.Join(item.InstallPath, skillMarkdownName))
	if err != nil {
		return ReadResult{}, err
	}
	content, truncated := truncateRunes(string(raw), maxSkillReadRunes)
	return ReadResult{ID: item.ID, Content: content, Truncated: truncated}, nil
}

func (s *Service) DirectoryTree(ctx context.Context, userID, id string) ([]string, error) {
	item, err := s.store.GetSkill(ctx, userID, normalizeSkillID(id))
	if err != nil {
		return nil, err
	}
	if !s.isInstalledPath(item.InstallPath) {
		return nil, fmt.Errorf("invalid skill install path")
	}
	var items []string
	err = filepath.WalkDir(item.InstallPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == item.InstallPath {
			return nil
		}
		rel, err := filepath.Rel(item.InstallPath, path)
		if err != nil {
			return err
		}
		items = append(items, filepath.ToSlash(rel))
		if len(items) >= 300 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		return nil
	})
	return items, err
}

func (s *Service) DirectoryTreeForUser(ctx context.Context, userID, role, id string) ([]string, error) {
	item, err := s.store.GetSkillForUser(ctx, userID, role, normalizeSkillID(id))
	if err != nil {
		return nil, err
	}
	if !s.isInstalledPath(item.InstallPath) {
		return nil, fmt.Errorf("invalid skill install path")
	}
	var items []string
	err = filepath.WalkDir(item.InstallPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == item.InstallPath {
			return nil
		}
		rel, err := filepath.Rel(item.InstallPath, path)
		if err != nil {
			return err
		}
		items = append(items, filepath.ToSlash(rel))
		if len(items) >= 300 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		return nil
	})
	return items, err
}

func (s *Service) skillForRead(ctx context.Context, userID, role, id string) (store.Skill, error) {
	id = normalizeSkillID(id)
	if role == "admin" {
		return s.store.GetSkillAny(ctx, id)
	}
	return s.store.GetSkillForUser(ctx, userID, role, id)
}

func normalizeSkillID(id string) string {
	value := strings.TrimSpace(id)
	for strings.HasPrefix(value, "skill_skill_") {
		value = strings.TrimPrefix(value, "skill_")
	}
	return value
}

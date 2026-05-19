package skills

import (
	"context"
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
		if _, err := s.store.UpsertSkill(ctx, store.UpsertSkillInput{
			ID:          entry.Name(),
			UserID:      userID,
			Name:        name,
			Description: description,
			SourcePath:  "",
			InstallPath: installPath,
			Enabled:     true,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) List(ctx context.Context, userID string) ([]store.Skill, error) {
	return s.store.ListSkills(ctx, userID)
}

func (s *Service) ListEnabled(ctx context.Context, userID string) ([]store.Skill, error) {
	return s.store.ListEnabledSkills(ctx, userID)
}

func (s *Service) Get(ctx context.Context, userID, id string) (store.Skill, error) {
	return s.store.GetSkill(ctx, userID, normalizeSkillID(id))
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

func (s *Service) ReadSkill(ctx context.Context, userID, id string, enabledOnly bool) (ReadResult, error) {
	id = normalizeSkillID(id)
	item, err := s.store.GetSkill(ctx, userID, id)
	if err != nil {
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
	return ReadResult{ID: id, Content: content, Truncated: truncated}, nil
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

func normalizeSkillID(id string) string {
	value := strings.TrimSpace(id)
	for strings.HasPrefix(value, "skill_skill_") {
		value = strings.TrimPrefix(value, "skill_")
	}
	return value
}

package skills

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"webide-backend/internal/store"
)

func (s *Service) InstallZip(ctx context.Context, userID string, raw []byte, enabled bool) (UploadResult, error) {
	uploadID, tempRoot, candidates, err := s.prepareZip(raw)
	if err != nil {
		return UploadResult{}, err
	}
	if len(candidates) == 1 {
		item, err := s.installCandidate(ctx, userID, tempRoot, candidates[0], enabled)
		_ = os.RemoveAll(tempRoot)
		if err != nil {
			return UploadResult{}, err
		}
		return UploadResult{Installed: []store.Skill{item}, Message: "Skill 已安装"}, nil
	}
	return UploadResult{
		Candidates: candidates,
		UploadID:   uploadID,
		Message:    "发现多个 SKILL.md，请选择要安装的 Skill。",
	}, nil
}

func (s *Service) InstallSelected(ctx context.Context, userID, uploadID string, candidateIDs []string, enabled bool) ([]store.Skill, error) {
	uploadID = cleanID(uploadID)
	if uploadID == "" {
		return nil, fmt.Errorf("upload_id is required")
	}
	tempRoot := filepath.Join(s.root, pendingDirName, uploadID)
	if !isWithin(filepath.Join(s.root, pendingDirName), tempRoot) {
		return nil, fmt.Errorf("invalid upload_id")
	}
	candidates, err := s.findCandidates(tempRoot)
	if err != nil {
		return nil, err
	}
	selected := map[string]bool{}
	for _, id := range candidateIDs {
		selected[id] = true
	}
	var installed []store.Skill
	for _, candidate := range candidates {
		if !selected[candidate.ID] {
			continue
		}
		item, err := s.installCandidate(ctx, userID, tempRoot, candidate, enabled)
		if err != nil {
			return installed, err
		}
		installed = append(installed, item)
	}
	if len(installed) == 0 {
		return nil, fmt.Errorf("no selected skill candidates found")
	}
	_ = os.RemoveAll(tempRoot)
	return installed, nil
}

func (s *Service) prepareZip(raw []byte) (string, string, []Candidate, error) {
	if len(raw) == 0 {
		return "", "", nil, fmt.Errorf("zip is empty")
	}
	if len(raw) > maxZipBytes {
		return "", "", nil, fmt.Errorf("zip 文件过大")
	}
	uploadID := "skill_upload_" + randomHex(8)
	tempRoot := filepath.Join(s.root, pendingDirName, uploadID)
	if err := os.MkdirAll(tempRoot, 0o755); err != nil {
		return "", "", nil, err
	}
	if err := unzipSafe(raw, tempRoot); err != nil {
		_ = os.RemoveAll(tempRoot)
		return "", "", nil, err
	}
	candidates, err := s.findCandidates(tempRoot)
	if err != nil {
		_ = os.RemoveAll(tempRoot)
		return "", "", nil, err
	}
	if len(candidates) == 0 {
		_ = os.RemoveAll(tempRoot)
		return "", "", nil, fmt.Errorf("未找到 SKILL.md")
	}
	return uploadID, tempRoot, candidates, nil
}

func (s *Service) findCandidates(root string) ([]Candidate, error) {
	var candidates []Candidate
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.EqualFold(d.Name(), skillMarkdownName) {
			return nil
		}
		dir := filepath.Dir(path)
		rel, err := filepath.Rel(root, dir)
		if err != nil {
			return err
		}
		if rel == "." {
			rel = ""
		}
		raw, _ := os.ReadFile(path)
		name, description := parseSkillMetadata(string(raw), filepath.Base(dir))
		sourcePath := filepath.ToSlash(rel)
		candidates = append(candidates, Candidate{
			ID:          candidateID(sourcePath),
			SourcePath:  sourcePath,
			Name:        name,
			Description: description,
		})
		return nil
	})
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].SourcePath < candidates[j].SourcePath
	})
	return candidates, err
}

func (s *Service) installCandidate(ctx context.Context, userID, tempRoot string, candidate Candidate, enabled bool) (store.Skill, error) {
	sourceDir := filepath.Clean(filepath.Join(tempRoot, filepath.FromSlash(candidate.SourcePath)))
	if !isWithin(tempRoot, sourceDir) {
		return store.Skill{}, fmt.Errorf("invalid skill source path")
	}
	if _, err := os.Stat(filepath.Join(sourceDir, skillMarkdownName)); err != nil {
		return store.Skill{}, fmt.Errorf("未找到 SKILL.md")
	}
	id := "skill_" + randomHex(8)
	if existing, err := s.store.FindSkillByNameSource(ctx, userID, candidate.Name, candidate.SourcePath); err == nil && s.isInstalledPath(existing.InstallPath) {
		id = existing.ID
		_ = os.RemoveAll(existing.InstallPath)
	}
	dest := filepath.Join(s.root, id)
	if err := copyDir(sourceDir, dest); err != nil {
		_ = os.RemoveAll(dest)
		return store.Skill{}, err
	}
	return s.store.UpsertSkill(ctx, store.UpsertSkillInput{
		ID:          id,
		UserID:      userID,
		Name:        candidate.Name,
		Description: candidate.Description,
		Visibility:  store.ResourceVisibilityPrivate,
		SourcePath:  candidate.SourcePath,
		InstallPath: dest,
		Enabled:     enabled,
	})
}

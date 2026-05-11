package skills

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

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
		SourcePath:  candidate.SourcePath,
		InstallPath: dest,
		Enabled:     enabled,
	})
}

func unzipSafe(raw []byte, dest string) error {
	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return fmt.Errorf("invalid zip: %w", err)
	}
	total := int64(0)
	for _, file := range reader.File {
		name := strings.ReplaceAll(file.Name, "\\", "/")
		if strings.HasPrefix(name, "/") || filepath.IsAbs(name) || hasUnsafePathElement(name) {
			return fmt.Errorf("zip 包含不安全路径")
		}
		target := filepath.Clean(filepath.Join(dest, filepath.FromSlash(name)))
		if !isWithin(dest, target) {
			return fmt.Errorf("zip 包含路径穿越风险")
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		total += int64(file.UncompressedSize64)
		if total > maxExtractedBytes {
			return fmt.Errorf("zip 解压后内容过大")
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		src, err := file.Open()
		if err != nil {
			return err
		}
		dst, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			_ = src.Close()
			return err
		}
		_, copyErr := io.Copy(dst, io.LimitReader(src, maxExtractedBytes+1))
		closeErr := dst.Close()
		_ = src.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func hasUnsafePathElement(name string) bool {
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

func parseSkillMetadata(content, fallbackName string) (string, string) {
	name := ""
	description := ""
	lines := strings.Split(content, "\n")
	inFrontMatter := len(lines) > 0 && strings.TrimSpace(lines[0]) == "---"
	if inFrontMatter {
		values := parseFrontMatterValues(lines)
		name = values["name"]
		description = values["description"]
	} else {
		for index, line := range lines {
			if index > 40 {
				break
			}
			trimmed := strings.TrimSpace(line)
			lower := strings.ToLower(trimmed)
			if strings.HasPrefix(lower, "name:") && name == "" {
				name = cleanMetaValue(trimmed[5:])
			}
			if strings.HasPrefix(lower, "description:") && description == "" {
				description = cleanMetaValue(trimmed[12:])
			}
		}
	}
	if name == "" {
		re := regexp.MustCompile(`(?m)^#\s+(.+)$`)
		if match := re.FindStringSubmatch(content); len(match) == 2 {
			name = strings.TrimSpace(match[1])
		}
	}
	if name == "" {
		name = fallbackName
	}
	return truncatePlain(name, 80), truncatePlain(description, 240)
}

func parseFrontMatterValues(lines []string) map[string]string {
	values := map[string]string{}
	for index := 1; index < len(lines); index++ {
		line := strings.TrimRight(lines[index], "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			break
		}
		key, rawValue, ok := splitYAMLKeyValue(trimmed)
		if !ok {
			continue
		}
		key = strings.ToLower(key)
		if key != "name" && key != "description" {
			continue
		}
		if rawValue == ">" || rawValue == ">-" || rawValue == ">+" || rawValue == "|" || rawValue == "|-" || rawValue == "|+" {
			block, nextIndex := collectYAMLBlock(lines, index+1, strings.HasPrefix(rawValue, ">"))
			values[key] = block
			index = nextIndex - 1
			continue
		}
		values[key] = cleanMetaValue(rawValue)
	}
	return values
}

func splitYAMLKeyValue(line string) (string, string, bool) {
	index := strings.Index(line, ":")
	if index <= 0 {
		return "", "", false
	}
	key := strings.TrimSpace(line[:index])
	value := strings.TrimSpace(line[index+1:])
	if key == "" {
		return "", "", false
	}
	return key, value, true
}

func collectYAMLBlock(lines []string, start int, folded bool) (string, int) {
	var parts []string
	for index := start; index < len(lines); index++ {
		line := strings.TrimRight(lines[index], "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" || isTopLevelYAMLKey(line) {
			return cleanYAMLBlock(parts, folded), index
		}
		if strings.TrimSpace(line) == "" {
			if !folded {
				parts = append(parts, "")
			}
			continue
		}
		parts = append(parts, strings.TrimSpace(line))
	}
	return cleanYAMLBlock(parts, folded), len(lines)
}

func isTopLevelYAMLKey(line string) bool {
	if line == "" || line[0] == ' ' || line[0] == '\t' {
		return false
	}
	_, _, ok := splitYAMLKeyValue(strings.TrimSpace(line))
	return ok
}

func cleanYAMLBlock(parts []string, folded bool) string {
	if folded {
		return strings.TrimSpace(strings.Join(parts, " "))
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func cleanMetaValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `"'`)
	return strings.TrimSpace(value)
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}

func (s *Service) isInstalledPath(path string) bool {
	clean := filepath.Clean(path)
	if !isWithin(s.root, clean) {
		return false
	}
	return !strings.Contains(clean, string(filepath.Separator)+pendingDirName+string(filepath.Separator))
}

func isWithin(root, target string) bool {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}

func candidateID(sourcePath string) string {
	if sourcePath == "" {
		return "root"
	}
	return strings.ReplaceAll(filepath.ToSlash(sourcePath), "/", "__")
}

func randomHex(size int) string {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(raw)
}

func cleanID(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, `/\`)
	if strings.Contains(value, "..") || strings.ContainsAny(value, `/\`) {
		return ""
	}
	return value
}

func truncateRunes(value string, limit int) (string, bool) {
	runes := []rune(value)
	if len(runes) <= limit {
		return value, false
	}
	return string(runes[:limit]) + "\n...[truncated]", true
}

func truncatePlain(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit])
}

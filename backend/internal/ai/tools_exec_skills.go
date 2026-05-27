package ai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	skillpkg "webide-backend/internal/skills"
)

func (s *Service) execListSkills(ctx context.Context, userID, argumentsJSON string) (string, string, string, string, error) {
	return s.execListSkillsForRole(ctx, userID, "admin", argumentsJSON)
}

func (s *Service) execListSkillsForRole(ctx context.Context, userID, role, argumentsJSON string) (string, string, string, string, error) {
	skillsSvc, err := s.factory.Skills(userID)
	if err != nil {
		return "", "", "正在查看 Skills...", "", err
	}
	items, err := skillsSvc.ListEnabledForUser(ctx, userID, normalizeRunUserRole(role))
	if err != nil {
		return "", "", "正在查看 Skills...", "", err
	}
	payloadItems := make([]map[string]string, 0, len(items))
	for _, item := range items {
		payloadItems = append(payloadItems, map[string]string{
			"id":          item.ID,
			"name":        item.Name,
			"description": item.Description,
		})
	}
	payload := map[string]any{"items": payloadItems}
	return mustJSON(payload), fmt.Sprintf("listed %d enabled skills", len(items)), "正在查看 Skills...", "", nil
}

func (s *Service) execReadSkill(ctx context.Context, userID, argumentsJSON string) (string, string, string, string, error) {
	return s.execReadSkillForRole(ctx, userID, "admin", argumentsJSON)
}

func (s *Service) execReadSkillForRole(ctx context.Context, userID, role, argumentsJSON string) (string, string, string, string, error) {
	var args struct {
		SkillID string `json:"skill_id"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", "", "正在读取 Skill...", "", fmt.Errorf("invalid tool arguments")
	}
	skillsSvc, err := s.factory.Skills(userID)
	if err != nil {
		return "", "", "正在读取 Skill...", "", err
	}
	item, err := skillsSvc.GetForUser(ctx, userID, normalizeRunUserRole(role), args.SkillID)
	if err != nil {
		return "", "", "正在读取 Skill...", "", err
	}
	ownerSkillsSvc := skillsSvc
	if item.UserID != userID {
		ownerSkillsSvc, err = s.factory.Skills(item.UserID)
		if err != nil {
			return "", "", "正在读取 Skill...", "", err
		}
	}
	result, err := ownerSkillsSvc.ReadSkillForUser(ctx, userID, normalizeRunUserRole(role), args.SkillID, true)
	if err != nil {
		return "", "", "正在读取 Skill...", "", err
	}
	summary := fmt.Sprintf("read skill %s chars=%d truncated=%t", args.SkillID, len([]rune(result.Content)), result.Truncated)
	return mustJSON(result), summary, "正在读取 Skill...", args.SkillID, nil
}

func (s *Service) execInstallSkillFromAttachment(ctx context.Context, userID, argumentsJSON string, opts RunOptions) (string, string, string, string, error) {
	if normalizeRunUserRole(opts.UserRole) != "admin" {
		return "", "", "正在安装 Skill...", "", fmt.Errorf("admin required")
	}
	var args struct {
		AttachmentID string `json:"attachment_id"`
		SkillID      string `json:"skill_id"`
		Enabled      *bool  `json:"enabled"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", "", "正在安装 Skill...", "", fmt.Errorf("invalid tool arguments")
	}
	skillsSvc, err := s.factory.Skills(userID)
	if err != nil {
		return "", "", "正在安装 Skill...", "", err
	}
	enabled := true
	if args.Enabled != nil {
		enabled = *args.Enabled
	}
	attachment, ok := findAttachment(opts.Attachments, args.AttachmentID)
	if !ok {
		return "", "", "正在安装 Skill...", "", fmt.Errorf("attachment not found")
	}
	if !strings.HasSuffix(strings.ToLower(attachment.Name), ".zip") {
		return "", "", "正在安装 Skill...", "", fmt.Errorf("只支持安装 .zip 格式的 Skill")
	}
	raw, err := base64.StdEncoding.DecodeString(attachment.RawBase64)
	if err != nil || len(raw) == 0 {
		return "", "", "正在安装 Skill...", "", fmt.Errorf("attachment content is unavailable")
	}
	result, err := skillsSvc.InstallZip(ctx, userID, raw, enabled)
	if err != nil {
		return "", "", "正在安装 Skill...", "", err
	}
	if args.SkillID != "" && len(result.Candidates) > 0 {
		installed, err := skillsSvc.InstallSelected(ctx, userID, result.UploadID, []string{args.SkillID}, enabled)
		if err != nil {
			return "", "", "正在安装 Skill...", "", err
		}
		result = skillpkg.UploadResult{Installed: installed, Message: "Skill 已安装"}
	}
	summary := fmt.Sprintf("skill install installed=%d candidates=%d", len(result.Installed), len(result.Candidates))
	return mustJSON(result), summary, "正在安装 Skill...", "", nil
}

func (s *Service) execDeleteSkill(ctx context.Context, userID, argumentsJSON string) (string, string, string, string, error) {
	return s.execDeleteSkillForRole(ctx, userID, "admin", argumentsJSON)
}

func (s *Service) execDeleteSkillForRole(ctx context.Context, userID, role, argumentsJSON string) (string, string, string, string, error) {
	if normalizeRunUserRole(role) != "admin" {
		return "", "", "正在删除 Skill...", "", fmt.Errorf("admin required")
	}
	var args struct {
		SkillID string `json:"skill_id"`
	}
	if err := json.Unmarshal([]byte(argumentsJSON), &args); err != nil {
		return "", "", "正在删除 Skill...", "", fmt.Errorf("invalid tool arguments")
	}
	skillsSvc, err := s.factory.Skills(userID)
	if err != nil {
		return "", "", "正在删除 Skill...", "", err
	}
	item, err := skillsSvc.GetForUser(ctx, userID, "admin", args.SkillID)
	if err != nil {
		return "", "", "正在删除 Skill...", args.SkillID, err
	}
	ownerSkillsSvc := skillsSvc
	if item.UserID != userID {
		ownerSkillsSvc, err = s.factory.Skills(item.UserID)
		if err != nil {
			return "", "", "正在删除 Skill...", args.SkillID, err
		}
	}
	if err := ownerSkillsSvc.DeleteAny(ctx, args.SkillID); err != nil {
		return "", "", "正在删除 Skill...", args.SkillID, err
	}
	payload := map[string]any{
		"skill_id":     args.SkillID,
		"name":         item.Name,
		"install_path": item.InstallPath,
		"deleted":      true,
	}
	summary := fmt.Sprintf("deleted skill %s (%s)", item.Name, args.SkillID)
	return mustJSON(payload), summary, "正在删除 Skill...", args.SkillID, nil
}

func findAttachment(items []AttachmentPayload, id string) (AttachmentPayload, bool) {
	id = strings.TrimSpace(id)
	for _, item := range items {
		if item.ID == id || (id == "" && strings.HasSuffix(strings.ToLower(item.Name), ".zip")) {
			return item, true
		}
	}
	return AttachmentPayload{}, false
}

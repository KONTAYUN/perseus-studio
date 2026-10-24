package ai

import (
	"context"
	"encoding/json"
	"fmt"
)

func (s *Service) execDocumentRepairPatch(ctx context.Context, raw string, opts RunOptions) (string, string, string, string, error) {
	action := opts.DocumentAction
	reject := func(err error) (string, string, string, string, error) {
		return "", "", "修复候选未应用", action.Path, err
	}
	var args struct {
		Patch string `json:"patch"`
	}
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return reject(err)
	}
	patch, err := parseWorkspacePatch(args.Patch)
	if err != nil {
		return reject(err)
	}
	if len(patch.Operations) != 1 {
		return reject(fmt.Errorf("格式修复只能更新一个目标文稿"))
	}
	op := patch.Operations[0]
	if op.Kind != workspacePatchUpdate || op.MovePath != "" || normalizeScopePath(op.Path) != action.Path {
		return reject(fmt.Errorf("格式修复不允许创建、删除、移动或操作其他文件"))
	}
	files, err := s.factory.Workspace(opts.UserID)
	if err != nil {
		return reject(err)
	}
	before, err := files.ReadFile(action.Path)
	if err != nil {
		return reject(err)
	}
	next, err := applyWorkspacePatchUpdate(before.Content, op.Hunks)
	if err != nil {
		return reject(err)
	}
	candidate, err := files.StageDocumentRepair(ctx, opts.RunID, action, next)
	if err != nil {
		return reject(err)
	}
	payload := map[string]any{"operation": "document_repair_staged", "path": action.Path, "candidate_id": candidate.ID, "hash": candidate.Hash, "base_hash": candidate.BaseHash, "attempt": candidate.Attempt, "state": "staged", "changed_paths": []string{}, "next_hint": "候选已暂存，原文件未修改。界面将检查候选并展示建议。请结束，不得声称已应用。"}
	return mustJSON(payload), "修复候选已暂存，原稿未修改", "已暂存建议，待检查", action.Path, nil
}

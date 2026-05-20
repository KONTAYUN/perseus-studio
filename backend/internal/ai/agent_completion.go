package ai

import (
	"encoding/json"
	"strings"
)

func (r *agentRuntime) handleCompletionAttempt(argumentsJSON string) (string, bool, bool) {
	var attempt completionAttempt
	if err := json.Unmarshal([]byte(argumentsJSON), &attempt); err != nil {
		r.recordToolError()
		return r.toolErrorJSON("attempt_completion", "invalid attempt_completion arguments: "+err.Error()), true, false
	}
	attempt.Summary = strings.TrimSpace(attempt.Summary)
	if attempt.Summary == "" {
		r.recordToolError()
		return r.toolErrorJSON("attempt_completion", "summary is required"), true, false
	}
	if r.performedMutation {
		if len(attempt.ChangedPaths) == 0 {
			r.recordToolError()
			reason := "workspace 变更任务完成时必须在 changed_paths 中列出已验证的变更路径。"
			emitToolLifecycleEvent(r.opts, "completion_rejected", "warn", map[string]any{"reason": reason})
			return r.toolErrorJSON("attempt_completion", reason), true, false
		}
		if missing := r.completionPathsMissingMutations(attempt.ChangedPaths); len(missing) > 0 {
			r.recordToolError()
			reason := "attempt_completion 的 changed_paths 没有覆盖本轮已验证的 workspace 变更：" + strings.Join(missing, ", ")
			emitToolLifecycleEvent(r.opts, "completion_rejected", "warn", map[string]any{"reason": reason, "missing_paths": missing})
			return r.toolErrorJSON("attempt_completion", reason), true, false
		}
	}
	if rejected := r.completionRejection(); rejected != "" {
		r.recordToolError()
		emitToolLifecycleEvent(r.opts, "completion_rejected", "warn", map[string]any{"reason": rejected})
		return r.toolErrorJSON("attempt_completion", rejected), true, false
	}
	content := attempt.Summary
	if strings.TrimSpace(attempt.Notes) != "" {
		content += "\n\n" + strings.TrimSpace(attempt.Notes)
	}
	r.completedContent = content
	r.finalizeActiveAssistantMessage(content)
	emitToolLifecycleEvent(r.opts, "model_message_completed", "info", map[string]any{"content": content})
	return mustJSON(map[string]any{"summary": attempt.Summary, "changed_paths": attempt.ChangedPaths, "failed_items": attempt.FailedItems, "notes": attempt.Notes}), false, true
}

func (r *agentRuntime) completionRejection() string {
	if r.toolLoopActive && r.toolCalls == 0 {
		return "当前任务已经进入工具工作流，但还没有执行任何工具。请先调用合适的工具，再使用 attempt_completion。"
	}
	if r.performedMutation && !r.hasSuccessfulWorkspaceMutation() {
		return "workspace 变更任务尚未产生可验证的成功变更，不能完成。请继续执行或修正 workspace 工具。"
	}
	if r.lastToolFailed {
		return "上一项工具调用失败且尚未修复，不能完成。请根据 tool_result 修正参数或明确说明无法完成。"
	}
	return ""
}

func (r *agentRuntime) hasSuccessfulWorkspaceMutation() bool {
	for _, event := range r.events {
		if event.Status == "success" && isWorkspaceMutationTool(event.ToolName) && len(event.ChangedPaths) > 0 {
			return true
		}
	}
	return false
}

func (r *agentRuntime) completionPathsMissingMutations(paths []string) []string {
	declared := map[string]bool{}
	for _, path := range paths {
		normalized := normalizeWorkspaceEventPath(path)
		if normalized != "" {
			declared[normalized] = true
		}
	}
	seenMissing := map[string]bool{}
	var missing []string
	for _, event := range r.events {
		if event.Status != "success" || !isWorkspaceMutationTool(event.ToolName) {
			continue
		}
		candidates := normalizedCompletionMutationPaths(event)
		if len(candidates) == 0 || completionPathCovered(candidates, declared) {
			continue
		}
		label := candidates[0]
		if !seenMissing[label] {
			seenMissing[label] = true
			missing = append(missing, label)
		}
	}
	return missing
}

func normalizedCompletionMutationPaths(event ToolEvent) []string {
	seen := map[string]bool{}
	var paths []string
	for _, path := range append([]string{event.Path, event.NewPath, event.OldPath}, event.ChangedPaths...) {
		normalized := normalizeWorkspaceEventPath(path)
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		paths = append(paths, normalized)
	}
	return paths
}

func completionPathCovered(candidates []string, declared map[string]bool) bool {
	for _, candidate := range candidates {
		if declared[candidate] {
			return true
		}
	}
	return false
}

func normalizeWorkspaceEventPath(path string) string {
	path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	path = strings.Trim(path, "/")
	if path == "." {
		return ""
	}
	return path
}

func (r *agentRuntime) verifiedCompletionLines() []string {
	paths := r.verifiedMutationPaths()
	if len(paths) > 0 {
		return paths
	}
	seen := map[string]bool{}
	var lines []string
	for _, event := range r.events {
		if event.Status != "success" {
			continue
		}
		line := ""
		switch event.ToolName {
		case "install_skill_from_attachment":
			line = "Skill 安装流程已完成"
		case "delete_skill":
			line = "Skill 删除已完成"
		case "read_skill":
			line = "Skill 内容已读取"
		}
		if line != "" && !seen[line] {
			seen[line] = true
			lines = append(lines, line)
		}
	}
	return lines
}

func (r *agentRuntime) verifiedMutationPaths() []string {
	seen := map[string]bool{}
	var paths []string
	for _, event := range r.events {
		if event.Status != "success" || !isWorkspaceMutationTool(event.ToolName) {
			continue
		}
		for _, path := range append([]string{event.Path, event.NewPath}, event.ChangedPaths...) {
			normalized := normalizeWorkspaceEventPath(path)
			if normalized == "" || seen[normalized] {
				continue
			}
			seen[normalized] = true
			paths = append(paths, normalized)
		}
	}
	return paths
}

func prefixLines(items []string, prefix string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item) != "" {
			out = append(out, prefix+item)
		}
	}
	return out
}

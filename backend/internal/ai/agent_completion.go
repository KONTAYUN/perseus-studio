package ai

import (
	"encoding/json"
	"strings"
)

func (r *agentRuntime) handleCompletionAttempt(argumentsJSON string) (string, bool, bool) {
	var attempt completionAttempt
	if err := json.Unmarshal([]byte(argumentsJSON), &attempt); err != nil {
		r.recordToolError("attempt_completion", argumentsJSON, "invalid attempt_completion arguments: "+err.Error())
		r.discardStreamedCompletionResult()
		return r.toolErrorJSON("attempt_completion", "invalid attempt_completion arguments: "+err.Error()), true, false
	}
	attempt.Result = strings.TrimSpace(attempt.Result)
	attempt.Summary = strings.TrimSpace(attempt.Summary)
	if attempt.Result == "" {
		errText := "result is required and must contain the final user-visible answer, not only a summary"
		r.recordToolError("attempt_completion", argumentsJSON, errText)
		r.discardStreamedCompletionResult()
		return r.toolErrorJSON("attempt_completion", errText), true, false
	}
	if reason := invalidCompletionUserVisibleContent(attempt.Result, attempt.Notes); reason != "" {
		r.recordToolError("attempt_completion", argumentsJSON, reason)
		r.discardStreamedCompletionResult()
		emitToolLifecycleEvent(r.opts, "completion_rejected", "warn", map[string]any{"reason": reason})
		return r.actionRejectedObservation(reason, "attempt_completion", attempt.ChangedPaths), true, false
	}
	if r.performedMutation {
		if !r.hasSuccessfulWorkspaceMutation() {
			reason := "workspace 变更任务尚未产生后端已验证的成功变更，不能完成；attempt_completion 不能创建或修改文件。"
			r.recordToolError("attempt_completion", argumentsJSON, reason)
			r.discardStreamedCompletionResult()
			emitToolLifecycleEvent(r.opts, "completion_rejected", "warn", map[string]any{"reason": reason})
			return r.actionRejectedObservation(reason, "attempt_completion", attempt.ChangedPaths), true, false
		}
	}
	previousFailure := r.lastToolFailure
	previousFailed := r.lastToolFailed
	if rejected := r.completionRejection(attempt); rejected != "" {
		r.recordToolError("attempt_completion", argumentsJSON, rejected)
		if previousFailed && previousFailure.ToolName != "" && previousFailure.Recoverable {
			r.lastToolFailed = true
			r.lastToolFailure = previousFailure
		}
		r.discardStreamedCompletionResult()
		emitToolLifecycleEvent(r.opts, "completion_rejected", "warn", map[string]any{"reason": rejected})
		return r.actionRejectedObservation(rejected, "attempt_completion", attempt.ChangedPaths), true, false
	}
	r.clearToolErrors()
	content := renderCompletionUserVisibleContent(attempt)
	r.completedContent = content
	r.finalizeActiveAssistantMessage(content)
	emitToolLifecycleEvent(r.opts, "model_message_completed", "info", map[string]any{"content": content})
	payload := map[string]any{"result": attempt.Result, "summary": attempt.Summary, "failed_items": attempt.FailedItems, "notes": attempt.Notes}
	if changed := r.verifiedWorkspaceChangedPaths(); len(changed) > 0 {
		payload["changed_paths"] = changed
	}
	return mustJSON(payload), false, true
}

func renderCompletionUserVisibleContent(attempt completionAttempt) string {
	var parts []string
	if result := strings.TrimSpace(attempt.Result); result != "" {
		parts = append(parts, result)
	}
	if len(attempt.FailedItems) > 0 {
		parts = append(parts, "未完成项："+strings.Join(attempt.FailedItems, "、"))
	}
	if notes := strings.TrimSpace(attempt.Notes); notes != "" {
		parts = append(parts, notes)
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func invalidCompletionUserVisibleContent(result, notes string) string {
	content := strings.TrimSpace(result)
	if strings.TrimSpace(notes) != "" {
		content += "\n\n" + strings.TrimSpace(notes)
	}
	if content == "" {
		return ""
	}
	if marker := firstInternalCompletionMarker(content); marker != "" {
		return "attempt_completion.result 必须是最终用户可见答复，不能包含内部编辑协议或工具名：" + marker + "。如果还需要编辑文件，请先执行真实 workspace 编辑动作；如果任务已完成，请改写成面向用户的结果说明。"
	}
	return ""
}

func firstInternalCompletionMarker(content string) string {
	for _, marker := range []string{
		"*** Begin Patch",
		"*** End Patch",
		"assistant_text_patch",
		"apply_workspace_patch",
		"save_workspace_file",
		"write_workspace_chunk",
		"finalize_workspace_write",
		"edit_workspace_text",
		"replace_workspace_lines",
		"补丁块",
		"补丁协议",
		"补丁编辑协议",
		"分块写入",
	} {
		if strings.Contains(content, marker) {
			return marker
		}
	}
	return ""
}

func (r *agentRuntime) discardStreamedCompletionResult() {
	if r != nil && r.opts.OnCompletionResultDiscard != nil {
		r.opts.OnCompletionResultDiscard()
	}
}

func (r *agentRuntime) completionRejection(attempt completionAttempt) string {
	if r.toolLoopActive && r.toolCalls == 0 {
		return "当前任务已经进入工具工作流，但还没有执行任何工具。请先调用合适的工具，再使用 attempt_completion。"
	}
	if unverified := r.completionPathsWithoutVerifiedMutation(attempt.ChangedPaths); len(unverified) > 0 {
		return "attempt_completion 声明了未被真实 workspace 变更验证的 changed_paths：" + strings.Join(unverified, ", ") + "。请先执行真实 workspace 编辑动作；attempt_completion 不能创建或修改文件。"
	}
	if r.performedMutation && !r.hasSuccessfulWorkspaceMutation() {
		return "workspace 变更任务尚未产生可验证的成功变更，不能完成。请继续执行或修正 workspace 工具。"
	}
	if r.lastToolFailed {
		if r.lastToolFailure.ToolName == "attempt_completion" {
			if attemptReportsFailedWork(attempt) {
				return "attempt_completion 正在报告未完成的 workspace 写入/修改，但本轮没有对应的工具失败结果。请继续执行真实编辑动作，或在确有工具失败后再总结缺失项。"
			}
			return ""
		}
		if r.lastToolFailure.Recoverable {
			detail := r.lastToolFailure.ToolName
			if r.lastToolFailure.Target != "" {
				detail += " (" + r.lastToolFailure.Target + ")"
			}
			return "上一项可恢复工具调用失败且尚未修复，不能完成：" + detail + "。请根据 tool_result 修正参数并继续执行。"
		}
		if len(attempt.FailedItems) > 0 && len(attempt.ChangedPaths) == 0 {
			return ""
		}
		return "上一项工具调用失败且尚未修复，不能完成。请根据 tool_result 修正参数或明确说明无法完成。"
	}
	if attemptReportsFailedWork(attempt) {
		return "attempt_completion 正在报告未完成的 workspace 写入/修改，但本轮没有对应的工具失败结果。请继续执行真实编辑动作，或在确有工具失败后再总结缺失项。"
	}
	return ""
}

func attemptReportsFailedWork(attempt completionAttempt) bool {
	if len(attempt.FailedItems) > 0 {
		return true
	}
	return completionResultContradictsFinality(attempt.Result, attempt.Notes)
}

func completionResultContradictsFinality(result, notes string) bool {
	content := strings.ToLower(strings.TrimSpace(result + "\n" + notes))
	if content == "" {
		return false
	}
	if containsAny(content, []string{
		"workspace",
		"文件",
		"写入",
		"保存",
		"创建",
		"修改",
		"变更",
		"file",
		"write",
		"written",
		"save",
		"saved",
		"create",
		"created",
		"edit",
		"edited",
		"modify",
		"modified",
	}) && containsAny(content, []string{
		"未能",
		"没能",
		"没有成功",
		"尚未",
		"还没有",
		"不能声称",
		"无法完成",
		"could not",
		"couldn't",
		"unable to",
		"not ",
		"failed",
	}) {
		return true
	}
	return false
}

func containsAny(content string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(content, needle) {
			return true
		}
	}
	return false
}

func (r *agentRuntime) hasSuccessfulWorkspaceMutation() bool {
	for _, event := range r.events {
		if event.Status == "success" && eventCountsAsWorkspaceMutation(event) && len(event.ChangedPaths) > 0 {
			return true
		}
	}
	return false
}

func (r *agentRuntime) completionPathsWithoutVerifiedMutation(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	verified := map[string]bool{}
	for _, event := range r.events {
		if event.Status != "success" || !eventCountsAsWorkspaceMutation(event) {
			continue
		}
		for _, path := range normalizedCompletionMutationPaths(event) {
			verified[path] = true
		}
	}
	seen := map[string]bool{}
	var missing []string
	for _, path := range paths {
		normalized := normalizeWorkspaceEventPath(path)
		if normalized == "" || verified[normalized] || seen[normalized] {
			continue
		}
		seen[normalized] = true
		missing = append(missing, normalized)
	}
	return missing
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
		if event.Status != "success" || !eventCountsAsWorkspaceMutation(event) {
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

func eventCountsAsWorkspaceMutation(event ToolEvent) bool {
	if isWorkspaceMutationTool(event.ToolName) {
		return true
	}
	return event.ToolName == "run_workspace_command" && len(event.ChangedPaths) > 0
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
		if event.Status != "success" || !eventCountsAsWorkspaceMutation(event) {
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

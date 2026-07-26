package ai

import (
	"encoding/json"
	"strings"
)

func (r *agentRuntime) handleCompletionAttempt(argumentsJSON string) (string, bool, bool) {
	var attempt completionAttempt
	if err := json.Unmarshal([]byte(argumentsJSON), &attempt); err != nil {
		r.recordToolError("attempt_completion", argumentsJSON, "invalid attempt_completion arguments: "+err.Error())
		return r.toolErrorJSON("attempt_completion", "invalid attempt_completion arguments: "+err.Error()), true, false
	}
	attempt.Result = strings.TrimSpace(attempt.Result)
	attempt.Summary = strings.TrimSpace(attempt.Summary)
	if attempt.Result == "" {
		errText := "result is required and must contain the final user-visible answer, not only a summary"
		r.recordToolError("attempt_completion", argumentsJSON, errText)
		return r.toolErrorJSON("attempt_completion", errText), true, false
	}
	if completionResultIsWorkspacePatch(attempt.Result) {
		reason := "attempt_completion.result must be a user-visible Markdown response, not a workspace patch. Call apply_workspace_patch with the patch argument, verify the result, then complete the task."
		r.recordToolError("attempt_completion", argumentsJSON, reason)
		emitToolLifecycleEvent(r.opts, "completion_rejected", "warn", map[string]any{"reason": reason})
		return r.actionRejectedObservation(reason, "attempt_completion", attempt.ChangedPaths), true, false
	}
	if r.performedMutation {
		if !r.hasSuccessfulWorkspaceMutation() {
			reason := "workspace 变更任务尚未产生后端已验证的成功变更，不能完成；attempt_completion 不能创建或修改文件。"
			r.recordToolError("attempt_completion", argumentsJSON, reason)
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
		emitToolLifecycleEvent(r.opts, "completion_rejected", "warn", map[string]any{"reason": rejected})
		return r.actionRejectedObservation(rejected, "attempt_completion", attempt.ChangedPaths), true, false
	}
	r.clearToolErrors()
	content := renderCompletionUserVisibleContent(attempt)
	r.completedContent = content
	r.terminalState = AgentTerminationCompleted
	r.appendKernelItem(AgentTurnItem{
		Kind:         "final_message",
		Phase:        "final",
		Status:       "completed",
		Content:      truncatePlainRunes(content, 2000),
		ChangedPaths: r.verifiedWorkspaceChangedPaths(),
	})
	r.finalizeActiveAssistantMessage(content)
	emitToolLifecycleEvent(r.opts, "model_message_completed", "info", map[string]any{"content": content})
	payload := map[string]any{"result": attempt.Result, "summary": attempt.Summary, "failed_items": attempt.FailedItems, "notes": attempt.Notes}
	if changed := r.verifiedWorkspaceChangedPaths(); len(changed) > 0 {
		payload["changed_paths"] = changed
	}
	return mustJSON(payload), false, true
}

func completionResultIsWorkspacePatch(result string) bool {
	result = strings.TrimSpace(result)
	if !strings.HasPrefix(result, "*** Begin Patch") || !strings.HasSuffix(result, "*** End Patch") {
		return false
	}
	normalized, err := normalizeWorkspacePatchEnvelope(result)
	if err != nil {
		return false
	}
	_, err = parseWorkspacePatch(normalized)
	return err == nil
}

func renderCompletionUserVisibleContent(attempt completionAttempt) string {
	return strings.TrimSpace(attempt.Result)
}

func (r *agentRuntime) completionRejection(attempt completionAttempt) string {
	if unverified := r.completionPathsWithoutVerifiedMutation(attempt.ChangedPaths); len(unverified) > 0 {
		return "attempt_completion 声明了未被真实 workspace 变更验证的 changed_paths：" + strings.Join(unverified, ", ") + "。请先执行真实 workspace 编辑动作；attempt_completion 不能创建或修改文件。"
	}
	if r.performedMutation && !r.hasSuccessfulWorkspaceMutation() {
		return "workspace 变更任务尚未产生可验证的成功变更，不能完成。请继续执行或修正 workspace 工具。"
	}
	if r.lastToolFailed {
		if r.lastToolFailure.ToolName == "attempt_completion" {
			return ""
		}
		if !r.lastToolFailure.Recoverable {
			return ""
		}
		if r.lastFailureConditionSatisfied() {
			return ""
		}
		detail := r.lastToolFailure.ToolName
		if r.lastToolFailure.Target != "" {
			detail += " (" + r.lastToolFailure.Target + ")"
		}
		return "The current turn still has an unresolved failed tool call: " + detail + ". Resolve it or choose another valid action before completing."
	}
	return ""
}

func (r *agentRuntime) hasPrefetchedWorkspaceFileContent() bool {
	if r == nil {
		return false
	}
	for _, attachment := range r.attachments {
		if attachment.Kind == "workspace_file" && attachment.IncludedInContext && strings.TrimSpace(attachment.Text) != "" {
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

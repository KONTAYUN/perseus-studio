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
	r.completedOutcome = completionOutcome(attempt)
	r.finalizeActiveAssistantMessage(content)
	emitToolLifecycleEvent(r.opts, "model_message_completed", "info", map[string]any{"content": content})
	payload := map[string]any{"result": attempt.Result, "summary": attempt.Summary, "failed_items": attempt.FailedItems, "notes": attempt.Notes, "outcome": r.completedOutcome}
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

// Display text is not evidence of actions or completion.
func invalidCompletionUserVisibleContent(result, notes string) string { return "" }

func (r *agentRuntime) discardStreamedCompletionResult() {
	if r != nil && r.opts.OnCompletionResultDiscard != nil {
		r.opts.OnCompletionResultDiscard()
	}
}

func (r *agentRuntime) completionRejection(attempt completionAttempt) string {
	outcome := completionOutcome(attempt)
	if outcome != "success" && outcome != "partial" && outcome != "blocked" {
		return "outcome must be success, partial, or blocked"
	}
	if unverified := r.completionPathsWithoutVerifiedMutation(attempt.ChangedPaths); len(unverified) > 0 {
		return "changed_paths contains unverified workspace changes: " + strings.Join(unverified, ", ")
	}
	if outcome != "success" {
		return ""
	}
	if len(attempt.FailedItems) > 0 {
		return "success cannot include failed_items; use partial or blocked"
	}
	if len(r.unresolvedCommands) > 0 {
		return "A verification command still has an unresolved execution outcome; rerun it successfully or report partial/blocked"
	}
	if r.toolLoopActive && r.toolCalls == 0 && !r.hasPrefetchedWorkspaceFileContent() {
		return "当前任务已进入工具工作流，但还没有执行任何工具；请执行工具，或用 blocked 如实结束。"
	}
	if r.performedMutation && !r.hasSuccessfulWorkspaceMutation() {
		return "workspace 变更尚未验证；不能以 success 完成，请核验或如实报告 partial/blocked。"
	}
	if r.lastToolFailed && r.lastToolFailure.ToolName != "attempt_completion" && !r.lastFailureConditionSatisfied() {
		return "上一工具失败尚未解决；请修正工具或用 partial/blocked 如实报告。"
	}
	return ""
}

func completionOutcome(attempt completionAttempt) string {
	if attempt.Outcome != "" {
		return attempt.Outcome
	}
	if len(attempt.FailedItems) > 0 {
		return "partial"
	}
	return "success"
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

package ai

import (
	"encoding/json"
	"fmt"
	"strings"

	"webide-backend/internal/store"
)

func serializeContextItems(protocol agentProtocol, items []store.AgentTurnItem, attachments []AttachmentPayload, workingSet string) ContextSnapshot {
	if protocol == agentProtocolAnthropic {
		return serializeAnthropicContextItems(items, attachments, workingSet)
	}
	return serializeOpenAIContextItems(items, attachments, workingSet)
}

func serializeOpenAIContextItems(items []store.AgentTurnItem, attachments []AttachmentPayload, workingSet string) ContextSnapshot {
	messages := []map[string]any{{"role": "system", "content": workspaceToolSystemPrompt}}
	if strings.TrimSpace(workingSet) != "" {
		messages = append(messages, map[string]any{"role": "system", "name": "perseus_task_working_set", "content": workingSet})
	}
	lastUserSourceID := latestUserSourceID(items)
	latestTaskPlanSeq := latestContextItemSeq(items, "task_plan")
	environments := contextEnvironmentsByTurn(items)
	for _, item := range items {
		payload := decodeContextItemPayload(item)
		switch item.Kind {
		case "user_message":
			if environment := environments[item.TurnID]; environment != "" {
				messages = append(messages, map[string]any{"role": "system", "name": "perseus_editor_environment", "content": environment})
			}
			content := any(contextPayloadString(payload, "content"))
			messageAttachments := attachmentsForContextMessage(attachments, item.SourceID, lastUserSourceID)
			if len(messageAttachments) > 0 {
				content = openAIUserContentWithAttachments(contextPayloadString(payload, "content"), messageAttachments)
			}
			messages = append(messages, map[string]any{"role": "user", "content": content})
		case "assistant_message":
			if content := contextPayloadString(payload, "content"); content != "" {
				if item.Status == "rejected" {
					messages = append(messages, map[string]any{
						"role": "system", "name": "perseus_rejected_assistant_action",
						"content": "Rejected assistant output retained for protocol correction:\n" + content,
					})
				} else {
					messages = append(messages, map[string]any{"role": "assistant", "content": content})
				}
			}
		case "protocol_feedback":
			if content := contextPayloadString(payload, "content"); content != "" {
				messages = append(messages, map[string]any{
					"role": "system", "name": "perseus_agent_protocol", "content": content,
				})
			}
		case "tool_call":
			id := contextItemToolCallID(item)
			name := contextItemToolName(item)
			if id == "" || name == "" {
				continue
			}
			messages = append(messages, map[string]any{
				"role": "assistant", "content": nil,
				"tool_calls": []map[string]any{{
					"id": id, "type": "function",
					"function": map[string]any{"name": name, "arguments": contextItemArgumentsJSON(item)},
				}},
			})
		case "tool_result", "aborted":
			id := contextItemToolCallID(item)
			if id == "" {
				continue
			}
			messages = append(messages, map[string]any{
				"role": "tool", "tool_call_id": id, "content": contextItemContent(item),
			})
		case "environment_snapshot":
			continue
		case "workspace_observation":
			if content := contextPayloadString(payload, "content"); content != "" {
				messages = append(messages, map[string]any{"role": "system", "name": "perseus_runtime_observation", "content": content})
			}
		case "task_plan":
			if item.Seq == latestTaskPlanSeq {
				messages = append(messages, map[string]any{
					"role": "system", "name": "perseus_task_plan",
					"content": taskPlanContextText(contextItemContent(item)),
				})
			}
		case "external_observation":
			if content := contextItemContent(item); content != "" {
				messages = append(messages, map[string]any{
					"role": "system", "name": "perseus_external_observation",
					"content": externalObservationContextText(content),
				})
			}
		case "file_change":
			if content := localFileChangeContextText(payload); content != "" {
				messages = append(messages, map[string]any{
					"role": "system", "name": "perseus_local_file_change", "content": content,
				})
			}
		case "compaction":
			if summary := contextPayloadString(payload, "summary", "content"); summary != "" {
				messages = append(messages, map[string]any{"role": "system", "name": "perseus_context_summary", "content": contextCompactionText(summary)})
			}
		}
	}
	return ContextSnapshot{OpenAIMessages: messages}
}

func serializeAnthropicContextItems(items []store.AgentTurnItem, attachments []AttachmentPayload, workingSet string) ContextSnapshot {
	system := workspaceToolSystemPrompt
	if strings.TrimSpace(workingSet) != "" {
		system = strings.TrimSpace(system + "\n\n" + workingSet)
	}
	var messages []map[string]any
	lastUserSourceID := latestUserSourceID(items)
	latestTaskPlanSeq := latestContextItemSeq(items, "task_plan")
	environments := contextEnvironmentsByTurn(items)
	for _, item := range items {
		payload := decodeContextItemPayload(item)
		switch item.Kind {
		case "user_message":
			if environment := environments[item.TurnID]; environment != "" {
				messages = appendAnthropicContextMessage(messages, "user", []map[string]any{{
					"type": "text", "text": "Editor environment for this turn (paths only; not user-authored content):\n" + environment,
				}})
			}
			blocks := []map[string]any{{"type": "text", "text": contextPayloadString(payload, "content")}}
			messageAttachments := attachmentsForContextMessage(attachments, item.SourceID, lastUserSourceID)
			if len(messageAttachments) > 0 {
				blocks = anthropicUserContentWithAttachments(contextPayloadString(payload, "content"), messageAttachments)
			}
			messages = appendAnthropicContextMessage(messages, "user", blocks)
		case "assistant_message":
			if content := contextPayloadString(payload, "content"); content != "" {
				if item.Status == "rejected" {
					system = strings.TrimSpace(system + "\n\nRejected assistant output retained for protocol correction:\n" + content)
				} else {
					messages = appendAnthropicContextMessage(messages, "assistant", []map[string]any{{"type": "text", "text": content}})
				}
			}
		case "protocol_feedback":
			if content := contextPayloadString(payload, "content"); content != "" {
				system = strings.TrimSpace(system + "\n\n" + content)
			}
		case "tool_call":
			id := contextItemToolCallID(item)
			name := contextItemToolName(item)
			if id == "" || name == "" {
				continue
			}
			input := map[string]any{}
			_ = json.Unmarshal([]byte(contextItemArgumentsJSON(item)), &input)
			messages = appendAnthropicContextMessage(messages, "assistant", []map[string]any{{
				"type": "tool_use", "id": id, "name": name, "input": input,
			}})
		case "tool_result", "aborted":
			id := contextItemToolCallID(item)
			if id == "" {
				continue
			}
			block := map[string]any{"type": "tool_result", "tool_use_id": id, "content": contextItemContent(item)}
			if item.Kind == "aborted" || item.Status == "failed" || item.Status == "error" {
				block["is_error"] = true
			}
			messages = appendAnthropicContextMessage(messages, "user", []map[string]any{block})
		case "environment_snapshot":
			continue
		case "workspace_observation":
			if content := contextPayloadString(payload, "content"); content != "" {
				messages = appendAnthropicContextMessage(messages, "user", []map[string]any{{"type": "text", "text": content}})
			}
		case "task_plan":
			if item.Seq == latestTaskPlanSeq {
				system = strings.TrimSpace(system + "\n\n" + taskPlanContextText(contextItemContent(item)))
			}
		case "external_observation":
			if content := contextItemContent(item); content != "" {
				system = strings.TrimSpace(system + "\n\n" + externalObservationContextText(content))
			}
		case "file_change":
			if content := localFileChangeContextText(payload); content != "" {
				system = strings.TrimSpace(system + "\n\n" + content)
			}
		case "compaction":
			if summary := contextPayloadString(payload, "summary", "content"); summary != "" {
				messages = appendAnthropicContextMessage(messages, "user", []map[string]any{{"type": "text", "text": contextCompactionText(summary)}})
			}
		}
	}
	return ContextSnapshot{AnthropicSystem: system, AnthropicMessages: messages}
}

func contextEnvironmentsByTurn(items []store.AgentTurnItem) map[string]string {
	result := map[string]string{}
	for _, item := range items {
		if item.Kind != "environment_snapshot" || item.TurnID == "" {
			continue
		}
		if content := environmentSnapshotText(decodeContextItemPayload(item)); content != "" {
			result[item.TurnID] = content
		}
	}
	return result
}

func appendAnthropicContextMessage(messages []map[string]any, role string, blocks []map[string]any) []map[string]any {
	if len(blocks) == 0 {
		return messages
	}
	if len(messages) > 0 && messages[len(messages)-1]["role"] == role {
		current, _ := messages[len(messages)-1]["content"].([]map[string]any)
		messages[len(messages)-1]["content"] = append(current, blocks...)
		return messages
	}
	return append(messages, map[string]any{"role": role, "content": blocks})
}

func latestUserSourceID(items []store.AgentTurnItem) string {
	for index := len(items) - 1; index >= 0; index-- {
		if items[index].Kind == "user_message" {
			return items[index].SourceID
		}
	}
	return ""
}

func attachmentsForContextMessage(attachments []AttachmentPayload, sourceID, latestUserSourceID string) []AttachmentPayload {
	result := make([]AttachmentPayload, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment.MessageID == sourceID || (attachment.MessageID == "" && sourceID == latestUserSourceID) {
			result = append(result, attachment)
		}
	}
	return result
}

func explicitModelAttachments(attachments []AttachmentPayload) []AttachmentPayload {
	filtered := make([]AttachmentPayload, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment.Kind == "current_file" || attachment.Kind == "editor_panels_context" {
			continue
		}
		filtered = append(filtered, attachment)
	}
	return filtered
}

func openAIUserContentWithAttachments(content string, attachments []AttachmentPayload) any {
	attachments = explicitModelAttachments(attachments)
	if len(attachments) == 0 {
		return content
	}
	parts := []map[string]any{{"type": "text", "text": buildUserTextWithAttachments(content, attachments)}}
	for _, attachment := range attachments {
		if attachment.Kind == "image" && attachment.DataBase64 != "" {
			parts = append(parts, map[string]any{
				"type":      "image_url",
				"image_url": map[string]any{"url": fmt.Sprintf("data:%s;base64,%s", attachment.MIMEType, attachment.DataBase64)},
			})
		}
	}
	return parts
}

func anthropicUserContentWithAttachments(content string, attachments []AttachmentPayload) []map[string]any {
	attachments = explicitModelAttachments(attachments)
	blocks := []map[string]any{{"type": "text", "text": buildUserTextWithAttachments(content, attachments)}}
	for _, attachment := range attachments {
		if attachment.Kind == "image" && attachment.DataBase64 != "" {
			blocks = append(blocks, map[string]any{
				"type":   "image",
				"source": map[string]any{"type": "base64", "media_type": attachment.MIMEType, "data": attachment.DataBase64},
			})
		}
	}
	return blocks
}

func environmentSnapshotText(payload contextItemPayload) string {
	paths := []string{contextPayloadString(payload, "active_file")}
	panels, _ := payload["panels"].([]any)
	for _, rawPanel := range panels {
		panel, _ := rawPanel.(map[string]any)
		paths = append(paths, stringFromAny(panel["active_file_path"]))
		paths = append(paths, stringSliceFromAnyLoose(panel["open_tabs"])...)
	}
	paths = normalizedProviderPaths(paths)
	if len(paths) == 0 {
		return ""
	}
	return "Local Perseus editor environment for this turn (path metadata only; no file content is included. Open paths do not alter task scope. Read a path with workspace tools when the task depends on its content):\n- " + strings.Join(paths, "\n- ")
}

func latestContextItemSeq(items []store.AgentTurnItem, kind string) int64 {
	for index := len(items) - 1; index >= 0; index-- {
		if items[index].Kind == kind {
			return items[index].Seq
		}
	}
	return 0
}

func taskPlanContextText(content string) string {
	return "Durable task plan. Keep local workspace targets separate from external systems. Editor tab changes do not replace this task scope:\n" + strings.TrimSpace(content)
}

func externalObservationContextText(content string) string {
	return "External evidence supplied or verified for this task. It does not change the Perseus workspace root, and local workspace files must not be searched for on the external system unless the user explicitly says they exist there:\n" + strings.TrimSpace(content)
}

func localFileChangeContextText(payload contextItemPayload) string {
	paths := normalizedProviderPaths(append(
		stringSliceFromAnyLoose(payload["changed_paths"]),
		contextPayloadString(payload, "path", "new_path", "old_path"),
	))
	if len(paths) == 0 {
		return ""
	}
	text := "Verified local Perseus workspace file change:\n- " + strings.Join(paths, "\n- ")
	if hash := contextPayloadString(payload, "hash", "new_hash"); hash != "" {
		text += "\nCurrent hash: " + hash
	}
	return text
}

func contextCompactionText(summary string) string {
	return "Conversation memory summary replacing earlier turn items. Preserve these facts while using workspace tools for current file content:\n\n" + strings.TrimSpace(summary)
}

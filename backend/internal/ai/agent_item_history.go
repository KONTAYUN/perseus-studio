package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"webide-backend/internal/store"
)

func (s *Service) prepareAgentTurnHistory(ctx context.Context, conversationID, messageID string, attachments []AttachmentPayload, opts RunOptions) ([]store.AgentTurnItem, error) {
	if s == nil || s.calls == nil {
		return nil, fmt.Errorf("agent turn item store is unavailable")
	}
	if err := s.calls.EnsureAgentTurnItems(ctx, opts.UserID, conversationID); err != nil {
		return nil, err
	}
	turnID := firstNonEmpty(messageID, opts.RunID)
	if turnID == "" {
		return nil, fmt.Errorf("turn id is required")
	}
	if err := s.persistCurrentUserTurn(ctx, conversationID, messageID, turnID, opts); err != nil {
		return nil, err
	}
	if err := s.persistTurnEnvironment(ctx, conversationID, messageID, turnID, attachments, opts); err != nil {
		return nil, err
	}
	repair, err := s.calls.RepairAgentTurnItemHistory(ctx, opts.UserID, conversationID)
	if err != nil {
		emitHistoryIntegrityError(opts, conversationID, err)
		return nil, err
	}
	if repair.Inserted > 0 || repair.Reconciled > 0 || repair.Conflicts > 0 {
		emitToolLifecycleEvent(opts, "agent_history_repaired", "info", map[string]any{
			"scanned": repair.Scanned, "inserted": repair.Inserted,
			"reconciled": repair.Reconciled, "conflicts": repair.Conflicts,
		})
	}
	if _, err := s.calls.RepairDanglingAgentToolCalls(ctx, opts.UserID, conversationID); err != nil {
		return nil, err
	}
	return s.calls.ListAgentTurnItems(ctx, opts.UserID, conversationID)
}

func (s *Service) persistCurrentUserTurn(ctx context.Context, conversationID, messageID, turnID string, opts RunOptions) error {
	message, err := s.calls.GetMessage(ctx, opts.UserID, messageID)
	if err != nil {
		return err
	}
	if message.ConversationID != conversationID || message.Role != "user" {
		return fmt.Errorf("turn source message is not the current conversation user message")
	}
	payloadValue := map[string]any{"content": message.Content}
	if attachments := canonicalMessageAttachments(message.AttachmentsJSON); len(attachments) > 0 {
		payloadValue["attachments"] = attachments
	}
	payload := mustJSON(payloadValue)
	_, err = s.calls.AppendAgentTurnItem(ctx, store.AgentTurnItemInput{
		UserID: opts.UserID, ConversationID: conversationID, RunID: opts.RunID, TurnID: turnID,
		Kind: "user_message", Role: "user", Status: "completed",
		Visibility: store.AgentItemVisibilityModel, SourceID: message.ID,
		PayloadJSON: payload, CreatedAt: message.CreatedAt,
	})
	return err
}

func canonicalMessageAttachments(raw string) []map[string]any {
	var values []map[string]any
	if json.Unmarshal([]byte(raw), &values) != nil {
		return nil
	}
	filtered := make([]map[string]any, 0, len(values))
	for _, value := range values {
		kind, _ := value["kind"].(string)
		if kind == "current_file" || kind == "editor_panels_context" {
			continue
		}
		filtered = append(filtered, value)
	}
	return filtered
}

func (s *Service) persistTurnEnvironment(ctx context.Context, conversationID, messageID, turnID string, attachments []AttachmentPayload, opts RunOptions) error {
	var panels []EditorPanelContext
	for _, attachment := range attachments {
		switch attachment.Kind {
		case "editor_panels_context":
			panels = append(panels, attachment.EditorPanels...)
		case "current_file":
			if strings.TrimSpace(attachment.Path) != "" {
				panels = append(panels, EditorPanelContext{
					PanelID: "current", Index: 0, PositionLabel: "active",
					ActiveFilePath: attachment.Path, ActiveFileName: attachment.Name,
					OpenTabs: []string{attachment.Path},
				})
			}
		}
	}
	if len(panels) > 0 {
		payload := mustJSON(map[string]any{
			"active_file": activeEditorFilePath(panels),
			"panels":      panels,
		})
		if _, err := s.calls.AppendAgentTurnItem(ctx, store.AgentTurnItemInput{
			UserID: opts.UserID, ConversationID: conversationID, RunID: opts.RunID, TurnID: turnID,
			Kind: "environment_snapshot", Role: "system", Status: "completed",
			Visibility: store.AgentItemVisibilityModel, SourceID: messageID + ":environment",
			PayloadJSON: payload, CreatedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
	}

	for index, attachment := range attachments {
		if attachment.Kind != "workspace_file" && attachment.Kind != "file" && attachment.Kind != "text" {
			continue
		}
		payload := map[string]any{
			"path": attachment.Path, "name": attachment.Name, "file_type": attachment.FileType,
			"source": "user_reference", "content_hash": "", "view": "attachment",
		}
		if attachment.Text != "" {
			payload["content"] = attachment.Text
			payload["content_state"] = "available"
		} else {
			payload["content_state"] = "metadata_only"
		}
		raw, _ := json.Marshal(payload)
		sourceID := fmt.Sprintf("%s:attachment:%d:%s", messageID, index, attachment.Path)
		if _, err := s.calls.AppendAgentTurnItem(ctx, store.AgentTurnItemInput{
			UserID: opts.UserID, ConversationID: conversationID, RunID: opts.RunID, TurnID: turnID,
			Kind: "workspace_observation", Role: "system", Status: "completed",
			Visibility: store.AgentItemVisibilityModel, SourceID: sourceID,
			PayloadJSON: string(raw), CreatedAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
	}
	return nil
}

func activeEditorFilePath(panels []EditorPanelContext) string {
	for _, panel := range panels {
		if strings.TrimSpace(panel.ActiveFilePath) != "" {
			return panel.ActiveFilePath
		}
	}
	return ""
}

func (r *agentRuntime) persistKernelTurnItem(item AgentTurnItem) {
	if r == nil || r.service == nil || r.service.calls == nil || r.contextPersistenceErr != nil {
		return
	}
	payload := map[string]any{
		"phase": item.Phase, "round": item.Round, "tool_call_id": item.ToolCallID,
		"tool_name": item.ToolName, "target": item.Target, "ledger_id": item.LedgerID,
		"content": item.Content, "error": item.Error, "changed_paths": item.ChangedPaths,
		"path": item.Path, "hash": item.Hash, "view": item.View, "mode": item.Mode,
		"block_id": item.BlockID, "section_id": item.SectionID, "doc_hash": item.DocHash,
	}
	raw, _ := json.Marshal(payload)
	role := "system"
	switch item.Kind {
	case "assistant_message", "final_message", "tool_call":
		role = "assistant"
	case "tool_result", "aborted":
		role = "tool"
	}
	kind := item.Kind
	if kind == "final_message" {
		kind = "assistant_message"
	}
	sourceID := kernelItemSourceID(r.opts.RunID, item)
	_, err := r.service.calls.AppendAgentTurnItem(context.Background(), store.AgentTurnItemInput{
		ID: item.ID, UserID: r.opts.UserID, ConversationID: r.conversationID,
		RunID: r.opts.RunID, TurnID: item.TurnID, Kind: kind, Role: role,
		Status: item.Status, Visibility: store.AgentItemVisibilityModel,
		SourceID: sourceID, PayloadJSON: string(raw), CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		r.contextPersistenceErr = err
		log.Printf("[ai] persist agent turn item failed conversation=%s run=%s kind=%s error=%v", r.conversationID, r.opts.RunID, item.Kind, err)
		emitToolLifecycleEvent(r.opts, "agent_item_persistence_failed", "error", map[string]any{
			"kind": item.Kind, "item_id": item.ID, "error": err.Error(),
		})
		emitHistoryIntegrityError(r.opts, r.conversationID, err)
	}
}

func emitHistoryIntegrityError(opts RunOptions, conversationID string, err error) {
	if err == nil {
		return
	}
	var conflict *store.AgentTurnItemConflictError
	if !errors.As(err, &conflict) {
		return
	}
	emitToolLifecycleEvent(opts, "history_integrity_error", "error", map[string]any{
		"conversation_id":  conversationID,
		"existing_item_id": conflict.Existing.ID,
		"source_id":        conflict.Input.SourceID,
		"kind":             conflict.Input.Kind,
		"error":            err.Error(),
	})
}

func kernelItemSourceID(runID string, item AgentTurnItem) string {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return item.ID
	}
	switch item.Kind {
	case "final_message":
		return runID + ":final"
	case "tool_call", "tool_result", "workspace_observation", "file_change", "aborted", "task_plan", "external_observation", "protocol_feedback":
		if strings.TrimSpace(item.ToolCallID) != "" {
			return runID + ":" + item.Kind + ":" + item.ToolCallID
		}
	}
	return item.ID
}

func (r *agentRuntime) loadPersistentContextItems(ctx context.Context) ([]store.AgentTurnItem, error) {
	if r == nil || r.service == nil || r.service.calls == nil {
		return nil, fmt.Errorf("agent turn item store is unavailable")
	}
	if r.contextPersistenceErr != nil {
		return nil, r.contextPersistenceErr
	}
	return r.service.calls.ListAgentTurnItems(ctx, r.opts.UserID, r.conversationID)
}

func (r *agentRuntime) currentUserMessageContent() string {
	for index := len(r.contextItems) - 1; index >= 0; index-- {
		if r.contextItems[index].Kind == "user_message" {
			return contextItemContent(r.contextItems[index])
		}
	}
	return ""
}

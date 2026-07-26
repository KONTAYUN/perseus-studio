package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"webide-backend/internal/store"
)

type pendingSteer struct {
	SteerID       string
	ClientSteerID string
	Content       string
	Preview       string
	Seq           int64
	ReceivedAt    time.Time
	MessageID     string
}

func (r *agentRuntime) applyPendingSteers(ctx context.Context, round int) error {
	if r == nil || r.service == nil || r.service.runs == nil || r.opts.UserID == "" || r.opts.RunID == "" {
		return nil
	}
	items, err := pendingSteersForRun(ctx, r.service.runs, r.opts.UserID, r.opts.RunID)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}
	for _, item := range items {
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		var steerMessage store.Message
		appliedAt := time.Now().UTC()
		if item.MessageID == "" && r.service.calls != nil {
			messageID, err := findStoredSteerMessageID(ctx, r.service.calls, r.opts.UserID, r.conversationID, item)
			if err != nil {
				return err
			}
			if messageID == "" {
				message, err := r.service.calls.AppendMessageWithMeta(ctx, store.MessageInput{
					UserID:          r.opts.UserID,
					ConversationID:  r.conversationID,
					Role:            "user",
					Content:         content,
					AttachmentsJSON: "[]",
					Status:          "final",
					MetaJSON:        store.SteerMessageMetaJSON(item.SteerID, item.ClientSteerID, r.opts.RunID, "applied", item.Seq, item.ReceivedAt, appliedAt),
					CreatedAt:       item.ReceivedAt,
				})
				if err != nil {
					return err
				}
				messageID = message.ID
				steerMessage = message
			}
			item.MessageID = messageID
		}
		if item.MessageID != "" && steerMessage.ID == "" && r.service.calls != nil {
			message, err := r.service.calls.PatchMessageMeta(ctx, r.opts.UserID, item.MessageID, store.SteerMessageMetaJSON(item.SteerID, item.ClientSteerID, r.opts.RunID, "applied", item.Seq, item.ReceivedAt, appliedAt), "final")
			if err != nil {
				return err
			}
			steerMessage = message
		}
		steerTurnID := firstNonEmpty(steerMessage.ID, item.MessageID, item.SteerID, item.ClientSteerID)
		if steerTurnID == "" {
			return fmt.Errorf("applied user guidance is missing a stable turn id")
		}
		if _, err := r.service.calls.AppendAgentTurnItem(ctx, store.AgentTurnItemInput{
			UserID: r.opts.UserID, ConversationID: r.conversationID, RunID: r.opts.RunID,
			TurnID: steerTurnID, Kind: "user_message", Role: "user", Status: "completed",
			Visibility: store.AgentItemVisibilityModel, SourceID: firstNonEmpty(steerMessage.ID, item.MessageID, "steer:"+steerTurnID),
			PayloadJSON: mustJSON(map[string]any{
				"content": formatSteerTranscriptText(content), "steer_id": item.SteerID,
				"client_steer_id": item.ClientSteerID, "received_seq": item.Seq,
			}),
			CreatedAt: item.ReceivedAt,
		}); err != nil {
			return err
		}
		r.ensureKernelState()
		r.kernel.TurnID = steerTurnID
		r.transcript.AppendUserText(formatSteerTranscriptText(content), map[string]any{
			"reason":           "runtime_user_steer",
			"steer_id":         item.SteerID,
			"client_steer_id":  item.ClientSteerID,
			"received_seq":     item.Seq,
			"applied_at_round": round + 1,
			"message_id":       item.MessageID,
		})
		payload := map[string]any{
			"steer_id":        item.SteerID,
			"client_steer_id": item.ClientSteerID,
			"content_preview": firstNonEmpty(item.Preview, contentPreviewForSteer(content, 120)),
			"round":           round + 1,
			"message_id":      item.MessageID,
			"received_seq":    item.Seq,
			"received_at":     item.ReceivedAt.Format(time.RFC3339Nano),
			"created_at":      time.Now().UTC().Format(time.RFC3339Nano),
			"message":         "引导已应用到当前任务",
		}
		if steerMessage.ID != "" {
			payload["user_message"] = steerMessage
		}
		if r.opts.Event == nil {
			if _, err := r.service.runs.AppendEvent(ctx, store.AgentEventInput{
				RunID:          r.opts.RunID,
				UserID:         r.opts.UserID,
				ConversationID: r.conversationID,
				MessageID:      r.messageID,
				Type:           "agent_steer_applied",
				Level:          "info",
				Payload:        payload,
			}); err != nil {
				return err
			}
			continue
		}
		emitToolLifecycleEvent(r.opts, "agent_steer_applied", "info", payload)
	}
	if err := r.service.saveRunCheckpoint(ctx, r.opts, r.checkpoint()); err != nil {
		return err
	}
	return nil
}

func (r *agentRuntime) hasPendingSteers(ctx context.Context) (bool, error) {
	if r == nil || r.service == nil || r.service.runs == nil || r.opts.UserID == "" || r.opts.RunID == "" {
		return false, nil
	}
	items, err := pendingSteersForRun(ctx, r.service.runs, r.opts.UserID, r.opts.RunID)
	if err != nil {
		return false, err
	}
	return len(items) > 0, nil
}

func pendingSteersForRun(ctx context.Context, runs *store.AgentRunStore, userID, runID string) ([]pendingSteer, error) {
	events, err := runs.ListEvents(ctx, userID, runID)
	if err != nil {
		return nil, err
	}
	applied := map[string]bool{}
	canceled := map[string]bool{}
	received := make([]pendingSteer, 0)
	for _, event := range events {
		payload := steerEventPayload(event)
		steerID := strings.TrimSpace(payload["steer_id"])
		clientSteerID := strings.TrimSpace(payload["client_steer_id"])
		key := steerEventKey(steerID, clientSteerID)
		if key == "" {
			continue
		}
		switch event.Type {
		case "agent_steer_received":
			content := strings.TrimSpace(payload["content"])
			if content == "" {
				continue
			}
			received = append(received, pendingSteer{
				SteerID:       steerID,
				ClientSteerID: clientSteerID,
				Content:       content,
				Preview:       strings.TrimSpace(payload["content_preview"]),
				Seq:           event.Seq,
				ReceivedAt:    steerReceivedAt(event, payload),
				MessageID:     strings.TrimSpace(payload["message_id"]),
			})
		case "agent_steer_applied":
			for _, eventKey := range steerEventKeys(steerID, clientSteerID) {
				applied[eventKey] = true
			}
		case "agent_steer_canceled":
			for _, eventKey := range steerEventKeys(steerID, clientSteerID) {
				canceled[eventKey] = true
			}
		}
	}
	pending := make([]pendingSteer, 0, len(received))
	for _, item := range received {
		keys := steerEventKeys(item.SteerID, item.ClientSteerID)
		if len(keys) > 0 && !containsSteerEventKey(applied, keys) && !containsSteerEventKey(canceled, keys) {
			pending = append(pending, item)
		}
	}
	sort.SliceStable(pending, func(i, j int) bool {
		return pending[i].Seq < pending[j].Seq
	})
	return pending, nil
}

func steerEventKey(steerID, clientSteerID string) string {
	steerID = strings.TrimSpace(steerID)
	if steerID != "" {
		return "steer:" + steerID
	}
	clientSteerID = strings.TrimSpace(clientSteerID)
	if clientSteerID != "" {
		return "client:" + clientSteerID
	}
	return ""
}

func steerEventKeys(steerID, clientSteerID string) []string {
	var keys []string
	if trimmed := strings.TrimSpace(steerID); trimmed != "" {
		keys = append(keys, "steer:"+trimmed)
	}
	if trimmed := strings.TrimSpace(clientSteerID); trimmed != "" {
		keys = append(keys, "client:"+trimmed)
	}
	return keys
}

func containsSteerEventKey(values map[string]bool, keys []string) bool {
	for _, key := range keys {
		if values[key] {
			return true
		}
	}
	return false
}

func steerReceivedAt(event store.AgentEvent, payload map[string]string) time.Time {
	if raw := strings.TrimSpace(payload["created_at"]); raw != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
			return parsed.UTC()
		}
	}
	if !event.CreatedAt.IsZero() {
		return event.CreatedAt.UTC()
	}
	return time.Now().UTC()
}

func findStoredSteerMessageID(ctx context.Context, conversations *store.ConversationStore, userID, conversationID string, item pendingSteer) (string, error) {
	if conversations == nil {
		return "", nil
	}
	return conversations.FindSteerMessageID(ctx, userID, conversationID, item.SteerID, item.ClientSteerID)
}

func steerEventPayload(event store.AgentEvent) map[string]string {
	var raw map[string]any
	if err := json.Unmarshal([]byte(event.PayloadJSON), &raw); err != nil {
		return nil
	}
	payload := make(map[string]string, len(raw))
	for key, value := range raw {
		payload[key] = fmt.Sprint(value)
	}
	return payload
}

func formatSteerTranscriptText(content string) string {
	return fmt.Sprintf("用户在任务运行中补充了最新引导，请优先按此调整后续操作：\n\n%s", strings.TrimSpace(content))
}

func contentPreviewForSteer(content string, maxRunes int) string {
	content = strings.TrimSpace(content)
	runes := []rune(content)
	if maxRunes <= 0 || len(runes) <= maxRunes {
		return content
	}
	return strings.TrimSpace(string(runes[:maxRunes])) + "..."
}

package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"webide-backend/internal/store"
)

type pendingSteer struct {
	SteerID       string
	ClientSteerID string
	Content       string
	Preview       string
	Seq           int64
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
		r.transcript.AppendUserText(formatSteerTranscriptText(content), map[string]any{
			"reason":           "runtime_user_steer",
			"steer_id":         item.SteerID,
			"client_steer_id":  item.ClientSteerID,
			"received_seq":     item.Seq,
			"applied_at_round": round + 1,
		})
		payload := map[string]any{
			"steer_id":        item.SteerID,
			"client_steer_id": item.ClientSteerID,
			"content_preview": firstNonEmpty(item.Preview, contentPreviewForSteer(content, 120)),
			"round":           round + 1,
			"message":         "引导已应用到当前任务",
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

func pendingSteersForRun(ctx context.Context, runs *store.AgentRunStore, userID, runID string) ([]pendingSteer, error) {
	events, err := runs.ListEvents(ctx, userID, runID)
	if err != nil {
		return nil, err
	}
	applied := map[string]bool{}
	received := make([]pendingSteer, 0)
	for _, event := range events {
		payload := steerEventPayload(event)
		steerID := strings.TrimSpace(payload["steer_id"])
		if steerID == "" {
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
				ClientSteerID: strings.TrimSpace(payload["client_steer_id"]),
				Content:       content,
				Preview:       strings.TrimSpace(payload["content_preview"]),
				Seq:           event.Seq,
			})
		case "agent_steer_applied":
			applied[steerID] = true
		}
	}
	pending := make([]pendingSteer, 0, len(received))
	for _, item := range received {
		if !applied[item.SteerID] {
			pending = append(pending, item)
		}
	}
	sort.SliceStable(pending, func(i, j int) bool {
		return pending[i].Seq < pending[j].Seq
	})
	return pending, nil
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

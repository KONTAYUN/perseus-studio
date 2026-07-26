package ai

import (
	"context"
	"fmt"
	"strings"
	"time"

	"webide-backend/internal/store"
)

const (
	contextRecentTurnSuffixTokens = 20000
	contextCompactionBatchTokens  = 16000
)

func (r *agentRuntime) compactAgentItemContext(ctx context.Context, reason string) (bool, error) {
	items, err := r.loadPersistentContextItems(ctx)
	if err != nil {
		return false, err
	}
	active, currentBoundary := activeAgentContextItems(items)
	retainBudget := contextRecentTurnSuffixTokens
	if available := contextBudgetTokens(r.profile) / 2; available > 0 && available < retainBudget {
		retainBudget = available
	}
	if retainBudget < 4000 {
		retainBudget = 4000
	}
	boundary := selectAgentCompactionBoundary(active, retainBudget)
	if boundary <= currentBoundary {
		return false, nil
	}
	chunk := make([]store.AgentTurnItem, 0, len(active))
	for _, item := range active {
		if item.Seq > currentBoundary && item.Seq <= boundary {
			chunk = append(chunk, item)
		}
	}
	if len(chunk) == 0 {
		return false, nil
	}

	emitToolLifecycleEvent(r.opts, "context_compaction_started", "info", map[string]any{
		"reason": reason, "replaces_through_seq": boundary, "item_count": len(chunk),
	})
	summary, err := r.service.summarizeAgentItemChunk(ctx, r.provider, chunk, r.opts)
	if err != nil {
		emitToolLifecycleEvent(r.opts, "context_compaction_failed", "warn", map[string]any{
			"reason": reason, "error": err.Error(), "replaces_through_seq": boundary,
		})
		return false, err
	}
	payload := mustJSON(map[string]any{
		"summary": summary, "reason": reason, "replaces_through_seq": boundary,
		"tokens_before": estimateAgentItemsTokens(active), "retained_turn_token_budget": retainBudget,
	})
	turnID := firstNonEmpty(r.kernel.TurnID, r.messageID, r.opts.RunID)
	_, err = r.service.calls.AppendAgentTurnItem(ctx, store.AgentTurnItemInput{
		UserID: r.opts.UserID, ConversationID: r.conversationID, RunID: r.opts.RunID,
		TurnID: turnID, Kind: "compaction", Role: "system", Status: "completed",
		Visibility:  store.AgentItemVisibilityModel,
		SourceID:    fmt.Sprintf("compaction:%s:%d", reason, boundary),
		PayloadJSON: payload, CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		return false, err
	}
	emitToolLifecycleEvent(r.opts, "context_compaction_completed", "info", map[string]any{
		"reason": reason, "replaces_through_seq": boundary, "item_count": len(chunk),
	})
	return true, nil
}

func selectAgentCompactionBoundary(items []store.AgentTurnItem, retainTokens int) int64 {
	if len(items) == 0 || retainTokens <= 0 {
		return 0
	}
	type turnGroup struct {
		turnID   string
		firstSeq int64
		lastSeq  int64
		tokens   int
	}
	var groups []turnGroup
	for _, item := range items {
		if item.Kind == "compaction" {
			continue
		}
		turnID := firstNonEmpty(item.TurnID, item.ID)
		if len(groups) == 0 || groups[len(groups)-1].turnID != turnID {
			groups = append(groups, turnGroup{turnID: turnID, firstSeq: item.Seq})
		}
		group := &groups[len(groups)-1]
		group.lastSeq = item.Seq
		group.tokens += estimateAgentItemTokens(item)
	}
	if len(groups) < 2 {
		return 0
	}
	retained := 0
	retainFrom := len(groups) - 1
	for index := len(groups) - 1; index >= 0; index-- {
		if index < len(groups)-1 && retained+groups[index].tokens > retainTokens {
			break
		}
		retained += groups[index].tokens
		retainFrom = index
	}
	if retainFrom <= 0 {
		return 0
	}
	return groups[retainFrom-1].lastSeq
}

func estimateAgentItemsTokens(items []store.AgentTurnItem) int {
	total := 0
	for _, item := range items {
		total += estimateAgentItemTokens(item)
	}
	return total
}

func estimateAgentItemTokens(item store.AgentTurnItem) int {
	return 4 + estimateTextTokens(item.Kind) + estimateTextTokens(item.Role) + estimateTextTokens(item.PayloadJSON)
}

func (s *Service) summarizeAgentItemChunk(ctx context.Context, provider store.ModelProvider, items []store.AgentTurnItem, opts RunOptions, userInstructions ...string) (string, error) {
	instruction := "Summarize earlier agent turn items into durable task memory. Preserve the user's goals, corrections, constraints, file paths, verified findings, completed changes, failures, and remaining work. Never claim an action succeeded unless a tool result says it succeeded. Do not include API keys or full file contents. Use the user's language when clear."
	if len(userInstructions) > 0 && strings.TrimSpace(userInstructions[0]) != "" {
		instruction += "\n\nUser compaction instructions:\n" + strings.TrimSpace(userInstructions[0])
	}
	previous := ""
	var batch strings.Builder
	batchTokens := 0
	flush := func() error {
		if strings.TrimSpace(batch.String()) == "" {
			return nil
		}
		prompt := "Existing compacted memory:\n" + firstNonEmpty(previous, "(none)") + "\n\nEarlier turn items to merge:\n" + batch.String()
		var next string
		var err error
		if strings.EqualFold(strings.TrimSpace(provider.Protocol), "anthropic") {
			next, err = s.summarizeAnthropic(ctx, provider, instruction, prompt, opts)
		} else {
			next, err = s.summarizeOpenAI(ctx, provider, instruction, prompt, opts)
		}
		if err != nil {
			return err
		}
		previous = strings.TrimSpace(next)
		batch.Reset()
		batchTokens = 0
		return nil
	}
	for _, item := range items {
		line := agentItemCompactionLine(item)
		lineTokens := estimateTextTokens(line)
		if batchTokens > 0 && batchTokens+lineTokens > contextCompactionBatchTokens {
			if err := flush(); err != nil {
				return "", err
			}
		}
		batch.WriteString(line)
		batch.WriteString("\n")
		batchTokens += lineTokens
	}
	if err := flush(); err != nil {
		return "", err
	}
	if previous == "" {
		return "", fmt.Errorf("context compaction produced an empty summary")
	}
	return previous, nil
}

func agentItemCompactionLine(item store.AgentTurnItem) string {
	payload := decodeContextItemPayload(item)
	switch item.Kind {
	case "user_message", "assistant_message", "compaction", "task_plan", "external_observation":
		return fmt.Sprintf("[%s turn=%s] %s", item.Kind, item.TurnID, contextItemContent(item))
	case "tool_call":
		return fmt.Sprintf("[tool_call turn=%s] %s %s", item.TurnID, contextItemToolName(item), contextItemArgumentsJSON(item))
	case "tool_result", "aborted":
		return fmt.Sprintf("[%s turn=%s status=%s] tool=%s path=%s summary=%s changed=%s", item.Kind, item.TurnID, item.Status,
			contextItemToolName(item), contextPayloadString(payload, "path", "target"),
			contextPayloadString(payload, "summary", "error"), strings.Join(contextPayloadStrings(payload, "changed_paths"), ","))
	case "workspace_observation", "file_change", "environment_snapshot":
		return fmt.Sprintf("[%s turn=%s] %s", item.Kind, item.TurnID, item.PayloadJSON)
	default:
		return fmt.Sprintf("[%s turn=%s status=%s]", item.Kind, item.TurnID, item.Status)
	}
}

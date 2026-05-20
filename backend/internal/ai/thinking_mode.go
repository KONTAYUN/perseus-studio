package ai

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"webide-backend/internal/store"
)

const (
	ThinkingModeStandard = "standard"
	ThinkingModeDeep     = "deep"
	ThinkingModeExpert   = "expert"
)

func normalizeThinkingMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case ThinkingModeDeep:
		return ThinkingModeDeep
	case ThinkingModeExpert:
		return ThinkingModeExpert
	default:
		return ThinkingModeStandard
	}
}

func (s *Service) applyThinkingMode(ctx context.Context, conversationID, messageID string, provider store.ModelProvider, messages []store.Message, attachments []AttachmentPayload, opts RunOptions, reply Reply) (Reply, error) {
	if reply.BackgroundTask != nil || strings.TrimSpace(reply.Content) == "" || opts.TaskID != "" {
		return reply, nil
	}

	rounds := 0
	switch normalizeThinkingMode(opts.ThinkingMode) {
	case ThinkingModeDeep:
		rounds = 1
	case ThinkingModeExpert:
		rounds = 2
	}
	if rounds == 0 {
		log.Printf("[ai] thinking mode conversation=%s mode=%s refinement_rounds=0", conversationID, normalizeThinkingMode(opts.ThinkingMode))
		return reply, nil
	}

	log.Printf("[ai] thinking mode conversation=%s mode=%s refinement_rounds=%d", conversationID, normalizeThinkingMode(opts.ThinkingMode), rounds)
	content := reply.Content
	for round := 0; round < rounds; round++ {
		logTask(opts, "info", fmt.Sprintf("thinking refinement round %d", round+1))
		log.Printf("[ai] thinking refinement conversation=%s mode=%s round=%d/%d", conversationID, normalizeThinkingMode(opts.ThinkingMode), round+1, rounds)
		next, err := s.refineReplyOnce(ctx, conversationID, messageID, provider, messages, attachments, opts, content, round+1, rounds, len(reply.ToolEvents) > 0)
		if err != nil {
			return reply, nil
		}
		if strings.TrimSpace(next) != "" {
			content = strings.TrimSpace(next)
		}
	}
	reply.Content = content
	return reply, nil
}

func (s *Service) refineReplyOnce(ctx context.Context, conversationID, messageID string, provider store.ModelProvider, messages []store.Message, attachments []AttachmentPayload, opts RunOptions, draft string, round, totalRounds int, hasToolEvents bool) (string, error) {
	switch provider.Protocol {
	case "openai":
		return s.refineOpenAIReply(ctx, conversationID, messageID, provider, messages, attachments, opts, draft, round, totalRounds, hasToolEvents)
	case "anthropic":
		return s.refineAnthropicReply(ctx, conversationID, messageID, provider, messages, attachments, opts, draft, round, totalRounds, hasToolEvents)
	default:
		return "", fmt.Errorf("unsupported protocol: %s", provider.Protocol)
	}
}

func thinkingRefinementInstruction(round, totalRounds int, hasToolEvents bool) string {
	var b strings.Builder
	b.WriteString("你正在进行 Perseus Studio 的内部思考强度改进流程。")
	b.WriteString("请审阅上一版回答，找出遗漏、含混、结构问题或可能误导用户的地方，然后直接输出改进后的最终回答。")
	b.WriteString("不要输出审阅过程、草稿、评分、标题如“修订版”，也不要提到你进行了内部迭代。")
	b.WriteString("保持用户原始语言和语气，优先让答案更准确、更有条理、更可执行。")
	if hasToolEvents {
		b.WriteString("本轮已经执行过工具；你只能基于已有工具结果改进总结，不得声称又执行了新的工具或改变了文件。")
	}
	if round < totalRounds {
		b.WriteString("这一轮仍是中间改进，但输出也必须像最终答案一样完整。")
	} else {
		b.WriteString("这是最后一轮，请只输出可以直接发给用户的最终答案。")
	}
	return b.String()
}

func (s *Service) refineOpenAIReply(ctx context.Context, conversationID, messageID string, provider store.ModelProvider, messages []store.Message, attachments []AttachmentPayload, opts RunOptions, draft string, round, totalRounds int, hasToolEvents bool) (string, error) {
	runtimeMessages := prependOpenAISystem(buildOpenAIMessages(messages, attachments))
	runtimeMessages = append(runtimeMessages,
		map[string]any{"role": "assistant", "content": draft},
		map[string]any{"role": "user", "content": thinkingRefinementInstruction(round, totalRounds, hasToolEvents)},
	)
	payload := map[string]any{
		"model":    provider.ModelID,
		"messages": runtimeMessages,
	}
	path := openAIChatPath()
	endpoint := providerSDKEndpoint(provider.BaseURL, path)
	startedAt := time.Now()
	resp, err := s.openAISDKPostRaw(ctx, provider, path, payload, opts, "thinking_refinement")
	duration := time.Since(startedAt)
	if err != nil {
		s.recordProviderDiagnostic(ctx, opts, provider, conversationID, messageID, "thinking_refinement", "warn", duration, err)
		return "", err
	}
	var data openAIResponse
	if err := decodeProviderResponse(resp, endpoint, &data); err != nil {
		s.recordProviderDiagnostic(ctx, opts, provider, conversationID, messageID, "thinking_refinement", "warn", duration, err)
		return "", err
	}
	if data.Error != nil {
		err := formatProviderError(resp.StatusCode, data.Error)
		s.recordProviderDiagnostic(ctx, opts, provider, conversationID, messageID, "thinking_refinement", "warn", duration, err)
		return "", err
	}
	if len(data.Choices) == 0 {
		return "", fmt.Errorf("empty model response")
	}
	return providerVisibleText(normalizeOpenAIContent(data.Choices[0].Message.Content)), nil
}

func (s *Service) refineAnthropicReply(ctx context.Context, conversationID, messageID string, provider store.ModelProvider, messages []store.Message, attachments []AttachmentPayload, opts RunOptions, draft string, round, totalRounds int, hasToolEvents bool) (string, error) {
	systemText, runtimeMessages := buildAnthropicMessages(messages, attachments)
	runtimeMessages = append(runtimeMessages,
		map[string]any{"role": "assistant", "content": draft},
		map[string]any{"role": "user", "content": thinkingRefinementInstruction(round, totalRounds, hasToolEvents)},
	)
	payload := map[string]any{
		"model":      provider.ModelID,
		"max_tokens": 2048,
		"messages":   runtimeMessages,
	}
	if systemText != "" {
		payload["system"] = systemText
	}
	path := anthropicMessagesPath(provider)
	endpoint := providerSDKEndpoint(provider.BaseURL, path)
	startedAt := time.Now()
	resp, err := s.anthropicSDKPostRaw(ctx, provider, path, payload, opts, "thinking_refinement")
	duration := time.Since(startedAt)
	if err != nil {
		s.recordProviderDiagnostic(ctx, opts, provider, conversationID, messageID, "thinking_refinement", "warn", duration, err)
		return "", err
	}
	var data anthropicResponse
	if err := decodeProviderResponse(resp, endpoint, &data); err != nil {
		s.recordProviderDiagnostic(ctx, opts, provider, conversationID, messageID, "thinking_refinement", "warn", duration, err)
		return "", err
	}
	if data.Error != nil {
		err := formatProviderError(resp.StatusCode, data.Error)
		s.recordProviderDiagnostic(ctx, opts, provider, conversationID, messageID, "thinking_refinement", "warn", duration, err)
		return "", err
	}
	return providerVisibleText(data.text()), nil
}

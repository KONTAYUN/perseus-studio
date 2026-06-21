package ai

import (
	"encoding/json"
	"strings"

	"webide-backend/internal/idgen"
)

type extractedTextPatchTurn struct {
	Turn               agentModelTurn
	Extracted          bool
	DiscardedToolCalls int
	DiscardedToolNames []string
}

func extractWorkspacePatchBlock(content string) (string, string, bool) {
	envelope, ok := parseWorkspacePatchEnvelope(content)
	return envelope.Visible, envelope.Patch, ok
}

type patchTextSpan struct {
	start int
	end   int
}

type WorkspacePatchEnvelope struct {
	Visible string
	Patch   string
	Spans   []patchTextSpan
}

func parseWorkspacePatchEnvelope(content string) (WorkspacePatchEnvelope, bool) {
	visible, patch, spans, ok := extractWorkspacePatchEnvelope(content)
	return WorkspacePatchEnvelope{Visible: visible, Patch: patch, Spans: spans}, ok
}

func extractWorkspacePatchEnvelope(content string) (string, string, []patchTextSpan, bool) {
	text := strings.TrimSpace(content)
	if text == "" {
		return "", "", nil, false
	}
	normalized := strings.ReplaceAll(strings.ReplaceAll(content, "\r\n", "\n"), "\r", "\n")
	patch, spans, ok := extractWorkspacePatchBlocksWithSpans(normalized)
	if !ok {
		return content, "", nil, false
	}
	var visibleParts []string
	cursor := 0
	for _, span := range spans {
		if span.start > cursor {
			visibleParts = append(visibleParts, normalized[cursor:span.start])
		}
		cursor = span.end
	}
	if cursor < len(normalized) {
		visibleParts = append(visibleParts, normalized[cursor:])
	}
	for index, part := range visibleParts {
		part = stripPatchFenceResidue(part)
		visibleParts[index] = part
	}
	visible := strings.TrimSpace(strings.Join(nonEmptyStrings(visibleParts...), "\n\n"))
	return visible, patch, spans, patch != ""
}

func extractWorkspacePatchBlocksWithSpans(normalized string) (string, []patchTextSpan, bool) {
	lines := strings.SplitAfter(normalized, "\n")
	var patches []string
	var spans []patchTextSpan
	inPatch := false
	startByte := 0
	cursor := 0
	var current strings.Builder
	for _, line := range lines {
		header := strings.TrimSpace(strings.TrimSuffix(line, "\n"))
		lineStart := cursor
		cursor += len(line)
		switch header {
		case "*** Begin Patch":
			if inPatch {
				current.WriteString(line)
				continue
			}
			inPatch = true
			startByte = lineStart
			current.Reset()
			current.WriteString("*** Begin Patch\n")
		case "*** End Patch":
			if !inPatch {
				continue
			}
			current.WriteString("*** End Patch")
			patches = append(patches, current.String())
			spans = append(spans, patchTextSpan{start: startByte, end: cursor})
			inPatch = false
			current.Reset()
		default:
			if inPatch {
				current.WriteString(line)
			}
		}
	}
	if len(patches) == 0 {
		return "", nil, false
	}
	if len(patches) == 1 {
		return strings.TrimSpace(patches[0]), spans, true
	}
	var builder strings.Builder
	builder.WriteString("*** Begin Patch\n")
	for _, patch := range patches {
		for index, line := range strings.SplitAfter(patch, "\n") {
			header := strings.TrimSpace(strings.TrimSuffix(line, "\n"))
			if index == 0 && header == "*** Begin Patch" {
				continue
			}
			if header == "*** End Patch" {
				continue
			}
			builder.WriteString(line)
		}
		if !strings.HasSuffix(builder.String(), "\n") {
			builder.WriteString("\n")
		}
	}
	builder.WriteString("*** End Patch")
	return strings.TrimSpace(builder.String()), spans, true
}

func stripPatchFenceResidue(text string) string {
	text = strings.TrimSpace(text)
	for _, fence := range []string{"```text", "```patch", "```diff", "```"} {
		text = strings.TrimSpace(strings.TrimSuffix(text, fence))
		text = strings.TrimSpace(strings.TrimPrefix(text, fence))
	}
	text = strings.TrimSpace(strings.TrimPrefix(text, "*** End Patch"))
	return text
}

func textPatchToolCall(patch string) agentToolCall {
	args := mustJSON(map[string]any{
		"patch":     patch,
		"transport": "assistant_text_patch",
	})
	return agentToolCall{
		ID:            idgen.New("text_patch"),
		Name:          "apply_workspace_patch",
		ArgumentsJSON: args,
		PatchText:     patch,
		AnthropicInput: map[string]any{
			"patch":     patch,
			"transport": "assistant_text_patch",
		},
	}
}

func turnWithExtractedTextPatch(turn agentModelTurn) (agentModelTurn, bool) {
	result := normalizeTextPatchTurn(turn)
	return result.Turn, result.Extracted
}

func normalizeTextPatchTurn(turn agentModelTurn) extractedTextPatchTurn {
	originalToolCalls := append([]agentToolCall(nil), turn.ToolCalls...)
	visible, patch, ok := extractWorkspacePatchBlock(turn.Content)
	if !ok {
		return extractedTextPatchTurn{Turn: turn}
	}
	call := textPatchToolCall(patch)
	turn.Content = visible
	if strings.TrimSpace(turn.PublicStreamedContent) != "" {
		turn.PublicTextStreamed = true
	}
	turn.PublicStreamedContent = strings.TrimSpace(visible)
	turn.ToolCalls = []agentToolCall{call}
	turn.OpenAIMessage.Content = visible
	turn.OpenAIMessage.ToolCalls = []openAIToolCall{{
		ID:   call.ID,
		Type: "function",
		Function: openAIFunctionCall{
			Name:      call.Name,
			Arguments: json.RawMessage(call.ArgumentsJSON),
		},
	}}
	turn.AnthropicRaw = []map[string]any{}
	if strings.TrimSpace(visible) != "" {
		turn.AnthropicRaw = append(turn.AnthropicRaw, map[string]any{"type": "text", "text": visible})
	}
	turn.AnthropicRaw = append(turn.AnthropicRaw, map[string]any{
		"type":  "tool_use",
		"id":    call.ID,
		"name":  call.Name,
		"input": cloneMapAny(call.AnthropicInput),
	})
	return extractedTextPatchTurn{
		Turn:               turn,
		Extracted:          true,
		DiscardedToolCalls: len(originalToolCalls),
		DiscardedToolNames: toolCallNames(originalToolCalls),
	}
}

func toolCallNames(calls []agentToolCall) []string {
	if len(calls) == 0 {
		return nil
	}
	names := make([]string, 0, len(calls))
	for _, call := range calls {
		name := strings.TrimSpace(call.Name)
		if name != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return nil
	}
	return names
}

func nonEmptyStrings(values ...string) []string {
	next := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			next = append(next, strings.TrimSpace(value))
		}
	}
	return next
}

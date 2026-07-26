package ai

import (
	"encoding/json"
	"math"
	"strings"
	"unicode"
)

func estimateTextTokens(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	runes := []rune(value)
	ascii := 0
	nonASCII := 0
	for _, r := range runes {
		if r <= unicode.MaxASCII {
			ascii++
		} else {
			nonASCII++
		}
	}
	tokens := (ascii + 3) / 4
	tokens += (nonASCII*3 + 1) / 2
	if tokens < 1 {
		return 1
	}
	return tokens
}

func estimateRuntimeMessagesTokens(messages []map[string]any) int {
	textTokens, images := estimateRuntimeMessagesTokenParts(messages)
	return textTokens + images*2048
}

func estimateRuntimeMessagesTokenParts(messages []map[string]any) (int, int) {
	total := 0
	images := 0
	for _, message := range messages {
		sanitized, count := sanitizeTokenEstimateValue(message)
		raw, err := json.Marshal(sanitized)
		if err == nil {
			total += estimateTextTokens(string(raw))
		}
		images += count
	}
	return total, images
}

func estimateMapTokens(value map[string]any) int {
	sanitized, images := sanitizeTokenEstimateValue(value)
	raw, err := json.Marshal(sanitized)
	if err != nil {
		return 0
	}
	return estimateTextTokens(string(raw)) + images*2048
}

func sanitizeTokenEstimateValue(value any) (any, int) {
	switch item := value.(type) {
	case map[string]any:
		typeName := strings.ToLower(strings.TrimSpace(stringFromAny(item["type"])))
		if typeName == "image" || typeName == "image_url" || typeName == "input_image" {
			return map[string]any{"type": typeName, "image": "[binary image omitted from text estimate]"}, 1
		}
		result := make(map[string]any, len(item))
		images := 0
		for key, child := range item {
			sanitized, count := sanitizeTokenEstimateValue(child)
			result[key] = sanitized
			images += count
		}
		return result, images
	case []map[string]any:
		result := make([]any, 0, len(item))
		images := 0
		for _, child := range item {
			sanitized, count := sanitizeTokenEstimateValue(child)
			result = append(result, sanitized)
			images += count
		}
		return result, images
	case []any:
		result := make([]any, 0, len(item))
		images := 0
		for _, child := range item {
			sanitized, count := sanitizeTokenEstimateValue(child)
			result = append(result, sanitized)
			images += count
		}
		return result, images
	case string:
		if strings.HasPrefix(item, "data:image/") && strings.Contains(item, ";base64,") {
			return "[binary image omitted from text estimate]", 1
		}
		return item, 0
	default:
		return value, 0
	}
}

func estimateVisualInputTokens(profile providerRuntimeProfile, attachments []AttachmentPayload) (int, string) {
	total := 0
	source := "compatibility_default_2048_per_image"
	strategy := builtInVisualTokenStrategies[strings.ToLower(strings.TrimSpace(profile.ModelID))]
	for _, attachment := range attachments {
		if attachment.Kind != "image" {
			continue
		}
		width, height := attachment.Width, attachment.Height
		if width <= 0 || height <= 0 {
			total += 2048
			continue
		}
		if strategy == visualTokenStrategyAnthropic {
			total += estimateAnthropicImageTokens(width, height)
			source = "builtin_anthropic_dimensions"
			continue
		}
		if strategy == visualTokenStrategyOpenAI {
			total += estimateOpenAIImageTokens(width, height)
			source = "builtin_openai_high_detail_tiles"
			continue
		}
		total += 2048
	}
	return total, source
}

func estimateOpenAIImageTokens(width, height int) int {
	w, h := float64(width), float64(height)
	if maxFloat(w, h) > 2048 {
		scale := 2048 / maxFloat(w, h)
		w *= scale
		h *= scale
	}
	if minFloat(w, h) > 768 {
		scale := 768 / minFloat(w, h)
		w *= scale
		h *= scale
	}
	tiles := int(math.Ceil(w/512) * math.Ceil(h/512))
	return 85 + 170*tiles
}

func estimateAnthropicImageTokens(width, height int) int {
	w, h := float64(width), float64(height)
	scale := 1.0
	if maxFloat(w, h) > 1568 {
		scale = 1568 / maxFloat(w, h)
	}
	if pixels := w * h; pixels*scale*scale > 1_150_000 {
		scale = math.Min(scale, math.Sqrt(1_150_000/pixels))
	}
	w *= scale
	h *= scale
	return int(math.Ceil((w * h) / 750))
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func contextWatermarkPercent(estimatedTokens, windowTokens int) int {
	if estimatedTokens <= 0 || windowTokens <= 0 {
		return 0
	}
	return int((int64(estimatedTokens) * 100) / int64(windowTokens))
}

func contextBudgetTokens(profile providerRuntimeProfile) int {
	window := profile.ContextWindowTokens
	if window <= 0 {
		window = defaultContextWindowTokens
	}
	threshold := profile.CompactThresholdPercent
	if threshold <= 0 {
		threshold = 80
	}
	thresholdBudget := window * threshold / 100
	reserved := profile.CompactReservedTokens
	if reserved == 0 {
		reserved = defaultCompactReservedTokens(window)
	}
	reservedBudget := window - reserved
	if reserved <= 0 || reservedBudget <= 0 {
		return thresholdBudget
	}
	if reservedBudget < thresholdBudget {
		return reservedBudget
	}
	return thresholdBudget
}

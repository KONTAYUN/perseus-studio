package ai

import (
	"encoding/json"
	"strings"
	"unicode"

	"webide-backend/internal/store"
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

func estimateMessagesTokens(messages []store.Message) int {
	total := 0
	for _, message := range messages {
		total += 4
		total += estimateTextTokens(message.Role)
		total += estimateTextTokens(message.Content)
		for _, attachment := range StoredMessageAttachments(message) {
			total += estimateTextTokens(attachment.Text)
			if attachment.Kind == "image" {
				total += 1600
			}
		}
		if strings.TrimSpace(message.AttachmentsJSON) != "" && strings.TrimSpace(message.AttachmentsJSON) != "[]" {
			total += estimateTextTokens(message.AttachmentsJSON)
		}
	}
	return total
}

func estimateRuntimeMessagesTokens(messages []map[string]any) int {
	total := 0
	for _, message := range messages {
		total += estimateMapTokens(message)
	}
	return total
}

func estimateMapTokens(value map[string]any) int {
	raw, err := json.Marshal(value)
	if err != nil {
		return 0
	}
	return estimateTextTokens(string(raw))
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

package ai

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

type providerOutputRepetition struct {
	Count          int
	FragmentHashes []string
}

func detectProviderOutputRepetition(content string) providerOutputRepetition {
	blocks := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n\n")
	seen := map[string]int{}
	hashes := map[string]struct{}{}
	result := providerOutputRepetition{}
	for _, block := range blocks {
		normalized := strings.Join(strings.Fields(block), " ")
		if len([]rune(normalized)) < 40 {
			continue
		}
		if seen[normalized] > 0 {
			result.Count++
			digest := sha256.Sum256([]byte(normalized))
			hashes[hex.EncodeToString(digest[:8])] = struct{}{}
		}
		seen[normalized]++
	}
	for hash := range hashes {
		result.FragmentHashes = append(result.FragmentHashes, hash)
	}
	return result
}

func (r *agentRuntime) recordProviderTurnDiagnostics(turn agentModelTurn) {
	if r == nil {
		return
	}
	r.lastContextSnapshot.Summary.ProviderResponseID = turn.ProviderResponseID
	r.lastContextSnapshot.Summary.StreamLastEventID = turn.StreamLastEventID
	r.lastContextSnapshot.Summary.StreamDuplicateEvents = turn.StreamDuplicateEvents
	if turn.StreamDuplicateEvents > 0 {
		emitToolLifecycleEvent(r.opts, "provider_stream_replay_deduplicated", "warn", map[string]any{
			"provider_id":          r.provider.ID,
			"model_id":             r.provider.ModelID,
			"protocol":             r.provider.Protocol,
			"provider_response_id": turn.ProviderResponseID,
			"last_event_id":        turn.StreamLastEventID,
			"duplicate_events":     turn.StreamDuplicateEvents,
		})
	}
	repetition := detectProviderOutputRepetition(turn.Content)
	r.lastContextSnapshot.Summary.ProviderOutputRepeatCount = repetition.Count
	if repetition.Count == 0 {
		return
	}
	emitToolLifecycleEvent(r.opts, "provider_output_repetition", "warn", map[string]any{
		"provider_id":          r.provider.ID,
		"model_id":             r.provider.ModelID,
		"protocol":             r.provider.Protocol,
		"provider_response_id": turn.ProviderResponseID,
		"repeat_count":         repetition.Count,
		"fragment_hashes":      repetition.FragmentHashes,
	})
}

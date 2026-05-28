package api

import (
	"context"
	"time"
)

const (
	assistantReplayChunkRunes = 96
	assistantReplayChunkDelay = 55 * time.Millisecond
)

func replayAssistantDeltaChunks(ctx context.Context, delta string, appendDelta func(string)) {
	if appendDelta == nil {
		return
	}
	chunks := splitAssistantDelta(delta, assistantReplayChunkRunes)
	for index, chunk := range chunks {
		appendDelta(chunk)
		if index < len(chunks)-1 && !waitBetweenAssistantReplayChunks(ctx) {
			return
		}
	}
}

func waitBetweenAssistantReplayChunks(ctx context.Context) bool {
	if assistantReplayChunkDelay <= 0 {
		return true
	}
	timer := time.NewTimer(assistantReplayChunkDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

package ai

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	editPreviewMinimumInterval    = 80 * time.Millisecond
	editPreviewMaximumInterval    = 500 * time.Millisecond
	editPreviewCheckpointInterval = 2 * time.Second
	editPreviewMinimumArgumentGap = 256
)

type editPreviewSnapshot struct {
	Path            string
	BaseHash        string
	DocumentVersion string
	Content         string
	Operation       string
	IsNew           bool
	Deleted         bool
	Changes         []map[string]any
}

type editPreviewMetrics struct {
	RawArgumentDeltas  int
	ParseableDeltas    int
	ThrottledDeltas    int
	DuplicateSnapshots int
}

type activeEditPreview struct {
	PreviewID       string
	ToolID          string
	Path            string
	Revision        int
	Content         string
	LastEmit        time.Time
	LastCheckpoint  time.Time
	ArgumentsSize   int
	ChangeCount     int
	Changes         []map[string]any
	ContentHash     string
	BroadcastBytes  int64
	CheckpointCount int
	Metrics         *editPreviewMetrics
}

type editPreviewManager struct {
	mu       sync.Mutex
	service  *Service
	userID   string
	runID    string
	opts     RunOptions
	byKey    map[string]*activeEditPreview
	toolKeys map[string][]string
	indexIDs map[int]string
	metrics  map[string]*editPreviewMetrics
}

func newEditPreviewManager(service *Service, opts RunOptions) *editPreviewManager {
	return &editPreviewManager{
		service: service, userID: opts.UserID, runID: opts.RunID, opts: opts,
		byKey: map[string]*activeEditPreview{}, toolKeys: map[string][]string{}, indexIDs: map[int]string{},
		metrics: map[string]*editPreviewMetrics{},
	}
}

func (m *editPreviewManager) handle(ctx context.Context, delta ToolCallArgumentsDelta, force bool) {
	if m == nil || !isEditPreviewTool(delta.ToolName) || strings.TrimSpace(delta.ArgumentsJSON) == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	toolID := strings.TrimSpace(delta.ToolCallID)
	if toolID == "" {
		toolID = m.indexIDs[delta.Index]
	}
	if toolID == "" {
		toolID = fmt.Sprintf("stream_tool_%d", delta.Index)
	}
	m.indexIDs[delta.Index] = toolID
	metrics := m.metrics[toolID]
	if metrics == nil {
		metrics = &editPreviewMetrics{}
		m.metrics[toolID] = metrics
	}
	metrics.RawArgumentDeltas++

	snapshots := m.buildSnapshots(ctx, delta.ToolName, delta.ArgumentsJSON)
	if len(snapshots) > 0 {
		metrics.ParseableDeltas++
	}
	now := time.Now()
	for _, snapshot := range snapshots {
		key := toolID + "\x00" + snapshot.Path
		current := m.byKey[key]
		if current != nil && current.Content == snapshot.Content {
			metrics.DuplicateSnapshots++
			continue
		}
		structuralChange := current != nil && len(snapshot.Changes) != current.ChangeCount
		if current != nil && !force {
			sinceEmit := now.Sub(current.LastEmit)
			argumentGrowth := len(delta.ArgumentsJSON) - current.ArgumentsSize
			if sinceEmit < editPreviewMinimumInterval ||
				(!structuralChange && argumentGrowth < editPreviewMinimumArgumentGap && sinceEmit < editPreviewMaximumInterval) {
				metrics.ThrottledDeltas++
				continue
			}
		}
		eventType := "workspace_edit_preview_updated"
		if current == nil {
			current = &activeEditPreview{
				PreviewID: stableEditPreviewID(m.runID, toolID, snapshot.Path),
				ToolID:    toolID, Path: snapshot.Path,
				Metrics: metrics,
			}
			m.byKey[key] = current
			m.toolKeys[toolID] = append(m.toolKeys[toolID], key)
			eventType = "workspace_edit_preview_started"
		}
		baseRevision := current.Revision
		changedBlocks, removedBlocks := diffPreviewChanges(current.Changes, snapshot.Changes)
		current.Revision++
		current.Content = snapshot.Content
		current.LastEmit = now
		current.ArgumentsSize = len(delta.ArgumentsJSON)
		current.ChangeCount = len(snapshot.Changes)
		current.Changes = snapshot.Changes
		contentHash := previewContentHash(snapshot.Content)
		current.ContentHash = contentHash
		fullPayload := map[string]any{
			"preview_id": current.PreviewID, "tool_call_id": toolID, "run_id": m.runID,
			"mutation_id": previewMutationID(m.runID, toolID),
			"path":        snapshot.Path, "base_hash": snapshot.BaseHash, "revision": current.Revision,
			"base_revision":    baseRevision,
			"document_version": snapshot.DocumentVersion,
			"operation":        snapshot.Operation, "changes": snapshot.Changes, "status": "previewing",
			"changed_blocks": snapshot.Changes, "removed_blocks": []string{},
			"content_hash": contentHash, "snapshot_full": true, "final": force,
			"is_new": snapshot.IsNew, "deleted": snapshot.Deleted,
			"file_type": detectPreviewFileType(snapshot.Path),
		}
		if snapshot.Operation != "edit_note_blocks" {
			fullPayload["content"] = snapshot.Content
		}
		transientPayload := clonePreviewPayload(fullPayload)
		if eventType == "workspace_edit_preview_updated" && snapshot.Operation == "edit_note_blocks" {
			transientPayload["changes"] = changedBlocks
			transientPayload["changed_blocks"] = changedBlocks
			transientPayload["removed_blocks"] = removedBlocks
			transientPayload["snapshot_full"] = false
		}
		if raw, err := json.Marshal(transientPayload); err == nil {
			current.BroadcastBytes += int64(len(raw))
		}
		emitTransientToolEvent(m.opts, eventType, "info", transientPayload)
		emitLatestEditPreview(m.opts, eventType, "info", fullPayload)
		if force || structuralChange || current.LastCheckpoint.IsZero() || now.Sub(current.LastCheckpoint) >= editPreviewCheckpointInterval {
			if err := checkpointEditPreview(ctx, m.opts, eventType, fullPayload); err != nil {
				logTask(m.opts, "warn", "failed to checkpoint edit preview: "+err.Error())
			} else {
				current.LastCheckpoint = now
				current.CheckpointCount++
			}
		}
	}
}

func stableEditPreviewID(runID, toolCallID, path string) string {
	raw := strings.Join([]string{
		strings.TrimSpace(runID),
		strings.TrimSpace(toolCallID),
		normalizeWorkspaceEventPath(path),
	}, "\x00")
	sum := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("preview_%x", sum[:12])
}

func (m *editPreviewManager) complete(toolID string, event ToolEvent) {
	m.finish(toolID, "workspace_edit_preview_completed", "completed", "", event)
}

func (m *editPreviewManager) revert(toolID, reason string) {
	m.finish(toolID, "workspace_edit_preview_reverted", "reverted", reason, ToolEvent{})
}

func (m *editPreviewManager) revertAll(reason string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	toolIDs := make([]string, 0, len(m.toolKeys))
	for toolID := range m.toolKeys {
		toolIDs = append(toolIDs, toolID)
	}
	m.mu.Unlock()
	for _, toolID := range toolIDs {
		m.revert(toolID, reason)
	}
}

func (m *editPreviewManager) finish(toolID, eventType, status, reason string, event ToolEvent) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := append([]string(nil), m.toolKeys[toolID]...)
	for _, key := range keys {
		current := m.byKey[key]
		if current == nil {
			continue
		}
		current.Revision++
		payload := map[string]any{
			"preview_id": current.PreviewID, "tool_call_id": toolID, "run_id": m.runID,
			"mutation_id": previewMutationID(m.runID, toolID),
			"path":        current.Path, "revision": current.Revision, "status": status,
			"operation": event.Operation, "hash": event.Hash, "version_id": event.VersionID,
			"document_version":          event.DocumentVersion,
			"changes":                   current.Changes,
			"changed_blocks":            current.Changes,
			"removed_blocks":            []string{},
			"content_hash":              current.ContentHash,
			"snapshot_full":             true,
			"final":                     true,
			"preview_updates":           current.Revision - 1,
			"preview_broadcast_bytes":   current.BroadcastBytes,
			"preview_checkpoint_count":  current.CheckpointCount,
			"raw_argument_deltas":       current.Metrics.RawArgumentDeltas,
			"parseable_argument_deltas": current.Metrics.ParseableDeltas,
			"throttled_argument_deltas": current.Metrics.ThrottledDeltas,
			"duplicate_snapshots":       current.Metrics.DuplicateSnapshots,
		}
		if reason != "" {
			payload["reason"] = reason
		}
		if err := checkpointEditPreview(context.Background(), m.opts, eventType, payload); err != nil {
			logTask(m.opts, "warn", "failed to clear edit preview checkpoint: "+err.Error())
		}
		emitLatestEditPreview(m.opts, eventType, eventLevel(event.Status), payload)
		emitToolLifecycleEvent(m.opts, eventType, eventLevel(event.Status), payload)
		delete(m.byKey, key)
	}
	delete(m.toolKeys, toolID)
	delete(m.metrics, toolID)
}

func previewMutationID(runID, toolCallID string) string {
	if strings.TrimSpace(runID) == "" {
		return strings.TrimSpace(toolCallID)
	}
	return strings.TrimSpace(runID) + ":" + strings.TrimSpace(toolCallID)
}

func previewContentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", sum[:])
}

func clonePreviewPayload(payload map[string]any) map[string]any {
	clone := make(map[string]any, len(payload))
	for key, value := range payload {
		clone[key] = value
	}
	return clone
}

func diffPreviewChanges(previous, next []map[string]any) ([]map[string]any, []string) {
	previousByID := make(map[string]map[string]any, len(previous))
	nextByID := make(map[string]map[string]any, len(next))
	for _, change := range previous {
		previousByID[previewChangeID(change)] = change
	}
	changed := make([]map[string]any, 0, len(next))
	for _, change := range next {
		id := previewChangeID(change)
		nextByID[id] = change
		before, exists := previousByID[id]
		if !exists || !previewChangesEqual(before, change) {
			changed = append(changed, change)
		}
	}
	removed := make([]string, 0)
	for id := range previousByID {
		if _, exists := nextByID[id]; !exists {
			removed = append(removed, id)
		}
	}
	sort.Strings(removed)
	return changed, removed
}

func previewChangeID(change map[string]any) string {
	return strings.TrimSpace(stringFromAny(change["block_id"]))
}

func previewChangesEqual(left, right map[string]any) bool {
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	return string(leftJSON) == string(rightJSON)
}

func emitTransientToolEvent(opts RunOptions, eventType, level string, payload map[string]any) {
	if opts.TransientEvent != nil {
		opts.TransientEvent(eventType, level, payload)
	}
}

func emitLatestEditPreview(opts RunOptions, eventType, level string, payload map[string]any) {
	if opts.EditPreviewLatest != nil {
		opts.EditPreviewLatest(eventType, level, payload)
	}
}

func checkpointEditPreview(ctx context.Context, opts RunOptions, eventType string, payload map[string]any) error {
	if opts.EditPreviewCheckpoint == nil {
		return nil
	}
	return opts.EditPreviewCheckpoint(ctx, eventType, payload)
}

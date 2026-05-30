package ai

import (
	"encoding/json"
	"fmt"
	"strings"
)

type chunkWriteArgs struct {
	Path       string `json:"path"`
	Content    string `json:"content"`
	Mode       string `json:"mode"`
	ChunkIndex int    `json:"chunk_index"`
}

type finalizeWriteArgs struct {
	Path           string `json:"path"`
	ExpectedChunks int    `json:"expected_chunks"`
	ExpectedLines  int    `json:"expected_lines"`
	ExpectedChars  int    `json:"expected_chars"`
	FinalHash      string `json:"final_hash"`
}

func (r *agentRuntime) validateChunkWriteToolCall(call agentToolCall) string {
	if r != nil && isWorkspaceMutationTool(call.Name) {
		if _, ok := r.appliedMutations[mutationOperationHash(call.Name, call.ArgumentsJSON)]; ok {
			return ""
		}
	}
	switch call.Name {
	case "write_workspace_chunk":
		args, err := parseChunkWriteArgs(call.ArgumentsJSON)
		if err != nil {
			return ""
		}
		path := normalizeWorkspaceEventPath(args.Path)
		mode := strings.ToLower(strings.TrimSpace(args.Mode))
		if r.chunkWrites == nil {
			r.chunkWrites = map[string]workspaceChunkWriteState{}
		}
		state, exists := r.chunkWrites[path]
		switch mode {
		case "create", "overwrite":
			if args.ChunkIndex != 1 {
				return "invalid tool arguments: create/overwrite chunk_index must be 1"
			}
			if exists && state.ChunksWritten > 0 {
				return fmt.Sprintf("invalid tool arguments: chunked write for %q already started; continue with mode=append and next chunk_index", path)
			}
		case "append":
			if !exists || state.ChunksWritten == 0 {
				return fmt.Sprintf("invalid tool arguments: append chunk for %q requires a prior create or overwrite chunk in this run", path)
			}
			expected := state.ChunksWritten + 1
			if args.ChunkIndex != expected {
				return fmt.Sprintf("invalid tool arguments: chunk_index must be %d for %q", expected, path)
			}
		}
	case "finalize_workspace_write":
		args, err := parseFinalizeWriteArgs(call.ArgumentsJSON)
		if err != nil {
			return ""
		}
		path := normalizeWorkspaceEventPath(args.Path)
		state, exists := r.chunkWrites[path]
		if !exists || state.ChunksWritten == 0 {
			return fmt.Sprintf("write finalization failed: no chunked write state for %q in this run", path)
		}
		if args.ExpectedChunks != state.ChunksWritten {
			return fmt.Sprintf("write finalization failed: expected_chunks mismatch (actual_chunks=%d expected_chunks=%d)", state.ChunksWritten, args.ExpectedChunks)
		}
		if args.ExpectedChars != state.TotalChars {
			return fmt.Sprintf("write finalization failed: expected_chars mismatch (actual_chars=%d expected_chars=%d)", state.TotalChars, args.ExpectedChars)
		}
	}
	return ""
}

func (r *agentRuntime) recordChunkWriteSuccess(call agentToolCall, event *ToolEvent) {
	if r == nil || event == nil {
		return
	}
	switch call.Name {
	case "write_workspace_chunk":
		args, err := parseChunkWriteArgs(call.ArgumentsJSON)
		if err != nil {
			return
		}
		path := normalizeWorkspaceEventPath(firstNonEmpty(event.Path, args.Path))
		if path == "" {
			return
		}
		if r.chunkWrites == nil {
			r.chunkWrites = map[string]workspaceChunkWriteState{}
		}
		state := r.chunkWrites[path]
		state.Path = path
		state.ChunksWritten = args.ChunkIndex
		state.TotalChars += len([]rune(args.Content))
		state.CurrentHash = event.Hash
		state.VersionID = event.VersionID
		r.chunkWrites[path] = state
		eventPayloadPatchChunkState(event, state)
	case "finalize_workspace_write":
		args, err := parseFinalizeWriteArgs(call.ArgumentsJSON)
		if err != nil {
			return
		}
		path := normalizeWorkspaceEventPath(firstNonEmpty(event.Path, args.Path))
		if path != "" {
			delete(r.chunkWrites, path)
		}
	}
}

func (r *agentRuntime) annotateChunkWriteResult(call agentToolCall, result string) string {
	if r == nil || call.Name != "write_workspace_chunk" {
		return result
	}
	args, err := parseChunkWriteArgs(call.ArgumentsJSON)
	if err != nil {
		return result
	}
	path := normalizeWorkspaceEventPath(args.Path)
	state, ok := r.chunkWrites[path]
	if !ok {
		return result
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		return result
	}
	payload["chunks_written"] = state.ChunksWritten
	payload["total_chars"] = state.TotalChars
	payload["current_hash"] = state.CurrentHash
	payload["needs_finalize"] = true
	payload["next_hint"] = "Continue with write_workspace_chunk mode=append for the next chunk, then call finalize_workspace_write before attempt_completion."
	return mustJSON(payload)
}

func eventPayloadPatchChunkState(event *ToolEvent, state workspaceChunkWriteState) {
	if event == nil {
		return
	}
	if event.Path == "" {
		event.Path = state.Path
	}
	if event.Hash == "" {
		event.Hash = state.CurrentHash
	}
}

func parseChunkWriteArgs(argumentsJSON string) (chunkWriteArgs, error) {
	var args chunkWriteArgs
	err := json.Unmarshal([]byte(argumentsJSON), &args)
	return args, err
}

func parseFinalizeWriteArgs(argumentsJSON string) (finalizeWriteArgs, error) {
	var args finalizeWriteArgs
	err := json.Unmarshal([]byte(argumentsJSON), &args)
	return args, err
}

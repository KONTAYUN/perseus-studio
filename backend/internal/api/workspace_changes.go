package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"webide-backend/internal/idgen"
	"webide-backend/internal/workspace"
)

type workspaceChangeRequest struct {
	ToolCallID string `json:"tool_call_id"`
	Index      int    `json:"index"`
}

func (s *Server) handleWorkspaceChanges(w http.ResponseWriter, r *http.Request) {
	user := MustUserFromContext(r.Context())
	var req workspaceChangeRequest
	switch r.Method {
	case http.MethodGet:
		req.ToolCallID = r.URL.Query().Get("tool_call_id")
		index, err := strconv.Atoi(r.URL.Query().Get("index"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid change index")
			return
		}
		req.Index = index
	case http.MethodPost:
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
	default:
		http.NotFound(w, r)
		return
	}
	// Resolve all paths, hashes and snapshots from the authenticated ledger.
	call, err := s.conversations.GetToolCall(r.Context(), user.UserID, req.ToolCallID)
	if err != nil || call.Status != "success" {
		writeError(w, http.StatusNotFound, "change record not found")
		return
	}
	var payload struct {
		Changes []workspace.FileChange `json:"changes"`
	}
	if err := json.Unmarshal([]byte(call.ResultJSON), &payload); err != nil || req.Index < 0 || req.Index >= len(payload.Changes) {
		writeError(w, http.StatusBadRequest, "change is unavailable")
		return
	}
	files, err := s.factory.Workspace(user.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	operationID := fmt.Sprintf("%s:%d", call.ID, req.Index)
	change := payload.Changes[req.Index]
	if r.Method == http.MethodGet {
		preview, err := files.PreviewChange(r.Context(), operationID, change)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, preview)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	jobID := idgen.New("revert")
	type outcome struct {
		change workspace.FileChange
		err    error
	}
	completed := make(chan outcome, 1)
	err = s.runQueue.Submit(jobID, user.UserID, func(jobCtx context.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				completed <- outcome{err: fmt.Errorf("撤销异常，请重新查看变更状态: %v", recovered)}
			}
		}()
		reverted, err := files.RevertChange(jobCtx, operationID, change)
		completed <- outcome{reverted, err}
	})
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	select {
	case <-ctx.Done():
		s.runQueue.Cancel(jobID)
		writeError(w, http.StatusRequestTimeout, "撤销等待超时，请重新查看变更状态")
		return
	case result := <-completed:
		if result.err != nil {
			writeError(w, http.StatusConflict, result.err.Error())
			return
		}
		inverse := result.change
		paths := []string{inverse.Path}
		var moved []map[string]string
		if inverse.Operation == "move" {
			paths = append(paths, inverse.OldPath)
			moved = append(moved, map[string]string{"old_path": inverse.OldPath, "new_path": inverse.Path, "status": "success"})
		} else if inverse.Operation == "delete" {
			paths = append(paths, inverse.TrashPath)
			moved = append(moved, map[string]string{"old_path": inverse.Path, "new_path": inverse.TrashPath, "status": "success"})
		}
		writeJSON(w, http.StatusOK, map[string]any{"type": "workspace_file_changed", "tool_name": "apply_workspace_patch", "tool_call_id": "revert:" + operationID, "status": "success", "operation": "patch", "changes": []workspace.FileChange{inverse}, "changed_paths": paths, "moved": moved, "path": inverse.Path, "hash": inverse.Hash})
	}
}

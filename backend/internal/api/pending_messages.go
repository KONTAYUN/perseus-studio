package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"webide-backend/internal/store"
)

func (s *Server) handlePendingMessages(w http.ResponseWriter, r *http.Request) {
	user := MustUserFromContext(r.Context())
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/pending-messages"), "/")
	parts := strings.Split(path, "/")
	if path == "" {
		switch r.Method {
		case http.MethodGet:
			items, err := s.runs.ListPendingMessages(r.Context(), user.UserID)
			if err != nil {
				writeError(w, 500, err.Error())
				return
			}
			writeJSON(w, 200, map[string]any{"items": items})
		case http.MethodPost:
			var p store.PendingMessage
			if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
				writeError(w, 400, "invalid json")
				return
			}
			if _, err := s.configs.GetModelProviderForUser(r.Context(), user.UserID, user.Role, p.ModelProviderID); err != nil {
				writeError(w, 400, "selected model is not available")
				return
			}
			if err := s.checkDocumentAction(r.Context(), user.UserID, p.DocumentAction); err != nil {
				writeError(w, 409, err.Error())
				return
			}
			if p.DocumentAction != nil {
				p.Content = p.DocumentAction.Prompt()
			}
			saved, err := s.runs.SavePendingMessage(r.Context(), user.UserID, p)
			if err != nil {
				writeRunPreparationError(w, err)
				return
			}
			writeJSON(w, 200, saved)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
		return
	}
	if len(parts) == 1 && r.Method == http.MethodDelete {
		if err := s.runs.DeletePendingMessage(r.Context(), user.UserID, parts[0]); err != nil {
			if errors.Is(err, store.ErrPendingAlreadySent) {
				writeError(w, http.StatusConflict, err.Error())
				return
			}
			writeError(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
		return
	}
	if len(parts) != 2 || parts[1] != "send" || r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	p, err := s.runs.GetPendingMessage(r.Context(), user.UserID, id)
	if errors.Is(err, sql.ErrNoRows) {
		// An accepted request may lose its HTTP response. Recover its identity;
		// never reconstruct or replay the input as a new request.
		run, lookupErr := s.runs.RunByRequestID(r.Context(), user.UserID, "pending:"+id)
		if lookupErr != nil {
			writeError(w, 404, "pending message not found")
			return
		}
		message, err := s.conversations.GetMessage(r.Context(), user.UserID, run.UserMessageID.String)
		if err != nil {
			writeError(w, 500, err.Error())
			return
		}
		s.writePendingRun(w, r, user, store.PreparedConversationRun{Run: run, Message: message, Replayed: true})
		return
	}
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	if _, err := s.configs.GetModelProviderForUser(r.Context(), user.UserID, user.Role, p.ModelProviderID); err != nil {
		writeError(w, 400, "selected model is not available; pending message retained")
		return
	}
	prepared, err := s.runs.PrepareConversationRun(r.Context(), store.PrepareConversationRunInput{
		MessageMetaJSON: p.DocumentAction.MetaJSON(),
		UserID:          user.UserID, ConversationID: p.ConversationID, ModelProviderID: p.ModelProviderID,
		Content: p.Content, AttachmentsJSON: "[]", ThinkingMode: p.ThinkingMode, RequestID: "pending:" + p.ID, PendingMessageID: p.ID,
	})
	if err != nil {
		writeRunPreparationError(w, err)
		return
	}
	s.writePendingRun(w, r, user, prepared)
}

func (s *Server) writePendingRun(w http.ResponseWriter, r *http.Request, user SessionInfo, prepared store.PreparedConversationRun) {
	run, message := prepared.Run, prepared.Message
	if run.Status == store.AgentRunQueued && !s.runQueue.Contains(run.ID) {
		// The original provider and thinking mode are recovered from committed
		// request data on replay, rather than the currently selected UI model.
		thinkingMode, err := s.runs.PendingRunThinkingMode(r.Context(), user.UserID, run.ID)
		if err != nil {
			writeError(w, 500, err.Error())
			return
		}
		if err := s.scheduleConversationRun(user.UserID, user.Role, run.ConversationID, message.ID, run.ModelProviderID, message.Content, message.AttachmentsJSON, nil, thinkingMode, run.ID); err != nil {
			writeError(w, 503, err.Error())
			return
		}
	}
	conversation, _ := s.conversations.GetConversation(r.Context(), user.UserID, run.ConversationID)
	writeJSON(w, http.StatusAccepted, map[string]any{"conversation": conversation, "user_message": message, "run": run})
}

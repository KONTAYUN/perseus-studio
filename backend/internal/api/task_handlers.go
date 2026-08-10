package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"webide-backend/internal/store"
	"webide-backend/internal/tasks"
)

func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	user := MustUserFromContext(r.Context())
	switch r.Method {
	case http.MethodGet:
		items, err := s.store.ListTasks(r.Context(), user.UserID, 100)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	case http.MethodPost:
		writeError(w, http.StatusGone, "manual test task creation has been removed")
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleTaskRoutes(w http.ResponseWriter, r *http.Request) {
	user := MustUserFromContext(r.Context())
	path := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	taskID := parts[0]

	if len(parts) == 1 && r.Method == http.MethodGet {
		task, err := s.store.GetTask(r.Context(), user.UserID, taskID)
		if err != nil {
			status := http.StatusInternalServerError
			if tasks.IsNotFound(err) {
				status = http.StatusNotFound
			}
			writeError(w, status, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, task)
		return
	}
	if len(parts) == 1 && r.Method == http.MethodDelete {
		task, err := s.store.GetTask(r.Context(), user.UserID, taskID)
		if err != nil {
			status := http.StatusInternalServerError
			if tasks.IsNotFound(err) {
				status = http.StatusNotFound
			}
			writeError(w, status, err.Error())
			return
		}
		if task.Status == store.TaskRunning || task.Status == store.TaskPending {
			writeError(w, http.StatusBadRequest, "running or pending task cannot be deleted; cancel it first")
			return
		}
		if err := s.store.DeleteTask(r.Context(), user.UserID, taskID); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": taskID})
		return
	}

	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}

	switch parts[1] {
	case "logs":
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		items, err := s.store.GetLogs(r.Context(), user.UserID, taskID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items})
	case "cancel":
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		task, err := s.store.RequestCancel(r.Context(), user.UserID, taskID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		logItem, err := s.store.AppendLog(r.Context(), user.UserID, taskID, "warn", "cancel requested")
		if err == nil {
			s.broker.Publish(taskID, tasks.Event{Type: "log", Log: logItem})
		}
		s.broker.Publish(taskID, tasks.Event{Type: "task", Task: task})
		writeJSON(w, http.StatusOK, task)
	case "stream":
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		s.handleTaskStream(w, r, taskID)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleTaskStream(w http.ResponseWriter, r *http.Request, taskID string) {
	user := MustUserFromContext(r.Context())
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	events, unsubscribe := s.broker.Subscribe(taskID)
	defer unsubscribe()
	task, err := s.store.GetTask(r.Context(), user.UserID, taskID)
	if err != nil {
		status := http.StatusInternalServerError
		if tasks.IsNotFound(err) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	cursor := streamCursor(r)
	if cursor > task.EventHeadSeq {
		cursor = 0
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	if writeSSE(w, "stream_opened", map[string]any{
		"task": task, "event_head_seq": task.EventHeadSeq, "final_seq": task.FinalSeq,
	}) != nil {
		return
	}
	drain := func() bool {
		logs, listErr := s.store.GetLogsAfter(r.Context(), user.UserID, taskID, cursor)
		if listErr != nil {
			return false
		}
		for _, item := range logs {
			if item.Seq != cursor+1 || writeSSEWithID(w, "log", strconv.FormatInt(item.Seq, 10), item) != nil {
				return false
			}
			cursor = item.Seq
		}
		return true
	}
	if !drain() {
		return
	}
	task, err = s.store.GetTask(r.Context(), user.UserID, taskID)
	if err != nil || writeSSE(w, "task", task) != nil ||
		writeSSE(w, "replay_complete", map[string]any{"through_seq": cursor, "final_seq": task.FinalSeq}) != nil {
		return
	}
	flusher.Flush()
	if taskIsTerminal(task.Status) && cursor >= task.FinalSeq {
		return
	}

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-events:
			if !drain() {
				return
			}
			task, err = s.store.GetTask(r.Context(), user.UserID, taskID)
			if err != nil || writeSSE(w, "task", task) != nil ||
				writeSSE(w, "replay_complete", map[string]any{"through_seq": cursor, "final_seq": task.FinalSeq}) != nil {
				return
			}
			flusher.Flush()
			if taskIsTerminal(task.Status) && cursor >= task.FinalSeq {
				return
			}
		case <-heartbeat.C:
			if !drain() {
				return
			}
			if err := writeSSEHeartbeat(w); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func taskIsTerminal(status store.TaskStatus) bool {
	return status == store.TaskDone || status == store.TaskFailed || status == store.TaskCanceled
}

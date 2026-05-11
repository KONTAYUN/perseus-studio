package api

import (
	"fmt"
	"net/http"
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

	task, err := s.store.GetTask(r.Context(), user.UserID, taskID)
	if err != nil {
		status := http.StatusInternalServerError
		if tasks.IsNotFound(err) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	logs, err := s.store.GetLogs(r.Context(), user.UserID, taskID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	writeSSE(w, "task", task)
	for _, item := range logs {
		writeSSE(w, "log", item)
	}
	flusher.Flush()

	events, unsubscribe := s.broker.Subscribe(taskID)
	defer unsubscribe()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-events:
			switch event.Type {
			case "task":
				writeSSE(w, "task", event.Task)
			case "log":
				writeSSE(w, "log", event.Log)
			}
			flusher.Flush()
		case <-heartbeat.C:
			fmt.Fprint(w, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
}

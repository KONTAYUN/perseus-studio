package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"webide-backend/internal/store"
)

type diagnosticUserSummary struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Disabled bool   `json:"disabled"`
}

func (s *Server) handleDiagnosticLogs(w http.ResponseWriter, r *http.Request) {
	user := MustUserFromContext(r.Context())
	switch r.Method {
	case http.MethodGet:
		logs, err := s.diagnostics.List(r.Context(), diagnosticQueryFromRequest(r, user.UserID, 50))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"logs": logs})
	case http.MethodDelete:
		if err := s.diagnostics.ClearUser(r.Context(), user.UserID); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleAdminDiagnosticLogs(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(r) {
		writeError(w, http.StatusForbidden, "admin required")
		return
	}
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	query := diagnosticQueryFromRequest(r, "", 200)
	query.AllUsers = true
	logs, err := s.diagnostics.List(r.Context(), query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	users, err := s.diagnosticUserSummaries(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"logs": logs, "users": users})
}

func (s *Server) handleDiagnosticExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	user := MustUserFromContext(r.Context())
	logs, err := s.diagnostics.List(r.Context(), diagnosticQueryFromRequest(r, user.UserID, 100))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	limit := parseLimit(r.URL.Query().Get("limit"), 100)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="perseus-diagnostic-logs.txt"`)
	_, _ = w.Write([]byte(formatDiagnosticExport(user.UserID, limit, logs)))
}

func (s *Server) handleAdminDiagnosticExport(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(r) {
		writeError(w, http.StatusForbidden, "admin required")
		return
	}
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	query := diagnosticQueryFromRequest(r, "", 200)
	query.AllUsers = true
	logs, err := s.diagnostics.List(r.Context(), query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	users, err := s.diagnosticUserSummaries(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	limit := parseLimit(r.URL.Query().Get("limit"), 200)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="perseus-diagnostic-logs-all-users.txt"`)
	_, _ = w.Write([]byte(formatAdminDiagnosticExport(limit, logs, users)))
}

func diagnosticQueryFromRequest(r *http.Request, userID string, defaultLimit int) store.DiagnosticLogQuery {
	var since *time.Time
	if raw := strings.TrimSpace(r.URL.Query().Get("since")); raw != "" {
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			since = &parsed
		}
	}
	return store.DiagnosticLogQuery{
		UserID: userID,
		Limit:  parseLimit(r.URL.Query().Get("limit"), defaultLimit),
		Level:  r.URL.Query().Get("level"),
		Since:  since,
	}
}

func parseLimit(raw string, fallback int) int {
	limit, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || limit <= 0 {
		limit = fallback
	}
	if limit > 200 {
		return 200
	}
	return limit
}

func (s *Server) diagnosticUserSummaries(ctx context.Context) (map[string]diagnosticUserSummary, error) {
	items := make(map[string]diagnosticUserSummary)
	if s.users == nil {
		return items, nil
	}
	users, err := s.users.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	for _, user := range users {
		items[user.ID] = diagnosticUserSummary{
			ID:       user.ID,
			Username: user.Username,
			Role:     user.Role,
			Disabled: user.Disabled,
		}
	}
	return items, nil
}

func formatDiagnosticExport(userID string, limit int, logs []store.DiagnosticLog) string {
	var b strings.Builder
	b.WriteString("Perseus Studio Diagnostic Logs\n")
	b.WriteString("user_id: " + userID + "\n")
	b.WriteString("exported_at: " + time.Now().UTC().Format(time.RFC3339) + "\n")
	b.WriteString(fmt.Sprintf("limit: %d\n\n", limit))
	for _, item := range logs {
		writeDiagnosticExportItem(&b, item, nil)
	}
	return b.String()
}

func formatAdminDiagnosticExport(limit int, logs []store.DiagnosticLog, users map[string]diagnosticUserSummary) string {
	var b strings.Builder
	b.WriteString("Perseus Studio Diagnostic Logs\n")
	b.WriteString("scope: all_users\n")
	b.WriteString("exported_at: " + time.Now().UTC().Format(time.RFC3339) + "\n")
	b.WriteString(fmt.Sprintf("limit: %d\n\n", limit))
	for _, item := range logs {
		writeDiagnosticExportItem(&b, item, users)
	}
	return b.String()
}

func writeDiagnosticExportItem(b *strings.Builder, item store.DiagnosticLog, users map[string]diagnosticUserSummary) {
	b.WriteString(fmt.Sprintf("[%s] %s\n", item.CreatedAt.Local().Format("2006-01-02 15:04:05"), strings.ToUpper(item.Level)))
	if users != nil {
		writeExportField(b, "user_id", item.UserID)
		if user, ok := users[item.UserID]; ok {
			writeExportField(b, "username", user.Username)
			writeExportField(b, "user_role", user.Role)
			if user.Disabled {
				b.WriteString("user_disabled: true\n")
			}
		}
	}
	writeExportField(b, "id", item.ID)
	writeExportField(b, "stage", item.Stage)
	writeExportField(b, "conversation_id", item.ConversationID)
	writeExportField(b, "message_id", item.MessageID)
	writeExportField(b, "task_id", item.TaskID)
	writeExportField(b, "model_provider_id", item.ModelProviderID)
	writeExportField(b, "provider", item.ProviderName)
	writeExportField(b, "protocol", item.Protocol)
	writeExportField(b, "model", firstNonEmpty(item.ModelName, item.ModelID))
	if item.HTTPStatus > 0 {
		b.WriteString(fmt.Sprintf("http_status: %d\n", item.HTTPStatus))
	}
	writeExportField(b, "content_type", item.ContentType)
	writeExportField(b, "base_url_host", item.BaseURLHost)
	writeExportField(b, "final_url_path", item.FinalURLPath)
	if item.DurationMS > 0 {
		b.WriteString(fmt.Sprintf("duration_ms: %d\n", item.DurationMS))
	}
	writeExportField(b, "error", item.ErrorMessage)
	if item.ResponseSnippet != "" {
		b.WriteString("response_snippet:\n")
		b.WriteString(item.ResponseSnippet)
		b.WriteString("\n")
	}
	if item.RequestSummary != "" {
		b.WriteString("request_summary:\n")
		b.WriteString(item.RequestSummary)
		b.WriteString("\n")
	}
	b.WriteString("\n---\n\n")
}

func writeExportField(b *strings.Builder, key, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	b.WriteString(key + ": " + value + "\n")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

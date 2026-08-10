package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"webide-backend/internal/ai"
	"webide-backend/internal/config"
	"webide-backend/internal/idgen"
	"webide-backend/internal/store"
	"webide-backend/internal/tasks"
	"webide-backend/internal/usersvc"
	"webide-backend/internal/version"
)

type SessionInfo struct {
	UserID   string
	Username string
	Role     string
	Expires  time.Time
}

type Server struct {
	store                *store.TaskStore
	runs                 *store.AgentRunStore
	configs              *store.ConfigStore
	conversations        *store.ConversationStore
	diagnostics          *store.DiagnosticStore
	usage                *store.UsageStore
	users                *store.UserStore
	sessions             *store.SessionStore
	broker               *tasks.Broker
	runBroker            *tasks.Broker
	factory              *usersvc.Factory
	ai                   *ai.Service
	cfg                  config.Config
	mux                  *http.ServeMux
	runCancelMu          sync.Mutex
	runCancels           map[string]*runCancelEntry
	allowedOrigins       map[string]bool
	trustedProxyNetworks []*net.IPNet
	loginLimiter         *tokenBucketLimiter
	registerLimiter      *fixedWindowLimiter
	uploadLimiter        *fixedWindowLimiter
	runScheduler         *agentRunScheduler
}

type runCancelEntry struct {
	cancel      context.CancelFunc
	source      string
	reason      string
	requestedAt time.Time
}

type pathRequest struct {
	Path string `json:"path"`
}

type searchFilesRequest struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

type writeFileRequest struct {
	Path                string `json:"path"`
	Content             string `json:"content"`
	BaseDocumentVersion string `json:"base_document_version"`
	UserRevision        int64  `json:"user_revision"`
	SaveRequestID       string `json:"save_request_id"`
}

type renameRequest struct {
	OldPath string `json:"old_path"`
	NewPath string `json:"new_path"`
}

type reorderRequest struct {
	Path             string `json:"path"`
	TargetParentPath string `json:"target_parent_path"`
	PreviousPath     string `json:"previous_path"`
	NextPath         string `json:"next_path"`
}

type restoreTrashRequest struct {
	Path      string `json:"path"`
	Overwrite bool   `json:"overwrite"`
}

type restoreFileVersionRequest struct {
	Path      string `json:"path"`
	VersionID string `json:"version_id"`
}

type deleteFileVersionRequest struct {
	Path      string `json:"path"`
	VersionID string `json:"version_id"`
}

type createConversationRequest struct {
	Title                   string `json:"title"`
	SelectedModelProviderID string `json:"selected_model_provider_id"`
}

type updateConversationRequest struct {
	Title string `json:"title"`
}

type createConversationMessageRequest struct {
	Content         string          `json:"content"`
	AttachmentsJSON json.RawMessage `json:"attachments"`
	AttachmentIDs   []string        `json:"attachment_ids"`
	ModelProviderID string          `json:"model_provider_id"`
	ThinkingMode    string          `json:"thinking_mode"`
}

type branchConversationRequest struct {
	MessageID string `json:"message_id"`
}

type regenerateConversationRequest struct {
	MessageID    string `json:"message_id"`
	ThinkingMode string `json:"thinking_mode"`
}

type compactConversationRequest struct {
	Instructions string `json:"instructions"`
}

func (s *Server) requireAdminOrWriteForbidden(w http.ResponseWriter, r *http.Request) bool {
	if s.requireAdmin(r) {
		return true
	}
	writeError(w, http.StatusForbidden, "admin required")
	return false
}

type chatAttachmentMetadata struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Size              int64  `json:"size"`
	Type              string `json:"type"`
	Kind              string `json:"kind"`
	MIMEType          string `json:"mime_type,omitempty"`
	Width             int    `json:"width,omitempty"`
	Height            int    `json:"height,omitempty"`
	Status            string `json:"status,omitempty"`
	Available         bool   `json:"available"`
	ContentURL        string `json:"content_url,omitempty"`
	DownloadURL       string `json:"download_url,omitempty"`
	Path              string `json:"path,omitempty"`
	FileType          string `json:"file_type,omitempty"`
	SkillID           string `json:"skill_id,omitempty"`
	Description       string `json:"description,omitempty"`
	IncludedInContext bool   `json:"included_in_context"`
	Truncated         bool   `json:"truncated"`
	Note              string `json:"note,omitempty"`
}

func NewServer(taskStore *store.TaskStore, runStore *store.AgentRunStore, configStore *store.ConfigStore, conversationStore *store.ConversationStore, diagnosticStore *store.DiagnosticStore, usageStore *store.UsageStore, userStore *store.UserStore, sessionStore *store.SessionStore, broker *tasks.Broker, runBroker *tasks.Broker, factory *usersvc.Factory, aiService *ai.Service, cfg config.Config) *Server {
	allowedOrigins := make(map[string]bool, len(cfg.CORSAllowedOrigins))
	for _, origin := range cfg.CORSAllowedOrigins {
		origin = strings.ToLower(strings.TrimSpace(origin))
		if origin != "" {
			allowedOrigins[origin] = true
		}
	}
	trustedProxyNetworks := make([]*net.IPNet, 0, len(cfg.TrustedProxyCIDRs))
	for _, raw := range cfg.TrustedProxyCIDRs {
		_, network, err := net.ParseCIDR(strings.TrimSpace(raw))
		if err == nil {
			trustedProxyNetworks = append(trustedProxyNetworks, network)
		}
	}
	s := &Server{
		store:                taskStore,
		runs:                 runStore,
		configs:              configStore,
		conversations:        conversationStore,
		diagnostics:          diagnosticStore,
		usage:                usageStore,
		users:                userStore,
		sessions:             sessionStore,
		broker:               broker,
		runBroker:            runBroker,
		factory:              factory,
		ai:                   aiService,
		cfg:                  cfg,
		mux:                  http.NewServeMux(),
		runCancels:           make(map[string]*runCancelEntry),
		allowedOrigins:       allowedOrigins,
		trustedProxyNetworks: trustedProxyNetworks,
		loginLimiter:         newTokenBucketLimiter(),
		registerLimiter:      newFixedWindowLimiter(),
		uploadLimiter:        newFixedWindowLimiter(),
	}
	s.runScheduler = newAgentRunScheduler(s, cfg.AgentMaxConcurrentGlobal, cfg.AgentMaxConcurrentPerUser)
	s.register()
	return s
}

func (s *Server) Routes() http.Handler {
	return s.withSecurityHeaders(s.withCORS(s.withAuth(s.withCSRFOrigin(s.withRequestBodyLimits(s.withJSONRequestTimeout(s.mux))))))
}

func (s *Server) register() {
	s.mux.HandleFunc("/api/health", s.handleHealth)
	s.mux.HandleFunc("/api/ready", s.handleReady)
	s.mux.HandleFunc("/api/version", s.handleVersion)
	s.mux.HandleFunc("/api/auth/login", s.handleAuthLogin)
	s.mux.HandleFunc("/api/auth/register", s.handleAuthRegister)
	s.mux.HandleFunc("/api/auth/me", s.handleAuthMe)
	s.mux.HandleFunc("/api/auth/logout", s.handleAuthLogout)
	s.mux.HandleFunc("/api/admin/users", s.handleAdminUsers)
	s.mux.HandleFunc("/api/admin/users/", s.handleAdminUserRoutes)
	s.mux.HandleFunc("/api/admin/settings", s.handleAdminSettings)
	s.mux.HandleFunc("/api/admin/diagnostics/logs", s.handleAdminDiagnosticLogs)
	s.mux.HandleFunc("/api/admin/diagnostics/logs/", s.handleAdminDiagnosticLogRoutes)
	s.mux.HandleFunc("/api/admin/diagnostics/export", s.handleAdminDiagnosticExport)
	s.mux.HandleFunc("/api/admin/usage/summary", s.handleAdminUsageSummary)
	s.mux.HandleFunc("/api/admin/usage/events", s.handleAdminUsageEvents)
	s.mux.HandleFunc("/api/admin/usage/prices", s.handleAdminUsagePrices)
	s.mux.HandleFunc("/api/admin/usage/prices/", s.handleAdminUsagePriceRoutes)
	s.mux.HandleFunc("/api/files/tree", s.handleFilesTree)
	s.mux.HandleFunc("/api/files/search", s.handleFilesSearch)
	s.mux.HandleFunc("/api/files/read", s.handleFilesRead)
	s.mux.HandleFunc("/api/files/content", s.handleFilesContent)
	s.mux.HandleFunc("/api/files/history", s.handleFilesHistory)
	s.mux.HandleFunc("/api/files/history/read", s.handleFilesHistoryRead)
	s.mux.HandleFunc("/api/files/history/restore", s.handleFilesHistoryRestore)
	s.mux.HandleFunc("/api/files/history/delete", s.handleFilesHistoryDelete)
	s.mux.HandleFunc("/api/files/history/clear", s.handleFilesHistoryClear)
	s.mux.HandleFunc("/api/files/export", s.handleFilesExport)
	s.mux.HandleFunc("/api/files/import", s.handleFilesImport)
	s.mux.HandleFunc("/api/files/upload", s.handleFilesUpload)
	s.mux.HandleFunc("/api/files/write", s.handleFilesWrite)
	s.mux.HandleFunc("/api/files/create", s.handleFilesCreate)
	s.mux.HandleFunc("/api/files/mkdir", s.handleFilesMkdir)
	s.mux.HandleFunc("/api/files/rename", s.handleFilesRename)
	s.mux.HandleFunc("/api/files/reorder", s.handleFilesReorder)
	s.mux.HandleFunc("/api/files/delete", s.handleFilesDelete)
	s.mux.HandleFunc("/api/files/trash", s.handleFilesTrash)
	s.mux.HandleFunc("/api/files/trash/restore", s.handleFilesTrashRestore)
	s.mux.HandleFunc("/api/files/trash/delete", s.handleFilesTrashDelete)
	s.mux.HandleFunc("/api/files/empty-trash", s.handleFilesEmptyTrash)
	s.mux.HandleFunc("/api/model-providers/discover", s.handleDiscoverModelProviders)
	s.mux.HandleFunc("/api/model-providers/test", s.handleTestModelProvider)
	s.mux.HandleFunc("/api/model-providers", s.handleModelProviders)
	s.mux.HandleFunc("/api/model-providers/", s.handleModelProviderRoutes)
	s.mux.HandleFunc("/api/search-providers", s.handleSearchProviders)
	s.mux.HandleFunc("/api/search-providers/", s.handleSearchProviderRoutes)
	s.mux.HandleFunc("/api/skills", s.handleSkills)
	s.mux.HandleFunc("/api/skills/", s.handleSkillRoutes)
	s.mux.HandleFunc("/api/conversations", s.handleConversations)
	s.mux.HandleFunc("/api/conversations/", s.handleConversationRoutes)
	s.mux.HandleFunc("/api/chat-attachments", s.handleChatAttachments)
	s.mux.HandleFunc("/api/chat-attachments/", s.handleChatAttachmentRoutes)
	s.mux.HandleFunc("/api/agent-runs/", s.handleAgentRunRoutes)
	s.mux.HandleFunc("/api/diagnostics/logs", s.handleDiagnosticLogs)
	s.mux.HandleFunc("/api/diagnostics/logs/", s.handleDiagnosticLogRoutes)
	s.mux.HandleFunc("/api/diagnostics/export", s.handleDiagnosticExport)
	s.mux.HandleFunc("/api/tasks", s.handleTasks)
	s.mux.HandleFunc("/api/tasks/", s.handleTaskRoutes)
	s.mux.HandleFunc("/api/debug/model-tool-test", s.handleDebugModelToolTest)
	s.mux.HandleFunc("/", s.handleStatic)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":             true,
		"version":        version.Version,
		"worker_count":   s.cfg.WorkerCount,
		"user_data_root": s.cfg.UserDataRoot,
	})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	if s.runs == nil || s.runs.Ping(r.Context()) != nil {
		writeCodedError(w, http.StatusServiceUnavailable, "database_not_ready", "database is not ready")
		return
	}
	if s.factory == nil || s.factory.Ready() != nil {
		writeCodedError(w, http.StatusServiceUnavailable, "user_data_not_ready", "user data directory is not writable")
		return
	}
	if s.runScheduler == nil || !s.runScheduler.Ready() {
		writeCodedError(w, http.StatusServiceUnavailable, "agent_scheduler_not_ready", "agent run scheduler is not ready")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ready": true, "version": version.Version})
}

func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version":  version.Version,
		"commit":   version.Commit,
		"built_at": version.BuiltAt,
	})
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.NotFound(w, r)
		return
	}
	staticRoot := strings.TrimSpace(s.cfg.StaticRoot)
	if staticRoot == "" {
		http.NotFound(w, r)
		return
	}
	cleanPath := strings.TrimPrefix(filepath.Clean("/"+r.URL.Path), string(filepath.Separator))
	if cleanPath == "." {
		cleanPath = "index.html"
	}
	target := filepath.Join(staticRoot, filepath.FromSlash(cleanPath))
	info, err := os.Stat(target)
	if err != nil || info.IsDir() {
		target = filepath.Join(staticRoot, "index.html")
		info, err = os.Stat(target)
		if err != nil || info.IsDir() {
			http.NotFound(w, r)
			return
		}
	}
	http.ServeFile(w, r, target)
}

func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin != "" {
			if !s.originAllowed(r, origin) {
				writeCodedError(w, http.StatusForbidden, "cors_origin_rejected", "request origin is not allowed")
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Last-Event-ID")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PATCH,DELETE,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeSSE(w http.ResponseWriter, event string, payload any) error {
	return writeSSEWithID(w, event, "", payload)
}

func writeSSEWithID(w http.ResponseWriter, event, id string, payload any) error {
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(15 * time.Second))
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if id != "" {
		if _, err := fmt.Fprintf(w, "id: %s\n", id); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", raw)
	return err
}

func writeSSEHeartbeat(w http.ResponseWriter) error {
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(15 * time.Second))
	_, err := fmt.Fprint(w, ": heartbeat\n\n")
	return err
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": message})
}

func writeCodedError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": message, "code": code})
}

func writeAIError(w http.ResponseWriter, status int, err error) {
	payload := map[string]any{"error": ai.UserFacingErrorMessage(err)}
	if id := ai.DiagnosticIDFromError(err); id != "" {
		payload["diagnostic_log_id"] = id
	}
	metadata := ai.FailureMetadataFromError(err)
	if metadata.FailureClass != "" {
		payload["failure_class"] = metadata.FailureClass
		payload["retry_after_ms"] = metadata.RetryAfterMS
		if !metadata.ResetAt.IsZero() {
			payload["reset_at"] = metadata.ResetAt
		}
		payload["provider_id"] = metadata.ProviderID
		payload["model_id"] = metadata.ModelID
		payload["can_resume_now"] = metadata.CanResumeNow
		payload["recommended_action"] = metadata.RecommendedAction
	}
	writeJSON(w, status, payload)
}

func newTaskID() string {
	return idgen.New("task")
}

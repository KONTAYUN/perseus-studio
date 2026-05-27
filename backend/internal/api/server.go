package api

import (
	"context"
	"encoding/json"
	"fmt"
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
	store         *store.TaskStore
	runs          *store.AgentRunStore
	configs       *store.ConfigStore
	conversations *store.ConversationStore
	diagnostics   *store.DiagnosticStore
	users         *store.UserStore
	sessions      *store.SessionStore
	broker        *tasks.Broker
	runBroker     *tasks.Broker
	factory       *usersvc.Factory
	ai            *ai.Service
	cfg           config.Config
	mux           *http.ServeMux
	runCancelMu   sync.Mutex
	runCancels    map[string]*runCancelEntry
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
	Path    string `json:"path"`
	Content string `json:"content"`
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
	Path              string `json:"path,omitempty"`
	FileType          string `json:"file_type,omitempty"`
	SkillID           string `json:"skill_id,omitempty"`
	Description       string `json:"description,omitempty"`
	IncludedInContext bool   `json:"included_in_context"`
	Truncated         bool   `json:"truncated"`
	Note              string `json:"note,omitempty"`
}

func NewServer(taskStore *store.TaskStore, runStore *store.AgentRunStore, configStore *store.ConfigStore, conversationStore *store.ConversationStore, diagnosticStore *store.DiagnosticStore, userStore *store.UserStore, sessionStore *store.SessionStore, broker *tasks.Broker, runBroker *tasks.Broker, factory *usersvc.Factory, aiService *ai.Service, cfg config.Config) *Server {
	s := &Server{
		store:         taskStore,
		runs:          runStore,
		configs:       configStore,
		conversations: conversationStore,
		diagnostics:   diagnosticStore,
		users:         userStore,
		sessions:      sessionStore,
		broker:        broker,
		runBroker:     runBroker,
		factory:       factory,
		ai:            aiService,
		cfg:           cfg,
		mux:           http.NewServeMux(),
		runCancels:    make(map[string]*runCancelEntry),
	}
	s.register()
	return s
}

func (s *Server) Routes() http.Handler {
	return s.withCORS(s.withAuth(s.mux))
}

func (s *Server) register() {
	s.mux.HandleFunc("/api/health", s.handleHealth)
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
	s.mux.HandleFunc("/api/files/tree", s.handleFilesTree)
	s.mux.HandleFunc("/api/files/search", s.handleFilesSearch)
	s.mux.HandleFunc("/api/files/read", s.handleFilesRead)
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
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PATCH,DELETE,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeSSE(w http.ResponseWriter, event string, payload any) {
	raw, _ := json.Marshal(payload)
	fmt.Fprintf(w, "event: %s\n", event)
	fmt.Fprintf(w, "data: %s\n\n", raw)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": message})
}

func writeAIError(w http.ResponseWriter, status int, err error) {
	payload := map[string]any{"error": ai.UserFacingErrorMessage(err)}
	if id := ai.DiagnosticIDFromError(err); id != "" {
		payload["diagnostic_log_id"] = id
	}
	writeJSON(w, status, payload)
}

func newTaskID() string {
	return idgen.New("task")
}

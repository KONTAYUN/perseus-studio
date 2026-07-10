package ai

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"sync"
	"time"

	"webide-backend/internal/store"
	"webide-backend/internal/usersvc"
)

const providerHTTPTransportProtocol = "http/1.1"

type Service struct {
	configs     *store.ConfigStore
	factory     *usersvc.Factory
	calls       *store.ConversationStore
	diag        *store.DiagnosticStore
	runs        *store.AgentRunStore
	usage       *store.UsageStore
	client      *http.Client
	rateLimitMu sync.Mutex
	rateLimits  map[string]ProviderRateLimitState
}

type AttachmentPayload struct {
	ID                string
	Name              string
	Size              int64
	MIMEType          string
	Kind              string
	Path              string
	FileType          string
	SkillID           string
	Description       string
	Text              string
	RawBase64         string
	DataBase64        string
	Truncated         bool
	IncludedInContext bool
	Note              string
	EditorPanels      []EditorPanelContext
}

type EditorPanelContext struct {
	PanelID        string   `json:"panel_id"`
	Index          int      `json:"index"`
	PositionLabel  string   `json:"position_label"`
	ActiveFilePath string   `json:"active_file_path"`
	ActiveFileName string   `json:"active_file_name"`
	OpenTabs       []string `json:"open_tabs"`
}

type ToolEvent struct {
	Type          string              `json:"type,omitempty"`
	ToolCallID    string              `json:"tool_call_id,omitempty"`
	ToolName      string              `json:"tool_name"`
	StatusKind    string              `json:"status_kind,omitempty"`
	Label         string              `json:"label"`
	Status        string              `json:"status"`
	TargetSummary string              `json:"target_summary,omitempty"`
	OperationHash string              `json:"operation_hash,omitempty"`
	Question      string              `json:"question,omitempty"`
	Options       []string            `json:"options,omitempty"`
	AllowFreeform *bool               `json:"allow_freeform,omitempty"`
	SkillID       string              `json:"skill_id,omitempty"`
	Path          string              `json:"path,omitempty"`
	OldPath       string              `json:"old_path,omitempty"`
	NewPath       string              `json:"new_path,omitempty"`
	ChangedPaths  []string            `json:"changed_paths,omitempty"`
	Moved         []WorkspacePathMove `json:"moved,omitempty"`
	Operation     string              `json:"operation,omitempty"`
	VersionID     string              `json:"version_id,omitempty"`
	Hash          string              `json:"hash,omitempty"`
	Command       string              `json:"command,omitempty"`
	Cwd           string              `json:"cwd,omitempty"`
	Stdout        string              `json:"stdout,omitempty"`
	Stderr        string              `json:"stderr,omitempty"`
	ExitCode      *int                `json:"exit_code,omitempty"`
	TimedOut      bool                `json:"timed_out,omitempty"`
	Provider      string              `json:"provider,omitempty"`
	URL           string              `json:"url,omitempty"`
	Error         string              `json:"error,omitempty"`
	DurationMS    int64               `json:"duration_ms,omitempty"`
}

type WorkspacePathMove struct {
	OldPath string `json:"old_path"`
	NewPath string `json:"new_path,omitempty"`
	Status  string `json:"status,omitempty"`
	Error   string `json:"error,omitempty"`
}

type WebSearchResult struct {
	Title         string `json:"title"`
	URL           string `json:"url"`
	Snippet       string `json:"snippet,omitempty"`
	Content       string `json:"content,omitempty"`
	Provider      string `json:"provider"`
	PublishedDate string `json:"published_date,omitempty"`
}

type WebExtractResult struct {
	URL       string `json:"url"`
	Title     string `json:"title,omitempty"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
	Provider  string `json:"provider"`
}

type Reply struct {
	Content          string                 `json:"content"`
	ToolEvents       []ToolEvent            `json:"tool_events,omitempty"`
	BackgroundTask   *BackgroundTaskRequest `json:"background_task,omitempty"`
	IncompleteReason string                 `json:"incomplete_reason,omitempty"`
	Recoverable      bool                   `json:"recoverable,omitempty"`
	Budget           *BudgetExhaustionInfo  `json:"budget,omitempty"`
}

type ConversationTitleResult struct {
	Title string `json:"title"`
}

type RunOptions struct {
	UserID                      string
	UserRole                    string
	RunID                       string
	TaskID                      string
	ForceTool                   string
	ThinkingMode                string
	Attachments                 []AttachmentPayload
	Log                         func(level, message string)
	Event                       func(eventType, level string, payload map[string]any)
	OnAssistantMessageStarted   func(messageID string, content string, segmentIndex int)
	OnAssistantMessageDelta     func(messageID string, delta string)
	OnAssistantMessageCompleted func(messageID string, finalContent string, segmentIndex int)
	OnAssistantMessageDiscarded func(messageID string, reason string, segmentIndex int)
	OnAssistantToolBoundary     func()
	OnCompletionResultDelta     func(delta string)
	OnCompletionResultDiscard   func()
	OnProviderAttempt           func()
}

type ExpectedOutput struct {
	Type        string `json:"type"`
	Path        string `json:"path,omitempty"`
	Description string `json:"description,omitempty"`
}

type BackgroundTaskRequest struct {
	Title           string           `json:"title"`
	Goal            string           `json:"goal"`
	ExpectedOutputs []ExpectedOutput `json:"expected_outputs"`
	SuggestedSteps  []string         `json:"suggested_steps"`
}

func NewService(configs *store.ConfigStore, factory *usersvc.Factory, calls *store.ConversationStore, diagnostics ...*store.DiagnosticStore) *Service {
	var diag *store.DiagnosticStore
	if len(diagnostics) > 0 {
		diag = diagnostics[0]
	}
	return &Service{
		configs:    configs,
		factory:    factory,
		calls:      calls,
		diag:       diag,
		client:     newProviderHTTPClient(),
		rateLimits: map[string]ProviderRateLimitState{},
	}
}

func newProviderHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 10
	transport.IdleConnTimeout = 90 * time.Second
	transport.ResponseHeaderTimeout = 0
	transport.ExpectContinueTimeout = 1 * time.Second
	transport.ForceAttemptHTTP2 = false
	transport.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
	tlsConfig := transport.TLSClientConfig
	if tlsConfig != nil {
		tlsConfig = tlsConfig.Clone()
	} else {
		tlsConfig = &tls.Config{}
	}
	tlsConfig.NextProtos = []string{providerHTTPTransportProtocol}
	transport.TLSClientConfig = tlsConfig
	return &http.Client{Transport: transport}
}

func (s *Service) closeIdleProviderConnections() {
	if s == nil || s.client == nil {
		return
	}
	if transport, ok := s.client.Transport.(interface{ CloseIdleConnections() }); ok {
		transport.CloseIdleConnections()
		return
	}
	s.client.CloseIdleConnections()
}

func (s *Service) AttachRunStore(runs *store.AgentRunStore) {
	if s == nil {
		return
	}
	s.runs = runs
}

func (s *Service) AttachUsageStore(usage *store.UsageStore) {
	if s == nil {
		return
	}
	s.usage = usage
}

func (s *Service) GenerateReply(ctx context.Context, conversationID, messageID, modelProviderID string, messages []store.Message, attachments []AttachmentPayload, opts RunOptions) (Reply, error) {
	return s.generateReply(ctx, conversationID, messageID, modelProviderID, messages, attachments, opts)
}

func (s *Service) GenerateTaskReply(ctx context.Context, conversationID, messageID, modelProviderID string, messages []store.Message, attachments []AttachmentPayload, opts RunOptions) (Reply, error) {
	return s.generateReply(ctx, conversationID, messageID, modelProviderID, messages, attachments, opts)
}

func (s *Service) generateReply(ctx context.Context, conversationID, messageID, modelProviderID string, messages []store.Message, attachments []AttachmentPayload, opts RunOptions) (Reply, error) {
	opts.UserRole = normalizeRunUserRole(opts.UserRole)
	attachments = s.enrichSelectedSkillAttachmentsForRole(ctx, opts.UserID, opts.UserRole, attachments)
	opts.Attachments = attachments
	opts.ThinkingMode = normalizeThinkingMode(opts.ThinkingMode)
	provider, err := s.configs.GetModelProviderForUser(ctx, opts.UserID, opts.UserRole, modelProviderID)
	if err != nil {
		return Reply{}, err
	}
	if !provider.Enabled {
		return Reply{}, fmt.Errorf("selected model is disabled")
	}
	if state, limited := s.activeProviderRateLimitState(opts.UserID, provider.ID, provider.ModelID, time.Now().UTC()); limited {
		return Reply{}, &ProviderRateLimitError{State: state, Err: fmt.Errorf("provider cooldown is active")}
	}
	if codexSidecarEnabled() {
		return s.generateCodexSidecar(ctx, conversationID, messageID, provider, messages, attachments, opts)
	}
	messages = s.prepareMessagesWithConversationSummary(ctx, conversationID, provider, messages, opts)

	switch provider.Protocol {
	case "openai":
		return s.generateOpenAI(ctx, conversationID, messageID, provider, messages, attachments, opts)
	case "anthropic":
		return s.generateAnthropic(ctx, conversationID, messageID, provider, messages, attachments, opts)
	default:
		return Reply{}, fmt.Errorf("unsupported protocol: %s", provider.Protocol)
	}
}

func normalizeRunUserRole(role string) string {
	if role == "admin" {
		return "admin"
	}
	return "user"
}

func (s *Service) enrichSelectedSkillAttachments(ctx context.Context, userID string, attachments []AttachmentPayload) []AttachmentPayload {
	return s.enrichSelectedSkillAttachmentsForRole(ctx, userID, "admin", attachments)
}

func (s *Service) enrichSelectedSkillAttachmentsForRole(ctx context.Context, userID, role string, attachments []AttachmentPayload) []AttachmentPayload {
	if s.factory == nil || userID == "" || len(attachments) == 0 {
		return attachments
	}
	skillsSvc, err := s.factory.Skills(userID)
	if err != nil {
		return attachments
	}
	next := make([]AttachmentPayload, len(attachments))
	copy(next, attachments)
	for index := range next {
		if next[index].Kind != "selected_skill" || next[index].SkillID == "" {
			continue
		}
		item, err := skillsSvc.GetForUser(ctx, userID, normalizeRunUserRole(role), next[index].SkillID)
		if err != nil {
			next[index].Note = fmt.Sprintf("用户选择了 Skill %q，但当前无法读取：%v", next[index].Name, err)
			continue
		}
		ownerSkillsSvc := skillsSvc
		if item.UserID != userID {
			if ownerSvc, ownerErr := s.factory.Skills(item.UserID); ownerErr == nil {
				ownerSkillsSvc = ownerSvc
			} else {
				next[index].Note = fmt.Sprintf("用户选择了 Skill %q，但当前无法读取：%v", next[index].Name, ownerErr)
				continue
			}
		}
		result, err := ownerSkillsSvc.ReadSkillForUser(ctx, userID, normalizeRunUserRole(role), next[index].SkillID, true)
		if err != nil {
			next[index].Note = fmt.Sprintf("用户选择了 Skill %q，但当前无法读取：%v", next[index].Name, err)
			continue
		}
		next[index].Text = result.Content
		next[index].Truncated = result.Truncated
		next[index].IncludedInContext = true
	}
	return next
}

package ai

import "webide-backend/internal/store"

const (
	agentChatMaxRounds               = 48
	agentWorkspaceMaxRounds          = 96
	agentTaskMaxRounds               = 240
	agentMaxInvalidTurns             = 2
	agentChatMaxToolCalls            = 60
	agentWorkspaceMaxToolCalls       = 160
	agentTaskMaxToolCalls            = 240
	agentMaxConsecutiveToolErrs      = 3
	defaultAgentMaxCheckpointResumes = 2
)

type agentProtocol string

const (
	agentProtocolOpenAI    agentProtocol = "openai"
	agentProtocolAnthropic agentProtocol = "anthropic"
)

type AgentExecutionMode string

const (
	AgentExecutionModeChat             AgentExecutionMode = "chat"
	AgentExecutionModeWorkspaceAction  AgentExecutionMode = "workspace_action"
	AgentExecutionModeToolLoop         AgentExecutionMode = "tool_loop"
	AgentExecutionModeTerminalPending  AgentExecutionMode = "terminal_pending"
	AgentExecutionModeCheckpointResume AgentExecutionMode = "checkpoint_resume"
	AgentExecutionModeBudgetSummary    AgentExecutionMode = "budget_summary"
)

type ModelResponseKind string

const (
	ModelResponseJSONToolCall         ModelResponseKind = "json_tool_call"
	ModelResponseTextPatchAction      ModelResponseKind = "text_patch_action"
	ModelResponseTerminalAction       ModelResponseKind = "terminal_action"
	ModelResponseContentOnly          ModelResponseKind = "content_only"
	ModelResponseEmptyOrReasoningOnly ModelResponseKind = "empty_or_reasoning_only"
)

type ModelResponseClassification struct {
	Kind      ModelResponseKind
	Turn      agentModelTurn
	Patch     extractedTextPatchTurn
	ToolNames []string
}

type ProviderNativePatchCapability string

const (
	ProviderNativePatchDisabled ProviderNativePatchCapability = "disabled"
	ProviderNativePatchEnabled  ProviderNativePatchCapability = "enabled"
)

type WorkspaceEditAction struct {
	Name      string                        `json:"name"`
	Provider  ProviderNativePatchCapability `json:"provider_native_patch"`
	Transport ToolTransport                 `json:"transport"`
}

type ActionSelectionState struct {
	WorkspaceLoaded              bool     `json:"workspace_loaded"`
	RequiresWorkspaceMutation    bool     `json:"requires_workspace_mutation"`
	HasVerifiedMutation          bool     `json:"has_verified_mutation"`
	VerifiedChangedPaths         []string `json:"verified_changed_paths,omitempty"`
	TargetPaths                  []string `json:"target_paths,omitempty"`
	DirectoryConditionsSatisfied []string `json:"directory_conditions_satisfied,omitempty"`
	LastFailureTool              string   `json:"last_failure_tool,omitempty"`
	LastFailureBlocking          bool     `json:"last_failure_blocking,omitempty"`
	NativePatchToolAvailable     bool     `json:"native_patch_tool_available,omitempty"`
}

type ActionProgressLedger struct {
	ConditionSatisfiedPaths []string `json:"condition_satisfied_paths,omitempty"`
	LastProgressRound       int      `json:"last_progress_round,omitempty"`
}

type AgentLoopGuard struct {
	LastActionKey           string `json:"last_action_key,omitempty"`
	RepeatedActionCount     int    `json:"repeated_action_count,omitempty"`
	TerminalRejectionCount  int    `json:"terminal_rejection_count,omitempty"`
	ContentRejectionCount   int    `json:"content_rejection_count,omitempty"`
	NoProgressRoundCount    int    `json:"no_progress_round_count,omitempty"`
	LastProgressFingerprint string `json:"last_progress_fingerprint,omitempty"`
}

type LoopGuardObservation struct {
	Status             string   `json:"status"`
	ReasonCode         string   `json:"reason_code"`
	RequiredNextAction string   `json:"required_next_action,omitempty"`
	AllowedNextActions []string `json:"allowed_next_actions,omitempty"`
	Message            string   `json:"message"`
}

type agentRuntime struct {
	service          *Service
	protocol         agentProtocol
	conversationID   string
	messageID        string
	provider         store.ModelProvider
	messages         []store.Message
	attachments      []AttachmentPayload
	opts             RunOptions
	profile          providerRuntimeProfile
	allowTask        bool
	recentReadMemory RecentWorkspaceReadMemory

	events                    []ToolEvent
	appliedMutations          map[string]ToolEvent
	chunkWrites               map[string]workspaceChunkWriteState
	performedMutation         bool
	lastToolFailed            bool
	lastToolFailure           runtimeToolFailure
	invalidTurns              int
	toolCalls                 int
	consecutiveToolErrors     int
	repeatedToolErrorKey      string
	repeatedToolErrorCount    int
	loadedToolsets            map[string]bool
	requiresCompletion        bool
	toolLoopActive            bool
	lastProviderFinishReason  string
	lastProviderEmptyResponse bool
	completedContent          string
	currentRound              int
	transcript                *AgentTranscript
	contextManager            *agentContextManager
	lastContextSnapshot       ContextSnapshot
	contextOverflowRetried    bool
	checkpointResumeAttempts  int
	checkpointResumeDiagIDs   []string
	activeAssistantMessageID  string
	activeAssistantSegment    int
	providerAttempts          int
	actionProgress            ActionProgressLedger
	loopGuard                 AgentLoopGuard
}

type runtimeToolFailure struct {
	ToolName     string
	Message      string
	FailureClass string
	Target       string
	Mutation     bool
	Recoverable  bool
}

type agentModelTurn struct {
	Content                  string
	FinishReason             string
	ToolCalls                []agentToolCall
	OpenAIMessage            openAIMessage
	AnthropicRaw             []map[string]any
	Streamed                 bool
	PublicTextStreamed       bool
	PublicStreamedContent    string
	CompletionResultStreamed bool
	Usage                    ProviderUsage
}

type agentToolCall struct {
	ID             string
	Name           string
	ArgumentsJSON  string
	AnthropicInput map[string]any
	PatchText      string
}

type workspaceChunkWriteState struct {
	Path          string `json:"path"`
	ChunksWritten int    `json:"chunks_written"`
	TotalChars    int    `json:"total_chars"`
	CurrentHash   string `json:"current_hash,omitempty"`
	VersionID     string `json:"version_id,omitempty"`
}

type completionAttempt struct {
	Result       string   `json:"result"`
	Summary      string   `json:"summary"`
	ChangedPaths []string `json:"changed_paths"`
	FailedItems  []string `json:"failed_items"`
	Notes        string   `json:"notes"`
}

type followupQuestionAttempt struct {
	Question      string   `json:"question"`
	Options       []string `json:"options"`
	AllowFreeform *bool    `json:"allow_freeform"`
	Reason        string   `json:"reason"`
}

type BudgetExhaustionInfo struct {
	Reason       string `json:"reason"`
	Round        int    `json:"round"`
	MaxRounds    int    `json:"max_rounds"`
	ToolCalls    int    `json:"tool_calls"`
	MaxToolCalls int    `json:"max_tool_calls"`
}

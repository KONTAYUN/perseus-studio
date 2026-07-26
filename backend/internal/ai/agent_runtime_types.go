package ai

import "webide-backend/internal/store"

const (
	agentChatMaxRounds               = 48
	agentWorkspaceMaxRounds          = 96
	agentTaskMaxRounds               = 240
	agentMaxProtocolMistakes         = 3
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
	ModelResponseTerminalAction       ModelResponseKind = "terminal_action"
	ModelResponseContentOnly          ModelResponseKind = "content_only"
	ModelResponseEmptyOrReasoningOnly ModelResponseKind = "empty_or_reasoning_only"
)

type ModelResponseClassification struct {
	Kind      ModelResponseKind
	Turn      agentModelTurn
	ToolNames []string
}

type WorkspaceEditAction struct {
	Name      string        `json:"name"`
	Transport ToolTransport `json:"transport"`
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
	StructuredPatchToolAvailable bool     `json:"structured_patch_tool_available,omitempty"`
}

type ActionProgressLedger struct {
	ConditionSatisfiedPaths []string `json:"condition_satisfied_paths,omitempty"`
	LastProgressRound       int      `json:"last_progress_round,omitempty"`
	LastReadProgressRound   int      `json:"last_read_progress_round,omitempty"`
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
	service        *Service
	protocol       agentProtocol
	conversationID string
	messageID      string
	provider       store.ModelProvider
	attachments    []AttachmentPayload
	opts           RunOptions
	profile        providerRuntimeProfile
	allowTask      bool

	events                    []ToolEvent
	presentationEventStart    int
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
	lastProviderFinishReason  string
	lastProviderEmptyResponse bool
	completedContent          string
	terminalState             AgentTerminationState
	currentRound              int
	transcript                *AgentTranscript
	lastContextSnapshot       ContextSnapshot
	contextOverflowRetried    bool
	checkpointResumeAttempts  int
	checkpointResumeDiagIDs   []string
	activeAssistantMessageID  string
	activeAssistantSegment    int
	providerAttempts          int
	actionProgress            ActionProgressLedger
	loopGuard                 AgentLoopGuard
	kernel                    AgentKernelState
	contextItems              []store.AgentTurnItem
	contextPersistenceErr     error
	pendingTaskPlan           *taskPlanUpdate
}

type runtimeToolFailure struct {
	ToolName          string
	Message           string
	FailureClass      string
	Category          string
	Target            string
	Mutation          bool
	Recoverable       bool
	RecommendedAction string
}

type agentModelTurn struct {
	Content                  string
	FinishReason             string
	ProviderResponseID       string
	StreamLastEventID        string
	StreamDuplicateEvents    int
	ToolCalls                []agentToolCall
	OpenAIMessage            openAIMessage
	AnthropicRaw             []map[string]any
	Streamed                 bool
	PublicTextStreamed       bool
	PublicStreamedContent    string
	Usage                    ProviderUsage
}

type agentToolCall struct {
	ID             string
	Name           string
	ArgumentsJSON  string
	AnthropicInput map[string]any
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

type userRequestAttempt struct {
	Question      string   `json:"question"`
	Options       []string `json:"options"`
	AllowFreeform *bool    `json:"allow_freeform,omitempty"` // Legacy input only; free-form answers are always allowed.
}

type taskPlanUpdate struct {
	Goal            string   `json:"goal"`
	LocalTargets    []string `json:"local_targets"`
	ExternalTargets []string `json:"external_targets"`
	CompletedSteps  []string `json:"completed_steps"`
	CurrentStep     string   `json:"current_step"`
	NextSteps       []string `json:"next_steps"`
	VerifiedFacts   []string `json:"verified_facts"`
	MissingInfo     []string `json:"missing_info"`
}

type BudgetExhaustionInfo struct {
	Reason       string `json:"reason"`
	Round        int    `json:"round"`
	MaxRounds    int    `json:"max_rounds"`
	ToolCalls    int    `json:"tool_calls"`
	MaxToolCalls int    `json:"max_tool_calls"`
}

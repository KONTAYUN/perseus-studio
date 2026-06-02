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

type agentRuntime struct {
	service        *Service
	protocol       agentProtocol
	conversationID string
	messageID      string
	provider       store.ModelProvider
	messages       []store.Message
	attachments    []AttachmentPayload
	opts           RunOptions
	profile        providerRuntimeProfile
	allowTask      bool

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

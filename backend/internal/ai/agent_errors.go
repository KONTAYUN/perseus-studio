package ai

type AgentRuntimeErrorCode string

const (
	AgentRuntimeProtocolViolation AgentRuntimeErrorCode = "agent_protocol_violation"
	AgentRuntimeToolErrorLoop     AgentRuntimeErrorCode = "agent_tool_error_loop"
	AgentRuntimePersistence       AgentRuntimeErrorCode = "local_run_persistence"
	AgentRuntimeHistoryIntegrity  AgentRuntimeErrorCode = "history_integrity_error"
)

type AgentRuntimeError struct {
	Code      AgentRuntimeErrorCode
	Category  string
	Retryable bool
	Message   string
	Cause     error
}

func (e *AgentRuntimeError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return string(e.Code)
}

func (e *AgentRuntimeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func newAgentRuntimeError(code AgentRuntimeErrorCode, message string, cause error) error {
	category := "runtime"
	retryable := false
	switch code {
	case AgentRuntimePersistence:
		category = "persistence"
	case AgentRuntimeHistoryIntegrity:
		category = "history"
	case AgentRuntimeToolErrorLoop:
		category = "tool_loop"
	case AgentRuntimeProtocolViolation:
		category = "protocol"
		retryable = true
	}
	return &AgentRuntimeError{
		Code: code, Category: category, Retryable: retryable,
		Message: message, Cause: cause,
	}
}

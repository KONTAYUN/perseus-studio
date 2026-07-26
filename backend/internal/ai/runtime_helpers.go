package ai

import (
	"fmt"
	"strings"
)

func formatProviderError(statusCode int, raw any) error {
	code, errorType, param, message, structured := providerErrorFields(raw)
	if message == "" {
		message = fmt.Sprintf("provider request failed with HTTP %d", statusCode)
	}
	return &ProviderResponseError{
		StatusCode:      statusCode,
		Message:         message,
		ErrorCode:       code,
		ErrorType:       errorType,
		ErrorParam:      param,
		StructuredError: structured,
	}
}

func providerErrorMessage(raw any) string {
	switch value := raw.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(value)
	case map[string]any:
		for _, key := range []string{"message", "error", "detail", "code"} {
			if item, ok := value[key]; ok {
				text := providerErrorMessage(item)
				if text != "" {
					return text
				}
			}
		}
	}
	text := strings.TrimSpace(fmt.Sprint(raw))
	text = strings.ReplaceAll(text, "map[", "")
	text = strings.TrimSuffix(text, "]")
	return text
}

func isContextOverflowError(err error) bool {
	if err == nil {
		return false
	}
	return classifyProviderFailure(err).Class == ProviderFailureContextExceeded
}

func isWorkspaceMutationTool(name string) bool {
	return manifestToolIsMutation(name)
}

func IsWorkspaceMutationTool(name string) bool {
	return isWorkspaceMutationTool(name)
}

func unsupportedToolCallingError() error {
	return &ProviderProtocolError{Code: "tool_use_not_supported", Cause: fmt.Errorf("current model adapter does not support tool calling")}
}

func missingWorkspaceMutationError() error {
	return newToolError(ToolErrorExecutionFailed, "model did not call a required workspace mutation tool", nil)
}

func attachmentsIncludeWorkspaceReference(attachments []AttachmentPayload) bool {
	for _, attachment := range attachments {
		switch attachment.Kind {
		case "workspace_file":
			return true
		}
	}
	return false
}

func nonEmptyStrings(values ...string) []string {
	next := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			next = append(next, strings.TrimSpace(value))
		}
	}
	return next
}

func toolCallNames(calls []agentToolCall) []string {
	if len(calls) == 0 {
		return nil
	}
	names := make([]string, 0, len(calls))
	for _, call := range calls {
		if name := strings.TrimSpace(call.Name); name != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return nil
	}
	return names
}

package ai

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"webide-backend/internal/store"
)

const (
	codexSidecarEngineEnv = "PERSEUS_AGENT_ENGINE"
	codexSidecarMode      = "codex_app_server"
	codexSidecarAPIVar    = "PERSEUS_CODEX_API_KEY"
)

func codexSidecarEnabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(codexSidecarEngineEnv)))
	return value == codexSidecarMode || value == "codex_sidecar" || value == "codex"
}

func (s *Service) generateCodexSidecar(ctx context.Context, conversationID, messageID string, provider store.ModelProvider, messages []store.Message, attachments []AttachmentPayload, opts RunOptions) (Reply, error) {
	if opts.OnProviderAttempt != nil {
		opts.OnProviderAttempt()
	}
	files, err := s.factory.Workspace(opts.UserID)
	if err != nil {
		return Reply{}, err
	}
	workspaceRoot := files.RootAbs()
	if workspaceRoot == "" {
		return Reply{}, fmt.Errorf("workspace root is unavailable")
	}
	client, err := newCodexAppServerClient(ctx, provider, opts.UserID)
	if err != nil {
		return Reply{}, err
	}
	defer client.Close()
	toolExecutor := func(ctx context.Context, name string, args map[string]any) (string, error) {
		argumentsJSON, err := json.Marshal(args)
		if err != nil {
			return "", err
		}
		result, _, _, _, err := s.executeTool(ctx, name, string(argumentsJSON), opts)
		return result, err
	}
	input := codexSidecarTurnInput(messages, attachments)
	if strings.TrimSpace(input) == "" {
		input = "Continue."
	}
	reply, err := client.RunTurn(ctx, codexSidecarRunRequest{
		ConversationID: conversationID,
		MessageID:      messageID,
		WorkspaceRoot:  workspaceRoot,
		UserInput:      input,
		ToolExecutor:   toolExecutor,
		OnDelta:        opts.OnAssistantMessageDelta,
		OnEvent:        opts.Event,
	})
	if err != nil {
		return Reply{}, s.classifyProviderError(opts, provider, err)
	}
	return Reply{Content: strings.TrimSpace(reply)}, nil
}

func codexSidecarTurnInput(messages []store.Message, attachments []AttachmentPayload) string {
	var builder strings.Builder
	for _, msg := range messages {
		role := strings.TrimSpace(msg.Role)
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		switch role {
		case "user":
			builder.WriteString("User:\n")
		case "assistant":
			builder.WriteString("Assistant:\n")
		default:
			builder.WriteString(role)
			builder.WriteString(":\n")
		}
		builder.WriteString(content)
		builder.WriteString("\n\n")
	}
	for _, attachment := range attachments {
		if strings.TrimSpace(attachment.Path) != "" {
			builder.WriteString("Attached workspace file: ")
			builder.WriteString(attachment.Path)
			builder.WriteString("\n")
		}
	}
	return strings.TrimSpace(builder.String())
}

type codexAppServerClient struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	lines    chan codexRPCMessage
	writeMu  sync.Mutex
	nextID   atomic.Int64
	provider store.ModelProvider
}

type codexSidecarRunRequest struct {
	ConversationID string
	MessageID      string
	WorkspaceRoot  string
	UserInput      string
	ToolExecutor   func(context.Context, string, map[string]any) (string, error)
	OnDelta        func(messageID string, delta string)
	OnEvent        func(eventType, level string, payload map[string]any)
}

type codexRPCMessage struct {
	ID     json.RawMessage            `json:"id,omitempty"`
	Method string                     `json:"method,omitempty"`
	Params map[string]any             `json:"params,omitempty"`
	Result map[string]any             `json:"result,omitempty"`
	Error  *codexRPCError             `json:"error,omitempty"`
	Raw    map[string]json.RawMessage `json:"-"`
}

type codexRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func newCodexAppServerClient(ctx context.Context, provider store.ModelProvider, userID string) (*codexAppServerClient, error) {
	bin := strings.TrimSpace(os.Getenv("PERSEUS_CODEX_BIN"))
	if bin == "" {
		bin = "codex"
	}
	providerID := "perseus_" + sanitizeCodexProviderID(provider.ID)
	args := []string{
		"app-server",
		"--listen", "stdio://",
		"--session-source", "vscode",
		"--config", "model=" + codexTomlString(provider.ModelID),
		"--config", "model_provider=" + codexTomlString(providerID),
		"--config", "approval_policy=\"never\"",
		"--config", "sandbox_mode=\"workspace-write\"",
		"--config", fmt.Sprintf("model_providers.%s.name=%s", providerID, codexTomlString(firstNonEmpty(provider.Name, providerID))),
		"--config", fmt.Sprintf("model_providers.%s.base_url=%s", providerID, codexTomlString(strings.TrimRight(provider.BaseURL, "/"))),
		"--config", fmt.Sprintf("model_providers.%s.env_key=%s", providerID, codexTomlString(codexSidecarAPIVar)),
		"--config", fmt.Sprintf("model_providers.%s.wire_api=\"responses\"", providerID),
		"--config", fmt.Sprintf("model_providers.%s.requires_openai_auth=false", providerID),
		"--config", fmt.Sprintf("model_providers.%s.request_max_retries=4", providerID),
		"--config", fmt.Sprintf("model_providers.%s.stream_max_retries=5", providerID),
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	codexHome := strings.TrimSpace(os.Getenv("PERSEUS_CODEX_HOME"))
	if codexHome == "" {
		codexHome = filepath.Join(os.TempDir(), "perseus-codex-home", sanitizeCodexProviderID(userID))
	}
	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		return nil, err
	}
	cmd.Env = append(os.Environ(),
		"CODEX_HOME="+codexHome,
		codexSidecarAPIVar+"="+provider.APIKey,
		"RUST_LOG=warn",
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start codex app-server: %w", err)
	}
	client := &codexAppServerClient{
		cmd:      cmd,
		stdin:    stdin,
		lines:    make(chan codexRPCMessage, 128),
		provider: provider,
	}
	client.nextID.Store(1)
	go client.readStdout(stdout)
	go discardCodexStderr(stderr)
	if err := client.initialize(ctx); err != nil {
		client.Close()
		return nil, err
	}
	return client, nil
}

func discardCodexStderr(stderr io.Reader) {
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		_ = scanner.Text()
	}
}

func (c *codexAppServerClient) readStdout(stdout io.Reader) {
	defer close(c.lines)
	scanner := bufio.NewScanner(stdout)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 16*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		msg := codexRPCMessage{Raw: raw}
		_ = json.Unmarshal([]byte(line), &msg)
		if params, ok := raw["params"]; ok {
			_ = json.Unmarshal(params, &msg.Params)
		}
		if result, ok := raw["result"]; ok {
			_ = json.Unmarshal(result, &msg.Result)
		}
		if id, ok := raw["id"]; ok {
			msg.ID = append(json.RawMessage(nil), id...)
		}
		c.lines <- msg
	}
}

func (c *codexAppServerClient) Close() {
	if c == nil {
		return
	}
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
		_, _ = c.cmd.Process.Wait()
	}
}

func (c *codexAppServerClient) initialize(ctx context.Context) error {
	_, err := c.call(ctx, "initialize", map[string]any{
		"clientInfo": map[string]any{
			"name":    "perseus_studio",
			"title":   "Perseus Studio",
			"version": "0.1.0",
		},
		"capabilities": map[string]any{
			"experimentalApi": true,
		},
	})
	if err != nil {
		return err
	}
	return c.notify(map[string]any{"method": "initialized"})
}

func (c *codexAppServerClient) RunTurn(ctx context.Context, req codexSidecarRunRequest) (string, error) {
	threadResp, err := c.call(ctx, "thread/start", map[string]any{
		"model":                      c.provider.ModelID,
		"modelProvider":              "perseus_" + sanitizeCodexProviderID(c.provider.ID),
		"cwd":                        filepath.ToSlash(req.WorkspaceRoot),
		"runtimeWorkspaceRoots":      []string{filepath.ToSlash(req.WorkspaceRoot)},
		"approvalPolicy":             "never",
		"sandbox":                    "workspaceWrite",
		"personality":                "friendly",
		"serviceName":                "perseus_studio",
		"sessionStartSource":         "startup",
		"dynamicTools":               codexDynamicTools(),
		"baseInstructions":           codexPerseusBaseInstructions(),
		"developerInstructions":      codexPerseusDeveloperInstructions(),
		"allowProviderModelFallback": false,
	})
	if err != nil {
		return "", err
	}
	threadID := nestedString(threadResp, "thread", "id")
	if threadID == "" {
		return "", fmt.Errorf("codex thread/start response missing thread.id")
	}
	if _, err := c.call(ctx, "turn/start", map[string]any{
		"threadId":            threadID,
		"clientUserMessageId": req.MessageID,
		"input": []map[string]any{{
			"type":         "text",
			"text":         req.UserInput,
			"textElements": []any{},
		}},
	}); err != nil {
		return "", err
	}
	var final strings.Builder
	for {
		msg, err := c.next(ctx)
		if err != nil {
			return final.String(), err
		}
		if msg.Method == "item/tool/call" {
			if err := c.handleDynamicToolCall(ctx, msg, req); err != nil {
				return final.String(), err
			}
			continue
		}
		if msg.Method == "item/agentMessage/delta" {
			delta := stringValue(msg.Params["delta"])
			if delta != "" {
				final.WriteString(delta)
				if req.OnDelta != nil {
					req.OnDelta(req.MessageID, delta)
				}
			}
			continue
		}
		if msg.Method == "item/completed" {
			text := codexCompletedAgentText(msg.Params)
			if text != "" && !strings.Contains(final.String(), text) {
				final.WriteString(text)
				if req.OnDelta != nil {
					req.OnDelta(req.MessageID, text)
				}
			}
			continue
		}
		if msg.Method == "turn/completed" {
			return final.String(), nil
		}
		if req.OnEvent != nil && strings.HasPrefix(msg.Method, "item/") {
			req.OnEvent("codex_sidecar_event", "info", map[string]any{
				"method": msg.Method,
				"params": msg.Params,
			})
		}
	}
}

func (c *codexAppServerClient) handleDynamicToolCall(ctx context.Context, msg codexRPCMessage, req codexSidecarRunRequest) error {
	id := msg.ID
	toolName := stringValue(msg.Params["tool"])
	args, _ := msg.Params["arguments"].(map[string]any)
	if args == nil {
		args = map[string]any{}
	}
	var result string
	var err error
	if req.ToolExecutor == nil {
		err = fmt.Errorf("tool executor is unavailable")
	} else {
		result, err = req.ToolExecutor(ctx, toolName, args)
	}
	success := err == nil
	if !success {
		result = err.Error()
	}
	return c.respond(id, map[string]any{
		"contentItems": []map[string]any{{
			"type": "inputText",
			"text": result,
		}},
		"success": success,
	})
}

func (c *codexAppServerClient) call(ctx context.Context, method string, params map[string]any) (map[string]any, error) {
	id := c.nextID.Add(1)
	if err := c.write(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	expectedID := strconv.FormatInt(id, 10)
	for {
		msg, err := c.next(ctx)
		if err != nil {
			return nil, err
		}
		if string(msg.ID) != expectedID {
			continue
		}
		if msg.Error != nil {
			return nil, codexSidecarError(msg.Error.Message)
		}
		return msg.Result, nil
	}
}

func (c *codexAppServerClient) respond(id json.RawMessage, result map[string]any) error {
	var decoded any
	if err := json.Unmarshal(id, &decoded); err != nil {
		return err
	}
	return c.write(map[string]any{"id": decoded, "result": result})
}

func (c *codexAppServerClient) notify(payload map[string]any) error {
	return c.write(payload)
}

func (c *codexAppServerClient) write(payload map[string]any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	_, err = c.stdin.Write(encoded)
	return err
}

func (c *codexAppServerClient) next(ctx context.Context) (codexRPCMessage, error) {
	select {
	case <-ctx.Done():
		return codexRPCMessage{}, ctx.Err()
	case msg, ok := <-c.lines:
		if !ok {
			return codexRPCMessage{}, fmt.Errorf("codex app-server closed")
		}
		return msg, nil
	}
}

func codexDynamicTools() []map[string]any {
	return []map[string]any{{
		"type":        "namespace",
		"name":        "perseus",
		"description": "Perseus Studio workspace and HTML note tools.",
		"tools": []map[string]any{
			codexToolSpec("inspect_workspace", "List workspace files and folders.", map[string]any{
				"type":                 "object",
				"properties":           map[string]any{"path": map[string]any{"type": "string"}},
				"additionalProperties": false,
			}),
			codexToolSpec("find_workspace_files", "Find workspace files by name.", map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
					"limit": map[string]any{"type": "integer"},
				},
				"required":             []string{"query"},
				"additionalProperties": false,
			}),
			codexToolSpec("search_workspace_text", "Search text in workspace files.", map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
					"limit": map[string]any{"type": "integer"},
				},
				"required":             []string{"query"},
				"additionalProperties": false,
			}),
			codexToolSpec("read_workspace_text", "Read a text file; HTML notes are automatically routed to note index view.", map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":       map[string]any{"type": "string"},
					"start_line": map[string]any{"type": "integer"},
					"end_line":   map[string]any{"type": "integer"},
					"max_chars":  map[string]any{"type": "integer"},
				},
				"required":             []string{"path"},
				"additionalProperties": false,
			}),
			codexToolSpec("read_workspace_note_view", "Read an HTML note as text, structure, format, raw, index, section, or block.", map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":        map[string]any{"type": "string"},
					"view":        map[string]any{"type": "string", "enum": []string{"text", "structure", "format", "raw"}},
					"mode":        map[string]any{"type": "string", "enum": []string{"full", "index", "block", "section"}},
					"block_id":    map[string]any{"type": "string"},
					"section_id":  map[string]any{"type": "string"},
					"doc_hash":    map[string]any{"type": "string"},
					"start_block": map[string]any{"type": "integer"},
					"max_blocks":  map[string]any{"type": "integer"},
					"max_chars":   map[string]any{"type": "integer"},
				},
				"required":             []string{"path"},
				"additionalProperties": false,
			}),
			codexToolSpec("manage_workspace_path", "Create, rename, move, copy, or trash workspace paths. Use copy for note version duplication.", map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action":           map[string]any{"type": "string", "enum": []string{"mkdir", "rename", "move", "copy", "trash"}},
					"path":             map[string]any{"type": "string"},
					"new_name":         map[string]any{"type": "string"},
					"new_parent_path":  map[string]any{"type": "string"},
					"destination_path": map[string]any{"type": "string"},
				},
				"required":             []string{"action", "path"},
				"additionalProperties": false,
			}),
			codexToolSpec("apply_workspace_patch", "Apply a small deterministic patch to workspace text files.", map[string]any{
				"type": "object",
				"properties": map[string]any{
					"patch": map[string]any{"type": "string"},
				},
				"required":             []string{"patch"},
				"additionalProperties": false,
			}),
		},
	}}
}

func codexToolSpec(name, description string, schema map[string]any) map[string]any {
	return map[string]any{
		"type":        "function",
		"name":        name,
		"description": description,
		"inputSchema": schema,
	}
}

func codexPerseusBaseInstructions() string {
	return strings.TrimSpace(`You are the Perseus Studio agent. Use the provided Perseus dynamic tools for workspace notes.
For .html notes, do not read by line range. Use read_workspace_note_view mode=index first, then read target block or section raw only when exact HTML edits are needed.
For copying or versioning a note, use manage_workspace_path action=copy first, then make small verified edits to the copied file.
Do not output whole long HTML documents in chat. Prefer deterministic tools and verify changes before final response.`)
}

func codexPerseusDeveloperInstructions() string {
	return strings.TrimSpace(`Keep intermediate tool work out of the final answer. Report only the outcome and any actionable error. If a provider rate limit occurs, stop and surface the structured usage-limit state instead of retrying manually.`)
}

func codexCompletedAgentText(params map[string]any) string {
	item, _ := params["item"].(map[string]any)
	if item == nil {
		return ""
	}
	if stringValue(item["type"]) != "agentMessage" {
		return ""
	}
	return stringValue(item["text"])
}

func codexTomlString(value string) string {
	return strconv.Quote(strings.TrimSpace(value))
}

func sanitizeCodexProviderID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "default"
	}
	re := regexp.MustCompile(`[^a-zA-Z0-9_-]+`)
	value = re.ReplaceAllString(value, "_")
	value = strings.Trim(value, "_-")
	if value == "" {
		return "default"
	}
	if len(value) > 48 {
		value = value[:48]
	}
	return value
}

func nestedString(value map[string]any, keys ...string) string {
	var current any = value
	for _, key := range keys {
		item, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = item[key]
	}
	return stringValue(current)
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return ""
	}
}

func codexSidecarError(message string) error {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "codex app-server request failed"
	}
	lower := strings.ToLower(message)
	if strings.Contains(lower, "rate limit") || strings.Contains(lower, "too many requests") || strings.Contains(lower, "tpm") {
		return &ProviderResponseError{StatusCode: 429, Message: message, ResponseSnippet: message}
	}
	if strings.Contains(lower, "context") && strings.Contains(lower, "exceed") {
		return fmt.Errorf("context window exceeded: %s", message)
	}
	return errors.New(message)
}

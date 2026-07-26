package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
)

const (
	AgentItemVisibilityModel      = "model"
	AgentItemVisibilityUI         = "ui"
	AgentItemVisibilityDiagnostic = "diagnostic"
)

type AgentTurnItem struct {
	Seq            int64           `json:"seq"`
	ID             string          `json:"id"`
	UserID         string          `json:"user_id"`
	ConversationID string          `json:"conversation_id"`
	RunID          string          `json:"run_id,omitempty"`
	TurnID         string          `json:"turn_id"`
	Kind           string          `json:"kind"`
	Role           string          `json:"role,omitempty"`
	Status         string          `json:"status"`
	Visibility     string          `json:"visibility"`
	SourceID       string          `json:"source_id,omitempty"`
	PayloadJSON    string          `json:"-"`
	Payload        json.RawMessage `json:"payload,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
}

type AgentTurnItemInput struct {
	ID             string
	UserID         string
	ConversationID string
	RunID          string
	TurnID         string
	Kind           string
	Role           string
	Status         string
	Visibility     string
	SourceID       string
	PayloadJSON    string
	CreatedAt      time.Time
}

type AgentItemHistoryRepairReport struct {
	Scanned       int `json:"scanned"`
	Inserted      int `json:"inserted"`
	Reconciled    int `json:"reconciled"`
	AlreadyExists int `json:"already_exists"`
	Conflicts     int `json:"conflicts"`
}

type AgentTurnItemConflictError struct {
	Existing AgentTurnItem
	Input    AgentTurnItemInput
}

func (e *AgentTurnItemConflictError) Error() string {
	return fmt.Sprintf("agent turn item idempotency conflict: existing_id=%s source_id=%s kind=%s", e.Existing.ID, e.Input.SourceID, e.Input.Kind)
}

func (s *ConversationStore) migrateAgentTurnItems(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS agent_turn_items (
  seq INTEGER PRIMARY KEY AUTOINCREMENT,
  id TEXT NOT NULL UNIQUE,
  user_id TEXT NOT NULL,
  conversation_id TEXT NOT NULL,
  run_id TEXT NOT NULL DEFAULT '',
  turn_id TEXT NOT NULL,
  kind TEXT NOT NULL,
  role TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'completed',
  visibility TEXT NOT NULL DEFAULT 'model',
  source_id TEXT NOT NULL DEFAULT '',
  payload_json TEXT NOT NULL DEFAULT '{}',
  created_at DATETIME NOT NULL,
  FOREIGN KEY(conversation_id) REFERENCES conversations(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS agent_item_migrations (
  user_id TEXT NOT NULL,
  conversation_id TEXT NOT NULL,
  status TEXT NOT NULL,
  error TEXT NOT NULL DEFAULT '',
  migrated_at DATETIME,
  PRIMARY KEY(user_id, conversation_id),
  FOREIGN KEY(conversation_id) REFERENCES conversations(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_agent_turn_items_conversation_seq
  ON agent_turn_items(user_id, conversation_id, seq);
CREATE INDEX IF NOT EXISTS idx_agent_turn_items_turn_seq
  ON agent_turn_items(user_id, conversation_id, turn_id, seq);
CREATE INDEX IF NOT EXISTS idx_agent_turn_items_run_seq
  ON agent_turn_items(user_id, run_id, seq);
CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_turn_items_source
  ON agent_turn_items(user_id, conversation_id, kind, source_id)
  WHERE source_id <> '';
`)
	return err
}

func (s *ConversationStore) EnsureAgentTurnItems(ctx context.Context, userID, conversationID string) error {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(conversationID) == "" {
		return fmt.Errorf("user_id and conversation_id are required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	failMigration := func(migrationErr error) error {
		_ = tx.Rollback()
		_, recordErr := s.db.ExecContext(ctx, `
INSERT INTO agent_item_migrations (user_id, conversation_id, status, error, migrated_at)
VALUES (?, ?, 'failed', ?, NULL)
ON CONFLICT(user_id, conversation_id) DO UPDATE SET status = 'failed', error = excluded.error, migrated_at = NULL
`, userID, conversationID, migrationErr.Error())
		if recordErr != nil {
			return fmt.Errorf("%w; recording migration failure: %v", migrationErr, recordErr)
		}
		return migrationErr
	}

	var status string
	err = tx.QueryRowContext(ctx, `
SELECT status FROM agent_item_migrations WHERE user_id = ? AND conversation_id = ?
`, userID, conversationID).Scan(&status)
	if err == nil && status == "completed" {
		return tx.Commit()
	}
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO agent_item_migrations (user_id, conversation_id, status, error, migrated_at)
VALUES (?, ?, 'running', '', NULL)
ON CONFLICT(user_id, conversation_id) DO UPDATE SET status = 'running', error = '', migrated_at = NULL
`, userID, conversationID); err != nil {
		return err
	}

	legacy, err := loadLegacyAgentItems(ctx, tx, userID, conversationID)
	if err != nil {
		return failMigration(err)
	}
	for _, input := range legacy {
		if _, err := appendAgentTurnItemTx(ctx, tx, input); err != nil {
			return failMigration(err)
		}
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE agent_item_migrations
SET status = 'completed', error = '', migrated_at = ?
WHERE user_id = ? AND conversation_id = ?
`, time.Now().UTC(), userID, conversationID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *ConversationStore) AppendAgentTurnItem(ctx context.Context, input AgentTurnItemInput) (AgentTurnItem, error) {
	return appendAgentTurnItemTx(ctx, s.db, input)
}

// RepairAgentTurnItemHistory reconciles final UI messages with canonical model
// history. It is intentionally safe to run before every turn: the source ID is
// the immutable message ID, while older kernel rows are matched by content and
// repaired in place when the historical source ID was generated incorrectly.
func (s *ConversationStore) RepairAgentTurnItemHistory(ctx context.Context, userID, conversationID string) (AgentItemHistoryRepairReport, error) {
	report := AgentItemHistoryRepairReport{}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return report, err
	}
	defer tx.Rollback()

	legacy, err := loadLegacyAgentItems(ctx, tx, userID, conversationID)
	if err != nil {
		return report, err
	}
	existing, err := listAgentTurnItemsTx(ctx, tx, userID, conversationID)
	if err != nil {
		return report, err
	}
	bySource := make(map[string]AgentTurnItem, len(existing))
	for _, item := range existing {
		if item.SourceID != "" {
			bySource[item.Kind+"\x00"+item.SourceID] = item
		}
	}

	for _, input := range legacy {
		if input.Kind != "user_message" && input.Kind != "assistant_message" {
			continue
		}
		report.Scanned++
		if _, ok := bySource[input.Kind+"\x00"+input.SourceID]; ok {
			report.AlreadyExists++
			continue
		}

		if input.Kind == "assistant_message" {
			matchIndex := matchingLegacyAssistantItem(existing, input)
			if matchIndex >= 0 {
				matched := existing[matchIndex]
				if _, err := tx.ExecContext(ctx, `
UPDATE agent_turn_items
SET source_id = ?, turn_id = ?, run_id = CASE WHEN run_id = '' THEN ? ELSE run_id END
WHERE seq = ?
`, input.SourceID, input.TurnID, input.RunID, matched.Seq); err != nil {
					report.Conflicts++
					return report, err
				}
				matched.SourceID = input.SourceID
				matched.TurnID = input.TurnID
				if matched.RunID == "" {
					matched.RunID = input.RunID
				}
				existing[matchIndex] = matched
				bySource[input.Kind+"\x00"+input.SourceID] = matched
				report.Reconciled++
				continue
			}
		}

		inserted, err := appendAgentTurnItemTx(ctx, tx, input)
		if err != nil {
			report.Conflicts++
			return report, err
		}
		existing = append(existing, inserted)
		bySource[input.Kind+"\x00"+input.SourceID] = inserted
		report.Inserted++
	}
	if err := tx.Commit(); err != nil {
		return report, err
	}
	return report, nil
}

func (s *ConversationStore) ListAgentTurnItems(ctx context.Context, userID, conversationID string) ([]AgentTurnItem, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT seq, id, user_id, conversation_id, run_id, turn_id, kind, role, status, visibility, source_id, payload_json, created_at
FROM agent_turn_items
WHERE user_id = ? AND conversation_id = ?
ORDER BY seq ASC
`, userID, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []AgentTurnItem
	for rows.Next() {
		item, err := scanAgentTurnItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *ConversationStore) LatestAgentTurnID(ctx context.Context, userID, conversationID string) (string, error) {
	var turnID string
	err := s.db.QueryRowContext(ctx, `
SELECT turn_id FROM agent_turn_items
WHERE user_id = ? AND conversation_id = ? AND turn_id <> ''
ORDER BY seq DESC LIMIT 1
`, userID, conversationID).Scan(&turnID)
	return turnID, err
}

func (s *ConversationStore) RepairDanglingAgentToolCalls(ctx context.Context, userID, conversationID string) (int, error) {
	items, err := s.ListAgentTurnItems(ctx, userID, conversationID)
	if err != nil {
		return 0, err
	}
	results := map[string]bool{}
	for _, item := range items {
		if item.Kind != "tool_result" && item.Kind != "aborted" {
			continue
		}
		payload := map[string]any{}
		_ = json.Unmarshal([]byte(item.PayloadJSON), &payload)
		if id, _ := payload["tool_call_id"].(string); strings.TrimSpace(id) != "" {
			results[id] = true
		}
	}
	repaired := 0
	for _, item := range items {
		if item.Kind != "tool_call" {
			continue
		}
		payload := map[string]any{}
		_ = json.Unmarshal([]byte(item.PayloadJSON), &payload)
		callID, _ := payload["tool_call_id"].(string)
		if strings.TrimSpace(callID) == "" || results[callID] {
			continue
		}
		toolName, _ := payload["tool_name"].(string)
		_, err := s.AppendAgentTurnItem(ctx, AgentTurnItemInput{
			UserID: userID, ConversationID: conversationID, RunID: item.RunID, TurnID: item.TurnID,
			Kind: "aborted", Role: "tool", Status: "aborted", Visibility: AgentItemVisibilityModel,
			SourceID: item.SourceID + ":aborted",
			PayloadJSON: mustStoreJSON(map[string]any{
				"tool_call_id": callID, "tool_name": toolName,
				"content": `{"status":"aborted","message":"tool call was interrupted before a result was recorded"}`,
			}),
			CreatedAt: time.Now().UTC(),
		})
		if err != nil {
			return repaired, err
		}
		results[callID] = true
		repaired++
	}
	return repaired, nil
}

type agentItemQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

type agentItemDB interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type agentItemRowScanner interface {
	Scan(...any) error
}

func appendAgentTurnItemTx(ctx context.Context, db agentItemDB, input AgentTurnItemInput) (AgentTurnItem, error) {
	input.UserID = strings.TrimSpace(input.UserID)
	input.ConversationID = strings.TrimSpace(input.ConversationID)
	input.TurnID = strings.TrimSpace(input.TurnID)
	input.Kind = strings.TrimSpace(input.Kind)
	if input.UserID == "" || input.ConversationID == "" || input.TurnID == "" || input.Kind == "" {
		return AgentTurnItem{}, fmt.Errorf("user_id, conversation_id, turn_id and kind are required")
	}
	if input.ID == "" {
		input.ID = nextStoreID("item", time.Now().UTC())
	}
	if input.Status == "" {
		input.Status = "completed"
	}
	if input.Visibility == "" {
		input.Visibility = AgentItemVisibilityModel
	}
	if input.PayloadJSON == "" {
		input.PayloadJSON = "{}"
	}
	if !json.Valid([]byte(input.PayloadJSON)) {
		return AgentTurnItem{}, fmt.Errorf("agent item payload must be valid json")
	}
	if input.CreatedAt.IsZero() {
		input.CreatedAt = time.Now().UTC()
	}
	result, err := db.ExecContext(ctx, `
INSERT INTO agent_turn_items
  (id, user_id, conversation_id, run_id, turn_id, kind, role, status, visibility, source_id, payload_json, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT DO NOTHING
`, input.ID, input.UserID, input.ConversationID, input.RunID, input.TurnID, input.Kind, input.Role, input.Status, input.Visibility, input.SourceID, input.PayloadJSON, input.CreatedAt.UTC())
	if err != nil {
		return AgentTurnItem{}, err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return AgentTurnItem{}, err
	}
	row := db.QueryRowContext(ctx, `
SELECT seq, id, user_id, conversation_id, run_id, turn_id, kind, role, status, visibility, source_id, payload_json, created_at
FROM agent_turn_items
WHERE id = ? OR (? <> '' AND user_id = ? AND conversation_id = ? AND kind = ? AND source_id = ?)
ORDER BY CASE WHEN id = ? THEN 0 ELSE 1 END
LIMIT 1
`, input.ID, input.SourceID, input.UserID, input.ConversationID, input.Kind, input.SourceID, input.ID)
	item, err := scanAgentTurnItem(row)
	if err != nil {
		return AgentTurnItem{}, err
	}
	if inserted == 0 && item.RunID == "" && input.RunID != "" && agentTurnItemMatchesInputIgnoringRun(item, input) {
		if _, err := db.ExecContext(ctx, `UPDATE agent_turn_items SET run_id = ? WHERE seq = ? AND run_id = ''`, input.RunID, item.Seq); err != nil {
			return AgentTurnItem{}, err
		}
		item.RunID = input.RunID
	}
	if inserted == 0 && !agentTurnItemMatchesInput(item, input) {
		return AgentTurnItem{}, &AgentTurnItemConflictError{Existing: item, Input: input}
	}
	return item, nil
}

func agentTurnItemMatchesInputIgnoringRun(item AgentTurnItem, input AgentTurnItemInput) bool {
	copyInput := input
	copyInput.RunID = item.RunID
	return agentTurnItemMatchesInput(item, copyInput)
}

func agentTurnItemMatchesInput(item AgentTurnItem, input AgentTurnItemInput) bool {
	return item.UserID == input.UserID &&
		item.ConversationID == input.ConversationID &&
		item.RunID == input.RunID &&
		item.TurnID == input.TurnID &&
		item.Kind == input.Kind &&
		item.Role == input.Role &&
		item.Status == input.Status &&
		item.Visibility == input.Visibility &&
		item.SourceID == input.SourceID &&
		agentItemPayloadEqual(item.PayloadJSON, input.PayloadJSON)
}

func agentItemPayloadEqual(left, right string) bool {
	var leftValue, rightValue any
	if json.Unmarshal([]byte(left), &leftValue) != nil || json.Unmarshal([]byte(right), &rightValue) != nil {
		return strings.TrimSpace(left) == strings.TrimSpace(right)
	}
	return reflect.DeepEqual(leftValue, rightValue)
}

func listAgentTurnItemsTx(ctx context.Context, tx *sql.Tx, userID, conversationID string) ([]AgentTurnItem, error) {
	rows, err := tx.QueryContext(ctx, `
SELECT seq, id, user_id, conversation_id, run_id, turn_id, kind, role, status, visibility, source_id, payload_json, created_at
FROM agent_turn_items
WHERE user_id = ? AND conversation_id = ?
ORDER BY seq ASC
`, userID, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []AgentTurnItem
	for rows.Next() {
		item, err := scanAgentTurnItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func matchingLegacyAssistantItem(items []AgentTurnItem, input AgentTurnItemInput) int {
	wanted := payloadContent(input.PayloadJSON)
	if wanted == "" {
		return -1
	}
	bestIndex := -1
	bestDistance := time.Duration(1<<63 - 1)
	for index, item := range items {
		if item.Kind != "assistant_message" || item.SourceID == input.SourceID {
			continue
		}
		content := payloadContent(item.PayloadJSON)
		if content == "" || (content != wanted && !strings.HasPrefix(wanted, content) && !strings.HasPrefix(content, wanted)) {
			continue
		}
		distance := item.CreatedAt.Sub(input.CreatedAt)
		if distance < 0 {
			distance = -distance
		}
		if distance <= 2*time.Minute && distance < bestDistance {
			bestIndex = index
			bestDistance = distance
		}
	}
	return bestIndex
}

func payloadContent(raw string) string {
	var payload map[string]any
	if json.Unmarshal([]byte(raw), &payload) != nil {
		return ""
	}
	value, _ := payload["content"].(string)
	return strings.TrimSpace(value)
}

func scanAgentTurnItem(row agentItemRowScanner) (AgentTurnItem, error) {
	var item AgentTurnItem
	if err := row.Scan(&item.Seq, &item.ID, &item.UserID, &item.ConversationID, &item.RunID, &item.TurnID,
		&item.Kind, &item.Role, &item.Status, &item.Visibility, &item.SourceID, &item.PayloadJSON, &item.CreatedAt); err != nil {
		return AgentTurnItem{}, err
	}
	item.Payload = json.RawMessage(item.PayloadJSON)
	return item, nil
}

type legacyAgentItem struct {
	createdAt time.Time
	order     int
	input     AgentTurnItemInput
}

func loadLegacyAgentItems(ctx context.Context, tx *sql.Tx, userID, conversationID string) ([]AgentTurnItemInput, error) {
	var legacy []legacyAgentItem
	rows, err := tx.QueryContext(ctx, `
SELECT id, role, content, attachments_json, status, meta_json, created_at
FROM messages
WHERE user_id = ? AND conversation_id = ?
ORDER BY created_at ASC, segment_index ASC
`, userID, conversationID)
	if err != nil {
		return nil, err
	}
	currentTurn := "legacy-" + conversationID
	order := 0
	for rows.Next() {
		var id, role, content, attachmentsJSON, status, metaJSON string
		var createdAt time.Time
		if err := rows.Scan(&id, &role, &content, &attachmentsJSON, &status, &metaJSON, &createdAt); err != nil {
			rows.Close()
			return nil, err
		}
		if status != "final" || (role != "user" && role != "assistant") {
			continue
		}
		if role == "user" {
			currentTurn = id
		}
		payload := map[string]any{"content": content}
		if filtered := modelAttachmentsJSON(attachmentsJSON); len(filtered) > 0 {
			payload["attachments"] = filtered
		}
		raw, _ := json.Marshal(payload)
		kind := role + "_message"
		runID := ""
		var meta map[string]any
		if json.Unmarshal([]byte(metaJSON), &meta) == nil {
			runID, _ = meta["run_id"].(string)
		}
		legacy = append(legacy, legacyAgentItem{createdAt: createdAt, order: order, input: AgentTurnItemInput{
			UserID: userID, ConversationID: conversationID, RunID: strings.TrimSpace(runID), TurnID: currentTurn, Kind: kind, Role: role,
			Status: "completed", Visibility: AgentItemVisibilityModel, SourceID: id,
			PayloadJSON: string(raw), CreatedAt: createdAt,
		}})
		order++
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	toolRows, err := tx.QueryContext(ctx, `
SELECT id, run_id, message_id, tool_name, arguments_json, result_json, result_summary, status, error, changed_paths_json, content_hash, created_at
FROM tool_calls
WHERE user_id = ? AND conversation_id = ?
ORDER BY created_at ASC
`, userID, conversationID)
	if err != nil {
		return nil, err
	}
	for toolRows.Next() {
		var id, runID, messageID, toolName, argsJSON, resultJSON, resultSummary, status, errorText, changedPathsJSON, contentHash string
		var createdAt time.Time
		if err := toolRows.Scan(&id, &runID, &messageID, &toolName, &argsJSON, &resultJSON, &resultSummary, &status, &errorText, &changedPathsJSON, &contentHash, &createdAt); err != nil {
			toolRows.Close()
			return nil, err
		}
		turnID := strings.TrimSpace(messageID)
		if turnID == "" {
			turnID = "legacy-" + conversationID
		}
		callPayload := mustStoreJSON(map[string]any{"tool_call_id": id, "tool_name": toolName, "arguments_json": validJSONObjectOrString(argsJSON)})
		legacy = append(legacy, legacyAgentItem{createdAt: createdAt, order: order, input: AgentTurnItemInput{
			UserID: userID, ConversationID: conversationID, RunID: runID, TurnID: turnID,
			Kind: "tool_call", Role: "assistant", Status: "completed", Visibility: AgentItemVisibilityModel,
			SourceID: id, PayloadJSON: callPayload, CreatedAt: createdAt,
		}})
		order++
		resultKind := "tool_result"
		resultStatus := status
		if resultStatus == "" {
			resultStatus = "completed"
		}
		if strings.TrimSpace(resultJSON) == "" {
			resultKind = "aborted"
			resultStatus = "aborted"
			resultJSON = mustStoreJSON(map[string]any{"status": "aborted", "message": firstStoreNonEmpty(errorText, resultSummary, "legacy tool result is unavailable")})
		}
		resultPayload := mustStoreJSON(map[string]any{
			"tool_call_id": id, "tool_name": toolName, "content": validJSONOrString(resultJSON),
			"summary": resultSummary, "error": errorText, "changed_paths": jsonStringSlice(changedPathsJSON), "content_hash": contentHash,
		})
		legacy = append(legacy, legacyAgentItem{createdAt: createdAt.Add(time.Nanosecond), order: order, input: AgentTurnItemInput{
			UserID: userID, ConversationID: conversationID, RunID: runID, TurnID: turnID,
			Kind: resultKind, Role: "tool", Status: resultStatus, Visibility: AgentItemVisibilityModel,
			SourceID: id, PayloadJSON: resultPayload, CreatedAt: createdAt.Add(time.Nanosecond),
		}})
		order++
	}
	if err := toolRows.Close(); err != nil {
		return nil, err
	}

	sort.SliceStable(legacy, func(i, j int) bool {
		if legacy[i].createdAt.Equal(legacy[j].createdAt) {
			return legacy[i].order < legacy[j].order
		}
		return legacy[i].createdAt.Before(legacy[j].createdAt)
	})
	items := make([]AgentTurnItemInput, 0, len(legacy))
	for _, item := range legacy {
		items = append(items, item.input)
	}
	return items, nil
}

func modelAttachmentsJSON(raw string) []map[string]any {
	var values []map[string]any
	if json.Unmarshal([]byte(raw), &values) != nil {
		return nil
	}
	filtered := values[:0]
	for _, value := range values {
		kind, _ := value["kind"].(string)
		if kind == "current_file" || kind == "editor_panels_context" {
			continue
		}
		filtered = append(filtered, value)
	}
	return filtered
}

func validJSONObjectOrString(raw string) any {
	var value any
	if json.Unmarshal([]byte(raw), &value) == nil {
		return value
	}
	return raw
}

func validJSONOrString(raw string) any { return validJSONObjectOrString(raw) }

func jsonStringSlice(raw string) []string {
	var values []string
	_ = json.Unmarshal([]byte(raw), &values)
	return values
}

func mustStoreJSON(value any) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}

func firstStoreNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

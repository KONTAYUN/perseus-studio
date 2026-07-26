package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

type MessagePresentationRepairReport struct {
	ScannedMessages      int
	UpdatedMessages      int
	RetainedQuestions    int
	RemovedDuplicates    int
	AmbiguousAttachments int
}

type presentationAttachmentIdentity struct {
	ToolCallID string `json:"tool_call_id"`
	ToolName   string `json:"tool_name"`
}

func (s *ConversationStore) RepairMessagePresentationEvents(ctx context.Context) (MessagePresentationRepairReport, error) {
	var report MessagePresentationRepairReport
	if _, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS message_presentation_repairs (
  message_id TEXT NOT NULL,
  run_id TEXT NOT NULL,
  tool_call_id TEXT NOT NULL,
  action TEXT NOT NULL,
  repaired_at DATETIME NOT NULL,
  PRIMARY KEY (message_id, run_id, tool_call_id, action)
)
`); err != nil {
		return report, err
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT id, attachments_json, meta_json
FROM messages
WHERE role = 'assistant' AND attachments_json <> '' AND attachments_json <> '[]'
ORDER BY conversation_id ASC, created_at ASC, id ASC
`)
	if err != nil {
		return report, err
	}
	type candidate struct {
		id          string
		attachments string
		meta        string
	}
	var candidates []candidate
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.id, &item.attachments, &item.meta); err != nil {
			rows.Close()
			return report, err
		}
		candidates = append(candidates, item)
	}
	if err := rows.Close(); err != nil {
		return report, err
	}
	if err := rows.Err(); err != nil {
		return report, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return report, err
	}
	defer tx.Rollback()

	seen := make(map[string]struct{})
	for _, item := range candidates {
		report.ScannedMessages++
		var attachments []json.RawMessage
		if err := json.Unmarshal([]byte(item.attachments), &attachments); err != nil {
			report.AmbiguousAttachments++
			continue
		}
		meta := parseMessagePresentationMeta(item.meta)
		runID := strings.TrimSpace(messageMetaString(meta, "run_id"))
		kept := make([]json.RawMessage, 0, len(attachments))
		retainedQuestion := false
		removedQuestion := false
		for _, raw := range attachments {
			var identity presentationAttachmentIdentity
			if err := json.Unmarshal(raw, &identity); err != nil || identity.ToolName != "ask_followup_question" {
				kept = append(kept, raw)
				continue
			}
			toolCallID := strings.TrimSpace(identity.ToolCallID)
			if runID == "" || toolCallID == "" {
				report.AmbiguousAttachments++
				kept = append(kept, raw)
				continue
			}
			key := runID + "\x00" + toolCallID
			if _, exists := seen[key]; !exists {
				seen[key] = struct{}{}
				retainedQuestion = true
				report.RetainedQuestions++
				kept = append(kept, raw)
				if err := recordMessagePresentationRepair(ctx, tx, item.id, runID, toolCallID, "retained"); err != nil {
					return report, err
				}
				continue
			}
			removedQuestion = true
			report.RemovedDuplicates++
			if err := recordMessagePresentationRepair(ctx, tx, item.id, runID, toolCallID, "removed_duplicate"); err != nil {
				return report, err
			}
		}

		changed := removedQuestion
		if retainedQuestion && messageMetaString(meta, "kind") != "followup_question" {
			meta["kind"] = "followup_question"
			changed = true
		} else if removedQuestion && !containsFollowupAttachment(kept) && messageMetaString(meta, "kind") == "followup_question" {
			meta["kind"] = "assistant_segment"
			changed = true
		}
		if !changed {
			continue
		}
		attachmentsJSON, err := json.Marshal(kept)
		if err != nil {
			return report, err
		}
		metaJSON, err := json.Marshal(meta)
		if err != nil {
			return report, err
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE messages
SET attachments_json = ?, meta_json = ?, updated_at = ?
WHERE id = ?
`, string(attachmentsJSON), string(metaJSON), time.Now().UTC(), item.id); err != nil {
			return report, err
		}
		report.UpdatedMessages++
	}
	if err := tx.Commit(); err != nil {
		return report, err
	}
	return report, nil
}

func parseMessagePresentationMeta(raw string) map[string]any {
	meta := make(map[string]any)
	if strings.TrimSpace(raw) != "" {
		_ = json.Unmarshal([]byte(raw), &meta)
	}
	return meta
}

func messageMetaString(meta map[string]any, key string) string {
	value, _ := meta[key].(string)
	return strings.TrimSpace(value)
}

func containsFollowupAttachment(items []json.RawMessage) bool {
	for _, raw := range items {
		var identity presentationAttachmentIdentity
		if json.Unmarshal(raw, &identity) == nil && identity.ToolName == "ask_followup_question" {
			return true
		}
	}
	return false
}

func recordMessagePresentationRepair(ctx context.Context, tx *sql.Tx, messageID, runID, toolCallID, action string) error {
	result, err := tx.ExecContext(ctx, `
INSERT OR IGNORE INTO message_presentation_repairs (message_id, run_id, tool_call_id, action, repaired_at)
VALUES (?, ?, ?, ?, ?)
`, messageID, runID, toolCallID, action, time.Now().UTC())
	if err != nil {
		return err
	}
	if rows, err := result.RowsAffected(); err != nil {
		return err
	} else if rows > 1 {
		return fmt.Errorf("unexpected presentation repair insert count: %d", rows)
	}
	return nil
}

func logMessagePresentationRepair(report MessagePresentationRepairReport) {
	if report.UpdatedMessages == 0 && report.AmbiguousAttachments == 0 {
		return
	}
	log.Printf(
		"message presentation repair: scanned=%d updated=%d retained=%d removed_duplicates=%d ambiguous=%d",
		report.ScannedMessages,
		report.UpdatedMessages,
		report.RetainedQuestions,
		report.RemovedDuplicates,
		report.AmbiguousAttachments,
	)
}

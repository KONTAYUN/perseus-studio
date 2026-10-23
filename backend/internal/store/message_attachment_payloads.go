package store

import (
	"context"
	"database/sql"
)

// Payloads are private message context, never returned by the public Message JSON.
// Admission and branch copies commit them with their owning message.
func saveMessageAttachmentPayloads(ctx context.Context, tx *sql.Tx, userID, messageID, payload string) error {
	if payload == "" || payload == "[]" || payload == "null" {
		return nil
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO message_attachment_payloads(message_id,user_id,payload_json) VALUES (?,?,?)`, messageID, userID, payload)
	return err
}

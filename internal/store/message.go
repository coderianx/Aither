package store

import (
	"database/sql"
	"fmt"

	"github.com/coderianx/Aither/internal/models"
)

const messageColumns = `id, conversation_id, role, content, created_at`

// CreateMessage appends a message to a conversation and returns the stored record.
func CreateMessage(
	conversationID int64,
	role, content string,
) (*models.Message, error) {
	res, err := DB.Exec(
		`INSERT INTO messages (conversation_id, role, content) VALUES (?, ?, ?)`,
		conversationID, role, content,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert message: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to read message id: %w", err)
	}

	var m models.Message

	if err := DB.QueryRow(
		`SELECT `+messageColumns+` FROM messages WHERE id = ?`, id,
	).Scan(
		&m.ID,
		&m.ConversationID,
		&m.Role,
		&m.Content,
		&m.CreatedAt,
	); err != nil {
		return nil, fmt.Errorf("failed to read message: %w", err)
	}

	return &m, nil
}

// ListMessages returns every message of a conversation in chronological order.
func ListMessages(conversationID int64) ([]models.Message, error) {
	rows, err := DB.Query(
		`SELECT `+messageColumns+`
		 FROM messages
		 WHERE conversation_id = ?
		 ORDER BY id ASC`,
		conversationID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()

	return scanMessages(rows)
}

// ListRecentMessages returns the last `limit` messages of a conversation in
// chronological order. Use it to build the LLM context window.
func ListRecentMessages(conversationID int64, limit int) ([]models.Message, error) {
	rows, err := DB.Query(
		`SELECT `+messageColumns+`
		 FROM (
			SELECT `+messageColumns+`
			FROM messages
			WHERE conversation_id = ?
			ORDER BY id DESC
			LIMIT ?
		 )
		 ORDER BY id ASC`,
		conversationID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query recent messages: %w", err)
	}
	defer rows.Close()

	return scanMessages(rows)
}

// CountMessages returns how many messages a conversation holds.
func CountMessages(conversationID int64) (int, error) {
	var count int

	if err := DB.QueryRow(
		`SELECT COUNT(*) FROM messages WHERE conversation_id = ?`,
		conversationID,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count messages: %w", err)
	}

	return count, nil
}

func scanMessages(rows *sql.Rows) ([]models.Message, error) {
	messages := make([]models.Message, 0)

	for rows.Next() {
		var m models.Message

		if err := rows.Scan(
			&m.ID,
			&m.ConversationID,
			&m.Role,
			&m.Content,
			&m.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}

		messages = append(messages, m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate messages: %w", err)
	}

	return messages, nil
}

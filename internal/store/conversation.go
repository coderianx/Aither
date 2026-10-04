package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/coderianx/Aither/internal/models"
)

const conversationColumns = `id, provider_id, title, model, system_prompt, created_at, updated_at`

// CreateConversation inserts a new conversation and returns the stored record.
func CreateConversation(
	providerID *int64,
	title, model, systemPrompt string,
) (*models.Conversation, error) {
	if title == "" {
		title = "New Chat"
	}

	res, err := DB.Exec(
		`INSERT INTO conversations (provider_id, title, model, system_prompt)
		 VALUES (?, ?, ?, ?)`,
		providerID, title, model, systemPrompt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert conversation: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to read conversation id: %w", err)
	}

	return GetConversation(id)
}

// ListConversations returns all conversations, most recently updated first.
func ListConversations() ([]models.Conversation, error) {
	rows, err := DB.Query(
		`SELECT ` + conversationColumns + `
		 FROM conversations
		 ORDER BY updated_at DESC, id DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query conversations: %w", err)
	}
	defer rows.Close()

	conversations := make([]models.Conversation, 0)

	for rows.Next() {
		c, err := scanConversation(rows)
		if err != nil {
			return nil, err
		}

		conversations = append(conversations, *c)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate conversations: %w", err)
	}

	return conversations, nil
}

// GetConversation returns a single conversation by ID.
func GetConversation(id int64) (*models.Conversation, error) {
	row := DB.QueryRow(
		`SELECT `+conversationColumns+` FROM conversations WHERE id = ?`, id,
	)

	c, err := scanConversation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return c, nil
}

// UpdateConversation applies partial updates and returns the updated record.
func UpdateConversation(
	id int64,
	req models.UpdateConversationReq,
) (*models.Conversation, error) {
	set := make([]string, 0, 4)
	args := make([]any, 0, 4)

	if req.ProviderID != nil {
		set = append(set, "provider_id = ?")
		args = append(args, *req.ProviderID)
	}
	if req.Title != nil {
		set = append(set, "title = ?")
		args = append(args, *req.Title)
	}
	if req.Model != nil {
		set = append(set, "model = ?")
		args = append(args, *req.Model)
	}
	if req.SystemPrompt != nil {
		set = append(set, "system_prompt = ?")
		args = append(args, *req.SystemPrompt)
	}

	if len(set) == 0 {
		return GetConversation(id)
	}

	set = append(set, "updated_at = CURRENT_TIMESTAMP")
	args = append(args, id)

	res, err := DB.Exec(
		`UPDATE conversations SET `+strings.Join(set, ", ")+` WHERE id = ?`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to update conversation: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to read update result: %w", err)
	}

	if affected == 0 {
		return nil, ErrNotFound
	}

	return GetConversation(id)
}

// TouchConversation bumps updated_at after new activity.
func TouchConversation(id int64) error {
	_, err := DB.Exec(
		`UPDATE conversations SET updated_at = CURRENT_TIMESTAMP WHERE id = ?`, id,
	)
	if err != nil {
		return fmt.Errorf("failed to touch conversation: %w", err)
	}

	return nil
}

// DeleteConversation removes a conversation and, by cascade, its messages.
func DeleteConversation(id int64) error {
	res, err := DB.Exec(`DELETE FROM conversations WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete conversation: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to read delete result: %w", err)
	}

	if affected == 0 {
		return ErrNotFound
	}

	return nil
}

// rowScanner is implemented by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanConversation(s rowScanner) (*models.Conversation, error) {
	var (
		c          models.Conversation
		providerID sql.NullInt64
	)

	if err := s.Scan(
		&c.ID,
		&providerID,
		&c.Title,
		&c.Model,
		&c.SystemPrompt,
		&c.CreatedAt,
		&c.UpdatedAt,
	); err != nil {
		return nil, err
	}

	if providerID.Valid {
		c.ProviderID = &providerID.Int64
	}

	return &c, nil
}

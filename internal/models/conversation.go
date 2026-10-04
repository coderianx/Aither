package models

import "time"

// Conversation is a chat thread bound to a provider and model.
type Conversation struct {
	ID           int64     `json:"id"`
	ProviderID   *int64    `json:"provider_id"`
	Title        string    `json:"title"`
	Model        string    `json:"model"`
	SystemPrompt string    `json:"system_prompt"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// CreateConversationReq is the request body for creating a conversation.
type CreateConversationReq struct {
	ProviderID   *int64 `json:"provider_id"`
	Title        string `json:"title"`
	Model        string `json:"model"`
	SystemPrompt string `json:"system_prompt"`
}

// UpdateConversationReq is the request body for updating a conversation.
// Nil fields are left untouched.
type UpdateConversationReq struct {
	ProviderID   *int64  `json:"provider_id"`
	Title        *string `json:"title"`
	Model        *string `json:"model"`
	SystemPrompt *string `json:"system_prompt"`
}

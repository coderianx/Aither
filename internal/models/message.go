package models

import "time"

// Message roles accepted by the messages table.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// Message is a single message inside a conversation.
type Message struct {
	ID             int64     `json:"id"`
	ConversationID int64     `json:"conversation_id"`
	Role           string    `json:"role"`
	Content        string    `json:"content"`
	CreatedAt      time.Time `json:"created_at"`
}

// CreateMessageReq is the request body for sending a message.
type CreateMessageReq struct {
	Content string `json:"content"`
	Stream  bool   `json:"stream"`
}

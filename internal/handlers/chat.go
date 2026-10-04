package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/coderianx/Aither/internal/models"
	"github.com/coderianx/Aither/internal/provider"
	"github.com/coderianx/Aither/internal/store"
)

// maxContextMessages caps how many stored messages are replayed to the
// provider. Older messages are dropped once a conversation grows past it.
const maxContextMessages = 200

// SendMessage stores the user message, replays the whole conversation history
// to the provider and stores the assistant reply. When the request asks for
// streaming the reply is delivered as server-sent events.
func SendMessage(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamID(w, r, "conversation_id")
	if !ok {
		return
	}

	var req models.CreateMessageReq

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	if strings.TrimSpace(req.Content) == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}

	conversation, ok := loadConversation(w, id)
	if !ok {
		return
	}

	if conversation.Model == "" {
		writeError(w, http.StatusBadRequest, "Conversation has no model")
		return
	}

	if conversation.ProviderID == nil {
		writeError(w, http.StatusBadRequest, "Conversation has no provider")
		return
	}

	storedProvider, err := store.GetProvider(*conversation.ProviderID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusBadRequest, "Provider not found")
			return
		}

		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	// The user message is persisted first so it is part of the history that is
	// sent to the provider on this very request.
	userMessage, ok := saveMessage(w, id, models.RoleUser, req.Content)
	if !ok {
		return
	}

	messages, ok := buildContext(w, conversation)
	if !ok {
		return
	}

	client := provider.NewClient(storedProvider.BaseURL, storedProvider.APIKey)

	if req.Stream {
		streamReply(w, r, client, conversation, userMessage, messages)
		return
	}

	reply, err := client.Chat(r.Context(), conversation.Model, messages)
	if err != nil {
		writeProviderError(w, err)
		return
	}

	if strings.TrimSpace(reply) == "" {
		writeError(w, http.StatusBadGateway, "Provider returned an empty response")
		return
	}

	assistantMessage, ok := saveMessage(w, id, models.RoleAssistant, reply)
	if !ok {
		return
	}

	if err := store.TouchConversation(id); err != nil {
		log.Printf("failed to touch conversation: %v", err)
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"user_message":      userMessage,
		"assistant_message": assistantMessage,
	})
}

// streamReply forwards the provider stream to the client as server-sent events
// and stores the assembled reply once the stream finishes.
func streamReply(
	w http.ResponseWriter,
	r *http.Request,
	client *provider.Client,
	conversation *models.Conversation,
	userMessage *models.Message,
	messages []provider.ChatMessage,
) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "Streaming is not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Ask reverse proxies not to buffer the stream.
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	if err := writeSSE(w, "user_message", userMessage); err != nil {
		return
	}

	flusher.Flush()

	var reply strings.Builder

	err := client.ChatStream(
		r.Context(),
		conversation.Model,
		messages,
		func(chunk string) error {
			reply.WriteString(chunk)

			if err := writeSSE(w, "delta", map[string]string{"content": chunk}); err != nil {
				return err
			}

			flusher.Flush()

			return nil
		},
	)
	if err != nil {
		// The client aborted the request: keep the text received so far so the
		// stored history stays consistent with what the user already saw.
		if errors.Is(err, context.Canceled) {
			persistPartialReply(conversation.ID, reply.String())
			return
		}

		// The response is already committed, so errors are reported in-band.
		writeSSE(w, "error", map[string]string{"error": providerErrorMessage(err)})
		flusher.Flush()

		return
	}

	text := strings.TrimSpace(reply.String())
	if text == "" {
		writeSSE(w, "error", map[string]string{"error": "Provider returned an empty response"})
		flusher.Flush()

		return
	}

	assistantMessage, err := store.CreateMessage(
		conversation.ID,
		models.RoleAssistant,
		text,
	)
	if err != nil {
		log.Printf("failed to store assistant message: %v", err)

		writeSSE(w, "error", map[string]string{"error": "Database error"})
		flusher.Flush()

		return
	}

	if err := store.TouchConversation(conversation.ID); err != nil {
		log.Printf("failed to touch conversation: %v", err)
	}

	writeSSE(w, "done", assistantMessage)
	flusher.Flush()
}

// persistPartialReply stores whatever was received before the client went
// away, so a stopped generation is still part of the conversation history.
func persistPartialReply(conversationID int64, reply string) {
	text := strings.TrimSpace(reply)
	if text == "" {
		return
	}

	if _, err := store.CreateMessage(conversationID, models.RoleAssistant, text); err != nil {
		log.Printf("failed to store partial reply: %v", err)
		return
	}

	if err := store.TouchConversation(conversationID); err != nil {
		log.Printf("failed to touch conversation: %v", err)
	}
}

// buildContext converts the stored history into provider messages: the
// conversation system prompt first, then every recent message in order.
func buildContext(
	w http.ResponseWriter,
	conversation *models.Conversation,
) ([]provider.ChatMessage, bool) {
	history, err := store.ListRecentMessages(conversation.ID, maxContextMessages)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return nil, false
	}

	messages := make([]provider.ChatMessage, 0, len(history)+1)

	if conversation.SystemPrompt != "" {
		messages = append(messages, provider.ChatMessage{
			Role:    models.RoleSystem,
			Content: conversation.SystemPrompt,
		})
	}

	for _, m := range history {
		messages = append(messages, provider.ChatMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	return messages, true
}

// loadConversation fetches a conversation or writes the matching error.
func loadConversation(w http.ResponseWriter, id int64) (*models.Conversation, bool) {
	conversation, err := store.GetConversation(id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Conversation not found")
			return nil, false
		}

		writeError(w, http.StatusInternalServerError, "Database error")
		return nil, false
	}

	return conversation, true
}

// saveMessage stores a message or writes the matching error.
func saveMessage(
	w http.ResponseWriter,
	conversationID int64,
	role, content string,
) (*models.Message, bool) {
	message, err := store.CreateMessage(conversationID, role, content)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return nil, false
	}

	return message, true
}

// writeProviderError maps a provider failure to an HTTP response.
func writeProviderError(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway

	var apiErr *provider.APIError

	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusTooManyRequests {
		status = http.StatusTooManyRequests
	}

	writeError(w, status, providerErrorMessage(err))
}

// providerErrorMessage turns a provider failure into a user-facing message
// without leaking the raw upstream body.
func providerErrorMessage(err error) string {
	var apiErr *provider.APIError

	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return "Provider rejected the API key"
		case http.StatusTooManyRequests:
			return "Provider rate limit reached"
		case http.StatusBadRequest:
			return "Provider rejected the request: " + apiErr.Message
		default:
			return fmt.Sprintf("Provider error (%d)", apiErr.StatusCode)
		}
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return "Provider request timed out"
	}

	if errors.Is(err, context.Canceled) {
		return "Request cancelled"
	}

	return "Failed to reach provider"
}

// writeSSE writes one server-sent event.
func writeSSE(w http.ResponseWriter, event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to encode event: %w", err)
	}

	if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data); err != nil {
		return fmt.Errorf("failed to write event: %w", err)
	}

	return nil
}

package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/coderianx/Aither/internal/models"
	"github.com/coderianx/Aither/internal/store"
)

// CreateConversation starts a new chat thread.
func CreateConversation(w http.ResponseWriter, r *http.Request) {
	var req models.CreateConversationReq

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	if req.Model == "" {
		writeError(w, http.StatusBadRequest, "model is required")
		return
	}

	if req.ProviderID != nil {
		if _, err := store.GetProvider(*req.ProviderID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "Provider not found")
				return
			}

			writeError(w, http.StatusInternalServerError, "Database error")
			return
		}
	}

	conversation, err := store.CreateConversation(
		req.ProviderID,
		req.Title,
		req.Model,
		req.SystemPrompt,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	writeJSON(w, http.StatusCreated, conversation)
}

// ListConversations returns every chat thread, newest activity first.
func ListConversations(w http.ResponseWriter, r *http.Request) {
	conversations, err := store.ListConversations()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	writeJSON(w, http.StatusOK, conversations)
}

// GetConversation returns a single chat thread by ID.
func GetConversation(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamID(w, r, "conversation_id")
	if !ok {
		return
	}

	conversation, err := store.GetConversation(id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Conversation not found")
			return
		}

		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	writeJSON(w, http.StatusOK, conversation)
}

// UpdateConversation renames a chat thread or changes its model and system prompt.
func UpdateConversation(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamID(w, r, "conversation_id")
	if !ok {
		return
	}

	var req models.UpdateConversationReq

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	conversation, err := store.UpdateConversation(id, req)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Conversation not found")
			return
		}

		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	writeJSON(w, http.StatusOK, conversation)
}

// DeleteConversation removes a chat thread and, by cascade, its messages.
func DeleteConversation(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamID(w, r, "conversation_id")
	if !ok {
		return
	}

	if err := store.DeleteConversation(id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Conversation not found")
			return
		}

		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message": "Conversation deleted successfully",
	})
}

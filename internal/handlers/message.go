package handlers

import (
	"errors"
	"net/http"

	"github.com/coderianx/Aither/internal/store"
)

// ListMessages returns every message of a conversation in chronological order.
func ListMessages(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamID(w, r, "conversation_id")
	if !ok {
		return
	}

	if _, err := store.GetConversation(id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Conversation not found")
			return
		}

		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	messages, err := store.ListMessages(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	writeJSON(w, http.StatusOK, messages)
}

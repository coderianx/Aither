package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/coderianx/Aither/internal/models"
	"github.com/coderianx/Aither/internal/provider"
	"github.com/coderianx/Aither/internal/store"
)

// Default base URLs for built-in providers. Both speak the OpenAI API format.
var (
	openRouterBase = "https://openrouter.ai/api/v1"
	groqBase       = "https://api.groq.com/openai/v1"
)

// ConnectProvider stores a new provider configuration.
func ConnectProvider(
	w http.ResponseWriter,
	r *http.Request,
) {
	var req models.ConnectProviderReq

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	if req.ProviderName == "" || req.APIKey == "" {
		writeError(w, http.StatusBadRequest, "provider_name and api_key are required")
		return
	}

	if req.BaseURL == "" {
		switch req.ProviderName {
		case "openrouter":
			req.BaseURL = openRouterBase
		case "groq":
			req.BaseURL = groqBase
		default:
			writeError(w, http.StatusBadRequest, "base_url is required for this provider")
			return
		}
	}

	id, err := store.CreateProvider(req.ProviderName, req.APIKey, req.BaseURL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":      id,
		"message": "Provider created successfully",
	})
}

// DeleteProvider removes a stored provider configuration.
func DeleteProvider(
	w http.ResponseWriter,
	r *http.Request,
) {
	providerID, ok := urlParamID(w, r, "provider_id")
	if !ok {
		return
	}

	if err := store.DeleteProvider(providerID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Provider not found")
			return
		}

		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message": "Provider deleted successfully",
	})
}

// ListProviders returns every configured provider.
func ListProviders(w http.ResponseWriter, r *http.Request) {
	providers, err := store.ListProviders()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	writeJSON(w, http.StatusOK, providers)
}

// ListModels proxies the model catalogue of a configured provider so the web
// client can offer a model picker.
func ListModels(w http.ResponseWriter, r *http.Request) {
	providerID, ok := urlParamID(w, r, "provider_id")
	if !ok {
		return
	}

	stored, err := store.GetProvider(providerID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "Provider not found")
			return
		}

		writeError(w, http.StatusInternalServerError, "Database error")
		return
	}

	client := provider.NewClient(stored.BaseURL, stored.APIKey)

	models, err := client.ListModels(r.Context())
	if err != nil {
		writeProviderError(w, err)
		return
	}

	if models == nil {
		models = []provider.Model{}
	}

	writeJSON(w, http.StatusOK, models)
}

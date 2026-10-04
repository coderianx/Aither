package models

import "time"

// ConnectProviderReq is the request body for connecting a new provider.
type ConnectProviderReq struct {
	ProviderName string `json:"provider_name"`
	APIKey       string `json:"api_key"`
	BaseURL      string `json:"base_url"`
}

// Provider is a stored LLM provider configuration.
type Provider struct {
	ID           int64     `json:"id"`
	ProviderName string    `json:"provider_name"`
	APIKey       string    `json:"api_key"`
	BaseURL      string    `json:"base_url"`
	CreatedAt    time.Time `json:"created_at"`
}

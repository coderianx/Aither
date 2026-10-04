package provider

// Package provider implements a client for OpenAI-compatible chat completion
// APIs. Both OpenRouter and Groq speak this format.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// ChatMessage is a single message in the OpenAI chat format.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// APIError is returned when the provider answers with a non-2xx status or
// reports an error inside a stream chunk.
type APIError struct {
	StatusCode int
	Message    string
}

// Error implements the error interface.
func (e *APIError) Error() string {
	return fmt.Sprintf("provider error (%d): %s", e.StatusCode, e.Message)
}

// Client talks to an OpenAI-compatible /chat/completions endpoint.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// NewClient creates a client for the given base URL and API key.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http: &http.Client{
			// Long completions can take minutes; the request context is the
			// real cancellation mechanism, this is only a safety net.
			Timeout: 5 * time.Minute,
		},
	}
}

// Chat sends the given history and returns the full assistant reply.
func (c *Client) Chat(
	ctx context.Context,
	model string,
	messages []ChatMessage,
) (string, error) {
	resp, err := c.do(ctx, chatRequest{Model: model, Messages: messages})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var parsed chatResponse

	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("failed to decode provider response: %w", err)
	}

	if parsed.Error != nil {
		return "", &APIError{StatusCode: resp.StatusCode, Message: parsed.Error.Message}
	}

	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("provider returned no choices")
	}

	return parsed.Choices[0].Message.Content, nil
}

// ChatStream sends the given history and invokes onDelta for every streamed
// chunk. The full reply is the concatenation of all chunks. Returning an error
// from onDelta aborts the stream.
func (c *Client) ChatStream(
	ctx context.Context,
	model string,
	messages []ChatMessage,
	onDelta func(chunk string) error,
) error {
	resp, err := c.do(ctx, chatRequest{
		Model:    model,
		Messages: messages,
		Stream:   true,
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	// Server-sent event lines can be far larger than the 64 KiB default.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Ignore comments (": OPENROUTER PROCESSING") and blank separators.
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))

		if payload == "" {
			continue
		}

		if payload == "[DONE]" {
			return nil
		}

		var chunk chatResponse

		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return fmt.Errorf("failed to decode stream chunk: %w", err)
		}

		if chunk.Error != nil {
			return &APIError{StatusCode: resp.StatusCode, Message: chunk.Error.Message}
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		if delta := chunk.Choices[0].Delta.Content; delta != "" {
			if err := onDelta(delta); err != nil {
				return err
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to read provider stream: %w", err)
	}

	return nil
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream,omitempty"`
}

// Model is one entry of a provider's model catalogue.
type Model struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ListModels returns the models the provider advertises on /models.
func (c *Client) ListModels(ctx context.Context) ([]Model, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		c.baseURL+"/models",
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build models request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call provider: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, newAPIError(resp)
	}

	var parsed struct {
		Data []Model `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("failed to decode models response: %w", err)
	}

	sort.Slice(parsed.Data, func(i, j int) bool {
		return parsed.Data[i].ID < parsed.Data[j].ID
	})

	return parsed.Data, nil
}

type chatChoice struct {
	Message      ChatMessage `json:"message"`
	Delta        ChatMessage `json:"delta"`
	FinishReason string      `json:"finish_reason"`
}

type chatResponse struct {
	Choices []chatChoice  `json:"choices"`
	Error   *apiErrorBody `json:"error"`
}

type apiErrorBody struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// do performs the HTTP call and turns non-2xx answers into *APIError.
func (c *Client) do(ctx context.Context, payload chatRequest) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to encode provider request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/chat/completions",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build provider request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call provider: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		defer resp.Body.Close()

		return nil, newAPIError(resp)
	}

	return resp, nil
}

// newAPIError reads the (truncated) error body of a failed response.
func newAPIError(resp *http.Response) error {
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return &APIError{
			StatusCode: resp.StatusCode,
			Message:    http.StatusText(resp.StatusCode),
		}
	}

	var body apiErrorBody

	if err := json.Unmarshal(raw, &body); err == nil && body.Message != "" {
		return &APIError{StatusCode: resp.StatusCode, Message: body.Message}
	}

	message := strings.TrimSpace(string(raw))
	if message == "" {
		message = http.StatusText(resp.StatusCode)
	}

	return &APIError{StatusCode: resp.StatusCode, Message: message}
}

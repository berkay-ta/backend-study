package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	httpClient *http.Client
	apiKey     string
	model      string
	baseURL    string
}

type Options struct {
	APIKey     string
	Model      string
	BaseURL    string
	Timeout    time.Duration
	HTTPClient *http.Client
}

func New(opt Options) (*Client, error) {
	if opt.APIKey == "" {
		return nil, fmt.Errorf("openai: APIKey is required")
	}
	if opt.Model == "" {
		return nil, fmt.Errorf("openai: Model is required")
	}
	base := opt.BaseURL
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	timeout := opt.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	hc := opt.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: timeout}
	}
	return &Client{
		httpClient: hc,
		apiKey:     opt.APIKey,
		model:      opt.Model,
		baseURL:    base,
	}, nil
}

type ResponseFormat struct {
	Type       string          `json:"type"`
	JSONSchema json.RawMessage `json:"json_schema,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model          string         `json:"model"`
	Messages       []Message      `json:"messages"`
	ResponseFormat ResponseFormat `json:"response_format,omitempty"`
	Temperature    float64        `json:"temperature,omitempty"`
}

type ChatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

// Complete posts req to /chat/completions and returns the first assistant
// message content (a JSON document matching the supplied json_schema).
func (c *Client) Complete(ctx context.Context, req ChatRequest) (string, error) {
	if req.Model == "" {
		req.Model = c.model
	}
	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("openai: marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("openai: new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("openai: do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("openai: read response: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("openai: status=%d body=%s", resp.StatusCode, truncate(raw, 512))
	}

	var parsed ChatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("openai: decode response: %w", err)
	}
	if parsed.Error != nil {
		return "", fmt.Errorf("openai: api error: %s (%s)", parsed.Error.Message, parsed.Error.Code)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("openai: empty choices")
	}
	return parsed.Choices[0].Message.Content, nil
}

// truncate clips b to at most n bytes so a large error body can't flood logs.
func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}

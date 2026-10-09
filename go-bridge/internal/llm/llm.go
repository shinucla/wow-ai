package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/chelinho139/wow-ai/go-bridge/internal/config"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Client struct {
	cfg    config.LLMConfig
	http   *http.Client
	primer string
}

func New(cfg config.LLMConfig, primer string) *Client {
	return &Client{
		cfg:    cfg,
		http:   &http.Client{Timeout: 10 * time.Minute},
		primer: primer,
	}
}

func (c *Client) Update(cfg config.LLMConfig) { c.cfg = cfg }

func (c *Client) AgentName() string {
	if c.cfg.AgentName != "" {
		return c.cfg.AgentName
	}
	return "api"
}

// Chat sends a user prompt with optional system context and returns assistant text.
func (c *Client) Chat(ctx context.Context, system, user string, history []Message) (string, error) {
	if strings.TrimSpace(c.cfg.APIKey) == "" {
		return "", fmt.Errorf("API key is empty — set it in Settings")
	}
	provider := strings.ToLower(c.cfg.Provider)
	switch provider {
	case "anthropic":
		return c.chatAnthropic(ctx, system, user, history)
	default:
		return c.chatOpenAI(ctx, system, user, history)
	}
}

func (c *Client) chatOpenAI(ctx context.Context, system, user string, history []Message) (string, error) {
	msgs := make([]Message, 0, len(history)+2)
	if system != "" {
		msgs = append(msgs, Message{Role: "system", Content: system})
	}
	msgs = append(msgs, history...)
	msgs = append(msgs, Message{Role: "user", Content: user})

	body := map[string]interface{}{
		"model":       c.cfg.Model,
		"messages":    msgs,
		"max_tokens":  c.cfg.MaxTokens,
		"temperature": c.cfg.Temperature,
	}
	base := strings.TrimRight(c.cfg.BaseURL, "/")
	if base == "" {
		if strings.EqualFold(c.cfg.Provider, "deepseek") {
			base = "https://api.deepseek.com/v1"
		} else {
			base = "https://api.openai.com/v1"
		}
	}
	return c.postJSON(ctx, base+"/chat/completions", map[string]string{
		"Authorization": "Bearer " + c.cfg.APIKey,
		"Content-Type":  "application/json",
	}, body, func(raw []byte) (string, error) {
		var resp struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return "", err
		}
		if resp.Error != nil {
			return "", fmt.Errorf("%s", resp.Error.Message)
		}
		if len(resp.Choices) == 0 {
			return "", fmt.Errorf("empty response from API")
		}
		return resp.Choices[0].Message.Content, nil
	})
}

func (c *Client) chatAnthropic(ctx context.Context, system, user string, history []Message) (string, error) {
	type amsg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	msgs := make([]amsg, 0, len(history)+1)
	for _, h := range history {
		role := h.Role
		if role == "system" {
			continue
		}
		if role != "assistant" {
			role = "user"
		}
		msgs = append(msgs, amsg{Role: role, Content: h.Content})
	}
	msgs = append(msgs, amsg{Role: "user", Content: user})

	body := map[string]interface{}{
		"model":      c.cfg.Model,
		"max_tokens": c.cfg.MaxTokens,
		"messages":   msgs,
	}
	if system != "" {
		body["system"] = system
	}
	base := strings.TrimRight(c.cfg.BaseURL, "/")
	if base == "" || strings.Contains(base, "openai.com") {
		base = "https://api.anthropic.com"
	}
	url := base + "/v1/messages"
	if strings.HasSuffix(base, "/v1") {
		url = base + "/messages"
	}
	return c.postJSON(ctx, url, map[string]string{
		"x-api-key":         c.cfg.APIKey,
		"anthropic-version": "2023-06-01",
		"Content-Type":      "application/json",
	}, body, func(raw []byte) (string, error) {
		var resp struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return "", err
		}
		if resp.Error != nil {
			return "", fmt.Errorf("%s", resp.Error.Message)
		}
		var b strings.Builder
		for _, c := range resp.Content {
			if c.Type == "text" {
				b.WriteString(c.Text)
			}
		}
		if b.Len() == 0 {
			return "", fmt.Errorf("empty response from Anthropic")
		}
		return b.String(), nil
	})
}

func (c *Client) postJSON(ctx context.Context, url string, headers map[string]string, body interface{}, parse func([]byte) (string, error)) (string, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	if res.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d: %s", res.StatusCode, truncate(string(raw), 400))
	}
	return parse(raw)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

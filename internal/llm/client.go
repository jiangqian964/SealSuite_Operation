package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"sealsuite-operation/internal/config"
)

type Client struct {
	cfg    config.LLMRoleConfig
	client *http.Client
}

type ChatRequest struct {
	Model           string
	SystemPrompt    string
	Prompt          string
	Input           interface{}
	Temperature     *float64
	MaxTokens       *int
	Thinking        *bool
	ReasoningEffort string
	ResponseFormat  map[string]interface{}
}

type ChatResponse struct {
	Model   string      `json:"model"`
	Content string      `json:"content"`
	Raw     interface{} `json:"raw,omitempty"`
}

func NewClient(cfg config.LLMRoleConfig) *Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30
	}
	return &Client{
		cfg:    cfg,
		client: &http.Client{Timeout: time.Duration(timeout) * time.Second},
	}
}

func (c *Client) Chat(req ChatRequest) (*ChatResponse, error) {
	if !c.cfg.Enabled {
		return nil, fmt.Errorf("llm is disabled")
	}
	if c.cfg.MockMode {
		return &ChatResponse{
			Model:   firstNonEmpty(req.Model, c.cfg.Model),
			Content: "mock llm response",
			Raw:     map[string]interface{}{"mock": true},
		}, nil
	}
	if c.cfg.BaseURL == "" || c.cfg.APIKey == "" {
		return nil, fmt.Errorf("llm base_url/api_key is required")
	}
	model := firstNonEmpty(req.Model, c.cfg.Model)
	if model == "" {
		return nil, fmt.Errorf("llm model is required")
	}

	messages := make([]map[string]interface{}, 0, 2)
	systemPrompt := firstNonEmpty(req.SystemPrompt, c.cfg.SystemPrompt)
	if systemPrompt != "" {
		messages = append(messages, map[string]interface{}{"role": "system", "content": systemPrompt})
	}
	userContent := req.Prompt
	if req.Input != nil {
		userContent = strings.TrimSpace(userContent + "\n\n输入数据：\n" + prettyJSON(req.Input))
	}
	messages = append(messages, map[string]interface{}{"role": "user", "content": userContent})

	body := map[string]interface{}{
		"model":    model,
		"messages": messages,
	}
	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	} else if c.cfg.Temperature > 0 {
		body["temperature"] = c.cfg.Temperature
	}
	if req.MaxTokens != nil {
		body["max_tokens"] = *req.MaxTokens
	} else if c.cfg.MaxTokens > 0 {
		body["max_tokens"] = c.cfg.MaxTokens
	}
	if req.Thinking != nil {
		body["thinking"] = map[string]interface{}{"type": ternaryThinking(*req.Thinking)}
	} else if c.cfg.Thinking {
		body["thinking"] = map[string]interface{}{"type": "enabled"}
	}
	if req.ReasoningEffort != "" {
		body["reasoning_effort"] = req.ReasoningEffort
	} else if c.cfg.ReasoningEffort != "" {
		body["reasoning_effort"] = c.cfg.ReasoningEffort
	}
	if len(req.ResponseFormat) > 0 {
		body["response_format"] = req.ResponseFormat
	} else if len(c.cfg.ResponseFormat) > 0 {
		body["response_format"] = c.cfg.ResponseFormat
	}
	applyProviderDefaults(body, c.cfg.Provider)

	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	url := strings.TrimRight(c.cfg.BaseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("llm http %d: %s", resp.StatusCode, string(rawBody))
	}
	var out map[string]interface{}
	if err := json.Unmarshal(rawBody, &out); err != nil {
		return nil, err
	}
	choices, _ := out["choices"].([]interface{})
	if len(choices) == 0 {
		return nil, fmt.Errorf("llm response has no choices")
	}
	first, _ := choices[0].(map[string]interface{})
	msg, _ := first["message"].(map[string]interface{})
	content, _ := msg["content"].(string)
	modelOut, _ := out["model"].(string)
	return &ChatResponse{
		Model:   firstNonEmpty(modelOut, model),
		Content: content,
		Raw:     out,
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func applyProviderDefaults(body map[string]interface{}, provider string) {
	switch provider {
	case "deepseek":
		if _, ok := body["reasoning_effort"]; !ok {
			body["reasoning_effort"] = "high"
		}
	case "volcengine_ark":
		if _, ok := body["response_format"]; !ok {
			body["response_format"] = map[string]interface{}{"type": "json_object"}
		}
	}
}

func ternaryThinking(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func prettyJSON(v interface{}) string {
	if v == nil {
		return "null"
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

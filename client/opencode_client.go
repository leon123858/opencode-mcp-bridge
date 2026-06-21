package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	bridgetypes "github.com/a0970/opencodemcpbridge/types"
)

const StartHint = "請先執行 `opencode serve` 啟動 OpenCode 伺服器"

var ErrUnavailable = errors.New("cannot connect to OpenCode server")

type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("OpenCode returned HTTP %d: %s", e.StatusCode, e.Body)
}

type Client struct {
	baseURL  string
	username string
	password string
	http     *http.Client
}

func New(baseURL, username, password string, httpClient *http.Client) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), username: username, password: password, http: httpClient}
}

func (c *Client) Health(ctx context.Context) (json.RawMessage, error) {
	return c.do(ctx, http.MethodGet, "/global/health", nil)
}

func (c *Client) Project(ctx context.Context) (json.RawMessage, error) {
	return c.do(ctx, http.MethodGet, "/project/current", nil)
}

func (c *Client) CreateSession(ctx context.Context, title string) (string, json.RawMessage, error) {
	body := map[string]any{}
	if title != "" {
		body["title"] = title
	}
	raw, err := c.do(ctx, http.MethodPost, "/session", body)
	if err != nil {
		return "", nil, err
	}
	id := findString(raw, "id", "sessionID", "sessionId")
	if id == "" {
		return "", raw, errors.New("OpenCode create session response did not contain a session id")
	}
	return id, raw, nil
}

func (c *Client) SendMessage(ctx context.Context, req bridgetypes.ReplyRequest) (bridgetypes.MessageResponse, error) {
	body := messageBody(req)
	raw, err := c.do(ctx, http.MethodPost, "/session/"+url.PathEscape(req.SessionID)+"/message", body)
	if err != nil {
		return bridgetypes.MessageResponse{}, err
	}
	return bridgetypes.MessageResponse{SessionID: req.SessionID, Message: extractMessage(raw), Raw: raw}, nil
}

func (c *Client) Ask(ctx context.Context, req bridgetypes.AskRequest) (bridgetypes.MessageResponse, error) {
	id, _, err := c.CreateSession(ctx, req.Title)
	if err != nil {
		return bridgetypes.MessageResponse{}, err
	}
	return c.SendMessage(ctx, bridgetypes.ReplyRequest{SessionID: id, Prompt: req.Prompt, ProviderID: req.ProviderID, ModelID: req.ModelID, Agent: req.Agent})
}

func (c *Client) Session(ctx context.Context, id string) (json.RawMessage, error) {
	return c.do(ctx, http.MethodGet, "/session/"+url.PathEscape(id), nil)
}

func (c *Client) Conversation(ctx context.Context, id string, limit int) (json.RawMessage, error) {
	path := "/session/" + url.PathEscape(id) + "/message"
	if limit > 0 {
		path += "?limit=" + fmt.Sprint(limit)
	}
	return c.do(ctx, http.MethodGet, path, nil)
}

func (c *Client) Sessions(ctx context.Context) (json.RawMessage, error) {
	return c.do(ctx, http.MethodGet, "/session", nil)
}

func (c *Client) MCPServers(ctx context.Context) (json.RawMessage, error) {
	return c.do(ctx, http.MethodGet, "/mcp", nil)
}

func (c *Client) SessionStatuses(ctx context.Context) (json.RawMessage, error) {
	return c.do(ctx, http.MethodGet, "/session/status", nil)
}

func (c *Client) Todos(ctx context.Context, id string) (json.RawMessage, error) {
	return c.do(ctx, http.MethodGet, "/session/"+url.PathEscape(id)+"/todo", nil)
}

func (c *Client) Diff(ctx context.Context, id string) (json.RawMessage, error) {
	return c.do(ctx, http.MethodGet, "/session/"+url.PathEscape(id)+"/diff", nil)
}

func (c *Client) Providers(ctx context.Context) (json.RawMessage, error) {
	return c.do(ctx, http.MethodGet, "/provider", nil)
}

func (c *Client) SendMessageAsync(ctx context.Context, req bridgetypes.ReplyRequest) error {
	body := messageBody(req)
	_, err := c.do(ctx, http.MethodPost, "/session/"+url.PathEscape(req.SessionID)+"/prompt_async", body)
	return err
}

func (c *Client) Run(ctx context.Context, req bridgetypes.AskRequest) (bridgetypes.RunResponse, error) {
	id, _, err := c.CreateSession(ctx, req.Title)
	if err != nil {
		return bridgetypes.RunResponse{}, err
	}
	reply := bridgetypes.ReplyRequest{SessionID: id, Prompt: req.Prompt, ProviderID: req.ProviderID, ModelID: req.ModelID, Agent: req.Agent}
	if err := c.SendMessageAsync(ctx, reply); err != nil {
		return bridgetypes.RunResponse{SessionID: id, Status: "failed"}, err
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return bridgetypes.RunResponse{SessionID: id, Status: "timeout"}, ctx.Err()
		case <-ticker.C:
			statuses, err := c.SessionStatuses(ctx)
			if err != nil {
				return bridgetypes.RunResponse{SessionID: id, Status: "failed"}, err
			}
			if sessionBusy(statuses, id) {
				continue
			}
			conversation, err := c.Conversation(ctx, id, 100)
			if err != nil {
				return bridgetypes.RunResponse{SessionID: id, Status: "failed"}, err
			}
			message := lastAssistantMessage(conversation)
			if message == "" {
				continue
			}
			return bridgetypes.RunResponse{SessionID: id, Status: "completed", Message: message, Raw: conversation}, nil
		}
	}
}

func messageBody(req bridgetypes.ReplyRequest) map[string]any {
	body := map[string]any{"parts": []map[string]string{{"type": "text", "text": req.Prompt}}}
	if req.ProviderID != "" || req.ModelID != "" {
		body["model"] = map[string]string{"providerID": req.ProviderID, "modelID": req.ModelID}
	}
	if req.Agent != "" {
		body["agent"] = req.Agent
	}
	return body
}

func (c *Client) do(ctx context.Context, method, path string, body any) (json.RawMessage, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.username != "" || c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) || errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &HTTPError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(data))}
	}
	if len(data) == 0 {
		return json.RawMessage("null"), nil
	}
	if !json.Valid(data) {
		return nil, errors.New("OpenCode returned invalid JSON")
	}
	return data, nil
}

func findString(raw json.RawMessage, keys ...string) string {
	var value map[string]any
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	for _, key := range keys {
		if text, ok := value[key].(string); ok {
			return text
		}
	}
	return ""
}

func extractMessage(raw json.RawMessage) string {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	var walk func(any) string
	walk = func(v any) string {
		switch item := v.(type) {
		case map[string]any:
			if text, ok := item["text"].(string); ok && text != "" {
				return text
			}
			for _, key := range []string{"parts", "content", "message", "data"} {
				if result := walk(item[key]); result != "" {
					return result
				}
			}
		case []any:
			var parts []string
			for _, child := range item {
				if result := walk(child); result != "" {
					parts = append(parts, result)
				}
			}
			return strings.Join(parts, "\n")
		case string:
			return item
		}
		return ""
	}
	return walk(value)
}

func sessionBusy(raw json.RawMessage, id string) bool {
	var statuses map[string]struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(raw, &statuses) != nil {
		return true
	}
	status, ok := statuses[id]
	return ok && status.Type != "idle"
}

func lastAssistantMessage(raw json.RawMessage) string {
	var messages []struct {
		Info struct {
			Role string `json:"role"`
		} `json:"info"`
		Parts []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"parts"`
	}
	if json.Unmarshal(raw, &messages) != nil {
		return ""
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Info.Role != "assistant" {
			continue
		}
		var parts []string
		for _, part := range messages[i].Parts {
			if part.Type == "text" && part.Text != "" {
				parts = append(parts, part.Text)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n")
		}
	}
	return ""
}

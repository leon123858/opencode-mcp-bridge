package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/a0970/opencodemcpbridge/client"
	"github.com/a0970/opencodemcpbridge/handlers"
	"github.com/a0970/opencodemcpbridge/server"
	bridgetypes "github.com/a0970/opencodemcpbridge/types"
)

type fakeClient struct {
	askResult bridgetypes.MessageResponse
	askErr    error
}

func (f fakeClient) Health(context.Context) (json.RawMessage, error) {
	return json.RawMessage(`{"ok":true}`), f.askErr
}
func (f fakeClient) Project(context.Context) (json.RawMessage, error) {
	return json.RawMessage(`{"name":"demo"}`), nil
}
func (f fakeClient) Ask(context.Context, bridgetypes.AskRequest) (bridgetypes.MessageResponse, error) {
	return f.askResult, f.askErr
}
func (f fakeClient) SendMessage(context.Context, bridgetypes.ReplyRequest) (bridgetypes.MessageResponse, error) {
	return f.askResult, f.askErr
}
func (f fakeClient) Session(context.Context, string) (json.RawMessage, error) {
	return json.RawMessage(`{"id":"s1"}`), f.askErr
}
func (f fakeClient) Conversation(context.Context, string, int) (json.RawMessage, error) {
	return json.RawMessage(`[]`), f.askErr
}
func (f fakeClient) Sessions(context.Context) (json.RawMessage, error) {
	return json.RawMessage(`[]`), f.askErr
}
func (f fakeClient) MCPServers(context.Context) (json.RawMessage, error) {
	return json.RawMessage(`[]`), f.askErr
}
func (f fakeClient) SessionStatuses(context.Context) (json.RawMessage, error) {
	return json.RawMessage(`{}`), f.askErr
}
func (f fakeClient) Todos(context.Context, string) (json.RawMessage, error) {
	return json.RawMessage(`[]`), f.askErr
}
func (f fakeClient) Diff(context.Context, string) (json.RawMessage, error) {
	return json.RawMessage(`[]`), f.askErr
}
func (f fakeClient) Providers(context.Context) (json.RawMessage, error) {
	return json.RawMessage(`{"all":[{"id":"provider-1","models":{"model-1":{}}}]}`), f.askErr
}
func (f fakeClient) Run(context.Context, bridgetypes.AskRequest) (bridgetypes.RunResponse, error) {
	return bridgetypes.RunResponse{SessionID: f.askResult.SessionID, Status: "completed", Message: f.askResult.Message}, f.askErr
}

func TestAskValidation(t *testing.T) {
	e := server.New(handlers.New(fakeClient{}))
	req := httptest.NewRequest(http.MethodPost, "/opencode/ask", strings.NewReader(`{"prompt":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUnavailableIncludesStartHint(t *testing.T) {
	e := server.New(handlers.New(fakeClient{askErr: errors.Join(client.ErrUnavailable, errors.New("connection refused"))}))
	req := httptest.NewRequest(http.MethodGet, "/opencode/setup", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "opencode serve") {
		t.Fatalf("unexpected response: %d %s", rec.Code, rec.Body.String())
	}
}

func TestCoreRoutes(t *testing.T) {
	e := server.New(handlers.New(fakeClient{askResult: bridgetypes.MessageResponse{SessionID: "s1", Message: "answer"}}))
	tests := []struct {
		name   string
		method string
		path   string
		body   string
		status int
	}{
		{name: "setup", method: http.MethodGet, path: "/opencode/setup", status: http.StatusOK},
		{name: "ask", method: http.MethodPost, path: "/opencode/ask", body: `{"prompt":"hello"}`, status: http.StatusOK},
		{name: "reply", method: http.MethodPost, path: "/opencode/reply", body: `{"sessionId":"s1","prompt":"more"}`, status: http.StatusOK},
		{name: "run", method: http.MethodPost, path: "/opencode/run", body: `{"prompt":"work","maxDurationSeconds":1}`, status: http.StatusOK},
		{name: "check", method: http.MethodGet, path: "/opencode/check?sessionId=s1&detailed=true", status: http.StatusOK},
		{name: "conversation", method: http.MethodGet, path: "/opencode/conversation?sessionId=s1&limit=10", status: http.StatusOK},
		{name: "sessions", method: http.MethodGet, path: "/opencode/sessions-overview", status: http.StatusOK},
		{name: "mcp servers", method: http.MethodGet, path: "/opencode/mcp-servers", status: http.StatusOK},
		{name: "provider", method: http.MethodPost, path: "/opencode/provider-test", body: `{"providerId":"provider-1","modelID":"model-1"}`, status: http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != tt.status {
				t.Fatalf("expected %d, got %d: %s", tt.status, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestRouteValidation(t *testing.T) {
	e := server.New(handlers.New(fakeClient{}))
	tests := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/opencode/ask", body: `{`},
		{method: http.MethodPost, path: "/opencode/reply", body: `{"prompt":"x"}`},
		{method: http.MethodPost, path: "/opencode/run", body: `{"prompt":"x","maxDurationSeconds":3601}`},
		{method: http.MethodGet, path: "/opencode/check"},
		{method: http.MethodGet, path: "/opencode/conversation?sessionId=s1&limit=bad"},
		{method: http.MethodPost, path: "/opencode/provider-test", body: `{"providerId":"missing"}`},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
		if tt.body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: expected 400, got %d", tt.path, rec.Code)
		}
	}
}

func TestUpstreamHTTPErrorIsPreserved(t *testing.T) {
	e := server.New(handlers.New(fakeClient{askErr: &client.HTTPError{StatusCode: http.StatusNotFound, Body: "missing"}}))
	req := httptest.NewRequest(http.MethodPost, "/opencode/ask", strings.NewReader(`{"prompt":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestLegacySSEPathIsMounted(t *testing.T) {
	called := false
	legacyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.URL.Path != "/mcp/sse" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	e := server.New(handlers.New(fakeClient{}), nil, legacyHandler)
	req := httptest.NewRequest(http.MethodGet, "/mcp/sse", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if !called || rec.Code != http.StatusNoContent {
		t.Fatalf("legacy SSE handler was not mounted: called=%v status=%d", called, rec.Code)
	}
}

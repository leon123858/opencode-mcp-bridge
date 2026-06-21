package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	bridgetypes "github.com/a0970/opencodemcpbridge/types"
)

func TestClientEndpointMethodsAndRun(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/project/current":
			_, _ = w.Write([]byte(`{"id":"project-1"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			_, _ = w.Write([]byte(`{"id":"session-1"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/session/session-1":
			_, _ = w.Write([]byte(`{"id":"session-1"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/session/session-1/message":
			if limit := r.URL.Query().Get("limit"); limit != "" && limit != "10" && limit != "100" {
				t.Errorf("unexpected limit %q", limit)
			}
			_, _ = w.Write([]byte(`[{"info":{"role":"assistant"},"parts":[{"type":"text","text":"done"}]}]`))
		case r.Method == http.MethodPost && r.URL.Path == "/session/session-1/prompt_async":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && r.URL.Path == "/session/status":
			_, _ = w.Write([]byte(`{"session-1":{"type":"idle"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/session/session-1/todo":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodGet && r.URL.Path == "/session/session-1/diff":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodGet && r.URL.Path == "/session":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodGet && r.URL.Path == "/mcp":
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodGet && r.URL.Path == "/provider":
			_, _ = w.Write([]byte(`{"all":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	c := New(upstream.URL, "", "", &http.Client{Timeout: 3 * time.Second})
	ctx := context.Background()
	calls := []func() error{
		func() error { _, err := c.Project(ctx); return err },
		func() error { _, err := c.Session(ctx, "session-1"); return err },
		func() error { _, err := c.Conversation(ctx, "session-1", 10); return err },
		func() error { _, err := c.Sessions(ctx); return err },
		func() error { _, err := c.MCPServers(ctx); return err },
		func() error { _, err := c.SessionStatuses(ctx); return err },
		func() error { _, err := c.Todos(ctx, "session-1"); return err },
		func() error { _, err := c.Diff(ctx, "session-1"); return err },
		func() error { _, err := c.Providers(ctx); return err },
		func() error {
			return c.SendMessageAsync(ctx, bridgetypes.ReplyRequest{SessionID: "session-1", Prompt: "work"})
		},
	}
	for i, call := range calls {
		if err := call(); err != nil {
			t.Fatalf("call %d failed: %v", i, err)
		}
	}
	result, err := c.Run(ctx, bridgetypes.AskRequest{Prompt: "work"})
	if err != nil || result.Status != "completed" || result.Message != "done" {
		t.Fatalf("unexpected run result: %#v, %v", result, err)
	}
}

func TestResponseAndParsingErrors(t *testing.T) {
	tests := []struct {
		name string
		body string
		code int
	}{
		{name: "upstream status", body: "failure", code: http.StatusTeapot},
		{name: "invalid JSON", body: "not-json", code: http.StatusOK},
		{name: "empty response", body: "", code: http.StatusNoContent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.code)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer upstream.Close()
			result, err := New(upstream.URL, "", "", upstream.Client()).Health(context.Background())
			if tt.name == "empty response" {
				if err != nil || string(result) != "null" {
					t.Fatalf("expected null response, got %s, %v", result, err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error")
			}
			if tt.name == "upstream status" {
				var httpErr *HTTPError
				if !errors.As(err, &httpErr) || httpErr.StatusCode != tt.code || httpErr.Error() == "" {
					t.Fatalf("unexpected HTTP error: %v", err)
				}
			}
		})
	}
}

func TestCreateSessionRequiresID(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"title":"missing id"}`))
	}))
	defer upstream.Close()
	_, _, err := New(upstream.URL, "", "", upstream.Client()).CreateSession(context.Background(), "title")
	if err == nil {
		t.Fatal("expected missing ID error")
	}
}

func TestParsingHelpers(t *testing.T) {
	body := messageBody(bridgetypes.ReplyRequest{Prompt: "hello", ProviderID: "p", ModelID: "m", Agent: "a"})
	if body["agent"] != "a" || body["model"] == nil {
		t.Fatalf("unexpected message body: %#v", body)
	}
	if findString(json.RawMessage(`not-json`), "id") != "" || extractMessage(json.RawMessage(`"text"`)) != "text" {
		t.Fatal("unexpected helper result")
	}
	if !sessionBusy(json.RawMessage(`not-json`), "s") || sessionBusy(json.RawMessage(`{"s":{"type":"idle"}}`), "s") {
		t.Fatal("unexpected busy state")
	}
	if got := lastAssistantMessage(json.RawMessage(`[{"info":{"role":"user"},"parts":[{"type":"text","text":"skip"}]},{"info":{"role":"assistant"},"parts":[{"type":"text","text":"answer"}]}]`)); got != "answer" {
		t.Fatalf("unexpected assistant message %q", got)
	}
}

func TestAskCreatesSessionAndSendsMessage(t *testing.T) {
	var messageCalled bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "user" || password != "pass" {
			t.Fatalf("missing basic authentication")
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"session-1"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/session/session-1/message":
			messageCalled = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"parts":[{"type":"text","text":"answer"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	c := New(upstream.URL, "user", "pass", &http.Client{Timeout: time.Second})
	result, err := c.Ask(context.Background(), bridgetypes.AskRequest{Prompt: "question"})
	if err != nil {
		t.Fatal(err)
	}
	if !messageCalled || result.SessionID != "session-1" || result.Message != "answer" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestUnavailableError(t *testing.T) {
	c := New("http://127.0.0.1:1", "", "", &http.Client{Timeout: 100 * time.Millisecond})
	_, err := c.Health(context.Background())
	if err == nil || !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
}

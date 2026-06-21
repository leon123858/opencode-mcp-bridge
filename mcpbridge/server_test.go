package mcpbridge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/a0970/opencodemcpbridge/client"
	bridgetypes "github.com/a0970/opencodemcpbridge/types"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type fakeClient struct {
	projectErr error
}

func (fakeClient) Health(context.Context) (json.RawMessage, error) {
	return json.RawMessage(`{"healthy":true}`), nil
}
func (f fakeClient) Project(context.Context) (json.RawMessage, error) {
	return json.RawMessage(`{"id":"project-1"}`), f.projectErr
}
func (fakeClient) Ask(context.Context, bridgetypes.AskRequest) (bridgetypes.MessageResponse, error) {
	return bridgetypes.MessageResponse{SessionID: "ses_1", Message: "answer"}, nil
}
func (fakeClient) SendMessage(context.Context, bridgetypes.ReplyRequest) (bridgetypes.MessageResponse, error) {
	return bridgetypes.MessageResponse{SessionID: "ses_1", Message: "answer"}, nil
}
func (fakeClient) Run(context.Context, bridgetypes.AskRequest) (bridgetypes.RunResponse, error) {
	return bridgetypes.RunResponse{SessionID: "ses_1", Status: "completed", Message: "answer"}, nil
}
func (fakeClient) Session(context.Context, string) (json.RawMessage, error) {
	return json.RawMessage(`{"id":"ses_1"}`), nil
}
func (fakeClient) SessionStatuses(context.Context) (json.RawMessage, error) {
	return json.RawMessage(`{"ses_1":{"type":"idle"}}`), nil
}
func (fakeClient) Todos(context.Context, string) (json.RawMessage, error) {
	return json.RawMessage(`[]`), nil
}
func (fakeClient) Diff(context.Context, string) (json.RawMessage, error) {
	return json.RawMessage(`[]`), nil
}
func (fakeClient) Conversation(context.Context, string, int) (json.RawMessage, error) {
	return json.RawMessage(`[]`), nil
}
func (fakeClient) Sessions(context.Context) (json.RawMessage, error) {
	return json.RawMessage(`[]`), nil
}
func (fakeClient) MCPServers(context.Context) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}
func (fakeClient) Providers(context.Context) (json.RawMessage, error) {
	return json.RawMessage(`{"all":[{"id":"provider-1","models":{"model-1":{}}}]}`), nil
}

func connectTestClient(t *testing.T, upstream OpenCodeClient) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	server, _ := New(upstream)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	clientSession, err := mcpClient.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })
	return clientSession
}

func TestToolDiscoveryIsLimitedToCoreTools(t *testing.T) {
	session := connectTestClient(t, fakeClient{})
	result, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"opencode_setup": true, "opencode_ask": true, "opencode_reply": true,
		"opencode_run": true, "opencode_check": true, "opencode_conversation": true,
		"opencode_sessions_overview": true, "opencode_mcp_servers": true,
		"opencode_provider_test": true,
	}
	if len(result.Tools) != len(want) {
		t.Fatalf("expected %d tools, got %d", len(want), len(result.Tools))
	}
	for _, tool := range result.Tools {
		if !want[tool.Name] {
			t.Errorf("unexpected tool %q", tool.Name)
		}
	}
}

func TestSetupAllowsMissingCurrentProject(t *testing.T) {
	session := connectTestClient(t, fakeClient{projectErr: &client.HTTPError{StatusCode: http.StatusNotFound, Body: "not found"}})
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "opencode_setup", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %#v", result.Content)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok || !strings.Contains(text.Text, `"healthy": true`) || !strings.Contains(text.Text, `"project": null`) {
		t.Fatalf("unexpected setup result: %#v", result.Content)
	}
}

func TestAskValidatesPrompt(t *testing.T) {
	session := connectTestClient(t, fakeClient{})
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "opencode_ask", Arguments: map[string]any{"prompt": " "}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected validation error")
	}
}

func TestCoreToolCalls(t *testing.T) {
	session := connectTestClient(t, fakeClient{})
	tests := []struct {
		name string
		args map[string]any
	}{
		{name: "opencode_ask", args: map[string]any{"prompt": "hello"}},
		{name: "opencode_reply", args: map[string]any{"sessionId": "ses_1", "prompt": "more"}},
		{name: "opencode_run", args: map[string]any{"prompt": "work", "maxDurationSeconds": 1}},
		{name: "opencode_check", args: map[string]any{"sessionId": "ses_1", "detailed": true}},
		{name: "opencode_conversation", args: map[string]any{"sessionId": "ses_1", "limit": 10}},
		{name: "opencode_sessions_overview", args: map[string]any{}},
		{name: "opencode_mcp_servers", args: map[string]any{}},
		{name: "opencode_provider_test", args: map[string]any{"providerId": "provider-1", "modelID": "model-1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatal(err)
			}
			if result.IsError {
				t.Fatalf("unexpected tool error: %#v", result.Content)
			}
		})
	}
}

func TestToolValidation(t *testing.T) {
	session := connectTestClient(t, fakeClient{})
	tests := []struct {
		name string
		args map[string]any
	}{
		{name: "opencode_reply", args: map[string]any{"prompt": "x"}},
		{name: "opencode_reply", args: map[string]any{"sessionId": "default", "prompt": "x"}},
		{name: "opencode_run", args: map[string]any{"prompt": "x", "maxDurationSeconds": 3601}},
		{name: "opencode_check", args: map[string]any{}},
		{name: "opencode_check", args: map[string]any{"sessionId": "session-1"}},
		{name: "opencode_conversation", args: map[string]any{"sessionId": "s", "limit": 10}},
		{name: "opencode_conversation", args: map[string]any{"sessionId": "ses_1", "limit": 1001}},
		{name: "opencode_provider_test", args: map[string]any{"providerId": "missing"}},
	}
	for _, tt := range tests {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
		if err != nil {
			t.Fatal(err)
		}
		if !result.IsError {
			t.Errorf("%s: expected validation error", tt.name)
		}
	}
}

func TestLegacySSETransport(t *testing.T) {
	server, _ := New(fakeClient{})
	httpServer := httptest.NewServer(NewLegacySSEHandler(server))
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "legacy-sse-test", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, &mcp.SSEClientTransport{Endpoint: httpServer.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 9 {
		t.Fatalf("expected 9 tools over legacy SSE, got %d", len(tools.Tools))
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "opencode_setup", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected setup error over legacy SSE: %#v", result.Content)
	}
}

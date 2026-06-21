package mcpbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/a0970/opencodemcpbridge/client"
	bridgetypes "github.com/a0970/opencodemcpbridge/types"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type OpenCodeClient interface {
	Health(context.Context) (json.RawMessage, error)
	Project(context.Context) (json.RawMessage, error)
	Ask(context.Context, bridgetypes.AskRequest) (bridgetypes.MessageResponse, error)
	SendMessage(context.Context, bridgetypes.ReplyRequest) (bridgetypes.MessageResponse, error)
	Run(context.Context, bridgetypes.AskRequest) (bridgetypes.RunResponse, error)
	Session(context.Context, string) (json.RawMessage, error)
	SessionStatuses(context.Context) (json.RawMessage, error)
	Todos(context.Context, string) (json.RawMessage, error)
	Diff(context.Context, string) (json.RawMessage, error)
	Conversation(context.Context, string, int) (json.RawMessage, error)
	Sessions(context.Context) (json.RawMessage, error)
	MCPServers(context.Context) (json.RawMessage, error)
	Providers(context.Context) (json.RawMessage, error)
}

type emptyArgs struct{}

type askArgs struct {
	Prompt     string `json:"prompt" jsonschema:"required,the prompt or task to send to OpenCode"`
	Title      string `json:"title,omitempty" jsonschema:"optional session title"`
	ProviderID string `json:"providerID,omitempty" jsonschema:"optional OpenCode provider ID"`
	ModelID    string `json:"modelID,omitempty" jsonschema:"optional model ID"`
	Agent      string `json:"agent,omitempty" jsonschema:"optional OpenCode agent name"`
}

type replyArgs struct {
	SessionID  string `json:"sessionId" jsonschema:"required,existing OpenCode session ID"`
	Prompt     string `json:"prompt" jsonschema:"required,the follow-up prompt"`
	ProviderID string `json:"providerID,omitempty"`
	ModelID    string `json:"modelID,omitempty"`
	Agent      string `json:"agent,omitempty"`
}

type runArgs struct {
	askArgs
	MaxDurationSeconds int `json:"maxDurationSeconds,omitempty" jsonschema:"maximum time to wait in seconds (1-3600)"`
}

type checkArgs struct {
	SessionID string `json:"sessionId" jsonschema:"required,OpenCode session ID"`
	Detailed  bool   `json:"detailed,omitempty" jsonschema:"include todos and file diffs"`
}

type conversationArgs struct {
	SessionID string `json:"sessionId" jsonschema:"required,OpenCode session ID"`
	Limit     int    `json:"limit,omitempty" jsonschema:"maximum number of messages (1-1000)"`
}

type providerTestArgs struct {
	ProviderID string `json:"providerId" jsonschema:"required,OpenCode provider ID"`
	ModelID    string `json:"modelID,omitempty" jsonschema:"optional model ID"`
}

func New(c OpenCodeClient) (*mcp.Server, http.Handler) {
	server := mcp.NewServer(&mcp.Implementation{Name: "opencode-mcp-bridge", Version: "0.1.0"}, &mcp.ServerOptions{
		Instructions: "Use these tools to communicate with an already running OpenCode server. Start OpenCode manually with `opencode serve` if setup reports it unavailable. Always pass the 'ses_...' prefix for session IDs.",
	})

	mcp.AddTool(server, &mcp.Tool{Name: "opencode_setup", Description: "Verify OpenCode is running and return the active project's workspace details."}, func(ctx context.Context, _ *mcp.CallToolRequest, _ emptyArgs) (*mcp.CallToolResult, any, error) {
		health, err := c.Health(ctx)
		if err != nil {
			return toolError(err), nil, nil
		}
		project, err := c.Project(ctx)
		if err != nil {
			var httpErr *client.HTTPError
			if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusNotFound {
				return toolError(err), nil, nil
			}
			project = nil
		}
		return jsonResult(map[string]any{"healthy": true, "health": health, "project": project}), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{Name: "opencode_ask", Description: "Start a new conversation with OpenCode. Provide a prompt. Returns a new sessionId starting with 'ses_' and the assistant's response. Do NOT use this if you want to follow up on an existing session."}, func(ctx context.Context, _ *mcp.CallToolRequest, args askArgs) (*mcp.CallToolResult, any, error) {
		if strings.TrimSpace(args.Prompt) == "" {
			return validationError("prompt is required"), nil, nil
		}
		result, err := c.Ask(ctx, toAskRequest(args))
		if err != nil {
			return toolError(err), nil, nil
		}
		return jsonResult(result), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{Name: "opencode_reply", Description: "Continue an existing OpenCode conversation. MUST provide a valid sessionId starting with 'ses_'. Returns the assistant's response."}, func(ctx context.Context, _ *mcp.CallToolRequest, args replyArgs) (*mcp.CallToolResult, any, error) {
		if errResult := validateSessionID(args.SessionID); errResult != nil {
			return errResult, nil, nil
		}
		if strings.TrimSpace(args.Prompt) == "" {
			return validationError("prompt is required"), nil, nil
		}
		result, err := c.SendMessage(ctx, bridgetypes.ReplyRequest{SessionID: args.SessionID, Prompt: args.Prompt, ProviderID: args.ProviderID, ModelID: args.ModelID, Agent: args.Agent})
		if err != nil {
			return toolError(err), nil, nil
		}
		return jsonResult(result), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{Name: "opencode_run", Description: "Start a long-running background task on OpenCode. The task runs asynchronously up to maxDurationSeconds. Returns the final session state."}, func(ctx context.Context, _ *mcp.CallToolRequest, args runArgs) (*mcp.CallToolResult, any, error) {
		if strings.TrimSpace(args.Prompt) == "" {
			return validationError("prompt is required"), nil, nil
		}
		duration := args.MaxDurationSeconds
		if duration == 0 {
			duration = 300
		}
		if duration < 1 || duration > 3600 {
			return validationError("maxDurationSeconds must be between 1 and 3600"), nil, nil
		}
		runCtx, cancel := context.WithTimeout(ctx, time.Duration(duration)*time.Second)
		defer cancel()
		result, err := c.Run(runCtx, toAskRequest(args.askArgs))
		if err != nil {
			return toolError(err), nil, nil
		}
		return jsonResult(result), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{Name: "opencode_check", Description: "Poll the status of an ongoing OpenCode session. MUST provide a valid sessionId starting with 'ses_'. Optionally include 'detailed' to see todos and file diffs."}, func(ctx context.Context, _ *mcp.CallToolRequest, args checkArgs) (*mcp.CallToolResult, any, error) {
		if errResult := validateSessionID(args.SessionID); errResult != nil {
			return errResult, nil, nil
		}
		session, err := c.Session(ctx, args.SessionID)
		if err != nil {
			return toolError(err), nil, nil
		}
		statuses, err := c.SessionStatuses(ctx)
		if err != nil {
			return toolError(err), nil, nil
		}
		result := map[string]any{"session": session, "status": selectStatus(statuses, args.SessionID)}
		if args.Detailed {
			if result["todos"], err = c.Todos(ctx, args.SessionID); err != nil {
				return toolError(err), nil, nil
			}
			if result["diff"], err = c.Diff(ctx, args.SessionID); err != nil {
				return toolError(err), nil, nil
			}
		}
		return jsonResult(result), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{Name: "opencode_conversation", Description: "Retrieve the history of messages for an existing session. MUST provide a valid sessionId starting with 'ses_'. Useful to see what was discussed."}, func(ctx context.Context, _ *mcp.CallToolRequest, args conversationArgs) (*mcp.CallToolResult, any, error) {
		if errResult := validateSessionID(args.SessionID); errResult != nil {
			return errResult, nil, nil
		}
		if args.Limit < 0 || args.Limit > 1000 {
			return validationError("limit must be between 1 and 1000"), nil, nil
		}
		result, err := c.Conversation(ctx, args.SessionID, args.Limit)
		if err != nil {
			return toolError(err), nil, nil
		}
		return rawResult(result), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{Name: "opencode_sessions_overview", Description: "List OpenCode sessions and their summaries."}, func(ctx context.Context, _ *mcp.CallToolRequest, _ emptyArgs) (*mcp.CallToolResult, any, error) {
		result, err := c.Sessions(ctx)
		if err != nil {
			return toolError(err), nil, nil
		}
		return rawResult(result), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{Name: "opencode_mcp_servers", Description: "List MCP servers configured inside OpenCode and their connection state."}, func(ctx context.Context, _ *mcp.CallToolRequest, _ emptyArgs) (*mcp.CallToolResult, any, error) {
		result, err := c.MCPServers(ctx)
		if err != nil {
			return toolError(err), nil, nil
		}
		return rawResult(result), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{Name: "opencode_provider_test", Description: "Send a minimal prompt using a provider/model to verify it works."}, func(ctx context.Context, _ *mcp.CallToolRequest, args providerTestArgs) (*mcp.CallToolResult, any, error) {
		if strings.TrimSpace(args.ProviderID) == "" {
			return validationError("providerId is required"), nil, nil
		}
		providers, err := c.Providers(ctx)
		if err != nil {
			return toolError(err), nil, nil
		}
		if !providerAvailable(providers, args.ProviderID, args.ModelID) {
			return validationError("provider or model is not available in OpenCode"), nil, nil
		}
		result, err := c.Ask(ctx, bridgetypes.AskRequest{Prompt: "Reply with OK.", Title: "Provider connectivity test", ProviderID: args.ProviderID, ModelID: args.ModelID})
		if err != nil {
			return toolError(err), nil, nil
		}
		return jsonResult(map[string]any{"available": true, "providerId": args.ProviderID, "modelID": args.ModelID, "sessionId": result.SessionID, "message": result.Message}), nil, nil
	})

	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{
		Stateless:      true,
		JSONResponse:   true,
		SessionTimeout: 30 * time.Minute,
	})
	return server, handler
}

// NewLegacySSEHandler exposes the HTTP+SSE transport from the MCP 2024-11-05
// specification. New clients should prefer the Streamable HTTP handler.
func NewLegacySSEHandler(server *mcp.Server) http.Handler {
	return mcp.NewSSEHandler(func(*http.Request) *mcp.Server { return server }, nil)
}

func toAskRequest(args askArgs) bridgetypes.AskRequest {
	return bridgetypes.AskRequest{Prompt: args.Prompt, Title: args.Title, ProviderID: args.ProviderID, ModelID: args.ModelID, Agent: args.Agent}
}

func jsonResult(value any) *mcp.CallToolResult {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return toolError(err)
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(data)}}}
}

func rawResult(raw json.RawMessage) *mcp.CallToolResult {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return toolError(err)
	}
	return jsonResult(value)
}

func validationError(message string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: message}}}
}

func validateSessionID(id string) *mcp.CallToolResult {
	id = strings.TrimSpace(id)
	if id == "" {
		return validationError("sessionId is required")
	}
	if !strings.HasPrefix(id, "ses_") {
		return validationError("sessionId must start with 'ses_'")
	}
	return nil
}

func toolError(err error) *mcp.CallToolResult {
	message := err.Error()
	if errors.Is(err, client.ErrUnavailable) || errors.Is(err, context.DeadlineExceeded) {
		message = fmt.Sprintf("%s. %s", client.ErrUnavailable, client.StartHint)
	}
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: message}}}
}

func selectStatus(raw json.RawMessage, id string) any {
	var statuses map[string]any
	if json.Unmarshal(raw, &statuses) == nil {
		if status, ok := statuses[id]; ok {
			return status
		}
	}
	return map[string]string{"type": "idle"}
}

func providerAvailable(raw json.RawMessage, providerID, modelID string) bool {
	var response struct {
		All []struct {
			ID     string                     `json:"id"`
			Models map[string]json.RawMessage `json:"models"`
		} `json:"all"`
	}
	if json.Unmarshal(raw, &response) != nil {
		return false
	}
	for _, provider := range response.All {
		if provider.ID == providerID {
			if modelID == "" {
				return true
			}
			_, ok := provider.Models[modelID]
			return ok
		}
	}
	return false
}

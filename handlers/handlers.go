package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/a0970/opencodemcpbridge/client"
	bridgetypes "github.com/a0970/opencodemcpbridge/types"
	"github.com/labstack/echo/v4"
)

type OpenCodeClient interface {
	Health(context.Context) (json.RawMessage, error)
	Project(context.Context) (json.RawMessage, error)
	Ask(context.Context, bridgetypes.AskRequest) (bridgetypes.MessageResponse, error)
	SendMessage(context.Context, bridgetypes.ReplyRequest) (bridgetypes.MessageResponse, error)
	Session(context.Context, string) (json.RawMessage, error)
	Conversation(context.Context, string, int) (json.RawMessage, error)
	Sessions(context.Context) (json.RawMessage, error)
	MCPServers(context.Context) (json.RawMessage, error)
	SessionStatuses(context.Context) (json.RawMessage, error)
	Todos(context.Context, string) (json.RawMessage, error)
	Diff(context.Context, string) (json.RawMessage, error)
	Providers(context.Context) (json.RawMessage, error)
	Run(context.Context, bridgetypes.AskRequest) (bridgetypes.RunResponse, error)
}

type Handler struct {
	client OpenCodeClient
}

func New(c OpenCodeClient) *Handler { return &Handler{client: c} }

func (h *Handler) Setup(c echo.Context) error {
	health, err := h.client.Health(c.Request().Context())
	if err != nil {
		return respondError(c, err)
	}
	project, err := h.client.Project(c.Request().Context())
	if err != nil {
		var httpErr *client.HTTPError
		if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusNotFound {
			return respondError(c, err)
		}
	}
	return c.JSON(http.StatusOK, bridgetypes.SetupResponse{Healthy: true, Health: health, Project: project})
}

func (h *Handler) Ask(c echo.Context) error {
	var req bridgetypes.AskRequest
	if err := bind(c, &req); err != nil {
		return err
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return badRequest(c, "prompt is required")
	}
	result, err := h.client.Ask(c.Request().Context(), req)
	if err != nil {
		return respondError(c, err)
	}
	return c.JSON(http.StatusOK, result)
}

func (h *Handler) Reply(c echo.Context) error {
	var req bridgetypes.ReplyRequest
	if err := bind(c, &req); err != nil {
		return err
	}
	if strings.TrimSpace(req.SessionID) == "" || strings.TrimSpace(req.Prompt) == "" {
		return badRequest(c, "sessionId and prompt are required")
	}
	result, err := h.client.SendMessage(c.Request().Context(), req)
	if err != nil {
		return respondError(c, err)
	}
	return c.JSON(http.StatusOK, result)
}

func (h *Handler) Run(c echo.Context) error {
	var req bridgetypes.RunRequest
	if err := bind(c, &req); err != nil {
		return err
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return badRequest(c, "prompt is required")
	}
	duration := req.MaxDurationSeconds
	if duration == 0 {
		duration = 300
	}
	if duration < 1 || duration > 3600 {
		return badRequest(c, "maxDurationSeconds must be between 1 and 3600")
	}
	ctx, cancel := context.WithTimeout(c.Request().Context(), time.Duration(duration)*time.Second)
	defer cancel()
	result, err := h.client.Run(ctx, req.AskRequest)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return c.JSON(http.StatusGatewayTimeout, bridgetypes.ErrorResponse{Error: "OpenCode task timed out"})
		}
		return respondError(c, err)
	}
	return c.JSON(http.StatusOK, result)
}

func (h *Handler) Check(c echo.Context) error {
	id := strings.TrimSpace(c.QueryParam("sessionId"))
	if id == "" {
		return badRequest(c, "sessionId is required")
	}
	ctx := c.Request().Context()
	raw, err := h.client.Session(ctx, id)
	if err != nil {
		return respondError(c, err)
	}
	statuses, err := h.client.SessionStatuses(ctx)
	if err != nil {
		return respondError(c, err)
	}
	response := bridgetypes.CheckResponse{Session: raw, Status: sessionStatus(statuses, id)}
	if c.QueryParam("detailed") == "true" {
		response.Todos, err = h.client.Todos(ctx, id)
		if err != nil {
			return respondError(c, err)
		}
		response.Diff, err = h.client.Diff(ctx, id)
		if err != nil {
			return respondError(c, err)
		}
	}
	return c.JSON(http.StatusOK, response)
}

func (h *Handler) Conversation(c echo.Context) error {
	id := strings.TrimSpace(c.QueryParam("sessionId"))
	if id == "" {
		return badRequest(c, "sessionId is required")
	}
	limit := 0
	if value := c.QueryParam("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 1000 {
			return badRequest(c, "limit must be between 1 and 1000")
		}
		limit = parsed
	}
	raw, err := h.client.Conversation(c.Request().Context(), id, limit)
	if err != nil {
		return respondError(c, err)
	}
	return rawJSON(c, raw)
}

func (h *Handler) SessionsOverview(c echo.Context) error {
	raw, err := h.client.Sessions(c.Request().Context())
	if err != nil {
		return respondError(c, err)
	}
	return rawJSON(c, raw)
}

func (h *Handler) MCPServers(c echo.Context) error {
	raw, err := h.client.MCPServers(c.Request().Context())
	if err != nil {
		return respondError(c, err)
	}
	return rawJSON(c, raw)
}

func (h *Handler) ProviderTest(c echo.Context) error {
	var req bridgetypes.ProviderTestRequest
	if err := bind(c, &req); err != nil {
		return err
	}
	if strings.TrimSpace(req.ProviderID) == "" {
		return badRequest(c, "providerId is required")
	}
	providers, err := h.client.Providers(c.Request().Context())
	if err != nil {
		return respondError(c, err)
	}
	if !providerExists(providers, req.ProviderID, req.ModelID) {
		return badRequest(c, "provider or model is not available in OpenCode")
	}
	result, err := h.client.Ask(c.Request().Context(), bridgetypes.AskRequest{
		Prompt: "Reply with OK.", Title: "Provider connectivity test", ProviderID: req.ProviderID, ModelID: req.ModelID,
	})
	if err != nil {
		return respondError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"available": true, "providerId": req.ProviderID, "modelID": req.ModelID, "sessionId": result.SessionID, "message": result.Message})
}

func sessionStatus(raw json.RawMessage, id string) json.RawMessage {
	var statuses map[string]json.RawMessage
	if json.Unmarshal(raw, &statuses) != nil {
		return json.RawMessage(`{"type":"unknown"}`)
	}
	if status, ok := statuses[id]; ok {
		return status
	}
	return json.RawMessage(`{"type":"idle"}`)
}

func providerExists(raw json.RawMessage, providerID, modelID string) bool {
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

func bind(c echo.Context, target any) error {
	if err := c.Bind(target); err != nil {
		return c.JSON(http.StatusBadRequest, bridgetypes.ErrorResponse{Error: "invalid JSON request"})
	}
	return nil
}

func badRequest(c echo.Context, message string) error {
	return c.JSON(http.StatusBadRequest, bridgetypes.ErrorResponse{Error: message})
}

func rawJSON(c echo.Context, raw json.RawMessage) error {
	return c.Blob(http.StatusOK, echo.MIMEApplicationJSONCharsetUTF8, raw)
}

func respondError(c echo.Context, err error) error {
	if errors.Is(err, client.ErrUnavailable) || errors.Is(err, context.DeadlineExceeded) {
		return c.JSON(http.StatusServiceUnavailable, bridgetypes.ErrorResponse{Error: client.ErrUnavailable.Error(), Hint: client.StartHint})
	}
	var httpErr *client.HTTPError
	if errors.As(err, &httpErr) {
		status := httpErr.StatusCode
		if status < 400 || status > 599 {
			status = http.StatusBadGateway
		}
		return c.JSON(status, bridgetypes.ErrorResponse{Error: httpErr.Error()})
	}
	return c.JSON(http.StatusBadGateway, bridgetypes.ErrorResponse{Error: err.Error()})
}

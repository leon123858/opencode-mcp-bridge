package types

import "encoding/json"

type ErrorResponse struct {
	Error string `json:"error"`
	Hint  string `json:"hint,omitempty"`
}

type AskRequest struct {
	Prompt     string `json:"prompt"`
	Title      string `json:"title,omitempty"`
	ProviderID string `json:"providerID,omitempty"`
	ModelID    string `json:"modelID,omitempty"`
	Agent      string `json:"agent,omitempty"`
}

type ReplyRequest struct {
	SessionID  string `json:"sessionId"`
	Prompt     string `json:"prompt"`
	ProviderID string `json:"providerID,omitempty"`
	ModelID    string `json:"modelID,omitempty"`
	Agent      string `json:"agent,omitempty"`
}

type RunRequest struct {
	AskRequest
	MaxDurationSeconds int `json:"maxDurationSeconds,omitempty"`
}

type ProviderTestRequest struct {
	ProviderID string `json:"providerId"`
	ModelID    string `json:"modelID,omitempty"`
}

type MessageResponse struct {
	SessionID string          `json:"sessionId"`
	Message   string          `json:"message"`
	Raw       json.RawMessage `json:"raw,omitempty"`
}

type SetupResponse struct {
	Healthy bool            `json:"healthy"`
	Health  json.RawMessage `json:"health,omitempty"`
	Project json.RawMessage `json:"project,omitempty"`
}

type RunResponse struct {
	SessionID string          `json:"sessionId"`
	Status    string          `json:"status"`
	Message   string          `json:"message,omitempty"`
	Raw       json.RawMessage `json:"raw,omitempty"`
}

type CheckResponse struct {
	Session json.RawMessage `json:"session"`
	Status  json.RawMessage `json:"status"`
	Todos   json.RawMessage `json:"todos,omitempty"`
	Diff    json.RawMessage `json:"diff,omitempty"`
}

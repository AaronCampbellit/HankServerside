package protocol

import (
	"encoding/json"
	"time"
)

const Version = "v1"

const (
	TypeAgentRegister        = "agent.register"
	TypeAgentRegistered      = "agent.registered"
	TypeAgentHeartbeat       = "agent.heartbeat"
	TypeAgentEvent           = "agent.event"
	TypeAgentError           = "agent.error"
	TypeAppCommand           = "app.command"
	TypeAppResponse          = "app.response"
	TypeAppEvent             = "app.event"
	TypeAppError             = "app.error"
	TypeCloudCommand         = "cloud.command"
	TypeCloudResponse        = "cloud.response"
	TypeFileTransferOpen     = "file.transfer.open"
	TypeFileTransferReady    = "file.transfer.ready"
	TypeFileTransferData     = "file.transfer.data"
	TypeFileTransferComplete = "file.transfer.complete"
	TypeFileTransferCancel   = "file.transfer.cancel"
	TypeFileTransferError    = "file.transfer.error"
)

const (
	CommandSystemPing    = "system.ping"
	CommandSystemRestart = "system.restart"
)

type Envelope struct {
	Version   string          `json:"version"`
	Type      string          `json:"type"`
	RequestID string          `json:"request_id,omitempty"`
	AgentID   string          `json:"agent_id,omitempty"`
	HomeID    string          `json:"home_id,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Error     *ErrorPayload   `json:"error,omitempty"`
}

type ErrorPayload struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type AgentRegister struct {
	AgentID   string            `json:"agent_id"`
	HomeName  string            `json:"home_name,omitempty"`
	AgentType string            `json:"agent_type,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type AgentRegistered struct {
	AcceptedAt   time.Time `json:"accepted_at"`
	HomeID       string    `json:"home_id"`
	Message      string    `json:"message"`
	Capabilities []string  `json:"capabilities,omitempty"`
}

type AgentHeartbeat struct {
	AgentID      string    `json:"agent_id"`
	SentAt       time.Time `json:"sent_at"`
	Capabilities []string  `json:"capabilities,omitempty"`
}

type RoutedCommand struct {
	Command string          `json:"command"`
	Body    json.RawMessage `json:"body,omitempty"`
}

type AppSubscribeRequest struct {
	Topics []string `json:"topics"`
}

type AppSubscribeResponse struct {
	Topics []string `json:"topics"`
}

type AppEvent struct {
	Event string          `json:"event"`
	Topic string          `json:"topic,omitempty"`
	Body  json.RawMessage `json:"body,omitempty"`
}

type AgentEvent struct {
	Event string          `json:"event"`
	Topic string          `json:"topic,omitempty"`
	Body  json.RawMessage `json:"body,omitempty"`
}

type RoutedResponse struct {
	OK   bool            `json:"ok"`
	Body json.RawMessage `json:"body,omitempty"`
}

type SystemPingRequest struct {
	Message string `json:"message,omitempty"`
}

type SystemPingResponse struct {
	Message string    `json:"message"`
	Time    time.Time `json:"time"`
}

type SystemRestartRequest struct {
	Reason string `json:"reason,omitempty"`
}

type SystemRestartResponse struct {
	OK        bool      `json:"ok"`
	Message   string    `json:"message"`
	RestartAt time.Time `json:"restart_at"`
}

func NewEnvelope(messageType string, requestID string, agentID string, homeID string, payload any) (Envelope, error) {
	envelope := Envelope{
		Version:   Version,
		Type:      messageType,
		RequestID: requestID,
		AgentID:   agentID,
		HomeID:    homeID,
		Timestamp: time.Now().UTC(),
	}

	if payload == nil {
		return envelope, nil
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, err
	}

	envelope.Payload = encoded
	return envelope, nil
}

func NewErrorEnvelope(messageType string, requestID string, agentID string, homeID string, code string, message string, details map[string]any) Envelope {
	return Envelope{
		Version:   Version,
		Type:      messageType,
		RequestID: requestID,
		AgentID:   agentID,
		HomeID:    homeID,
		Timestamp: time.Now().UTC(),
		Error: &ErrorPayload{
			Code:    code,
			Message: message,
			Details: details,
		},
	}
}

func DecodePayload[T any](envelope Envelope) (T, error) {
	var payload T
	err := json.Unmarshal(envelope.Payload, &payload)
	return payload, err
}

func EncodeBody(payload any) (json.RawMessage, error) {
	if payload == nil {
		return nil, nil
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	return encoded, nil
}

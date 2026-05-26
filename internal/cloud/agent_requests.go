package cloud

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/dropfile/hankremote/internal/protocol"
)

type agentRequestRegistry struct {
	mu      sync.Mutex
	pending map[string]chan protocol.Envelope
}

func newAgentRequestRegistry() *agentRequestRegistry {
	return &agentRequestRegistry{pending: make(map[string]chan protocol.Envelope)}
}

func (r *agentRequestRegistry) Register(requestID string) (<-chan protocol.Envelope, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.pending[requestID]; ok {
		return nil, errors.New("duplicate agent request")
	}
	ch := make(chan protocol.Envelope, 1)
	r.pending[requestID] = ch
	return ch, nil
}

func (r *agentRequestRegistry) Resolve(envelope protocol.Envelope) bool {
	r.mu.Lock()
	ch, ok := r.pending[envelope.RequestID]
	if ok {
		delete(r.pending, envelope.RequestID)
	}
	r.mu.Unlock()
	if !ok {
		return false
	}
	ch <- envelope
	return true
}

func (r *agentRequestRegistry) Cancel(requestID string) {
	r.mu.Lock()
	delete(r.pending, requestID)
	r.mu.Unlock()
}

func (s *Server) sendAgentCommand(ctx context.Context, homeID string, command string, body any) (protocol.Envelope, error) {
	agentConn, ok := s.router.GetAgent(homeID)
	if !ok {
		s.recordAssistantTrace(ctx, assistantTraceEvent{
			Level:   "error",
			Scope:   "agent",
			Event:   "agent.command.offline",
			Summary: "Home agent is offline before command dispatch.",
			HomeID:  homeID,
			Details: traceDetails(map[string]any{
				"command": command,
			}),
		})
		return protocol.Envelope{}, errors.New("agent offline")
	}

	requestID := newID("sync")
	startedAt := time.Now()
	traceCtx := assistantTraceContextFrom(ctx)
	traceCtx.HomeID = firstNonBlank(traceCtx.HomeID, homeID)
	traceCtx.RequestID = requestID
	ctx = withAssistantTraceContext(ctx, traceCtx)
	s.recordAssistantTrace(ctx, assistantTraceEvent{
		Scope:   "agent",
		Event:   "agent.command.start",
		Summary: "Sending command to the home agent.",
		Details: traceDetails(map[string]any{
			"command":  command,
			"agent_id": agentConn.agent.ID,
		}),
	})
	responseCh, err := s.agentRequests.Register(requestID)
	if err != nil {
		s.recordAssistantTrace(ctx, assistantTraceEvent{
			Level:   "error",
			Scope:   "agent",
			Event:   "agent.command.register_failed",
			Summary: "Could not register the pending agent request.",
			Details: traceDetails(map[string]any{
				"command": command,
				"error":   err.Error(),
			}),
		})
		return protocol.Envelope{}, err
	}
	defer s.agentRequests.Cancel(requestID)

	commandBody, err := protocol.EncodeBody(body)
	if err != nil {
		s.recordAssistantTrace(ctx, assistantTraceEvent{
			Level:   "error",
			Scope:   "agent",
			Event:   "agent.command.encode_failed",
			Summary: "Could not encode the agent command body.",
			Details: traceDetails(map[string]any{
				"command": command,
				"error":   err.Error(),
			}),
		})
		return protocol.Envelope{}, err
	}
	envelope, err := protocol.NewEnvelope(protocol.TypeCloudCommand, requestID, agentConn.agent.ID, homeID, protocol.RoutedCommand{
		Command: command,
		Body:    commandBody,
	})
	if err != nil {
		s.recordAssistantTrace(ctx, assistantTraceEvent{
			Level:   "error",
			Scope:   "agent",
			Event:   "agent.command.envelope_failed",
			Summary: "Could not create the agent command envelope.",
			Details: traceDetails(map[string]any{
				"command": command,
				"error":   err.Error(),
			}),
		})
		return protocol.Envelope{}, err
	}
	if err := agentConn.peer.Write(ctx, envelope); err != nil {
		s.recordAssistantTrace(ctx, assistantTraceEvent{
			Level:   "error",
			Scope:   "agent",
			Event:   "agent.command.write_failed",
			Summary: "Failed while sending command to the home agent.",
			Details: traceDetails(map[string]any{
				"command":    command,
				"error":      err.Error(),
				"elapsed_ms": time.Since(startedAt).Milliseconds(),
			}),
		})
		return protocol.Envelope{}, err
	}

	timeout := s.requestTimeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	select {
	case <-waitCtx.Done():
		s.recordAssistantTrace(ctx, assistantTraceEvent{
			Level:   "error",
			Scope:   "agent",
			Event:   "agent.command.timeout",
			Summary: "Timed out waiting for the home agent response.",
			Details: traceDetails(map[string]any{
				"command":    command,
				"error":      waitCtx.Err().Error(),
				"elapsed_ms": time.Since(startedAt).Milliseconds(),
			}),
		})
		return protocol.Envelope{}, waitCtx.Err()
	case response := <-responseCh:
		level := "info"
		event := "agent.command.completed"
		summary := "Home agent returned a response."
		details := traceDetails(map[string]any{
			"command":    command,
			"elapsed_ms": time.Since(startedAt).Milliseconds(),
		})
		if response.Error != nil {
			level = "error"
			event = "agent.command.error"
			summary = "Home agent returned an error."
			details["error"] = response.Error.Message
		}
		s.recordAssistantTrace(ctx, assistantTraceEvent{
			Level:   level,
			Scope:   "agent",
			Event:   event,
			Summary: summary,
			Details: details,
		})
		return response, nil
	}
}

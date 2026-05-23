package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	agentfiles "github.com/dropfile/hankremote/internal/agent/files"
	agentha "github.com/dropfile/hankremote/internal/agent/homeassistant"
	agentmedia "github.com/dropfile/hankremote/internal/agent/media"
	agentnotes "github.com/dropfile/hankremote/internal/agent/notes"
	"github.com/dropfile/hankremote/internal/protocol"
)

const maxMessageSize = 2 << 20

type Client struct {
	cloudURL   string
	agentID    string
	token      string
	homeName   string
	configPath string
	logger     *slog.Logger
	dispatcher commandDispatcher
	writeMu    sync.Mutex
	uploadsMu  sync.Mutex
	uploads    map[string]*uploadTransfer
}

func NewClient(cloudURL string, agentID string, token string, homeName string, configPath string, ha *agentha.Client, files *agentfiles.Service, media *agentmedia.Service, notes *agentnotes.Service, logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.Default()
	}
	if ha == nil {
		ha = agentha.New("", "", 0)
	}
	if files == nil {
		files = agentfiles.New("")
	}
	if media == nil {
		media = agentmedia.New(agentmedia.Config{}, files, logger)
	}
	if notes == nil {
		notes = agentnotes.New("")
	}

	return &Client{
		cloudURL:   cloudURL,
		agentID:    agentID,
		token:      token,
		homeName:   homeName,
		configPath: configPath,
		logger:     logger,
		dispatcher: commandDispatcher{
			ha:     ha,
			files:  files,
			media:  media,
			notes:  notes,
			config: newConfigManager(configPath, ha, files),
		},
		uploads: make(map[string]*uploadTransfer),
	}
}

func (c *Client) Run(ctx context.Context) error {
	backoff := 2 * time.Second

	for {
		err := c.runOnce(ctx)
		if err == nil || ctx.Err() != nil {
			return err
		}

		c.logger.Warn("agent connection ended; retrying", "agent_id", c.agentID, "error", err, "retry_in", backoff.String())

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}

		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (c *Client) runOnce(ctx context.Context) error {
	wsURL, err := c.connectionURL()
	if err != nil {
		return err
	}

	c.logger.Info("connecting Hank Remote agent", "agent_id", c.agentID, "cloud_url", wsURL)
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	})
	if err != nil {
		return err
	}
	defer conn.Close(websocket.StatusNormalClosure, "agent shutting down")
	conn.SetReadLimit(maxMessageSize)

	if err := c.sendRegister(ctx, conn); err != nil {
		return err
	}

	heartbeatTicker := time.NewTicker(15 * time.Second)
	defer heartbeatTicker.Stop()
	monitorCtx, stopMonitors := context.WithCancel(ctx)
	defer stopMonitors()

	readErr := make(chan error, 1)
	go func() {
		readErr <- c.readLoop(ctx, conn)
	}()
	go c.emitHomeAssistantChanges(monitorCtx, conn)
	go c.emitFileDirectoryChanges(monitorCtx, conn, "/")
	if c.dispatcher.media != nil {
		c.dispatcher.media.SetEventSink(func(ctx context.Context, event string, topic string, payload any) error {
			return c.sendAgentEvent(ctx, conn, event, topic, payload)
		})
		defer c.dispatcher.media.SetEventSink(nil)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-readErr:
			return err
		case <-heartbeatTicker.C:
			if err := c.sendHeartbeat(ctx, conn); err != nil {
				return err
			}
		}
	}
}

func (c *Client) connectionURL() (string, error) {
	parsed, err := url.Parse(c.cloudURL)
	if err != nil {
		return "", fmt.Errorf("invalid cloud websocket url: %w", err)
	}

	query := parsed.Query()
	query.Set("agent_id", c.agentID)
	query.Set("token", c.token)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func (c *Client) sendRegister(ctx context.Context, conn *websocket.Conn) error {
	envelope, err := protocol.NewEnvelope(protocol.TypeAgentRegister, "", c.agentID, "", protocol.AgentRegister{
		AgentID:   c.agentID,
		HomeName:  c.homeName,
		AgentType: "home-agent",
		Metadata: map[string]string{
			"build": "dev",
		},
	})
	if err != nil {
		return err
	}

	if err := c.writeJSON(ctx, conn, envelope); err != nil {
		return err
	}

	c.logger.Info("sent agent registration", "agent_id", c.agentID, "home_name", c.homeName)
	return nil
}

func (c *Client) sendHeartbeat(ctx context.Context, conn *websocket.Conn) error {
	envelope, err := protocol.NewEnvelope(protocol.TypeAgentHeartbeat, "", c.agentID, "", protocol.AgentHeartbeat{
		AgentID:      c.agentID,
		SentAt:       time.Now().UTC(),
		Capabilities: c.capabilities(),
	})
	if err != nil {
		return err
	}

	if err := c.writeJSON(ctx, conn, envelope); err != nil {
		return err
	}

	c.logger.Debug("sent agent heartbeat", "agent_id", c.agentID)
	return nil
}

func (c *Client) readLoop(ctx context.Context, conn *websocket.Conn) error {
	for {
		var envelope protocol.Envelope
		if err := wsjson.Read(ctx, conn, &envelope); err != nil {
			return err
		}

		switch envelope.Type {
		case protocol.TypeAgentRegistered:
			payload, err := protocol.DecodePayload[protocol.AgentRegistered](envelope)
			if err != nil {
				return err
			}
			c.logger.Info("agent registration accepted", "agent_id", c.agentID, "home_id", payload.HomeID, "accepted_at", payload.AcceptedAt)

		case protocol.TypeAgentError:
			if envelope.Error != nil {
				c.logger.Warn("cloud returned protocol error", "agent_id", c.agentID, "code", envelope.Error.Code, "message", envelope.Error.Message)
			} else {
				c.logger.Warn("cloud returned unknown protocol error", "agent_id", c.agentID)
			}

		case protocol.TypeCloudCommand:
			if err := c.handleCommand(ctx, conn, envelope); err != nil {
				return err
			}

		case protocol.TypeFileTransferOpen:
			if err := c.handleTransferOpen(ctx, conn, envelope); err != nil {
				return err
			}

		case protocol.TypeFileTransferData:
			if err := c.handleTransferData(ctx, conn, envelope); err != nil {
				return err
			}

		case protocol.TypeFileTransferComplete:
			if err := c.handleTransferComplete(ctx, conn, envelope); err != nil {
				return err
			}

		default:
			c.logger.Info("received unsupported cloud message", "agent_id", c.agentID, "type", envelope.Type)
		}
	}
}

func (c *Client) handleCommand(ctx context.Context, conn *websocket.Conn, envelope protocol.Envelope) error {
	command, err := protocol.DecodePayload[protocol.RoutedCommand](envelope)
	if err != nil {
		return c.writeError(ctx, conn, envelope.RequestID, envelope.HomeID, "invalid_command_payload", err.Error(), nil)
	}

	c.logger.Info("handling cloud command", "agent_id", c.agentID, "home_id", envelope.HomeID, "command", command.Command, "request_id", envelope.RequestID)

	body, commandErr := c.dispatcher.dispatch(ctx, command)
	if commandErr != nil {
		return c.writeError(ctx, conn, envelope.RequestID, envelope.HomeID, commandErr.Code, commandErr.Message, commandErr.Details)
	}

	responseBody, err := protocol.EncodeBody(body)
	if err != nil {
		return c.writeError(ctx, conn, envelope.RequestID, envelope.HomeID, "encoding_failed", err.Error(), nil)
	}

	response := protocol.Envelope{
		Version:   protocol.Version,
		Type:      protocol.TypeCloudResponse,
		RequestID: envelope.RequestID,
		AgentID:   c.agentID,
		HomeID:    envelope.HomeID,
		Timestamp: time.Now().UTC(),
		Payload:   responseBody,
	}
	return c.writeJSON(ctx, conn, response)
}

func (c *Client) writeError(ctx context.Context, conn *websocket.Conn, requestID string, homeID string, code string, message string, details map[string]any) error {
	envelope := protocol.NewErrorEnvelope(protocol.TypeCloudResponse, requestID, c.agentID, homeID, code, message, details)
	return c.writeJSON(ctx, conn, envelope)
}

func (c *Client) capabilities() []string {
	capabilities := []string{protocol.CommandSystemPing}
	if c.dispatcher.ha.Enabled() {
		capabilities = append(capabilities,
			"homeassistant.health",
			"homeassistant.fetch_states",
			"homeassistant.fetch_state",
			"homeassistant.call_service",
		)
	}
	if c.dispatcher.files.Enabled() {
		capabilities = append(capabilities,
			"files.list",
			"files.stat",
			"files.search",
			"files.download",
			"files.upload",
			"files.create_directory",
			"files.rename",
			"files.delete",
		)
	}
	if c.dispatcher.notes.Enabled() {
		capabilities = append(capabilities,
			"notes.list",
			"notes.fetch",
			"notes.save",
			"notes.rename",
			"notes.delete",
			"notes.sync",
			"notes.search",
			"notes.tags",
			"notes.tag_rollup",
		)
	}
	if c.dispatcher.media != nil {
		capabilities = append(capabilities,
			protocol.CommandMediaSettingsStatus,
			protocol.CommandMediaSettingsApply,
			protocol.CommandMediaDownloadJobs,
			protocol.CommandMediaDownloadCancel,
		)
	}
	if c.dispatcher.media != nil && c.dispatcher.media.Enabled() {
		capabilities = append(capabilities,
			protocol.CommandMediaSearch,
			protocol.CommandMediaPlanDownload,
			protocol.CommandMediaDownloadStart,
			protocol.CommandMediaDownloadStatus,
		)
	}
	capabilities = append(capabilities, "config.status", "config.apply")
	return capabilities
}

func (c *Client) writeJSON(ctx context.Context, conn *websocket.Conn, payload any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return wsjson.Write(ctx, conn, payload)
}

func (c *Client) emitHomeAssistantChanges(ctx context.Context, conn *websocket.Conn) {
	if !c.dispatcher.ha.Enabled() {
		return
	}

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	seen := map[string]string{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			states, err := c.dispatcher.ha.FetchStates(ctx)
			if err != nil {
				c.logger.Debug("home assistant realtime poll failed", "agent_id", c.agentID, "error", err)
				continue
			}
			for _, state := range states {
				hash := stableJSONHash(state)
				if previous, ok := seen[state.EntityID]; ok && previous == hash {
					continue
				}
				seen[state.EntityID] = hash
				if err := c.sendAgentEvent(ctx, conn, "homeassistant.state_changed", "homeassistant.states", map[string]any{"state": state}); err != nil {
					c.logger.Debug("home assistant realtime event failed", "agent_id", c.agentID, "error", err)
					return
				}
			}
		}
	}
}

func (c *Client) emitFileDirectoryChanges(ctx context.Context, conn *websocket.Conn, path string) {
	if !c.dispatcher.files.Enabled() {
		return
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	var previous string
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			items, err := c.dispatcher.files.List(ctx, path)
			if err != nil {
				c.logger.Debug("file realtime poll failed", "agent_id", c.agentID, "path", path, "error", err)
				continue
			}
			hash := stableJSONHash(items)
			if previous == hash {
				continue
			}
			previous = hash
			if err := c.sendAgentEvent(ctx, conn, "files.directory_changed", "files.directory:"+path, map[string]any{"path": path, "items": items}); err != nil {
				c.logger.Debug("file realtime event failed", "agent_id", c.agentID, "path", path, "error", err)
				return
			}
		}
	}
}

func (c *Client) sendAgentEvent(ctx context.Context, conn *websocket.Conn, event string, topic string, payload any) error {
	body, err := protocol.EncodeBody(payload)
	if err != nil {
		return err
	}
	envelope, err := protocol.NewEnvelope(protocol.TypeAgentEvent, "", c.agentID, "", protocol.AgentEvent{
		Event: event,
		Topic: topic,
		Body:  body,
	})
	if err != nil {
		return err
	}
	return c.writeJSON(ctx, conn, envelope)
}

func stableJSONHash(value any) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

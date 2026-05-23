package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/dropfile/hankremote/internal/protocol"
)

func TestAssistantMediaSettingsEndpointAppliesThroughAgent(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	testServer, homeID, agentID, sessionToken, agentConn := setupServerAndAgent(t, ctx)
	defer testServer.Close()
	defer agentConn.Close(websocket.StatusNormalClosure, "done")

	requestCh := make(chan protocol.MediaSettingsApplyRequest, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- serveOneMediaAgentCommand(ctx, agentConn, agentID, homeID, protocol.CommandMediaSettingsApply, func(body json.RawMessage) (any, error) {
			var request protocol.MediaSettingsApplyRequest
			if err := json.Unmarshal(body, &request); err != nil {
				return nil, err
			}
			requestCh <- request
			settings := request.Settings
			settings.HasPassword = request.Password != ""
			return protocol.MediaSettingsApplyResponse{Settings: settings}, nil
		})
	}()

	var response assistantMediaSettingsResponse
	requestJSON(t, testServer, sessionToken, http.MethodPut, "/v1/home/assistant/media-settings", map[string]any{
		"settings": map[string]any{
			"enabled":          true,
			"base_url":         "https://gramaton.io",
			"username":         "media@example.com",
			"destination_path": "Shows",
		},
		"password": "test-password",
	}, &response)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	request := <-requestCh
	if !request.Persist || request.Password != "test-password" || request.Settings.Username != "media@example.com" || request.Settings.DestinationPath != "Shows" {
		t.Fatalf("settings apply request = %#v", request)
	}
	if !response.Online || !response.Settings.Enabled || !response.Settings.HasPassword {
		t.Fatalf("settings response = %#v", response)
	}
}

func TestAssistantMediaJobCancelEndpointRoutesToAgent(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	testServer, homeID, agentID, sessionToken, agentConn := setupServerAndAgent(t, ctx)
	defer testServer.Close()
	defer agentConn.Close(websocket.StatusNormalClosure, "done")

	requestCh := make(chan protocol.MediaDownloadCancelRequest, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- serveOneMediaAgentCommand(ctx, agentConn, agentID, homeID, protocol.CommandMediaDownloadCancel, func(body json.RawMessage) (any, error) {
			var request protocol.MediaDownloadCancelRequest
			if err := json.Unmarshal(body, &request); err != nil {
				return nil, err
			}
			requestCh <- request
			return protocol.MediaDownloadCancelResponse{Job: protocol.MediaDownloadJobStatus{
				JobID:  request.JobID,
				Status: protocol.MediaJobStatusCancelled,
				Title:  "Fixture Movie",
			}}, nil
		})
	}()

	var response assistantMediaJobCancelResponse
	requestJSON(t, testServer, sessionToken, http.MethodPost, "/v1/home/assistant/media-jobs/job_fixture/cancel", nil, &response)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	request := <-requestCh
	if request.JobID != "job_fixture" {
		t.Fatalf("cancel request = %#v", request)
	}
	if !response.Online || response.Job.Status != protocol.MediaJobStatusCancelled {
		t.Fatalf("cancel response = %#v", response)
	}
}

func serveOneMediaAgentCommand(ctx context.Context, agentConn *websocket.Conn, agentID string, homeID string, wantCommand string, handler func(json.RawMessage) (any, error)) error {
	for {
		var envelope protocol.Envelope
		if err := wsjson.Read(ctx, agentConn, &envelope); err != nil {
			return err
		}
		if envelope.Type != protocol.TypeCloudCommand {
			continue
		}
		command, err := protocol.DecodePayload[protocol.RoutedCommand](envelope)
		if err != nil {
			return err
		}
		if command.Command != wantCommand {
			return fmt.Errorf("command = %q, want %q", command.Command, wantCommand)
		}
		responsePayload, err := handler(command.Body)
		if err != nil {
			return err
		}
		response, err := protocol.NewEnvelope(protocol.TypeCloudResponse, envelope.RequestID, agentID, homeID, responsePayload)
		if err != nil {
			return err
		}
		return wsjson.Write(ctx, agentConn, response)
	}
}

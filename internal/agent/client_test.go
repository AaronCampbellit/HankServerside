package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	agentfiles "github.com/dropfile/hankremote/internal/agent/files"
	"github.com/dropfile/hankremote/internal/protocol"
)

type discardHandler struct{}

func (discardHandler) Enabled(context.Context, slog.Level) bool  { return false }
func (discardHandler) Handle(context.Context, slog.Record) error { return nil }
func (discardHandler) WithAttrs([]slog.Attr) slog.Handler        { return discardHandler{} }
func (discardHandler) WithGroup(string) slog.Handler             { return discardHandler{} }

func TestClientSystemRestartAcknowledgesBeforeRestartHook(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	restarted := make(chan struct{}, 1)
	client := NewClient("ws://example.invalid", "agent_1", "token", "Home", "", nil, nil, nil, nil, slog.New(discardHandler{}))
	client.restartFn = func() {
		restarted <- struct{}{}
	}

	body, err := protocol.EncodeBody(protocol.SystemRestartRequest{Reason: "test"})
	if err != nil {
		t.Fatal(err)
	}
	commandBody, err := protocol.EncodeBody(protocol.RoutedCommand{Command: protocol.CommandSystemRestart, Body: body})
	if err != nil {
		t.Fatal(err)
	}
	envelope := protocol.Envelope{
		Version:   protocol.Version,
		Type:      protocol.TypeCloudCommand,
		RequestID: "restart_req",
		AgentID:   "agent_1",
		HomeID:    "home_1",
		Timestamp: time.Now().UTC(),
		Payload:   commandBody,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "done")
		if err := client.handleCommand(ctx, conn, envelope); err != nil {
			t.Errorf("handleCommand: %v", err)
		}
	}))
	defer server.Close()

	conn, _, err := websocket.Dial(ctx, "ws"+server.URL[len("http"):], nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "done")

	var response protocol.Envelope
	if err := wsjson.Read(ctx, conn, &response); err != nil {
		t.Fatalf("read response: %v", err)
	}
	if response.Type != protocol.TypeCloudResponse || response.RequestID != envelope.RequestID {
		t.Fatalf("response envelope = %#v", response)
	}
	var payload protocol.SystemRestartResponse
	if err := json.Unmarshal(response.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.OK || payload.Message == "" || payload.RestartAt.IsZero() {
		t.Fatalf("restart response = %#v", payload)
	}

	select {
	case <-restarted:
	case <-ctx.Done():
		t.Fatal("restart hook was not called")
	}
}

func TestDownloadTransferHonorsRequestedLength(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "demo.txt"), []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	client := NewClient("ws://example.invalid", "agent_1", "token", "Home", "", nil, agentfiles.New(root), nil, nil, slog.New(discardHandler{}))

	open, err := protocol.NewEnvelope(protocol.TypeFileTransferOpen, "xfer_1", "agent_1", "home_1", protocol.FileTransferOpen{
		Operation: protocol.FileTransferOperationDownload,
		Path:      "demo.txt",
		Offset:    6,
		Length:    5,
	})
	if err != nil {
		t.Fatal(err)
	}

	releaseServer := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "done")
		if err := client.handleTransferOpen(ctx, conn, open); err != nil {
			t.Errorf("handleTransferOpen: %v", err)
			return
		}
		<-releaseServer
	}))
	defer server.Close()
	defer close(releaseServer)

	conn, _, err := websocket.Dial(ctx, "ws"+server.URL[len("http"):], nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "done")

	var ready protocol.Envelope
	if err := wsjson.Read(ctx, conn, &ready); err != nil {
		t.Fatalf("read ready: %v", err)
	}
	if ready.Type != protocol.TypeFileTransferReady {
		t.Fatalf("ready envelope type = %q", ready.Type)
	}

	var dataEnvelope protocol.Envelope
	if err := wsjson.Read(ctx, conn, &dataEnvelope); err != nil {
		t.Fatalf("read data: %v", err)
	}
	if dataEnvelope.Type != protocol.TypeFileTransferData {
		t.Fatalf("data envelope type = %q", dataEnvelope.Type)
	}
	chunk, err := protocol.DecodePayload[protocol.FileTransferChunk](dataEnvelope)
	if err != nil {
		t.Fatal(err)
	}
	data, err := base64.StdEncoding.DecodeString(chunk.ContentBase64)
	if err != nil {
		t.Fatal(err)
	}
	if chunk.Offset != 6 || string(data) != "world" {
		t.Fatalf("chunk offset/data = %d/%q, want 6/world", chunk.Offset, string(data))
	}

	var completeEnvelope protocol.Envelope
	if err := wsjson.Read(ctx, conn, &completeEnvelope); err != nil {
		t.Fatalf("read complete: %v", err)
	}
	complete, err := protocol.DecodePayload[protocol.FileTransferComplete](completeEnvelope)
	if err != nil {
		t.Fatal(err)
	}
	if completeEnvelope.Type != protocol.TypeFileTransferComplete || complete.Offset != 11 || complete.Size != 11 {
		t.Fatalf("complete = type %q payload %#v", completeEnvelope.Type, complete)
	}
}

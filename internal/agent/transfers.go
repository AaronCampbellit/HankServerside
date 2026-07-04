package agent

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/coder/websocket"

	agentfiles "github.com/dropfile/hankremote/internal/agent/files"
	"github.com/dropfile/hankremote/internal/protocol"
)

const transferChunkSize = 32 * 1024

type uploadTransfer struct {
	sourceID string
	path     string
	file     agentfiles.WriteHandle
	size     int64
}

func (c *Client) handleTransferOpen(ctx context.Context, conn *websocket.Conn, envelope protocol.Envelope) error {
	open, err := protocol.DecodePayload[protocol.FileTransferOpen](envelope)
	if err != nil {
		return c.writeTransferError(ctx, conn, envelope.RequestID, envelope.HomeID, "invalid_transfer_open", err.Error())
	}

	switch open.Operation {
	case protocol.FileTransferOperationDownload:
		return c.startDownloadTransfer(ctx, conn, envelope.RequestID, envelope.HomeID, open.SourceID, open.Path, open.Offset, open.Length)
	case protocol.FileTransferOperationUpload:
		return c.startUploadTransfer(ctx, conn, envelope.RequestID, envelope.HomeID, open.SourceID, open.Path, open.Offset)
	default:
		return c.writeTransferError(ctx, conn, envelope.RequestID, envelope.HomeID, "invalid_transfer_operation", "unsupported file transfer operation")
	}
}

func (c *Client) handleTransferData(ctx context.Context, conn *websocket.Conn, envelope protocol.Envelope) error {
	chunk, err := protocol.DecodePayload[protocol.FileTransferChunk](envelope)
	if err != nil {
		return c.writeTransferError(ctx, conn, envelope.RequestID, envelope.HomeID, "invalid_transfer_chunk", err.Error())
	}

	data, err := base64.StdEncoding.DecodeString(chunk.ContentBase64)
	if err != nil {
		return c.writeTransferError(ctx, conn, envelope.RequestID, envelope.HomeID, "invalid_transfer_chunk", err.Error())
	}

	upload, ok := c.getUpload(envelope.RequestID)
	if !ok {
		return c.writeTransferError(ctx, conn, envelope.RequestID, envelope.HomeID, "transfer_not_found", "upload transfer was not prepared")
	}
	if chunk.Offset != upload.size {
		return c.writeTransferError(ctx, conn, envelope.RequestID, envelope.HomeID, "transfer_offset_mismatch", fmt.Sprintf("expected offset %d, got %d", upload.size, chunk.Offset))
	}

	written, err := upload.file.Write(data)
	if err != nil {
		c.deleteUpload(envelope.RequestID)
		_ = upload.file.Close()
		return c.writeTransferError(ctx, conn, envelope.RequestID, envelope.HomeID, "transfer_write_failed", err.Error())
	}
	upload.size += int64(written)
	return nil
}

func (c *Client) handleTransferComplete(ctx context.Context, conn *websocket.Conn, envelope protocol.Envelope) error {
	complete, err := protocol.DecodePayload[protocol.FileTransferComplete](envelope)
	if err != nil {
		return c.writeTransferError(ctx, conn, envelope.RequestID, envelope.HomeID, "invalid_transfer_complete", err.Error())
	}

	if complete.Operation != protocol.FileTransferOperationUpload {
		return nil
	}

	upload, ok := c.deleteUpload(envelope.RequestID)
	if !ok {
		return c.writeTransferError(ctx, conn, envelope.RequestID, envelope.HomeID, "transfer_not_found", "upload transfer was not prepared")
	}
	if complete.Offset != upload.size {
		_ = upload.file.Close()
		return c.writeTransferError(ctx, conn, envelope.RequestID, envelope.HomeID, "transfer_offset_mismatch", fmt.Sprintf("expected final offset %d, got %d", upload.size, complete.Offset))
	}

	if err := upload.file.Close(); err != nil {
		return c.writeTransferError(ctx, conn, envelope.RequestID, envelope.HomeID, "transfer_close_failed", err.Error())
	}

	reply, err := protocol.NewEnvelope(protocol.TypeFileTransferComplete, envelope.RequestID, c.agentID, envelope.HomeID, protocol.FileTransferComplete{
		Operation: protocol.FileTransferOperationUpload,
		SourceID:  upload.sourceID,
		Path:      upload.path,
		Offset:    upload.size,
		Size:      upload.size,
	})
	if err != nil {
		return err
	}

	return c.writeJSON(ctx, conn, reply)
}

func (c *Client) handleTransferCancel(envelope protocol.Envelope) {
	c.cancelDownload(envelope.RequestID)
}

func (c *Client) startDownloadTransfer(ctx context.Context, conn *websocket.Conn, transferID string, homeID string, sourceID string, path string, offset int64, length int64) error {
	if existing := c.setDownloadCancel(transferID, nil); existing != nil {
		existing()
	}
	file, info, err := c.dispatcher.files.OpenReaderSource(ctx, sourceID, path, offset)
	if err != nil {
		return c.writeTransferError(ctx, conn, transferID, homeID, "transfer_open_failed", err.Error())
	}

	ready, err := protocol.NewEnvelope(protocol.TypeFileTransferReady, transferID, c.agentID, homeID, protocol.FileTransferReady{
		Operation: protocol.FileTransferOperationDownload,
		SourceID:  sourceID,
		Path:      path,
		Offset:    offset,
		Size:      info.Size(),
	})
	if err != nil {
		_ = file.Close()
		return err
	}
	if err := c.writeJSON(ctx, conn, ready); err != nil {
		_ = file.Close()
		return err
	}

	transferCtx, cancel := context.WithCancel(ctx)
	if existing := c.setDownloadCancel(transferID, cancel); existing != nil {
		existing()
	}
	go c.streamDownload(transferCtx, conn, transferID, homeID, sourceID, path, offset, info.Size(), length, file)
	return nil
}

func (c *Client) startUploadTransfer(ctx context.Context, conn *websocket.Conn, transferID string, homeID string, sourceID string, path string, offset int64) error {
	if existing, ok := c.deleteUpload(transferID); ok {
		_ = existing.file.Close()
	}

	file, size, err := c.dispatcher.files.OpenWriterSource(ctx, sourceID, path, offset)
	if err != nil {
		return c.writeTransferError(ctx, conn, transferID, homeID, "transfer_open_failed", err.Error())
	}

	c.uploadsMu.Lock()
	c.uploads[transferID] = &uploadTransfer{
		sourceID: sourceID,
		path:     path,
		file:     file,
		size:     size,
	}
	c.uploadsMu.Unlock()

	ready, err := protocol.NewEnvelope(protocol.TypeFileTransferReady, transferID, c.agentID, homeID, protocol.FileTransferReady{
		Operation: protocol.FileTransferOperationUpload,
		SourceID:  sourceID,
		Path:      path,
		Offset:    size,
		Size:      size,
	})
	if err != nil {
		_, _ = c.deleteUpload(transferID)
		_ = file.Close()
		return err
	}

	return c.writeJSON(ctx, conn, ready)
}

func (c *Client) streamDownload(ctx context.Context, conn *websocket.Conn, transferID string, homeID string, sourceID string, path string, offset int64, totalSize int64, length int64, file agentfiles.ReadHandle) {
	defer file.Close()
	defer c.deleteDownload(transferID)

	buffer := make([]byte, transferChunkSize)
	currentOffset := offset
	remaining := length

	for {
		if remaining == 0 && length > 0 {
			break
		}
		readBuffer := buffer
		if remaining > 0 && remaining < int64(len(readBuffer)) {
			readBuffer = readBuffer[:remaining]
		}
		n, err := file.Read(readBuffer)
		if n > 0 {
			envelope, envelopeErr := protocol.NewEnvelope(protocol.TypeFileTransferData, transferID, c.agentID, homeID, protocol.FileTransferChunk{
				Offset:        currentOffset,
				ContentBase64: base64.StdEncoding.EncodeToString(readBuffer[:n]),
			})
			if envelopeErr != nil {
				_ = c.writeTransferError(context.Background(), conn, transferID, homeID, "transfer_encoding_failed", envelopeErr.Error())
				return
			}
			if writeErr := c.writeJSON(ctx, conn, envelope); writeErr != nil {
				return
			}
			currentOffset += int64(n)
			if remaining > 0 {
				remaining -= int64(n)
			}
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			_ = c.writeTransferError(context.Background(), conn, transferID, homeID, "transfer_read_failed", err.Error())
			return
		}
	}

	if ctx.Err() != nil {
		return
	}
	complete, envelopeErr := protocol.NewEnvelope(protocol.TypeFileTransferComplete, transferID, c.agentID, homeID, protocol.FileTransferComplete{
		Operation: protocol.FileTransferOperationDownload,
		SourceID:  sourceID,
		Path:      path,
		Offset:    currentOffset,
		Size:      totalSize,
	})
	if envelopeErr != nil {
		_ = c.writeTransferError(context.Background(), conn, transferID, homeID, "transfer_encoding_failed", envelopeErr.Error())
		return
	}
	_ = c.writeJSON(ctx, conn, complete)
}

func (c *Client) writeTransferError(ctx context.Context, conn *websocket.Conn, requestID string, homeID string, code string, message string) error {
	envelope := protocol.NewErrorEnvelope(protocol.TypeFileTransferError, requestID, c.agentID, homeID, code, message, nil)
	return c.writeJSON(ctx, conn, envelope)
}

func (c *Client) getUpload(transferID string) (*uploadTransfer, bool) {
	c.uploadsMu.Lock()
	defer c.uploadsMu.Unlock()
	upload, ok := c.uploads[transferID]
	return upload, ok
}

func (c *Client) deleteUpload(transferID string) (*uploadTransfer, bool) {
	c.uploadsMu.Lock()
	defer c.uploadsMu.Unlock()
	upload, ok := c.uploads[transferID]
	if ok {
		delete(c.uploads, transferID)
	}
	return upload, ok
}

func (c *Client) setDownloadCancel(transferID string, cancel context.CancelFunc) context.CancelFunc {
	c.downloadsMu.Lock()
	defer c.downloadsMu.Unlock()
	existing := c.downloads[transferID]
	if cancel == nil {
		delete(c.downloads, transferID)
	} else {
		c.downloads[transferID] = cancel
	}
	return existing
}

func (c *Client) cancelDownload(transferID string) bool {
	cancel := c.setDownloadCancel(transferID, nil)
	if cancel == nil {
		return false
	}
	cancel()
	return true
}

func (c *Client) deleteDownload(transferID string) {
	c.setDownloadCancel(transferID, nil)
}

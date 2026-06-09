package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dropfile/hankremote/internal/agent/files"
	"github.com/dropfile/hankremote/internal/protocol"
)

const (
	rangeProbeSize      int64 = 32
	rangeRetryBaseDelay       = 250 * time.Millisecond
)

var (
	errRangeDownloadUnavailable = errors.New("ranged media download is unavailable")
	rangeDownloadChunkSize      = int64(8 << 20)
	rangeDownloadMaxWorkers     = 4
	rangeDownloadMaxRetries     = 3
)

type rangeDownloadProbe struct {
	totalSize    int64
	sniff        []byte
	contentType  string
	etag         string
	lastModified string
}

type byteRange struct {
	start int64
	end   int64
}

func (s *Service) downloadOneRanged(ctx context.Context, job *downloadJob, download plannedDownload, partPath string) error {
	probe, err := s.probeRangedDownload(ctx, download)
	if err != nil {
		return err
	}
	job.update(func(status *protocol.MediaDownloadJobStatus) {
		status.DownloadMode = protocol.MediaDownloadModeRange
		status.Verification = "downloading"
		status.BytesTotal = probe.totalSize
		status.FallbackUsed = false
	})

	writer, err := s.files.OpenRandomWriter(ctx, partPath)
	if err != nil {
		return err
	}
	if err := writer.Truncate(0); err != nil {
		_ = writer.Close()
		return err
	}

	err = s.downloadRanges(ctx, job, download, probe, writer)
	closeErr := writer.Close()
	if err != nil {
		_ = s.files.Delete(context.Background(), partPath, false)
		return err
	}
	if closeErr != nil {
		_ = s.files.Delete(context.Background(), partPath, false)
		return closeErr
	}

	job.update(func(status *protocol.MediaDownloadJobStatus) {
		status.Verification = "verifying"
	})
	if err := s.verifyRangedPart(ctx, partPath, probe); err != nil {
		_ = s.files.Delete(context.Background(), partPath, false)
		return err
	}
	job.update(func(status *protocol.MediaDownloadJobStatus) {
		status.Verification = "verified"
		status.BytesWritten = probe.totalSize
	})
	return nil
}

func (s *Service) probeRangedDownload(ctx context.Context, download plannedDownload) (rangeDownloadProbe, error) {
	req, err := s.newDownloadRequest(ctx, download)
	if err != nil {
		return rangeDownloadProbe{}, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", rangeProbeSize-1))

	resp, err := s.client.Do(req)
	if err != nil {
		return rangeDownloadProbe{}, err
	}
	defer resp.Body.Close()

	sniff, err := io.ReadAll(io.LimitReader(resp.Body, rangeProbeSize+1))
	if err != nil {
		return rangeDownloadProbe{}, err
	}
	if err := validateDownloadResponse(resp, sniff); err != nil {
		return rangeDownloadProbe{}, err
	}
	if resp.StatusCode != http.StatusPartialContent {
		return rangeDownloadProbe{}, errRangeDownloadUnavailable
	}

	start, end, total, err := parseContentRange(resp.Header.Get("Content-Range"))
	if err != nil {
		return rangeDownloadProbe{}, err
	}
	if start != 0 || end < start || end+1 != int64(len(sniff)) || total <= 0 {
		return rangeDownloadProbe{}, fmt.Errorf("invalid range probe response")
	}
	if int64(len(sniff)) > rangeProbeSize {
		return rangeDownloadProbe{}, fmt.Errorf("range probe returned too many bytes")
	}
	if err := validateMediaSignature(sniff, resp.Header.Get("Content-Type")); err != nil {
		return rangeDownloadProbe{}, err
	}
	return rangeDownloadProbe{
		totalSize:    total,
		sniff:        append([]byte(nil), sniff...),
		contentType:  resp.Header.Get("Content-Type"),
		etag:         resp.Header.Get("ETag"),
		lastModified: resp.Header.Get("Last-Modified"),
	}, nil
}

func (s *Service) downloadRanges(ctx context.Context, job *downloadJob, download plannedDownload, probe rangeDownloadProbe, writer files.RandomWriteHandle) error {
	chunks := rangedChunks(probe.totalSize, rangeDownloadChunkSize)
	if len(chunks) == 0 {
		return fmt.Errorf("download has no ranged chunks")
	}

	workerCount := rangeDownloadMaxWorkers
	if workerCount <= 0 {
		workerCount = 1
	}
	if workerCount > len(chunks) {
		workerCount = len(chunks)
	}

	downloadCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var firstErr error
	var errOnce sync.Once
	setErr := func(err error) {
		if err == nil {
			return
		}
		errOnce.Do(func() {
			firstErr = err
			cancel()
		})
	}

	var written atomic.Int64
	var writerMu sync.Mutex
	chunkCh := make(chan byteRange)
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for chunk := range chunkCh {
				if downloadCtx.Err() != nil {
					continue
				}
				if err := s.downloadRangeChunk(downloadCtx, job, download, probe, chunk, writer, &writerMu, &written); err != nil {
					setErr(err)
				}
			}
		}()
	}

	for _, chunk := range chunks {
		if downloadCtx.Err() != nil {
			break
		}
		select {
		case chunkCh <- chunk:
		case <-downloadCtx.Done():
			break
		}
	}
	close(chunkCh)
	wg.Wait()

	if firstErr != nil {
		return firstErr
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if written.Load() != probe.totalSize {
		return fmt.Errorf("ranged download wrote %d bytes, want %d", written.Load(), probe.totalSize)
	}
	return nil
}

func (s *Service) downloadRangeChunk(ctx context.Context, job *downloadJob, download plannedDownload, probe rangeDownloadProbe, chunk byteRange, writer files.RandomWriteHandle, writerMu *sync.Mutex, written *atomic.Int64) error {
	expected := chunk.end - chunk.start + 1
	var lastErr error
	retries := rangeDownloadMaxRetries
	if retries <= 0 {
		retries = 1
	}
	for attempt := 0; attempt < retries; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		body, err := s.fetchRangeChunk(ctx, download, probe, chunk, expected)
		if err != nil {
			lastErr = err
			if !sleepBeforeRangeRetry(ctx, attempt) {
				return ctx.Err()
			}
			continue
		}

		writerMu.Lock()
		n, writeErr := writer.WriteAt(body, chunk.start)
		writerMu.Unlock()
		if writeErr != nil {
			return writeErr
		}
		if n != len(body) {
			return io.ErrShortWrite
		}

		total := written.Add(int64(len(body)))
		job.update(func(status *protocol.MediaDownloadJobStatus) {
			status.BytesWritten = total
		})
		return nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("range chunk failed")
	}
	return fmt.Errorf("range chunk %d-%d failed: %w", chunk.start, chunk.end, lastErr)
}

func (s *Service) fetchRangeChunk(ctx context.Context, download plannedDownload, probe rangeDownloadProbe, chunk byteRange, expected int64) ([]byte, error) {
	req, err := s.newDownloadRequest(ctx, download)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", chunk.start, chunk.end))

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, expected+1))
	if readErr != nil {
		return nil, readErr
	}
	if len(body) > int(expected) {
		return nil, fmt.Errorf("range chunk returned too many bytes")
	}
	if err := validateRangeChunkResponse(resp, body, probe, chunk, expected); err != nil {
		return nil, err
	}
	return body, nil
}

func validateRangeChunkResponse(resp *http.Response, body []byte, probe rangeDownloadProbe, chunk byteRange, expected int64) error {
	if resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("range chunk returned status %d", resp.StatusCode)
	}
	if err := validateDownloadResponse(resp, mediaSniff(body)); err != nil {
		return err
	}
	start, end, total, err := parseContentRange(resp.Header.Get("Content-Range"))
	if err != nil {
		return err
	}
	if start != chunk.start || end != chunk.end || total != probe.totalSize {
		return fmt.Errorf("range chunk returned content range %q, want bytes %d-%d/%d", resp.Header.Get("Content-Range"), chunk.start, chunk.end, probe.totalSize)
	}
	if int64(len(body)) != expected {
		return fmt.Errorf("range chunk returned %d bytes, want %d", len(body), expected)
	}
	if probe.etag != "" && resp.Header.Get("ETag") != probe.etag {
		return fmt.Errorf("range chunk ETag changed")
	}
	if probe.lastModified != "" && resp.Header.Get("Last-Modified") != probe.lastModified {
		return fmt.Errorf("range chunk Last-Modified changed")
	}
	return nil
}

func (s *Service) verifyRangedPart(ctx context.Context, partPath string, probe rangeDownloadProbe) error {
	reader, info, err := s.files.OpenReader(ctx, partPath, 0)
	if err != nil {
		return err
	}
	defer reader.Close()

	if info.Size() != probe.totalSize {
		return fmt.Errorf("verified file size = %d, want %d", info.Size(), probe.totalSize)
	}
	sniff := make([]byte, len(probe.sniff))
	if _, err := io.ReadFull(reader, sniff); err != nil {
		return err
	}
	if !bytes.Equal(sniff, probe.sniff) {
		return fmt.Errorf("verified media signature does not match range probe")
	}
	return validateMediaSignature(sniff, probe.contentType)
}

func rangedChunks(totalSize int64, chunkSize int64) []byteRange {
	if totalSize <= 0 {
		return nil
	}
	if chunkSize <= 0 {
		chunkSize = rangeDownloadChunkSize
	}
	chunks := make([]byteRange, 0, (totalSize+chunkSize-1)/chunkSize)
	for start := int64(0); start < totalSize; start += chunkSize {
		end := start + chunkSize - 1
		if end >= totalSize {
			end = totalSize - 1
		}
		chunks = append(chunks, byteRange{start: start, end: end})
	}
	return chunks
}

func parseContentRange(value string) (int64, int64, int64, error) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(strings.ToLower(value), "bytes ") {
		return 0, 0, 0, fmt.Errorf("missing byte content range")
	}
	value = strings.TrimSpace(value[len("bytes "):])
	rangePart, totalPart, ok := strings.Cut(value, "/")
	if !ok || strings.TrimSpace(totalPart) == "*" {
		return 0, 0, 0, fmt.Errorf("invalid byte content range")
	}
	startPart, endPart, ok := strings.Cut(strings.TrimSpace(rangePart), "-")
	if !ok {
		return 0, 0, 0, fmt.Errorf("invalid byte content range")
	}
	start, err := strconv.ParseInt(strings.TrimSpace(startPart), 10, 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid byte content range")
	}
	end, err := strconv.ParseInt(strings.TrimSpace(endPart), 10, 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid byte content range")
	}
	total, err := strconv.ParseInt(strings.TrimSpace(totalPart), 10, 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid byte content range")
	}
	if start < 0 || end < start || total <= end {
		return 0, 0, 0, fmt.Errorf("invalid byte content range")
	}
	return start, end, total, nil
}

func sleepBeforeRangeRetry(ctx context.Context, attempt int) bool {
	delay := rangeRetryBaseDelay * time.Duration(attempt+1)
	if delay > 2*time.Second {
		delay = 2 * time.Second
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func mediaSniff(body []byte) []byte {
	if len(body) > downloadSniffByteSize {
		return body[:downloadSniffByteSize]
	}
	return body
}

func validateMediaSignature(sniff []byte, contentType string) error {
	if len(sniff) == 0 {
		return fmt.Errorf("media verification found an empty file")
	}
	loweredType := strings.ToLower(strings.TrimSpace(contentType))
	loweredBody := strings.ToLower(strings.TrimSpace(string(mediaSniff(sniff))))
	if strings.Contains(loweredType, "text/html") ||
		strings.HasPrefix(loweredBody, "<!doctype html") ||
		strings.HasPrefix(loweredBody, "<html") ||
		strings.Contains(loweredBody, "</html>") ||
		strings.Contains(loweredBody, "<form") ||
		(strings.Contains(loweredBody, "login") && strings.Contains(loweredBody, "password")) {
		return fmt.Errorf("media verification found a web page instead of media")
	}
	if strings.Contains(loweredType, "video/mp4") || hasMP4Signature(sniff) {
		if !hasMP4Signature(sniff) {
			return fmt.Errorf("media verification did not find an MP4 signature")
		}
	}
	return nil
}

func hasMP4Signature(sniff []byte) bool {
	return len(sniff) >= 12 && string(sniff[4:8]) == "ftyp"
}

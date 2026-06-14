package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/dropfile/hankremote/internal/agent/apps"
	agentfiles "github.com/dropfile/hankremote/internal/agent/files"
	"github.com/dropfile/hankremote/internal/config"
)

const (
	defaultFormat         = "best"
	defaultOutputTemplate = "%(title).200B [%(id)s].%(ext)s"
	defaultSubLangs       = "en.*,all,-live_chat"
	defaultSubFormat      = "srt/best"
	defaultTimeoutSeconds = 900
	maxRequestBytes       = 1 << 20
	maxStderrBytes        = 64 << 10
)

type appConfig struct {
	SourceID           string `json:"source_id,omitempty"`
	DestinationPath    string `json:"destination_path,omitempty"`
	Format             string `json:"format,omitempty"`
	OutputTemplate     string `json:"output_template,omitempty"`
	WriteSubtitles     *bool  `json:"write_subtitles,omitempty"`
	WriteAutoSubtitles *bool  `json:"write_auto_subtitles,omitempty"`
	SubtitleLanguages  string `json:"subtitle_languages,omitempty"`
	SubtitleFormat     string `json:"subtitle_format,omitempty"`
	WriteThumbnail     *bool  `json:"write_thumbnail,omitempty"`
	WriteInfoJSON      *bool  `json:"write_info_json,omitempty"`
	DownloadPlaylist   *bool  `json:"download_playlist,omitempty"`
	RestrictFilenames  *bool  `json:"restrict_filenames,omitempty"`
	NoOverwrite        *bool  `json:"no_overwrite,omitempty"`
	RateLimit          string `json:"rate_limit,omitempty"`
	ProxyURL           string `json:"proxy_url,omitempty"`
	CookiesFilePath    string `json:"cookies_file_path,omitempty"`
	YTDLPPath          string `json:"yt_dlp_path,omitempty"`
	TimeoutSeconds     int    `json:"timeout_seconds,omitempty"`
	FilesRoot          string `json:"files_root,omitempty"`
}

type commandInput struct {
	URL             string `json:"url"`
	DestinationPath string `json:"destination_path,omitempty"`
	FilenamePrefix  string `json:"filename_prefix,omitempty"`
}

type commandOutput struct {
	Text            string           `json:"text"`
	SourceID        string           `json:"source_id,omitempty"`
	DestinationPath string           `json:"destination_path"`
	Files           []downloadedFile `json:"files"`
}

type downloadedFile struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type appError struct {
	code    string
	message string
}

func (e appError) Error() string {
	return e.code + ": " + e.message
}

func main() {
	os.Exit(run(context.Background(), os.Stdin, os.Stdout, os.Stderr))
}

func run(ctx context.Context, input io.Reader, output io.Writer, stderr io.Writer) int {
	var request apps.AppStdioRequest
	if err := json.NewDecoder(io.LimitReader(input, maxRequestBytes)).Decode(&request); err != nil {
		return writeFailure(output, stderr, "", "invalid_request", "invalid app request")
	}
	if request.AppID != "" && request.AppID != "ydownload" {
		return writeFailure(output, stderr, request.RequestID, "invalid_request", "invalid app id")
	}
	if request.CommandID != "download" {
		return writeFailure(output, stderr, request.RequestID, "invalid_request", "unsupported YDownload command")
	}

	response, err := runCommand(ctx, request, stderr)
	if err != nil {
		var appErr appError
		if errors.As(err, &appErr) {
			return writeFailure(output, stderr, request.RequestID, appErr.code, appErr.message)
		}
		return writeFailure(output, stderr, request.RequestID, "app_error", "YDownload command failed")
	}
	rawOutput, err := json.Marshal(response)
	if err != nil {
		return writeFailure(output, stderr, request.RequestID, "internal_error", "failed to encode app response")
	}
	return writeResponse(output, stderr, apps.AppStdioResponse{
		RequestID: request.RequestID,
		OK:        true,
		Output:    rawOutput,
	}, 0)
}

func runCommand(ctx context.Context, request apps.AppStdioRequest, stderr io.Writer) (commandOutput, error) {
	var cfg appConfig
	if err := decodeRaw(request.Config, &cfg); err != nil {
		return commandOutput{}, appError{"invalid_request", "invalid YDownload config"}
	}
	var body commandInput
	if err := decodeRaw(request.Input, &body); err != nil {
		return commandOutput{}, appError{"invalid_request", "invalid download input"}
	}
	url := strings.TrimSpace(body.URL)
	if url == "" {
		return commandOutput{}, appError{"invalid_request", "video URL is required"}
	}

	agentCfg, err := config.LoadAgent()
	if err != nil {
		return commandOutput{}, appError{"invalid_environment", "agent environment is not available"}
	}
	filesRoot := firstNonBlank(cfg.FilesRoot, agentCfg.FilesRoot)
	files := agentfiles.NewWithConfig(agentfiles.Config{
		Root:   filesRoot,
		Shares: agentSMBShares(agentCfg.SMBShares),
	})

	destinationPath := cleanRelativePath(firstNonBlank(body.DestinationPath, cfg.DestinationPath, "YouTube"))
	sourceID := strings.TrimSpace(cfg.SourceID)
	timeoutSeconds := cfg.TimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = defaultTimeoutSeconds
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	tempDir, err := os.MkdirTemp("", "hank-ydownload-*")
	if err != nil {
		return commandOutput{}, appError{"internal_error", "failed to create download workspace"}
	}
	defer os.RemoveAll(tempDir)

	args := ytDLPArgs(cfg, body, tempDir, url)
	cmd := exec.CommandContext(runCtx, firstNonBlank(cfg.YTDLPPath, "yt-dlp"), args...)
	var stderrBuf boundedBuffer
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderrBuf
	if err := cmd.Run(); err != nil {
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return commandOutput{}, appError{"timeout", "yt-dlp download timed out"}
		}
		return commandOutput{}, appError{"download_failed", sanitizeYTDLPError(stderrBuf.String())}
	}

	downloaded, err := copyDownloadedFiles(runCtx, files, sourceID, tempDir, destinationPath)
	if err != nil {
		return commandOutput{}, err
	}
	if len(downloaded) == 0 {
		return commandOutput{}, appError{"download_failed", "yt-dlp did not produce any files"}
	}

	return commandOutput{
		Text:            fmt.Sprintf("Downloaded %d file(s) to %s.", len(downloaded), destinationPath),
		SourceID:        sourceID,
		DestinationPath: destinationPath,
		Files:           downloaded,
	}, nil
}

func ytDLPArgs(cfg appConfig, body commandInput, tempDir string, url string) []string {
	args := []string{
		"--no-progress",
		"--newline",
		"--paths", tempDir,
		"--format", firstNonBlank(cfg.Format, defaultFormat),
		"--output", outputTemplate(cfg, body),
	}
	if boolValue(cfg.WriteSubtitles, false) {
		args = append(args, "--write-subs")
	}
	if boolValue(cfg.WriteAutoSubtitles, false) {
		args = append(args, "--write-auto-subs")
	}
	if boolValue(cfg.WriteSubtitles, false) || boolValue(cfg.WriteAutoSubtitles, false) {
		args = append(args, "--sub-langs", firstNonBlank(cfg.SubtitleLanguages, defaultSubLangs))
		args = append(args, "--sub-format", firstNonBlank(cfg.SubtitleFormat, defaultSubFormat))
	}
	if boolValue(cfg.WriteThumbnail, false) {
		args = append(args, "--write-thumbnail")
	}
	if boolValue(cfg.WriteInfoJSON, false) {
		args = append(args, "--write-info-json")
	}
	if !boolValue(cfg.DownloadPlaylist, false) {
		args = append(args, "--no-playlist")
	}
	if boolValue(cfg.RestrictFilenames, true) {
		args = append(args, "--restrict-filenames")
	}
	if boolValue(cfg.NoOverwrite, true) {
		args = append(args, "--no-overwrites")
	}
	if rateLimit := strings.TrimSpace(cfg.RateLimit); rateLimit != "" {
		args = append(args, "--limit-rate", rateLimit)
	}
	if proxy := strings.TrimSpace(cfg.ProxyURL); proxy != "" {
		args = append(args, "--proxy", proxy)
	}
	if cookies := strings.TrimSpace(cfg.CookiesFilePath); cookies != "" {
		args = append(args, "--cookies", cookies)
	}
	return append(args, url)
}

func outputTemplate(cfg appConfig, body commandInput) string {
	template := firstNonBlank(cfg.OutputTemplate, defaultOutputTemplate)
	if prefix := cleanFilenamePrefix(body.FilenamePrefix); prefix != "" {
		return prefix + " - " + template
	}
	return template
}

func copyDownloadedFiles(ctx context.Context, files *agentfiles.Service, sourceID string, tempDir string, destinationPath string) ([]downloadedFile, error) {
	var downloaded []downloadedFile
	err := filepath.WalkDir(tempDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() == 0 {
			return nil
		}
		rel, err := filepath.Rel(tempDir, path)
		if err != nil {
			return err
		}
		targetPath := filepath.ToSlash(filepath.Join(destinationPath, filepath.ToSlash(rel)))
		if err := uploadFile(ctx, files, sourceID, path, targetPath); err != nil {
			return err
		}
		downloaded = append(downloaded, downloadedFile{Path: targetPath, Size: info.Size()})
		return nil
	})
	if err != nil {
		return nil, appError{"file_error", "failed to store downloaded file"}
	}
	return downloaded, nil
}

func uploadFile(ctx context.Context, files *agentfiles.Service, sourceID string, sourcePath string, targetPath string) error {
	reader, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer reader.Close()

	writer, _, err := files.OpenWriterSource(ctx, sourceID, targetPath, 0)
	if err != nil {
		return err
	}
	defer writer.Close()
	if _, err := io.Copy(writer, reader); err != nil {
		return err
	}
	return nil
}

func cleanRelativePath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "YouTube"
	}
	cleaned := filepath.ToSlash(filepath.Clean("/" + value))
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "." || cleaned == "" {
		return "YouTube"
	}
	return cleaned
}

func cleanFilenamePrefix(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "/", "-")
	value = strings.ReplaceAll(value, "\\", "-")
	return strings.TrimSpace(value)
}

func boolValue(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func decodeRaw[T any](raw json.RawMessage, out *T) error {
	if len(raw) == 0 {
		raw = []byte(`{}`)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(out)
}

func agentSMBShares(shares []config.SMB) []agentfiles.SMBConfig {
	configs := make([]agentfiles.SMBConfig, 0, len(shares))
	for _, share := range shares {
		configs = append(configs, agentfiles.SMBConfig{
			ID:       share.ID,
			Name:     share.Name,
			Host:     share.Host,
			Share:    share.Share,
			Username: share.Username,
			Password: share.Password,
			Domain:   share.Domain,
			Policy: agentfiles.AccessPolicy{
				Read:            share.Policy.Read,
				Write:           share.Policy.Write,
				Delete:          share.Policy.Delete,
				AllowedPrefixes: append([]string(nil), share.Policy.AllowedPrefixes...),
				BlockedPrefixes: append([]string(nil), share.Policy.BlockedPrefixes...),
				MaxUploadBytes:  share.Policy.MaxUploadBytes,
			},
		})
	}
	return configs
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func sanitizeYTDLPError(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "yt-dlp download failed"
	}
	lines := strings.Split(value, "\n")
	last := strings.TrimSpace(lines[len(lines)-1])
	if last == "" {
		return "yt-dlp download failed"
	}
	if len(last) > 300 {
		last = last[:300]
	}
	return last
}

type boundedBuffer struct {
	buf bytes.Buffer
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	originalLen := len(p)
	if b.buf.Len() >= maxStderrBytes {
		return originalLen, nil
	}
	remaining := maxStderrBytes - b.buf.Len()
	if len(p) > remaining {
		p = p[:remaining]
	}
	_, _ = b.buf.Write(p)
	return originalLen, nil
}

func (b *boundedBuffer) String() string {
	return b.buf.String()
}

func writeFailure(output io.Writer, stderr io.Writer, requestID string, code string, message string) int {
	return writeResponse(output, stderr, apps.AppStdioResponse{
		RequestID: requestID,
		OK:        false,
		Error: &apps.AppError{
			Code:    code,
			Message: message,
		},
	}, 1)
}

func writeResponse(output io.Writer, stderr io.Writer, response apps.AppStdioResponse, code int) int {
	encoder := json.NewEncoder(output)
	if err := encoder.Encode(response); err != nil {
		_, _ = fmt.Fprintln(stderr, "failed to write app response")
		return 1
	}
	if code != 0 && response.Error != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", response.Error.Code, response.Error.Message)
	}
	return code
}

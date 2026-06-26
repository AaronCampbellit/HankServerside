package files

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/dropfile/hankremote/internal/protocol"
)

var ErrDisabled = errors.New("files root is not configured")

const (
	LocalSourceID      = "local"
	DefaultSMBSourceID = "smb"

	fileSourceTypeLocal = "local"
	fileSourceTypeSMB   = "smb"
)

type ReadHandle interface {
	io.Reader
	io.Closer
}

type WriteHandle interface {
	io.Writer
	io.Closer
}

type RandomWriteHandle interface {
	io.WriterAt
	io.Closer
	Truncate(size int64) error
}

type SMBConfig struct {
	ID       string
	Name     string
	Host     string
	Share    string
	Username string
	Password string
	Domain   string
	Policy   AccessPolicy
}

func (c SMBConfig) Enabled() bool {
	return strings.TrimSpace(c.Host) != "" && strings.TrimSpace(c.Share) != ""
}

type Config struct {
	Root   string
	Shares []SMBConfig
	Policy AccessPolicy
}

type AccessPolicy struct {
	Read            *bool    `json:"read,omitempty"`
	Write           *bool    `json:"write,omitempty"`
	Delete          *bool    `json:"delete,omitempty"`
	AllowedPrefixes []string `json:"allowed_prefixes,omitempty"`
	BlockedPrefixes []string `json:"blocked_prefixes,omitempty"`
	MaxUploadBytes  int64    `json:"max_upload_bytes,omitempty"`
}

func (p AccessPolicy) HasRules() bool {
	return p.Read != nil || p.Write != nil || p.Delete != nil || len(p.AllowedPrefixes) > 0 || len(p.BlockedPrefixes) > 0 || p.MaxUploadBytes > 0
}

type SourceSnapshot struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Type             string `json:"type"`
	Root             string `json:"root,omitempty"`
	SMBHost          string `json:"smb_host,omitempty"`
	SMBShare         string `json:"smb_share,omitempty"`
	SMBUsername      string `json:"smb_username,omitempty"`
	SMBDomain        string `json:"smb_domain,omitempty"`
	SMBEnabled       bool   `json:"smb_enabled,omitempty"`
	SMBPasswordSet   bool   `json:"smb_password_set,omitempty"`
	LocalRootEnabled bool   `json:"local_root_enabled,omitempty"`
}

type smbShareSnapshot struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Host        string `json:"host"`
	Share       string `json:"share"`
	Username    string `json:"username,omitempty"`
	Domain      string `json:"domain,omitempty"`
	Enabled     bool   `json:"enabled"`
	PasswordSet bool   `json:"password_set"`
}

type Service struct {
	mu             sync.RWMutex
	root           string
	localPolicy    AccessPolicy
	smbShares      []SMBConfig
	smbConnections map[string]*smbConnection
}

type fileSourceSelection struct {
	ID     string
	Type   string
	Policy AccessPolicy
}

func New(root string) *Service {
	return NewWithConfig(Config{Root: root})
}

func NewWithConfig(cfg Config) *Service {
	return &Service{
		root:           strings.TrimSpace(cfg.Root),
		localPolicy:    cfg.Policy,
		smbShares:      normalizeSMBConfigs(cfg.Shares),
		smbConnections: make(map[string]*smbConnection),
	}
}

func (s *Service) Enabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.root != "" || hasEnabledSMBConfig(s.smbShares)
}

func (s *Service) defaultSourceLocked() (fileSourceSelection, error) {
	for _, cfg := range s.smbShares {
		if cfg.Enabled() {
			return fileSourceSelection{ID: cfg.ID, Type: fileSourceTypeSMB, Policy: cfg.Policy}, nil
		}
	}
	if s.root != "" {
		return fileSourceSelection{ID: LocalSourceID, Type: fileSourceTypeLocal, Policy: s.localPolicy}, nil
	}
	return fileSourceSelection{}, ErrDisabled
}

func (s *Service) sourceForID(sourceID string) (fileSourceSelection, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sourceID = cleanSourceID(sourceID)
	if sourceID == "" {
		return s.defaultSourceLocked()
	}
	if sourceID == LocalSourceID {
		if s.root == "" {
			return fileSourceSelection{}, ErrDisabled
		}
		return fileSourceSelection{ID: LocalSourceID, Type: fileSourceTypeLocal, Policy: s.localPolicy}, nil
	}
	for _, cfg := range s.smbShares {
		if cfg.ID == sourceID {
			if !cfg.Enabled() {
				return fileSourceSelection{}, ErrDisabled
			}
			return fileSourceSelection{ID: sourceID, Type: fileSourceTypeSMB, Policy: cfg.Policy}, nil
		}
	}
	return fileSourceSelection{}, fmt.Errorf("file source %q is not configured", sourceID)
}

func (s *Service) ApplySMBConfig(cfg SMBConfig) {
	s.ApplySMBConfigs([]SMBConfig{cfg})
}

func (s *Service) ApplySMBConfigs(configs []SMBConfig) {
	next := normalizeSMBConfigs(configs)

	s.mu.Lock()
	defer s.mu.Unlock()

	existingPasswords := make(map[string]string, len(s.smbShares))
	for _, cfg := range s.smbShares {
		existingPasswords[cfg.ID] = cfg.Password
	}
	for i := range next {
		if next[i].Password == "" {
			next[i].Password = existingPasswords[next[i].ID]
		}
	}

	keep := make(map[string]SMBConfig, len(next))
	for _, cfg := range next {
		keep[cfg.ID] = cfg
	}
	for sourceID, conn := range s.smbConnections {
		cfg, ok := keep[sourceID]
		if !ok || !sameSMBConfig(conn.cfg, cfg) {
			_ = conn.close()
			delete(s.smbConnections, sourceID)
		}
	}
	s.smbShares = next
}

func (s *Service) SMBConfigs() []SMBConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneSMBConfigs(s.smbShares)
}

func normalizeSMBConfigs(configs []SMBConfig) []SMBConfig {
	normalized := make([]SMBConfig, 0, len(configs))
	seen := map[string]int{}
	for index, cfg := range configs {
		cfg.Host = NormalizeSMBHost(cfg.Host)
		cfg.Share = strings.TrimSpace(cfg.Share)
		cfg.Username = strings.TrimSpace(cfg.Username)
		cfg.Domain = strings.TrimSpace(cfg.Domain)
		cfg.Name = strings.TrimSpace(cfg.Name)

		fallbackID := DefaultSMBSourceID
		if index > 0 {
			fallbackID = fmt.Sprintf("%s-%d", DefaultSMBSourceID, index+1)
		}
		if cfg.ID == "" {
			cfg.ID = firstNonBlank(cfg.Name, cfg.Share, fallbackID)
		}
		cfg.ID = cleanSourceID(cfg.ID)
		if cfg.ID == "" {
			cfg.ID = fallbackID
		}
		baseID := cfg.ID
		if count := seen[baseID]; count > 0 {
			cfg.ID = fmt.Sprintf("%s-%d", baseID, count+1)
		}
		seen[baseID]++
		if cfg.Name == "" {
			cfg.Name = cfg.Share
		}
		if cfg.Name == "" {
			cfg.Name = cfg.ID
		}
		normalized = append(normalized, cfg)
	}
	return normalized
}

func hasEnabledSMBConfig(configs []SMBConfig) bool {
	for _, cfg := range configs {
		if cfg.Enabled() {
			return true
		}
	}
	return false
}

func cloneSMBConfigs(configs []SMBConfig) []SMBConfig {
	cloned := make([]SMBConfig, len(configs))
	copy(cloned, configs)
	return cloned
}

func cleanSourceID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	lastDash := false
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z', char >= '0' && char <= '9':
			builder.WriteRune(char)
			lastDash = false
		case char == '_' || char == '-' || char == '.':
			builder.WriteRune(char)
			lastDash = false
		default:
			if !lastDash {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(builder.String(), "-")
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func NormalizeSMBHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	host = strings.ReplaceAll(host, "\\", "/")

	if parsed, err := url.Parse(host); err == nil && parsed.Scheme != "" {
		switch strings.ToLower(parsed.Scheme) {
		case "http", "https":
			return strings.TrimSpace(parsed.Hostname())
		default:
			if parsed.Host != "" {
				return strings.TrimSpace(parsed.Host)
			}
		}
	}

	host = strings.TrimPrefix(host, "smb://")
	host = strings.TrimPrefix(host, "cifs://")
	host = strings.TrimLeft(host, "/")
	if slash := strings.Index(host, "/"); slash >= 0 {
		host = host[:slash]
	}
	return strings.TrimSpace(host)
}

func (s *Service) Snapshot() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sources := s.sourceSnapshotsLocked()
	primary := SMBConfig{}
	if len(s.smbShares) > 0 {
		primary = s.smbShares[0]
	}
	return map[string]any{
		"root":               s.root,
		"smb_host":           primary.Host,
		"smb_share":          primary.Share,
		"smb_username":       primary.Username,
		"smb_domain":         primary.Domain,
		"smb_enabled":        primary.Enabled(),
		"smb_password_set":   primary.Password != "",
		"local_root_enabled": s.root != "",
		"active_source_id":   defaultSourceID(sources),
		"file_sources":       sources,
		"sources":            sources,
		"shares":             s.smbShareSnapshotsLocked(),
	}
}

func (s *Service) sourceSnapshotsLocked() []SourceSnapshot {
	sources := make([]SourceSnapshot, 0, len(s.smbShares)+1)
	for _, cfg := range s.smbShares {
		sources = append(sources, SourceSnapshot{
			ID:             cfg.ID,
			Name:           cfg.Name,
			Type:           fileSourceTypeSMB,
			SMBHost:        cfg.Host,
			SMBShare:       cfg.Share,
			SMBUsername:    cfg.Username,
			SMBDomain:      cfg.Domain,
			SMBEnabled:     cfg.Enabled(),
			SMBPasswordSet: cfg.Password != "",
		})
	}
	if s.root != "" {
		sources = append(sources, SourceSnapshot{
			ID:               LocalSourceID,
			Name:             "Home connector files",
			Type:             fileSourceTypeLocal,
			Root:             s.root,
			LocalRootEnabled: true,
		})
	}
	return sources
}

func (s *Service) smbShareSnapshotsLocked() []smbShareSnapshot {
	shares := make([]smbShareSnapshot, 0, len(s.smbShares))
	for _, cfg := range s.smbShares {
		shares = append(shares, smbShareSnapshot{
			ID:          cfg.ID,
			Name:        cfg.Name,
			Host:        cfg.Host,
			Share:       cfg.Share,
			Username:    cfg.Username,
			Domain:      cfg.Domain,
			Enabled:     cfg.Enabled(),
			PasswordSet: cfg.Password != "",
		})
	}
	return shares
}

func defaultSourceID(sources []SourceSnapshot) string {
	for _, source := range sources {
		if source.Type == fileSourceTypeSMB && source.SMBEnabled {
			return source.ID
		}
	}
	for _, source := range sources {
		if source.Type == fileSourceTypeLocal && source.LocalRootEnabled {
			return source.ID
		}
	}
	return ""
}

func (s fileSourceSelection) authorize(action string, path string, size int64) error {
	return s.Policy.allow(action, path, size)
}

func (p AccessPolicy) allow(action string, rawPath string, size int64) error {
	switch action {
	case "read":
		if p.Read != nil && !*p.Read {
			return errors.New("file source policy denies read")
		}
	case "write":
		if p.Write != nil && !*p.Write {
			return errors.New("file source policy denies write")
		}
		if p.MaxUploadBytes > 0 && size > p.MaxUploadBytes {
			return errors.New("file source policy upload size limit exceeded")
		}
	case "delete":
		if p.Delete != nil && !*p.Delete {
			return errors.New("file source policy denies delete")
		}
	}
	path := cleanPolicyPath(rawPath)
	for _, prefix := range p.BlockedPrefixes {
		if pathHasPolicyPrefix(path, prefix) {
			return errors.New("file source policy blocks this path")
		}
	}
	if len(p.AllowedPrefixes) > 0 {
		for _, prefix := range p.AllowedPrefixes {
			if pathHasPolicyPrefix(path, prefix) {
				return nil
			}
		}
		return errors.New("file source policy does not allow this path")
	}
	return nil
}

func cleanPolicyPath(value string) string {
	value = filepath.ToSlash(filepath.Clean("/" + strings.TrimSpace(value)))
	return strings.TrimSuffix(value, "/")
}

func pathHasPolicyPrefix(path string, prefix string) bool {
	prefix = cleanPolicyPath(prefix)
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

func decodedBase64Size(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	size := base64.StdEncoding.DecodedLen(len(value))
	for strings.HasSuffix(value, "=") && size > 0 {
		size--
		value = strings.TrimSuffix(value, "=")
	}
	return int64(size)
}

func (s *Service) List(ctx context.Context, path string) ([]protocol.FileItem, error) {
	return s.ListSource(ctx, "", path)
}

func (s *Service) ListSource(ctx context.Context, sourceID string, path string) ([]protocol.FileItem, error) {
	source, err := s.sourceForID(sourceID)
	if err != nil {
		return nil, err
	}
	if err := source.authorize("read", path, 0); err != nil {
		return nil, err
	}
	var items []protocol.FileItem
	if source.Type == fileSourceTypeSMB {
		items, err = s.listSMB(ctx, source.ID, path)
	} else {
		items, err = s.listLocal(ctx, path)
	}
	if err != nil {
		return nil, err
	}
	return decorateFileItemsSource(items, source.ID), nil
}

func (s *Service) Stat(ctx context.Context, path string) (protocol.FileItem, error) {
	return s.StatSource(ctx, "", path)
}

func (s *Service) StatSource(ctx context.Context, sourceID string, path string) (protocol.FileItem, error) {
	source, err := s.sourceForID(sourceID)
	if err != nil {
		return protocol.FileItem{}, err
	}
	if err := source.authorize("read", path, 0); err != nil {
		return protocol.FileItem{}, err
	}
	var item protocol.FileItem
	if source.Type == fileSourceTypeSMB {
		item, err = s.statSMB(ctx, source.ID, path)
	} else {
		item, err = s.statLocal(ctx, path)
	}
	if err != nil {
		return protocol.FileItem{}, err
	}
	item.SourceID = source.ID
	return item, nil
}

func (s *Service) Search(ctx context.Context, query string, limit int) ([]protocol.FileItem, error) {
	return s.SearchSource(ctx, "", query, limit)
}

func (s *Service) SearchSource(ctx context.Context, sourceID string, query string, limit int) ([]protocol.FileItem, error) {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	type queueItem struct {
		path  string
		depth int
	}
	queue := []queueItem{{path: "", depth: 0}}
	matches := make([]protocol.FileItem, 0)
	visited := 0
	for len(queue) > 0 && visited < 500 {
		current := queue[0]
		queue = queue[1:]
		visited++
		items, err := s.ListSource(ctx, sourceID, current.path)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if fileSearchScore(item, query) > 0 {
				matches = append(matches, item)
			}
			if item.IsDirectory && current.depth < 5 {
				queue = append(queue, queueItem{path: item.Path, depth: current.depth + 1})
			}
		}
	}

	sort.Slice(matches, func(i int, j int) bool {
		left := fileSearchScore(matches[i], query)
		right := fileSearchScore(matches[j], query)
		if left == right {
			return matches[i].Path < matches[j].Path
		}
		return left > right
	})
	if len(matches) > limit {
		matches = matches[:limit]
	}
	return matches, nil
}

func (s *Service) CreateDirectory(ctx context.Context, path string) error {
	return s.CreateDirectorySource(ctx, "", path)
}

func (s *Service) CreateDirectorySource(ctx context.Context, sourceID string, path string) error {
	source, err := s.sourceForID(sourceID)
	if err != nil {
		return err
	}
	if err := source.authorize("write", path, 0); err != nil {
		return err
	}
	if source.Type == fileSourceTypeSMB {
		return s.createDirectorySMB(ctx, source.ID, path)
	}
	return s.createDirectoryLocal(ctx, path)
}

func (s *Service) Rename(ctx context.Context, from string, to string) error {
	return s.RenameSource(ctx, "", from, to)
}

func (s *Service) RenameSource(ctx context.Context, sourceID string, from string, to string) error {
	source, err := s.sourceForID(sourceID)
	if err != nil {
		return err
	}
	if err := source.authorize("write", from, 0); err != nil {
		return err
	}
	if err := source.authorize("write", to, 0); err != nil {
		return err
	}
	if source.Type == fileSourceTypeSMB {
		return s.renameSMB(ctx, source.ID, from, to)
	}
	return s.renameLocal(ctx, from, to)
}

func (s *Service) MoveBetweenSources(ctx context.Context, sourceID string, destinationSourceID string, from string, to string, isDirectory bool) error {
	_, err := s.MoveBetweenSourcesWithProgress(ctx, sourceID, destinationSourceID, from, to, isDirectory, nil)
	return err
}

func (s *Service) copyBetweenSources(ctx context.Context, sourceID string, destinationSourceID string, from string, to string, isDirectory bool) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if isDirectory {
		if err := s.CreateDirectorySource(ctx, destinationSourceID, to); err != nil {
			return err
		}
		items, err := s.ListSource(ctx, sourceID, from)
		if err != nil {
			return err
		}
		for _, item := range items {
			if err := s.copyBetweenSources(ctx, sourceID, destinationSourceID, item.Path, filepath.ToSlash(filepath.Join(to, item.Name)), item.IsDirectory); err != nil {
				return err
			}
		}
		return nil
	}

	reader, _, err := s.OpenReaderSource(ctx, sourceID, from, 0)
	if err != nil {
		return err
	}
	defer reader.Close()

	writer, _, err := s.OpenWriterSource(ctx, destinationSourceID, to, 0)
	if err != nil {
		return err
	}
	if _, err := io.Copy(writer, reader); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return s.verifyCopiedFile(ctx, sourceID, destinationSourceID, from, to)
}

func (s *Service) verifyCopiedFile(ctx context.Context, sourceID string, destinationSourceID string, from string, to string) error {
	sourceInfo, err := s.StatSource(ctx, sourceID, from)
	if err != nil {
		return err
	}
	destinationInfo, err := s.StatSource(ctx, destinationSourceID, to)
	if err != nil {
		return err
	}
	if sourceInfo.IsDirectory || destinationInfo.IsDirectory {
		return fmt.Errorf("copy verification expected files")
	}
	if sourceInfo.Size != destinationInfo.Size {
		return fmt.Errorf("copy verification failed: size mismatch")
	}
	sourceHash, err := s.fileHash(ctx, sourceID, from)
	if err != nil {
		return err
	}
	destinationHash, err := s.fileHash(ctx, destinationSourceID, to)
	if err != nil {
		return err
	}
	if sourceHash != destinationHash {
		return fmt.Errorf("copy verification failed: checksum mismatch")
	}
	return nil
}

func (s *Service) fileHash(ctx context.Context, sourceID string, filePath string) (string, error) {
	reader, _, err := s.OpenReaderSource(ctx, sourceID, filePath, 0)
	if err != nil {
		return "", err
	}
	defer reader.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, reader); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func (s *Service) Delete(ctx context.Context, path string, isDirectory bool) error {
	return s.DeleteSource(ctx, "", path, isDirectory)
}

func (s *Service) DeleteSource(ctx context.Context, sourceID string, path string, isDirectory bool) error {
	source, err := s.sourceForID(sourceID)
	if err != nil {
		return err
	}
	if err := source.authorize("delete", path, 0); err != nil {
		return err
	}
	if source.Type == fileSourceTypeSMB {
		return s.deleteSMB(ctx, source.ID, path, isDirectory)
	}
	return s.deleteLocal(ctx, path, isDirectory)
}

func (s *Service) RollbackMoveDestination(ctx context.Context, destinationSourceID string, to string, isDirectory bool) error {
	to = strings.TrimSpace(to)
	if to == "" {
		return fmt.Errorf("rollback destination path is required")
	}
	if _, err := s.StatSource(ctx, destinationSourceID, to); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return s.DeleteSource(ctx, destinationSourceID, to, isDirectory)
}

func (s *Service) Download(ctx context.Context, path string) (string, error) {
	return s.DownloadSource(ctx, "", path)
}

func (s *Service) DownloadSource(ctx context.Context, sourceID string, path string) (string, error) {
	source, err := s.sourceForID(sourceID)
	if err != nil {
		return "", err
	}
	if err := source.authorize("read", path, 0); err != nil {
		return "", err
	}
	if source.Type == fileSourceTypeSMB {
		return s.downloadSMB(ctx, source.ID, path)
	}
	return s.downloadLocal(ctx, path)
}

func (s *Service) Upload(ctx context.Context, path string, contentBase64 string) error {
	return s.UploadSource(ctx, "", path, contentBase64)
}

func (s *Service) UploadSource(ctx context.Context, sourceID string, path string, contentBase64 string) error {
	source, err := s.sourceForID(sourceID)
	if err != nil {
		return err
	}
	if err := source.authorize("write", path, decodedBase64Size(contentBase64)); err != nil {
		return err
	}
	if source.Type == fileSourceTypeSMB {
		return s.uploadSMB(ctx, source.ID, path, contentBase64)
	}
	return s.uploadLocal(ctx, path, contentBase64)
}

func (s *Service) OpenReader(ctx context.Context, path string, offset int64) (ReadHandle, fs.FileInfo, error) {
	return s.OpenReaderSource(ctx, "", path, offset)
}

func (s *Service) OpenReaderSource(ctx context.Context, sourceID string, path string, offset int64) (ReadHandle, fs.FileInfo, error) {
	source, err := s.sourceForID(sourceID)
	if err != nil {
		return nil, nil, err
	}
	if err := source.authorize("read", path, 0); err != nil {
		return nil, nil, err
	}
	if source.Type == fileSourceTypeSMB {
		return s.openReaderSMB(ctx, source.ID, path, offset)
	}
	return s.openReaderLocal(ctx, path, offset)
}

func (s *Service) OpenWriter(ctx context.Context, path string, offset int64) (WriteHandle, int64, error) {
	return s.OpenWriterSource(ctx, "", path, offset)
}

func (s *Service) OpenWriterSource(ctx context.Context, sourceID string, path string, offset int64) (WriteHandle, int64, error) {
	source, err := s.sourceForID(sourceID)
	if err != nil {
		return nil, 0, err
	}
	if err := source.authorize("write", path, offset); err != nil {
		return nil, 0, err
	}
	maxBytes := source.Policy.MaxUploadBytes
	if source.Type == fileSourceTypeSMB {
		writer, size, err := s.openWriterSMB(ctx, source.ID, path, offset)
		if err != nil {
			return nil, 0, err
		}
		return limitWriteHandle(writer, maxBytes, offset), size, nil
	}
	writer, size, err := s.openWriterLocal(ctx, path, offset)
	if err != nil {
		return nil, 0, err
	}
	return limitWriteHandle(writer, maxBytes, offset), size, nil
}

func (s *Service) OpenRandomWriter(ctx context.Context, path string) (RandomWriteHandle, error) {
	return s.OpenRandomWriterSource(ctx, "", path)
}

func (s *Service) OpenRandomWriterSource(ctx context.Context, sourceID string, path string) (RandomWriteHandle, error) {
	source, err := s.sourceForID(sourceID)
	if err != nil {
		return nil, err
	}
	if err := source.authorize("write", path, 0); err != nil {
		return nil, err
	}
	maxBytes := source.Policy.MaxUploadBytes
	if source.Type == fileSourceTypeSMB {
		writer, err := s.openRandomWriterSMB(ctx, source.ID, path)
		if err != nil {
			return nil, err
		}
		return limitRandomWriteHandle(writer, maxBytes), nil
	}
	writer, err := s.openRandomWriterLocal(ctx, path)
	if err != nil {
		return nil, err
	}
	return limitRandomWriteHandle(writer, maxBytes), nil
}

func (s *Service) listLocal(ctx context.Context, path string) ([]protocol.FileItem, error) {
	resolved, err := s.resolveLocal(path)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(resolved)
	if err != nil {
		return nil, err
	}

	items := make([]protocol.FileItem, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		relative := filepath.Join(cleanPath(path), entry.Name())
		items = append(items, fileItem(relative, info))
	}

	sortItems(items)
	return items, nil
}

func (s *Service) statLocal(_ context.Context, path string) (protocol.FileItem, error) {
	resolved, err := s.resolveLocal(path)
	if err != nil {
		return protocol.FileItem{}, err
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return protocol.FileItem{}, err
	}

	return fileItem(cleanPath(path), info), nil
}

func (s *Service) createDirectoryLocal(_ context.Context, path string) error {
	resolved, err := s.resolveLocalForWrite(path)
	if err != nil {
		return err
	}
	return os.MkdirAll(resolved, 0o755)
}

func (s *Service) renameLocal(_ context.Context, from string, to string) error {
	resolvedFrom, err := s.resolveLocal(from)
	if err != nil {
		return err
	}
	resolvedTo, err := s.resolveLocalForWrite(to)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(resolvedTo), 0o755); err != nil {
		return err
	}
	return os.Rename(resolvedFrom, resolvedTo)
}

func (s *Service) deleteLocal(_ context.Context, path string, isDirectory bool) error {
	resolved, err := s.resolveLocal(path)
	if err != nil {
		return err
	}
	if isDirectory {
		return os.RemoveAll(resolved)
	}
	return os.Remove(resolved)
}

func (s *Service) downloadLocal(_ context.Context, path string) (string, error) {
	resolved, err := s.resolveLocal(path)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(data), nil
}

func (s *Service) uploadLocal(_ context.Context, path string, contentBase64 string) error {
	resolved, err := s.resolveLocalForWrite(path)
	if err != nil {
		return err
	}

	data, err := base64.StdEncoding.DecodeString(contentBase64)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return err
	}

	return os.WriteFile(resolved, data, 0o644)
}

func (s *Service) openReaderLocal(_ context.Context, path string, offset int64) (ReadHandle, fs.FileInfo, error) {
	resolved, err := s.resolveLocal(path)
	if err != nil {
		return nil, nil, err
	}

	file, err := os.Open(resolved)
	if err != nil {
		return nil, nil, err
	}

	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	if info.IsDir() {
		_ = file.Close()
		return nil, nil, fmt.Errorf("path is a directory")
	}
	if offset < 0 || offset > info.Size() {
		_ = file.Close()
		return nil, nil, fmt.Errorf("invalid read offset")
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		_ = file.Close()
		return nil, nil, err
	}

	return file, info, nil
}

func (s *Service) openWriterLocal(_ context.Context, path string, offset int64) (WriteHandle, int64, error) {
	resolved, err := s.resolveLocalForWrite(path)
	if err != nil {
		return nil, 0, err
	}

	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return nil, 0, err
	}

	flags := os.O_CREATE | os.O_WRONLY
	if offset == 0 {
		flags |= os.O_TRUNC
	}
	file, err := os.OpenFile(resolved, flags, 0o644)
	if err != nil {
		return nil, 0, err
	}
	if offset == 0 {
		return file, 0, nil
	}

	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, 0, err
	}

	size := info.Size()
	if offset < 0 || offset > size {
		_ = file.Close()
		return nil, 0, fmt.Errorf("invalid write offset")
	}
	if size > offset {
		if err := file.Truncate(offset); err != nil {
			_ = file.Close()
			return nil, 0, err
		}
		size = offset
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		_ = file.Close()
		return nil, 0, err
	}

	return file, size, nil
}

func (s *Service) openRandomWriterLocal(_ context.Context, path string) (RandomWriteHandle, error) {
	resolved, err := s.resolveLocalForWrite(path)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return nil, err
	}

	return os.OpenFile(resolved, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o644)
}

type maxWriteHandle struct {
	inner   WriteHandle
	max     int64
	current int64
}

func limitWriteHandle(inner WriteHandle, max int64, current int64) WriteHandle {
	if inner == nil || max <= 0 {
		return inner
	}
	return &maxWriteHandle{inner: inner, max: max, current: current}
}

func (w *maxWriteHandle) Write(data []byte) (int, error) {
	if w.max > 0 && w.current+int64(len(data)) > w.max {
		return 0, errors.New("file source policy upload size limit exceeded")
	}
	n, err := w.inner.Write(data)
	w.current += int64(n)
	return n, err
}

func (w *maxWriteHandle) Close() error {
	return w.inner.Close()
}

type maxRandomWriteHandle struct {
	inner RandomWriteHandle
	max   int64
}

func limitRandomWriteHandle(inner RandomWriteHandle, max int64) RandomWriteHandle {
	if inner == nil || max <= 0 {
		return inner
	}
	return &maxRandomWriteHandle{inner: inner, max: max}
}

func (w *maxRandomWriteHandle) WriteAt(data []byte, offset int64) (int, error) {
	if w.max > 0 && offset+int64(len(data)) > w.max {
		return 0, errors.New("file source policy upload size limit exceeded")
	}
	return w.inner.WriteAt(data, offset)
}

func (w *maxRandomWriteHandle) Truncate(size int64) error {
	if w.max > 0 && size > w.max {
		return errors.New("file source policy upload size limit exceeded")
	}
	return w.inner.Truncate(size)
}

func (w *maxRandomWriteHandle) Close() error {
	return w.inner.Close()
}

func (s *Service) resolveLocal(path string) (string, error) {
	if !s.Enabled() {
		return "", ErrDisabled
	}
	if s.root == "" {
		return "", ErrDisabled
	}

	cleaned := cleanPath(path)
	joined := filepath.Join(s.root, filepath.FromSlash(cleaned))
	resolved := filepath.Clean(joined)
	root, err := filepath.EvalSymlinks(filepath.Clean(s.root))
	if err != nil {
		return "", err
	}
	if real, err := filepath.EvalSymlinks(resolved); err == nil {
		resolved = real
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	} else {
		return "", err
	}

	if resolved != root && !strings.HasPrefix(resolved, root+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes root")
	}

	return resolved, nil
}

func (s *Service) resolveLocalForWrite(path string) (string, error) {
	resolved, err := s.resolveLocal(path)
	if err == nil {
		return resolved, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	cleaned := cleanPath(path)
	joined := filepath.Join(s.root, filepath.FromSlash(cleaned))
	root, err := filepath.EvalSymlinks(filepath.Clean(s.root))
	if err != nil {
		return "", err
	}
	parent := filepath.Dir(filepath.Clean(joined))
	existingParent := parent
	missing := []string{filepath.Base(joined)}
	for {
		if realParent, err := filepath.EvalSymlinks(existingParent); err == nil {
			if realParent != root && !strings.HasPrefix(realParent, root+string(filepath.Separator)) {
				return "", fmt.Errorf("path escapes root")
			}
			for i := len(missing) - 1; i >= 0; i-- {
				realParent = filepath.Join(realParent, missing[i])
			}
			return realParent, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		if filepath.Clean(existingParent) == root || filepath.Dir(existingParent) == existingParent {
			return "", err
		}
		missing = append(missing, filepath.Base(existingParent))
		existingParent = filepath.Dir(existingParent)
	}
}

func resolveSharePath(path string) string {
	cleaned := cleanPath(path)
	if cleaned == "" {
		return "."
	}
	return filepath.ToSlash(cleaned)
}

func cleanPath(path string) string {
	cleaned := filepath.ToSlash(filepath.Clean("/" + strings.TrimSpace(path)))
	return strings.TrimPrefix(cleaned, "/")
}

func sortItems(items []protocol.FileItem) {
	sort.Slice(items, func(i int, j int) bool {
		if items[i].IsDirectory != items[j].IsDirectory {
			return items[i].IsDirectory
		}
		return items[i].Name < items[j].Name
	})
}

func decorateFileItemsSource(items []protocol.FileItem, sourceID string) []protocol.FileItem {
	for i := range items {
		items[i].SourceID = sourceID
	}
	return items
}

func fileSearchScore(item protocol.FileItem, query string) int {
	query = strings.ToLower(strings.TrimSpace(query))
	name := strings.ToLower(item.Name)
	path := strings.ToLower(item.Path)
	score := 0
	if strings.Contains(name, query) {
		score += 8
	}
	if strings.Contains(path, query) {
		score += 5
	}
	for _, token := range strings.Fields(query) {
		if strings.Contains(name, token) {
			score += 3
		}
		if strings.Contains(path, token) {
			score++
		}
	}
	if score > 0 && item.IsDirectory {
		score++
	}
	return score
}

func fileItem(path string, info fs.FileInfo) protocol.FileItem {
	return protocol.FileItem{
		Path:        filepath.ToSlash(path),
		Name:        info.Name(),
		IsDirectory: info.IsDir(),
		Size:        info.Size(),
		ModifiedAt:  info.ModTime().UTC(),
	}
}

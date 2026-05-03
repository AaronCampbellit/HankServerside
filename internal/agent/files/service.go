package files

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/dropfile/hankremote/internal/protocol"
)

var ErrDisabled = errors.New("files root is not configured")

type ReadHandle interface {
	io.Reader
	io.Closer
}

type WriteHandle interface {
	io.Writer
	io.Closer
}

type SMBConfig struct {
	Host     string
	Share    string
	Username string
	Password string
	Domain   string
}

func (c SMBConfig) Enabled() bool {
	return strings.TrimSpace(c.Host) != "" && strings.TrimSpace(c.Share) != ""
}

type Config struct {
	Root string
	SMB  SMBConfig
}

type Service struct {
	mu   sync.RWMutex
	root string
	smb  SMBConfig
}

func New(root string) *Service {
	return NewWithConfig(Config{Root: root})
}

func NewWithConfig(cfg Config) *Service {
	return &Service{
		root: strings.TrimSpace(cfg.Root),
		smb: SMBConfig{
			Host:     strings.TrimSpace(cfg.SMB.Host),
			Share:    strings.TrimSpace(cfg.SMB.Share),
			Username: strings.TrimSpace(cfg.SMB.Username),
			Password: cfg.SMB.Password,
			Domain:   strings.TrimSpace(cfg.SMB.Domain),
		},
	}
}

func (s *Service) Enabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.root != "" || s.smb.Enabled()
}

func (s *Service) usingSMB() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.smb.Enabled()
}

func (s *Service) ApplySMBConfig(cfg SMBConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.smb = SMBConfig{
		Host:     strings.TrimSpace(cfg.Host),
		Share:    strings.TrimSpace(cfg.Share),
		Username: strings.TrimSpace(cfg.Username),
		Password: cfg.Password,
		Domain:   strings.TrimSpace(cfg.Domain),
	}
}

func (s *Service) Snapshot() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return map[string]any{
		"root":               s.root,
		"smb_host":           s.smb.Host,
		"smb_share":          s.smb.Share,
		"smb_username":       s.smb.Username,
		"smb_domain":         s.smb.Domain,
		"smb_enabled":        s.smb.Enabled(),
		"smb_password_set":   s.smb.Password != "",
		"local_root_enabled": s.root != "",
	}
}

func (s *Service) List(ctx context.Context, path string) ([]protocol.FileItem, error) {
	if s.usingSMB() {
		return s.listSMB(ctx, path)
	}
	return s.listLocal(ctx, path)
}

func (s *Service) Stat(ctx context.Context, path string) (protocol.FileItem, error) {
	if s.usingSMB() {
		return s.statSMB(ctx, path)
	}
	return s.statLocal(ctx, path)
}

func (s *Service) Search(ctx context.Context, query string, limit int) ([]protocol.FileItem, error) {
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
		items, err := s.List(ctx, current.path)
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
	if s.usingSMB() {
		return s.createDirectorySMB(ctx, path)
	}
	return s.createDirectoryLocal(ctx, path)
}

func (s *Service) Rename(ctx context.Context, from string, to string) error {
	if s.usingSMB() {
		return s.renameSMB(ctx, from, to)
	}
	return s.renameLocal(ctx, from, to)
}

func (s *Service) Delete(ctx context.Context, path string, isDirectory bool) error {
	if s.usingSMB() {
		return s.deleteSMB(ctx, path, isDirectory)
	}
	return s.deleteLocal(ctx, path, isDirectory)
}

func (s *Service) Download(ctx context.Context, path string) (string, error) {
	if s.usingSMB() {
		return s.downloadSMB(ctx, path)
	}
	return s.downloadLocal(ctx, path)
}

func (s *Service) Upload(ctx context.Context, path string, contentBase64 string) error {
	if s.usingSMB() {
		return s.uploadSMB(ctx, path, contentBase64)
	}
	return s.uploadLocal(ctx, path, contentBase64)
}

func (s *Service) OpenReader(ctx context.Context, path string, offset int64) (ReadHandle, fs.FileInfo, error) {
	if s.usingSMB() {
		return s.openReaderSMB(ctx, path, offset)
	}
	return s.openReaderLocal(ctx, path, offset)
}

func (s *Service) OpenWriter(ctx context.Context, path string, offset int64) (WriteHandle, int64, error) {
	if s.usingSMB() {
		return s.openWriterSMB(ctx, path, offset)
	}
	return s.openWriterLocal(ctx, path, offset)
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
	resolved, err := s.resolveLocal(path)
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
	resolvedTo, err := s.resolveLocal(to)
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
	resolved, err := s.resolveLocal(path)
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
	resolved, err := s.resolveLocal(path)
	if err != nil {
		return nil, 0, err
	}

	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return nil, 0, err
	}

	file, err := os.OpenFile(resolved, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, 0, err
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
	root := filepath.Clean(s.root)

	if resolved != root && !strings.HasPrefix(resolved, root+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes root")
	}

	return resolved, nil
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
	if item.IsDirectory {
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

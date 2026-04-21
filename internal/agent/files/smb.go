package files

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path"
	"strings"
	"time"

	"github.com/hirochachacha/go-smb2"

	"github.com/dropfile/hankremote/internal/protocol"
)

const smbDialTimeout = 15 * time.Second

func (s *Service) listSMB(ctx context.Context, filePath string) ([]protocol.FileItem, error) {
	share, cleanup, err := s.dialSMBShare(ctx)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	entries, err := share.ReadDir(resolveSharePath(filePath))
	if err != nil {
		return nil, err
	}

	items := make([]protocol.FileItem, 0, len(entries))
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		relative := path.Join(cleanPath(filePath), entry.Name())
		items = append(items, fileItem(relative, entry))
	}

	sortItems(items)
	return items, nil
}

func (s *Service) statSMB(ctx context.Context, filePath string) (protocol.FileItem, error) {
	share, cleanup, err := s.dialSMBShare(ctx)
	if err != nil {
		return protocol.FileItem{}, err
	}
	defer cleanup()

	info, err := share.Stat(resolveSharePath(filePath))
	if err != nil {
		return protocol.FileItem{}, err
	}
	return fileItem(cleanPath(filePath), info), nil
}

func (s *Service) createDirectorySMB(ctx context.Context, filePath string) error {
	share, cleanup, err := s.dialSMBShare(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	return share.MkdirAll(resolveSharePath(filePath), 0o755)
}

func (s *Service) renameSMB(ctx context.Context, from string, to string) error {
	share, cleanup, err := s.dialSMBShare(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	destination := resolveSharePath(to)
	if dir := path.Dir(destination); dir != "." {
		if err := share.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	return share.Rename(resolveSharePath(from), destination)
}

func (s *Service) deleteSMB(ctx context.Context, filePath string, isDirectory bool) error {
	share, cleanup, err := s.dialSMBShare(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	if isDirectory {
		return share.RemoveAll(resolveSharePath(filePath))
	}
	return share.Remove(resolveSharePath(filePath))
}

func (s *Service) downloadSMB(ctx context.Context, filePath string) (string, error) {
	share, cleanup, err := s.dialSMBShare(ctx)
	if err != nil {
		return "", err
	}
	defer cleanup()

	data, err := share.ReadFile(resolveSharePath(filePath))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func (s *Service) uploadSMB(ctx context.Context, filePath string, contentBase64 string) error {
	share, cleanup, err := s.dialSMBShare(ctx)
	if err != nil {
		return err
	}
	defer cleanup()

	data, err := base64.StdEncoding.DecodeString(contentBase64)
	if err != nil {
		return err
	}

	if dir := path.Dir(resolveSharePath(filePath)); dir != "." {
		if err := share.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	return share.WriteFile(resolveSharePath(filePath), data, 0o644)
}

func (s *Service) openReaderSMB(ctx context.Context, filePath string, offset int64) (ReadHandle, fs.FileInfo, error) {
	share, cleanup, err := s.dialSMBShare(ctx)
	if err != nil {
		return nil, nil, err
	}

	file, err := share.Open(resolveSharePath(filePath))
	if err != nil {
		_ = cleanup()
		return nil, nil, err
	}

	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		_ = cleanup()
		return nil, nil, err
	}
	if info.IsDir() {
		_ = file.Close()
		_ = cleanup()
		return nil, nil, fmt.Errorf("path is a directory")
	}
	if offset < 0 || offset > info.Size() {
		_ = file.Close()
		_ = cleanup()
		return nil, nil, fmt.Errorf("invalid read offset")
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		_ = file.Close()
		_ = cleanup()
		return nil, nil, err
	}

	return &smbReadHandle{file: file, cleanup: cleanup}, info, nil
}

func (s *Service) openWriterSMB(ctx context.Context, filePath string, offset int64) (WriteHandle, int64, error) {
	share, cleanup, err := s.dialSMBShare(ctx)
	if err != nil {
		return nil, 0, err
	}

	resolved := resolveSharePath(filePath)
	if dir := path.Dir(resolved); dir != "." {
		if err := share.MkdirAll(dir, 0o755); err != nil {
			_ = cleanup()
			return nil, 0, err
		}
	}

	file, err := share.OpenFile(resolved, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		_ = cleanup()
		return nil, 0, err
	}

	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		_ = cleanup()
		return nil, 0, err
	}

	size := info.Size()
	if offset < 0 || offset > size {
		_ = file.Close()
		_ = cleanup()
		return nil, 0, fmt.Errorf("invalid write offset")
	}
	if size > offset {
		if err := file.Truncate(offset); err != nil {
			_ = file.Close()
			_ = cleanup()
			return nil, 0, err
		}
		size = offset
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		_ = file.Close()
		_ = cleanup()
		return nil, 0, err
	}

	return &smbWriteHandle{file: file, cleanup: cleanup}, size, nil
}

func (s *Service) dialSMBShare(ctx context.Context) (*smb2.Share, func() error, error) {
	s.mu.RLock()
	cfg := s.smb
	s.mu.RUnlock()

	if !cfg.Enabled() {
		return nil, nil, ErrDisabled
	}

	address := smbAddress(cfg.Host)
	conn, err := (&net.Dialer{Timeout: smbDialTimeout}).DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, nil, err
	}

	session, err := (&smb2.Dialer{
		Initiator: &smb2.NTLMInitiator{
			User:     cfg.Username,
			Password: cfg.Password,
			Domain:   cfg.Domain,
		},
	}).DialContext(ctx, conn)
	if err != nil {
		_ = conn.Close()
		return nil, nil, err
	}

	share, err := session.Mount(cfg.Share)
	if err != nil {
		_ = session.Logoff()
		_ = conn.Close()
		return nil, nil, err
	}

	return share.WithContext(ctx), func() error {
		return errors.Join(share.Umount(), session.Logoff(), conn.Close())
	}, nil
}

func smbAddress(host string) string {
	host = strings.TrimSpace(host)
	host = strings.TrimPrefix(host, "smb://")
	host = strings.TrimPrefix(host, "//")
	if _, _, err := net.SplitHostPort(host); err == nil {
		return host
	}
	return net.JoinHostPort(host, "445")
}

type smbReadHandle struct {
	file    *smb2.File
	cleanup func() error
}

func (h *smbReadHandle) Read(p []byte) (int, error) {
	return h.file.Read(p)
}

func (h *smbReadHandle) Close() error {
	return errors.Join(h.file.Close(), h.cleanup())
}

type smbWriteHandle struct {
	file    *smb2.File
	cleanup func() error
}

func (h *smbWriteHandle) Write(p []byte) (int, error) {
	return h.file.Write(p)
}

func (h *smbWriteHandle) Close() error {
	return errors.Join(h.file.Close(), h.cleanup())
}

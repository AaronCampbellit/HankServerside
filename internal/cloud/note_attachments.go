package cloud

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/dropfile/hankremote/internal/domain"
	"github.com/dropfile/hankremote/internal/protocol"
	"github.com/dropfile/hankremote/internal/store"
)

const maxNoteAttachmentBytes = 100 << 20

var unsafeAttachmentFilenameRunes = regexp.MustCompile(`[^A-Za-z0-9._ -]+`)

func splitNoteAttachmentRoute(parts []string) (noteID string, attachmentID string, ok bool) {
	for index := 1; index < len(parts); index++ {
		if parts[index] != "attachments" {
			continue
		}
		noteID = strings.TrimSpace(strings.Join(parts[1:index], "/"))
		if noteID == "" {
			return "", "", false
		}
		if index+1 < len(parts) {
			attachmentID = strings.TrimSpace(parts[index+1])
		}
		return noteID, attachmentID, true
	}
	return "", "", false
}

func (s *Server) handleProfileNoteAttachmentsHTTP(w http.ResponseWriter, r *http.Request, auth authContext, noteID string, attachmentID string) {
	note, err := s.store.GetProfileNote(r.Context(), auth.User.ID, noteID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.handleNoteAttachmentsHTTP(w, r, note, "profile", auth.User.ID, attachmentID)
}

func (s *Server) handleHomeNoteAttachmentsHTTP(w http.ResponseWriter, r *http.Request, home domain.Home, auth authContext, noteID string, attachmentID string) {
	note, err := s.store.GetHomeNoteVisibleToUser(r.Context(), home.ID, auth.User.ID, noteID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.handleNoteAttachmentsHTTP(w, r, note, "home", auth.User.ID, attachmentID)
}

func (s *Server) handleNoteAttachmentsHTTP(w http.ResponseWriter, r *http.Request, note domain.UserNote, scope string, userID string, attachmentID string) {
	if note.DeletedAt != nil {
		http.NotFound(w, r)
		return
	}

	if attachmentID == "" {
		switch r.Method {
		case http.MethodGet:
			attachments, err := s.store.ListNoteAttachments(r.Context(), note.ID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, http.StatusOK, protocol.NoteAttachmentsResponse{
				Attachments: noteAttachmentsToProtocol(attachments, note, scope),
			})
		case http.MethodPost:
			attachment, err := s.storeUploadedNoteAttachment(r, note, scope, userID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			s.emitNoteAttachmentChanged(r.Context(), note, scope, userID)
			writeJSON(w, http.StatusCreated, noteAttachmentToProtocol(attachment, note, scope))
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	attachment, err := s.store.GetNoteAttachment(r.Context(), note.ID, attachmentID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.serveNoteAttachment(w, r, attachment)
	case http.MethodDelete:
		updatedNote, err := noteWithoutAttachmentReference(note, userID, attachment)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := s.store.DeleteNoteAttachmentAndSaveNote(r.Context(), note.ID, attachmentID, time.Now().UTC(), updatedNote); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if attachmentPath, err := s.noteAttachmentPath(attachment.StorageKey); err == nil {
			_ = os.Remove(attachmentPath)
		}
		s.indexNoteAfterAttachmentUpload(r.Context(), updatedNote, userID)
		s.emitNoteAttachmentChanged(r.Context(), note, scope, userID)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) storeUploadedNoteAttachment(r *http.Request, note domain.UserNote, scope string, userID string) (domain.NoteAttachment, error) {
	filename := safeAttachmentFilename(firstNonBlank(r.URL.Query().Get("filename"), r.Header.Get("X-Hank-Filename"), "Attachment"))
	contentType := strings.TrimSpace(r.Header.Get("Content-Type"))
	if parsed, _, err := mime.ParseMediaType(contentType); err == nil && parsed != "" {
		contentType = parsed
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	attachmentID := newID("natt")
	storageKey := filepath.Join(note.ID, attachmentID+"-"+filename)
	targetPath, err := s.noteAttachmentPathForWrite(storageKey)
	if err != nil {
		return domain.NoteAttachment{}, err
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o700); err != nil {
		return domain.NoteAttachment{}, err
	}
	tempPath := targetPath + ".tmp"
	file, err := os.Create(tempPath)
	if err != nil {
		return domain.NoteAttachment{}, err
	}
	defer file.Close()
	defer os.Remove(tempPath)

	limited := io.LimitReader(r.Body, maxNoteAttachmentBytes+1)
	hasher := sha256.New()
	written, err := io.Copy(file, io.TeeReader(limited, hasher))
	if err != nil {
		return domain.NoteAttachment{}, err
	}
	if written > maxNoteAttachmentBytes {
		return domain.NoteAttachment{}, errors.New("attachment is too large")
	}
	if written <= 0 {
		return domain.NoteAttachment{}, errors.New("attachment body is empty")
	}
	if err := file.Close(); err != nil {
		return domain.NoteAttachment{}, err
	}
	if err := os.Rename(tempPath, targetPath); err != nil {
		return domain.NoteAttachment{}, err
	}

	now := time.Now().UTC()
	attachment := domain.NoteAttachment{
		ID:             attachmentID,
		NoteID:         note.ID,
		HomeID:         note.HomeID,
		OwnerUserID:    note.OwnerUserID,
		Filename:       filename,
		ContentType:    contentType,
		SizeBytes:      written,
		ChecksumSHA256: hex.EncodeToString(hasher.Sum(nil)),
		StorageKey:     storageKey,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	updatedNote, err := noteWithAttachmentReference(note, scope, userID, attachment)
	if err != nil {
		_ = os.Remove(targetPath)
		return domain.NoteAttachment{}, err
	}
	if err := s.store.CreateNoteAttachmentAndSaveNote(r.Context(), attachment, updatedNote); err != nil {
		_ = os.Remove(targetPath)
		return domain.NoteAttachment{}, err
	}
	s.indexNoteAfterAttachmentUpload(r.Context(), updatedNote, userID)
	return attachment, nil
}

func noteWithAttachmentReference(note domain.UserNote, scope string, userID string, attachment domain.NoteAttachment) (domain.UserNote, error) {
	body := strings.TrimSpace(noteBodyText(note))
	if body != "" {
		body += "\n\n"
	}
	body += noteAttachmentMarkdownReference(note, scope, attachment)

	revision, checksum, err := revisionAndChecksum(body, note.PageType, note.BoardJSON)
	if err != nil {
		return domain.UserNote{}, err
	}
	note.Content = body
	note.BodyMarkdown = body
	note.BodyFormat = "markdown"
	note.Revision = revision
	note.Checksum = checksum
	note.UpdatedAt = time.Now().UTC()
	note.UpdatedBy = userID
	return note, nil
}

func noteWithoutAttachmentReference(note domain.UserNote, userID string, attachment domain.NoteAttachment) (domain.UserNote, error) {
	body := noteBodyText(note)
	escapedID := regexp.QuoteMeta(url.PathEscape(attachment.ID))
	pattern, err := regexp.Compile(`(?m)\n{0,2}!?\[[^\]]+\]\(hank-note-attachment://` + escapedID + `[^)]*\)`)
	if err != nil {
		return domain.UserNote{}, err
	}
	body = strings.TrimSpace(pattern.ReplaceAllString(body, ""))
	revision, checksum, err := revisionAndChecksum(body, note.PageType, note.BoardJSON)
	if err != nil {
		return domain.UserNote{}, err
	}
	note.Content = body
	note.BodyMarkdown = body
	note.BodyFormat = "markdown"
	note.Revision = revision
	note.Checksum = checksum
	note.UpdatedAt = time.Now().UTC()
	note.UpdatedBy = userID
	return note, nil
}

func (s *Server) indexNoteAfterAttachmentUpload(ctx context.Context, note domain.UserNote, userID string) {
	homeID := note.HomeID
	if homeID == "" {
		if home, err := s.store.GetSingletonHomeForUser(ctx, userID); err == nil {
			homeID = home.ID
		}
	}
	if homeID != "" {
		if err := s.indexAssistantNote(ctx, homeID, userID, assistantNoteSourceType(note), note); err != nil {
			s.logger.Warn("assistant note index refresh failed after attachment upload", "note_id", note.ID, "error", err)
		}
	}
}

func (s *Server) serveNoteAttachment(w http.ResponseWriter, r *http.Request, attachment domain.NoteAttachment) {
	attachmentPath, err := s.noteAttachmentPath(attachment.StorageKey)
	if err != nil {
		http.Error(w, "invalid attachment path", http.StatusBadRequest)
		return
	}
	file, err := os.Open(attachmentPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", attachment.ContentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", attachment.SizeBytes))
	disposition := "attachment"
	if isInlineNoteImageContentType(attachment.ContentType) {
		disposition = "inline"
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": attachment.Filename}))
	http.ServeContent(w, r, attachment.Filename, attachment.UpdatedAt, file)
}

func (s *Server) noteAttachmentPath(storageKey string) (string, error) {
	return s.resolveNoteAttachmentPath(storageKey, false)
}

func (s *Server) noteAttachmentPathForWrite(storageKey string) (string, error) {
	return s.resolveNoteAttachmentPath(storageKey, true)
}

func (s *Server) pruneNoteAttachmentFiles(ctx context.Context, now time.Time, retention time.Duration) error {
	if retention <= 0 {
		retention = 30 * 24 * time.Hour
	}
	root, err := filepath.EvalSymlinks(filepath.Clean(s.noteAttachmentRoot))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	liveKeys, err := s.store.ListLiveNoteAttachmentStorageKeys(ctx)
	if err != nil {
		return err
	}
	cutoff := now.Add(-retention)
	removed := 0
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		storageKey := filepath.ToSlash(relative)
		if _, ok := liveKeys[storageKey]; ok {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.ModTime().After(cutoff) {
			return nil
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		removed++
		return nil
	})
	if err != nil {
		return err
	}
	if removed > 0 {
		s.logger.Info("pruned orphaned note attachment files", "count", removed, "root", root)
	}
	return nil
}

func (s *Server) resolveNoteAttachmentPath(storageKey string, forWrite bool) (string, error) {
	cleaned := filepath.Clean(strings.TrimSpace(storageKey))
	if cleaned == "." || filepath.IsAbs(cleaned) || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) || cleaned == ".." {
		return "", fmt.Errorf("attachment path escapes root")
	}
	root, err := filepath.EvalSymlinks(filepath.Clean(s.noteAttachmentRoot))
	if err != nil {
		if forWrite && errors.Is(err, os.ErrNotExist) {
			if err := os.MkdirAll(s.noteAttachmentRoot, 0o700); err != nil {
				return "", err
			}
			root, err = filepath.EvalSymlinks(filepath.Clean(s.noteAttachmentRoot))
		}
		if err != nil {
			return "", err
		}
	}
	joined := filepath.Join(root, cleaned)
	resolved, err := filepath.EvalSymlinks(joined)
	if err == nil {
		if resolved != root && !strings.HasPrefix(resolved, root+string(filepath.Separator)) {
			return "", fmt.Errorf("attachment path escapes root")
		}
		return resolved, nil
	}
	if !forWrite || !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	parent := filepath.Dir(joined)
	realParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		realParent, err = nearestExistingAttachmentParent(root, parent)
		if err != nil {
			return "", err
		}
	}
	if realParent != root && !strings.HasPrefix(realParent, root+string(filepath.Separator)) {
		return "", fmt.Errorf("attachment path escapes root")
	}
	if realParent != parent {
		relative, err := filepath.Rel(realParent, joined)
		if err != nil || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || relative == ".." {
			return "", fmt.Errorf("attachment path escapes root")
		}
		return filepath.Join(realParent, relative), nil
	}
	return filepath.Join(realParent, filepath.Base(joined)), nil
}

func nearestExistingAttachmentParent(root string, path string) (string, error) {
	current := filepath.Clean(path)
	for {
		if current == root || strings.HasPrefix(current, root+string(filepath.Separator)) {
			resolved, err := filepath.EvalSymlinks(current)
			if err == nil {
				return resolved, nil
			}
			if !errors.Is(err, os.ErrNotExist) {
				return "", err
			}
		} else {
			return "", fmt.Errorf("attachment path escapes root")
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("attachment path escapes root")
		}
		current = parent
	}
}

func (s *Server) emitNoteAttachmentChanged(ctx context.Context, note domain.UserNote, scope string, userID string) {
	if scope == "home" {
		s.markHomeNotesDirty(ctx, note.HomeID, "")
		s.emitHomeNotesChanged(ctx, "notes.attachment_changed", map[string]any{"home_id": note.HomeID, "note_id": note.NoteID})
		return
	}
	s.emitProfileNotesChanged(ctx, map[string]any{"user_id": userID, "note_id": note.NoteID})
}

func (s *Server) addNoteAttachmentsToResponse(ctx context.Context, response protocol.NotesFetchResponse, note domain.UserNote, scope string) protocol.NotesFetchResponse {
	attachments, err := s.store.ListNoteAttachments(ctx, note.ID)
	if err != nil {
		return response
	}
	response.Attachments = noteAttachmentsToProtocol(attachments, note, scope)
	return response
}

func noteAttachmentMarkdownReference(note domain.UserNote, scope string, attachment domain.NoteAttachment) string {
	values := url.Values{}
	values.Set("note_id", note.NoteID)
	values.Set("scope", scope)
	values.Set("filename", attachment.Filename)
	prefix := ""
	if isInlineNoteImageContentType(attachment.ContentType) {
		prefix = "!"
	}
	return fmt.Sprintf("%s[%s](hank-note-attachment://%s?%s)", prefix, attachment.Filename, url.PathEscape(attachment.ID), values.Encode())
}

func isInlineNoteImageContentType(contentType string) bool {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/heic", "image/heif":
		return true
	default:
		return false
	}
}

func noteAttachmentsToProtocol(attachments []domain.NoteAttachment, note domain.UserNote, scope string) []protocol.NoteAttachment {
	results := make([]protocol.NoteAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		results = append(results, noteAttachmentToProtocol(attachment, note, scope))
	}
	return results
}

func noteAttachmentToProtocol(attachment domain.NoteAttachment, note domain.UserNote, scope string) protocol.NoteAttachment {
	return protocol.NoteAttachment{
		ID:             attachment.ID,
		NoteID:         note.NoteID,
		Filename:       attachment.Filename,
		ContentType:    attachment.ContentType,
		SizeBytes:      attachment.SizeBytes,
		ChecksumSHA256: attachment.ChecksumSHA256,
		DownloadURL:    noteAttachmentDownloadPath(note, scope, attachment.ID),
		CreatedAt:      attachment.CreatedAt,
		UpdatedAt:      attachment.UpdatedAt,
	}
}

func noteAttachmentDownloadPath(note domain.UserNote, scope string, attachmentID string) string {
	escapedNoteID := pathEscapeSegments(note.NoteID)
	if scope == "home" {
		return fmt.Sprintf("/v1/home/notes/%s/attachments/%s", escapedNoteID, url.PathEscape(attachmentID))
	}
	return fmt.Sprintf("/v1/me/notes/%s/attachments/%s", escapedNoteID, url.PathEscape(attachmentID))
}

func pathEscapeSegments(value string) string {
	parts := strings.Split(value, "/")
	for index, part := range parts {
		parts[index] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func safeAttachmentFilename(value string) string {
	value = strings.TrimSpace(value)
	value = filepath.Base(value)
	value = unsafeAttachmentFilenameRunes.ReplaceAllString(value, "-")
	value = strings.Trim(value, ". ")
	if value == "" {
		return "Attachment"
	}
	if len(value) > 180 {
		ext := filepath.Ext(value)
		base := strings.TrimSuffix(value, ext)
		if len(ext) > 24 {
			ext = ""
		}
		if len(base) > 180-len(ext) {
			base = base[:180-len(ext)]
		}
		value = strings.TrimSpace(base) + ext
	}
	return value
}

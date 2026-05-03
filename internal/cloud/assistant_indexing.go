package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dropfile/hankremote/internal/domain"
	"github.com/dropfile/hankremote/internal/protocol"
)

func (s *Server) refreshAssistantIndex(ctx context.Context, home domain.Home, membership domain.HomeMembership, auth authContext, settings domain.AssistantSettings, prompt string) {
	settings = normalizeAssistantSettings(settings)
	if settings.NotesEnabled && (settings.ProfileNotesEnabled || settings.HomeNotesEnabled) {
		if err := s.indexAssistantNotes(ctx, home, membership, auth, settings); err != nil {
			s.logger.Warn("assistant note indexing failed", "error", err)
		}
	}
	if settings.ProjectDocsEnabled {
		if err := s.indexAssistantProjectDocs(ctx, home.ID, auth.User.ID); err != nil {
			s.logger.Warn("assistant project docs indexing failed", "error", err)
		}
	}
	if settings.CalendarEnabled {
		if err := s.indexAssistantCalendarSnapshot(ctx, home.ID, auth.User.ID); err != nil {
			s.logger.Warn("assistant calendar indexing failed", "error", err)
		}
	}
	if settings.HomeAssistantEnabled && shouldIndexHomeAssistant(prompt) {
		if err := s.indexAssistantHomeAssistantStates(ctx, home, membership, auth); err != nil {
			s.logger.Warn("assistant Home Assistant indexing failed", "error", err)
		}
	}
	if settings.FilesEnabled && shouldIndexFiles(prompt) {
		if err := s.indexAssistantFiles(ctx, home, membership, auth); err != nil {
			s.logger.Warn("assistant file indexing failed", "error", err)
		}
	}
}

func (s *Server) indexAssistantNotes(ctx context.Context, home domain.Home, membership domain.HomeMembership, auth authContext, settings domain.AssistantSettings) error {
	if err := s.requireHomeFeature(ctx, home, membership, auth.User.ID, domain.HomePermissionFeatureNotes); err != nil {
		return nil
	}
	if settings.ProfileNotesEnabled {
		profileNotes, err := s.store.ListProfileNotes(ctx, auth.User.ID, false)
		if err != nil {
			return err
		}
		for _, note := range profileNotes {
			if note.DeletedAt != nil {
				continue
			}
			if err := s.indexAssistantNote(ctx, home.ID, auth.User.ID, "profile_note", note); err != nil {
				return err
			}
		}
	}
	if !settings.HomeNotesEnabled {
		return nil
	}
	homeNotes, err := s.store.ListVisibleHomeNotes(ctx, home.ID, auth.User.ID, false)
	if err != nil {
		return err
	}
	for _, note := range homeNotes {
		if note.DeletedAt != nil {
			continue
		}
		if err := s.indexAssistantNote(ctx, home.ID, auth.User.ID, "shared_note", note); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) indexAssistantNote(ctx context.Context, homeID string, userID string, sourceType string, note domain.UserNote) error {
	body := firstNonBlank(note.BodyMarkdown, note.Content)
	metadata, _ := json.Marshal(map[string]any{
		"page_type": note.PageType,
		"note_id":   note.NoteID,
	})
	userIDCopy := userID
	sourceKey := strings.Join([]string{sourceType, userID, note.ID}, ":")
	document := domain.AssistantDocument{
		ID:           stableAssistantID("adoc", sourceKey),
		HomeID:       homeID,
		UserID:       &userIDCopy,
		SourceType:   sourceType,
		SourceID:     note.ID,
		SourceKey:    sourceKey,
		Title:        firstNonBlank(note.Title, note.NoteID),
		Path:         note.NoteID,
		CanonicalURI: "hank://notes/" + note.NoteID,
		MetadataJSON: string(metadata),
		SearchText:   strings.TrimSpace(note.Title + "\n" + note.NoteID + "\n" + body),
		UpdatedAt:    note.UpdatedAt,
	}
	return s.store.UpsertAssistantDocumentWithChunks(ctx, document, s.assistantChunksForText(ctx, userID, document.ID, document.SearchText, note.UpdatedAt))
}

func (s *Server) indexAssistantCalendarSnapshot(ctx context.Context, homeID string, userID string) error {
	entries, err := s.store.ListAssistantCalendarEntries(ctx, homeID, userID)
	if err != nil {
		return err
	}
	return s.indexAssistantCalendarEntries(ctx, homeID, userID, entries)
}

func (s *Server) indexAssistantCalendarEntries(ctx context.Context, homeID string, userID string, entries []domain.AssistantCalendarEntry) error {
	for _, entry := range entries {
		metadata, _ := json.Marshal(map[string]any{
			"calendar_id":       entry.CalendarID,
			"device_id":         entry.DeviceID,
			"external_event_id": entry.ExternalEventID,
			"starts_at":         entry.StartsAt,
			"ends_at":           entry.EndsAt,
			"is_all_day":        entry.IsAllDay,
		})
		userIDCopy := userID
		sourceKey := strings.Join([]string{"calendar_event", userID, entry.DeviceID, entry.ExternalEventID}, ":")
		searchText := strings.TrimSpace(strings.Join([]string{
			entry.Title,
			entry.CalendarID,
			entry.Location,
			entry.Notes,
			entry.StartsAt.Format(time.RFC3339),
			entry.SearchText,
		}, "\n"))
		document := domain.AssistantDocument{
			ID:           stableAssistantID("adoc", sourceKey),
			HomeID:       homeID,
			UserID:       &userIDCopy,
			SourceType:   "calendar_event",
			SourceID:     entry.ExternalEventID,
			SourceKey:    sourceKey,
			Title:        firstNonBlank(entry.Title, "Calendar Event"),
			Path:         entry.StartsAt.Format(time.RFC3339),
			CanonicalURI: "hank://calendar/" + entry.ExternalEventID,
			MetadataJSON: string(metadata),
			SearchText:   searchText,
			UpdatedAt:    entry.UpdatedAt,
		}
		if err := s.store.UpsertAssistantDocumentWithChunks(ctx, document, s.assistantChunksForText(ctx, userID, document.ID, searchText, entry.UpdatedAt)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) indexAssistantHomeAssistantStates(ctx context.Context, home domain.Home, membership domain.HomeMembership, auth authContext) error {
	if err := s.requireHomeFeature(ctx, home, membership, auth.User.ID, domain.HomePermissionFeatureHomeAssistant); err != nil {
		return nil
	}
	envelope, err := s.sendAgentCommand(ctx, home.ID, "homeassistant.fetch_states", map[string]any{})
	if err != nil || envelope.Error != nil {
		return err
	}
	payload, err := protocol.DecodePayload[protocol.HomeAssistantFetchStatesResponse](envelope)
	if err != nil {
		return err
	}
	for index, state := range payload.States {
		if index >= 250 {
			break
		}
		metadata, _ := json.Marshal(map[string]any{
			"state":        state.State,
			"attributes":   state.Attributes,
			"last_changed": state.LastChanged,
			"last_updated": state.LastUpdated,
		})
		sourceKey := "homeassistant_entity:" + home.ID + ":" + state.EntityID
		searchText := state.EntityID + "\n" + state.State + "\n" + assistantAttributesText(state.Attributes)
		document := domain.AssistantDocument{
			ID:           stableAssistantID("adoc", sourceKey),
			HomeID:       home.ID,
			SourceType:   "homeassistant_entity",
			SourceID:     state.EntityID,
			SourceKey:    sourceKey,
			Title:        state.EntityID,
			Path:         state.EntityID,
			CanonicalURI: "hank://homeassistant/" + state.EntityID,
			MetadataJSON: string(metadata),
			SearchText:   searchText,
			UpdatedAt:    firstTime(state.LastUpdated, state.LastChanged, time.Now().UTC()),
		}
		if err := s.store.UpsertAssistantDocumentWithChunks(ctx, document, s.assistantChunksForText(ctx, auth.User.ID, document.ID, searchText, document.UpdatedAt)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) indexAssistantFiles(ctx context.Context, home domain.Home, membership domain.HomeMembership, auth authContext) error {
	if err := s.requireHomeFeature(ctx, home, membership, auth.User.ID, domain.HomePermissionFeatureFiles); err != nil {
		return nil
	}
	items, err := s.crawlFilesForAssistantIndex(ctx, home.ID, 300)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, item := range items {
		embedding, model, version := s.embedAssistantText(ctx, auth.User.ID, item.Path+" "+item.Name)
		embeddingJSON, _ := json.Marshal(embedding)
		metadata, _ := json.Marshal(map[string]any{"is_directory": item.IsDirectory})
		modifiedAt := item.ModifiedAt
		if modifiedAt.IsZero() {
			modifiedAt = now
		}
		if err := s.store.UpsertAssistantFileIndex(ctx, domain.AssistantFileIndex{
			ID:               stableAssistantID("afile", home.ID+":"+item.Path),
			HomeID:           home.ID,
			Path:             item.Path,
			Name:             item.Name,
			IsDirectory:      item.IsDirectory,
			SizeBytes:        item.Size,
			ModifiedAt:       &modifiedAt,
			SearchText:       item.Path + "\n" + item.Name,
			MetadataJSON:     string(metadata),
			EmbeddingJSON:    string(embeddingJSON),
			EmbeddingModel:   model,
			EmbeddingVersion: version,
			UpdatedAt:        now,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) crawlFilesForAssistantIndex(ctx context.Context, homeID string, maxItems int) ([]protocol.FileItem, error) {
	type queueItem struct {
		path  string
		depth int
	}
	queue := []queueItem{{path: "", depth: 0}}
	items := make([]protocol.FileItem, 0)
	visited := 0
	for len(queue) > 0 && len(items) < maxItems && visited < maxItems {
		current := queue[0]
		queue = queue[1:]
		visited++
		envelope, err := s.sendAgentCommand(ctx, homeID, "files.list", protocol.FilesListRequest{Path: current.path})
		if err != nil {
			return nil, err
		}
		if envelope.Error != nil {
			return nil, errors.New(envelope.Error.Message)
		}
		payload, err := protocol.DecodePayload[protocol.FilesListResponse](envelope)
		if err != nil {
			return nil, err
		}
		for _, item := range payload.Items {
			items = append(items, item)
			if len(items) >= maxItems {
				break
			}
			if item.IsDirectory && current.depth < 4 {
				queue = append(queue, queueItem{path: item.Path, depth: current.depth + 1})
			}
		}
	}
	return items, nil
}

func (s *Server) assistantChunksForText(ctx context.Context, userID string, documentID string, text string, updatedAt time.Time) []domain.AssistantChunk {
	parts := chunkAssistantText(text, 1800)
	chunks := make([]domain.AssistantChunk, 0, len(parts))
	for index, part := range parts {
		embedding, model, version := s.embedAssistantText(ctx, userID, part)
		embeddingJSON, _ := json.Marshal(embedding)
		chunks = append(chunks, domain.AssistantChunk{
			ID:               stableAssistantID("achunk", fmt.Sprintf("%s:%d:%x", documentID, index, embeddingJSON[:min(len(embeddingJSON), 16)])),
			DocumentID:       documentID,
			ChunkIndex:       index,
			Content:          part,
			TokenCount:       len(strings.Fields(part)),
			EmbeddingJSON:    string(embeddingJSON),
			EmbeddingModel:   model,
			EmbeddingVersion: version,
			UpdatedAt:        updatedAt,
		})
	}
	return chunks
}

func chunkAssistantText(text string, maxChars int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return []string{""}
	}
	var chunks []string
	for len(text) > maxChars {
		cut := strings.LastIndex(text[:maxChars], "\n")
		if cut < maxChars/3 {
			cut = strings.LastIndex(text[:maxChars], " ")
		}
		if cut < maxChars/3 {
			cut = maxChars
		}
		chunks = append(chunks, strings.TrimSpace(text[:cut]))
		text = strings.TrimSpace(text[cut:])
	}
	if text != "" {
		chunks = append(chunks, text)
	}
	return chunks
}

func shouldIndexFiles(prompt string) bool {
	lowered := strings.ToLower(prompt)
	return strings.Contains(lowered, "file") || strings.Contains(lowered, "folder") || strings.Contains(lowered, "smb") || strings.Contains(lowered, "share")
}

func shouldIndexHomeAssistant(prompt string) bool {
	lowered := strings.ToLower(prompt)
	return strings.Contains(lowered, "home assistant") || strings.Contains(lowered, "entity") || strings.Contains(lowered, "light") || strings.Contains(lowered, "sensor") || strings.Contains(lowered, "switch")
}

func assistantAttributesText(attributes map[string]any) string {
	if len(attributes) == 0 {
		return ""
	}
	encoded, _ := json.Marshal(attributes)
	return string(encoded)
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstTime(values ...any) time.Time {
	for _, value := range values {
		switch typed := value.(type) {
		case *time.Time:
			if typed != nil && !typed.IsZero() {
				return *typed
			}
		case time.Time:
			if !typed.IsZero() {
				return typed
			}
		}
	}
	return time.Now().UTC()
}

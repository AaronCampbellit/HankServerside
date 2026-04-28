package cloud

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/dropfile/hankremote/internal/protocol"
	"github.com/dropfile/hankremote/internal/store"
)

const (
	topicHomeAssistantStates = "homeassistant.states"
	topicNotesHome           = "notes.home"
	topicNotesProfile        = "notes.profile"
	topicHomeStatus          = "home.status"
	topicHomeSettings        = "home.settings"
	topicHomeMembers         = "home.members"
	topicHomePermissions     = "home.permissions"
)

func fileDirectoryTopic(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "/"
	}
	return "files.directory:" + path
}

func noteCollabTopic(noteID string) string {
	return "notes.collab:" + strings.TrimSpace(noteID)
}

func scopedNoteCollabTopic(scope string, noteID string) string {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return noteCollabTopic(noteID)
	}
	return "notes.collab:" + scope + ":" + strings.TrimSpace(noteID)
}

func (s *Server) forwardStoreNotifications(ctx context.Context) {
	notifications, err := s.store.Listen(ctx, store.NotificationChannelNotes, store.NotificationChannelProfiles)
	if err != nil {
		s.logger.Warn("postgres realtime listener unavailable", "error", err)
		return
	}
	for notification := range notifications {
		switch notification.Channel {
		case store.NotificationChannelNotes:
			s.forwardNoteNotification(ctx, notification.Payload)
		case store.NotificationChannelProfiles:
			s.forwardProfileNotification(ctx, notification.Payload)
		}
	}
}

func (s *Server) forwardNoteNotification(ctx context.Context, payload json.RawMessage) {
	var event struct {
		Event         string `json:"event"`
		NoteID        string `json:"note_id"`
		HomeID        string `json:"home_id"`
		OwnerUserID   string `json:"owner_user_id"`
		UpdatedBy     string `json:"updated_by"`
		CollabVersion int64  `json:"collab_version"`
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		s.logger.Warn("bad note notification payload", "error", err)
		return
	}
	if strings.TrimSpace(event.Event) == "" {
		event.Event = "notes.changed"
	}
	body := map[string]any{
		"note_id":        event.NoteID,
		"home_id":        event.HomeID,
		"user_id":        event.OwnerUserID,
		"owner_user_id":  event.OwnerUserID,
		"updated_by":     event.UpdatedBy,
		"collab_version": event.CollabVersion,
	}
	s.broadcastAppEvent(ctx, topicNotesProfile, event.Event, body)
	if event.HomeID != "" {
		s.broadcastAppEvent(ctx, topicNotesHome, event.Event, body)
		s.broadcastAppEvent(ctx, scopedNoteCollabTopic("home", event.NoteID), event.Event, body)
	}
	s.broadcastAppEvent(ctx, noteCollabTopic(event.NoteID), event.Event, body)
	s.broadcastAppEvent(ctx, scopedNoteCollabTopic("profile", event.NoteID), event.Event, body)
}

func (s *Server) forwardProfileNotification(ctx context.Context, payload json.RawMessage) {
	var event struct {
		Event    string `json:"event"`
		UserID   string `json:"user_id"`
		Revision int    `json:"revision"`
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		s.logger.Warn("bad profile notification payload", "error", err)
		return
	}
	if strings.TrimSpace(event.Event) == "" {
		event.Event = "profile.changed"
	}
	topic := "profile.settings"
	if event.Event == "profile.secret_vault_changed" {
		topic = "profile.secret_vault"
	}
	s.broadcastAppEvent(ctx, topic, event.Event, map[string]any{
		"user_id":  event.UserID,
		"revision": event.Revision,
	})
}

func (s *Server) handleRealtimeCommand(ctx context.Context, app *appConnection, peer *wsPeer, envelope protocol.Envelope, command protocol.RoutedCommand) bool {
	switch command.Command {
	case "app.subscribe", "app.unsubscribe":
		var request protocol.AppSubscribeRequest
		if len(command.Body) > 0 {
			if err := json.Unmarshal(command.Body, &request); err != nil {
				_ = s.writeAppCommandError(ctx, peer, envelope, "bad_subscribe_payload", err.Error(), nil)
				return true
			}
		}
		topics := cleanTopics(request.Topics)
		var current []string
		if command.Command == "app.subscribe" {
			current = app.subscribe(topics)
		} else {
			current = app.unsubscribe(topics)
		}
		s.logger.Info("app realtime subscription changed", "session_id", app.sessionID, "user_id", app.userID, "command", command.Command, "topics", strings.Join(topics, ","), "current_topics", strings.Join(current, ","))
		_ = writeAppResponse(ctx, peer, envelope, protocol.AppSubscribeResponse{Topics: current})
		return true
	default:
		return false
	}
}

func cleanTopics(topics []string) []string {
	cleaned := make([]string, 0, len(topics))
	seen := map[string]struct{}{}
	for _, topic := range topics {
		topic = strings.TrimSpace(topic)
		if topic == "" {
			continue
		}
		if _, ok := seen[topic]; ok {
			continue
		}
		seen[topic] = struct{}{}
		cleaned = append(cleaned, topic)
	}
	return cleaned
}

func (s *Server) broadcastAppEvent(ctx context.Context, topic string, event string, payload any) {
	apps := s.router.AppsForTopic(topic)
	if strings.HasPrefix(topic, "notes.") {
		s.logger.Info("broadcasting notes realtime event", "topic", topic, "event", event, "subscriber_count", len(apps))
	}
	for _, app := range apps {
		_ = writeAppEvent(ctx, app.peer, event, topic, payload)
	}
}

func writeAppResponse(ctx context.Context, peer *wsPeer, envelope protocol.Envelope, payload any) error {
	body, err := protocol.EncodeBody(payload)
	if err != nil {
		return err
	}
	return peer.Write(ctx, protocol.Envelope{
		Version:   protocol.Version,
		Type:      protocol.TypeAppResponse,
		RequestID: envelope.RequestID,
		AgentID:   envelope.AgentID,
		HomeID:    envelope.HomeID,
		Timestamp: time.Now().UTC(),
		Payload:   body,
	})
}

func (s *Server) emitHomeStatus(ctx context.Context, homeID string, payload any) {
	s.broadcastAppEvent(ctx, topicHomeStatus, "home.status_changed", payload)
}

func (s *Server) emitSyncStatus(ctx context.Context, homeID string) {
	s.broadcastAppEvent(ctx, topicHomeStatus, "sync.status_changed", map[string]any{"home_id": homeID})
}

func (s *Server) emitSettingsChanged(ctx context.Context, event string, payload any) {
	s.broadcastAppEvent(ctx, topicHomeSettings, event, payload)
}

func (s *Server) emitMembersChanged(ctx context.Context, payload any) {
	s.broadcastAppEvent(ctx, topicHomeMembers, "members.changed", payload)
}

func (s *Server) emitPermissionsChanged(ctx context.Context, payload any) {
	s.broadcastAppEvent(ctx, topicHomePermissions, "permissions.changed", payload)
}

func (s *Server) emitHomeNotesChanged(ctx context.Context, event string, payload any) {
	s.broadcastAppEvent(ctx, topicNotesHome, event, payload)
}

func (s *Server) emitProfileNotesChanged(ctx context.Context, payload any) {
	s.broadcastAppEvent(ctx, topicNotesProfile, "notes.changed", payload)
}

func (s *Server) emitFileDirectoryChanged(ctx context.Context, path string, payload any) {
	s.broadcastAppEvent(ctx, fileDirectoryTopic(path), "files.directory_changed", payload)
}

func (s *Server) emitHomeAssistantStateChanged(ctx context.Context, payload any) {
	s.broadcastAppEvent(ctx, topicHomeAssistantStates, "homeassistant.state_changed", payload)
}

func (s *Server) emitCommandSideEffect(ctx context.Context, command string, payload json.RawMessage) {
	switch command {
	case "homeassistant.fetch_state":
		s.broadcastRawAppEvent(ctx, topicHomeAssistantStates, "homeassistant.state_changed", payload)
	case "files.create_directory", "files.upload", "files.rename", "files.delete":
		s.emitFileDirectoryChanged(ctx, "/", map[string]any{"path": "/"})
	}
}

func (s *Server) handleAgentEvent(ctx context.Context, homeID string, envelope protocol.Envelope) {
	event, err := protocol.DecodePayload[protocol.AgentEvent](envelope)
	if err != nil {
		s.logger.Warn("bad agent event payload", "home_id", homeID, "error", err)
		return
	}

	switch event.Event {
	case "homeassistant.state_changed":
		s.broadcastRawAppEvent(ctx, topicHomeAssistantStates, event.Event, event.Body)
	case "files.directory_changed":
		topic := event.Topic
		if !strings.HasPrefix(topic, "files.directory:") {
			topic = fileDirectoryTopic("/")
		}
		s.broadcastRawAppEvent(ctx, topic, event.Event, event.Body)
	case "sync.status_changed":
		s.broadcastRawAppEvent(ctx, topicHomeStatus, event.Event, event.Body)
	default:
		s.logger.Debug("ignored agent event", "home_id", homeID, "event", event.Event)
	}
}

func (s *Server) broadcastRawAppEvent(ctx context.Context, topic string, event string, body json.RawMessage) {
	for _, app := range s.router.AppsForTopic(topic) {
		_ = app.peer.Write(ctx, protocol.Envelope{
			Version:   protocol.Version,
			Type:      protocol.TypeAppEvent,
			Timestamp: time.Now().UTC(),
			Payload:   mustEventBody(event, topic, body),
		})
	}
}

package domain

import (
	"encoding/json"
	"time"
)

const (
	AgentStatusOffline = "offline"
	AgentStatusOnline  = "online"

	HomeRoleAdmin  = "admin"
	HomeRoleMember = "member"
	HomeRoleOwner  = HomeRoleAdmin

	SyncStatusHealthy   = "healthy"
	SyncStatusDegraded  = "degraded"
	SyncStatusOutOfSync = "out_of_sync"
	SyncStatusOffline   = "offline"
	SyncStatusPending   = "pending"

	ServiceTypeHomeAssistant = "homeassistant"
	ServiceTypeSMB           = "smb"

	HomePermissionFeatureHomeAssistant = "homeassistant"
	HomePermissionFeatureFiles         = "files"
	HomePermissionFeatureNotes         = "notes"
)

type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Home struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type HomeMembership struct {
	HomeID    string    `json:"home_id"`
	UserID    string    `json:"user_id"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type HomeMember struct {
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type HomeInvitation struct {
	ID         string     `json:"id"`
	HomeID     string     `json:"home_id"`
	Email      string     `json:"email"`
	Role       string     `json:"role"`
	TokenHash  string     `json:"-"`
	AcceptedAt *time.Time `json:"accepted_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type Agent struct {
	ID         string     `json:"id"`
	HomeID     string     `json:"home_id"`
	Name       string     `json:"name"`
	Status     string     `json:"status"`
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type AgentToken struct {
	ID        string     `json:"id"`
	HomeID    string     `json:"home_id"`
	AgentID   string     `json:"agent_id"`
	TokenHash string     `json:"-"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type AppSession struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	TokenHash string     `json:"-"`
	ExpiresAt time.Time  `json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type HomeNote struct {
	HomeID    string     `json:"home_id"`
	NoteID    string     `json:"note_id"`
	Title     string     `json:"title"`
	Content   string     `json:"content"`
	PageType  string     `json:"page_type"`
	BoardJSON string     `json:"board_json,omitempty"`
	Revision  string     `json:"revision"`
	Checksum  string     `json:"checksum"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
	UpdatedAt time.Time  `json:"updated_at"`
	UpdatedBy string     `json:"updated_by"`
}

type HomeNoteSyncState struct {
	HomeID               string     `json:"home_id"`
	AgentID              string     `json:"agent_id"`
	LastManifestAt       *time.Time `json:"last_manifest_at,omitempty"`
	LastPullAt           *time.Time `json:"last_pull_at,omitempty"`
	LastPushAt           *time.Time `json:"last_push_at,omitempty"`
	Status               string     `json:"status"`
	LastError            string     `json:"last_error,omitempty"`
	PendingPullCount     int        `json:"pending_pull_count"`
	PendingPushCount     int        `json:"pending_push_count"`
	LastSuccessfulSyncAt *time.Time `json:"last_successful_sync_at,omitempty"`
}

type HomeServiceProfile struct {
	HomeID           string     `json:"home_id"`
	ServiceType      string     `json:"service_type"`
	PublicConfigJSON string     `json:"public_config_json,omitempty"`
	SecretVersion    int        `json:"secret_version"`
	AppliedVersion   int        `json:"applied_version"`
	Status           string     `json:"status"`
	UpdatedAt        time.Time  `json:"updated_at"`
	UpdatedBy        string     `json:"updated_by"`
	LastBackupAt     *time.Time `json:"last_backup_at,omitempty"`
	LastError        string     `json:"last_error,omitempty"`
}

type HomePermissions struct {
	HomeID               string    `json:"home_id"`
	HomeAssistantEnabled bool      `json:"homeassistant"`
	FilesEnabled         bool      `json:"files"`
	NotesEnabled         bool      `json:"notes"`
	UpdatedAt            time.Time `json:"updated_at"`
	UpdatedBy            string    `json:"updated_by"`
}

type HomeMemberPermissions struct {
	HomeID               string    `json:"home_id"`
	UserID               string    `json:"user_id"`
	HomeAssistantEnabled *bool     `json:"homeassistant,omitempty"`
	FilesEnabled         *bool     `json:"files,omitempty"`
	NotesEnabled         *bool     `json:"notes,omitempty"`
	UpdatedAt            time.Time `json:"updated_at"`
	UpdatedBy            string    `json:"updated_by"`
}

type UserProfileBackup struct {
	UserID    string          `json:"user_id"`
	Revision  int             `json:"revision"`
	Snapshot  json.RawMessage `json:"snapshot"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type AssistantSession struct {
	ID            string    `json:"id"`
	HomeID        string    `json:"home_id"`
	UserID        string    `json:"user_id"`
	Title         string    `json:"title"`
	LastMessageAt time.Time `json:"last_message_at"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type AssistantMessage struct {
	ID          string    `json:"id"`
	SessionID   string    `json:"session_id"`
	Role        string    `json:"role"`
	Status      string    `json:"status"`
	ContentJSON string    `json:"content_json"`
	ModelName   string    `json:"model_name"`
	CreatedAt   time.Time `json:"created_at"`
}

type AssistantRun struct {
	ID                   string     `json:"id"`
	SessionID            string     `json:"session_id"`
	MessageID            string     `json:"message_id"`
	State                string     `json:"state"`
	RequiresClientTools  bool       `json:"requires_client_tools"`
	RequiresConfirmation bool       `json:"requires_confirmation"`
	PendingActionJSON    string     `json:"pending_action_json"`
	CreatedAt            time.Time  `json:"created_at"`
	CompletedAt          *time.Time `json:"completed_at,omitempty"`
}

type AssistantCalendarEntry struct {
	ID              string    `json:"id"`
	HomeID          string    `json:"home_id"`
	UserID          string    `json:"user_id"`
	DeviceID        string    `json:"device_id"`
	ExternalEventID string    `json:"external_event_id"`
	CalendarID      string    `json:"calendar_id"`
	Title           string    `json:"title"`
	Location        string    `json:"location"`
	Notes           string    `json:"notes"`
	StartsAt        time.Time `json:"starts_at"`
	EndsAt          time.Time `json:"ends_at"`
	IsAllDay        bool      `json:"is_all_day"`
	SearchText      string    `json:"search_text"`
	MetadataJSON    string    `json:"metadata_json"`
	UpdatedAt       time.Time `json:"updated_at"`
}

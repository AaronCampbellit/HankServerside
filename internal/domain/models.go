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
	ServiceTypeHermes        = "hermes"

	HomePermissionFeatureHomeAssistant = "homeassistant"
	HomePermissionFeatureFiles         = "files"
	HomePermissionFeatureNotes         = "notes"
)

const (
	QuickLinkStatusUnchecked = "unchecked"
	QuickLinkStatusUp        = "up"
	QuickLinkStatusDown      = "down"
	QuickLinkStatusDisabled  = "disabled"
)

type User struct {
	ID                     string     `json:"id"`
	Email                  string     `json:"email"`
	PasswordHash           string     `json:"-"`
	PasswordChangeRequired bool       `json:"password_change_required"`
	PasswordChangedAt      *time.Time `json:"password_changed_at,omitempty"`
	PasswordResetAt        *time.Time `json:"password_reset_at,omitempty"`
	PasswordResetBy        string     `json:"password_reset_by,omitempty"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
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

const (
	NotificationCategoryStorage           = "storage"
	NotificationCategoryNotes             = "notes"
	NotificationCategoryDashboardEntities = "dashboard_entities"
)

type NotificationSettings struct {
	UserID                   string    `json:"user_id"`
	StorageEnabled           bool      `json:"storage"`
	NotesEnabled             bool      `json:"notes"`
	DashboardEntitiesEnabled bool      `json:"dashboard_entities"`
	UpdatedAt                time.Time `json:"updated_at"`
}

type APNSDevice struct {
	UserID            string          `json:"user_id"`
	SessionID         string          `json:"session_id"`
	DeviceID          string          `json:"device_id"`
	Token             string          `json:"token"`
	Environment       string          `json:"environment"`
	BundleID          string          `json:"bundle_id"`
	EnabledCategories json.RawMessage `json:"enabled_categories"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
	LastRegisteredAt  time.Time       `json:"last_registered_at"`
}

type UserProfileSettings struct {
	UserID    string          `json:"user_id"`
	Revision  int             `json:"revision"`
	Settings  json.RawMessage `json:"settings"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type UserProfileSecretVault struct {
	UserID    string          `json:"user_id"`
	Revision  int             `json:"revision"`
	KeyID     string          `json:"key_id"`
	Vault     json.RawMessage `json:"vault"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
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

type HomeQuickLink struct {
	ID                 string     `json:"id"`
	HomeID             string     `json:"home_id"`
	Title              string     `json:"title"`
	URL                string     `json:"url"`
	Description        string     `json:"description,omitempty"`
	SortOrder          int        `json:"sort_order"`
	HealthCheckEnabled bool       `json:"health_check_enabled"`
	Status             string     `json:"status"`
	StatusCode         int        `json:"status_code"`
	LastCheckedAt      *time.Time `json:"last_checked_at,omitempty"`
	LastError          string     `json:"last_error,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	UpdatedBy          string     `json:"updated_by"`
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

type AssistantAttachment struct {
	ID                 string     `json:"id"`
	SessionID          string     `json:"session_id"`
	UserID             string     `json:"user_id"`
	ClientAttachmentID string     `json:"client_attachment_id"`
	Filename           string     `json:"filename"`
	ContentType        string     `json:"content_type"`
	Kind               string     `json:"kind"`
	SizeBytes          int64      `json:"size_bytes"`
	ChecksumSHA256     string     `json:"checksum_sha256"`
	Status             string     `json:"status"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	CommittedAt        *time.Time `json:"committed_at,omitempty"`
}

type NoteAttachment struct {
	ID             string     `json:"id"`
	NoteID         string     `json:"note_id"`
	HomeID         string     `json:"home_id,omitempty"`
	OwnerUserID    string     `json:"owner_user_id"`
	Filename       string     `json:"filename"`
	ContentType    string     `json:"content_type"`
	SizeBytes      int64      `json:"size_bytes"`
	ChecksumSHA256 string     `json:"checksum_sha256"`
	StorageKey     string     `json:"storage_key"`
	DeletedAt      *time.Time `json:"deleted_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
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

type AssistantDocument struct {
	ID               string    `json:"id"`
	HomeID           string    `json:"home_id"`
	UserID           *string   `json:"user_id,omitempty"`
	SourceType       string    `json:"source_type"`
	SourceID         string    `json:"source_id"`
	SourceKey        string    `json:"source_key"`
	Title            string    `json:"title"`
	Path             string    `json:"path"`
	CanonicalURI     string    `json:"canonical_uri"`
	MetadataJSON     string    `json:"metadata_json"`
	SearchText       string    `json:"search_text"`
	EmbeddingModel   string    `json:"embedding_model"`
	EmbeddingVersion string    `json:"embedding_version"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type AssistantChunk struct {
	ID               string    `json:"id"`
	DocumentID       string    `json:"document_id"`
	ChunkIndex       int       `json:"chunk_index"`
	Content          string    `json:"content"`
	TokenCount       int       `json:"token_count"`
	EmbeddingJSON    string    `json:"embedding_json"`
	EmbeddingModel   string    `json:"embedding_model"`
	EmbeddingVersion string    `json:"embedding_version"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type AssistantFileIndex struct {
	ID               string     `json:"id"`
	HomeID           string     `json:"home_id"`
	ServiceProfileID string     `json:"service_profile_id"`
	Path             string     `json:"path"`
	Name             string     `json:"name"`
	IsDirectory      bool       `json:"is_directory"`
	SizeBytes        int64      `json:"size_bytes"`
	ModifiedAt       *time.Time `json:"modified_at,omitempty"`
	SearchText       string     `json:"search_text"`
	MetadataJSON     string     `json:"metadata_json"`
	EmbeddingJSON    string     `json:"embedding_json"`
	EmbeddingModel   string     `json:"embedding_model"`
	EmbeddingVersion string     `json:"embedding_version"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type AssistantRetrievedContext struct {
	SourceType   string    `json:"source_type"`
	SourceID     string    `json:"source_id"`
	Title        string    `json:"title"`
	Path         string    `json:"path"`
	CanonicalURI string    `json:"canonical_uri"`
	Snippet      string    `json:"snippet"`
	Score        float64   `json:"score"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type AssistantIndexSourceCount struct {
	SourceType         string `json:"source_type"`
	DocumentCount      int    `json:"document_count"`
	ChunkCount         int    `json:"chunk_count"`
	EmbeddedChunkCount int    `json:"embedded_chunk_count"`
}

type AssistantIndexStats struct {
	VectorAvailable    bool                        `json:"vector_available"`
	VectorMode         string                      `json:"vector_mode"`
	DocumentsBySource  []AssistantIndexSourceCount `json:"documents_by_source"`
	ChunkCount         int                         `json:"chunk_count"`
	EmbeddedChunkCount int                         `json:"embedded_chunk_count"`
	FileCount          int                         `json:"file_count"`
	EmbeddedFileCount  int                         `json:"embedded_file_count"`
	ConversationCount  int                         `json:"conversation_count"`
}

type AssistantSettings struct {
	HomeID               string    `json:"home_id"`
	UserID               string    `json:"user_id"`
	ProfileNotesEnabled  bool      `json:"profile_notes_enabled"`
	HomeNotesEnabled     bool      `json:"home_notes_enabled"`
	FilesEnabled         bool      `json:"files_enabled"`
	CalendarEnabled      bool      `json:"calendar_enabled"`
	HomeAssistantEnabled bool      `json:"homeassistant_enabled"`
	ProjectDocsEnabled   bool      `json:"project_docs_enabled"`
	ConversationsEnabled bool      `json:"conversations_enabled"`
	SystemPrompt         string    `json:"system_prompt"`
	MaxContextItems      int       `json:"max_context_items"`
	AIProvider           string    `json:"ai_provider"`
	OllamaBaseURL        string    `json:"ollama_base_url"`
	ChatModel            string    `json:"chat_model"`
	EmbeddingModel       string    `json:"embedding_model"`
	PromptProfile        string    `json:"prompt_profile"`
	PlannerEnabled       bool      `json:"planner_enabled"`
	PlannerModel         string    `json:"planner_model"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
	UpdatedBy            string    `json:"updated_by"`
}

type OpenAIAccount struct {
	UserID          string     `json:"user_id"`
	ProviderUserID  string     `json:"provider_user_id"`
	AuthProvider    string     `json:"auth_provider"`
	ChatGPTPlanType string     `json:"chatgpt_plan_type"`
	AccessToken     string     `json:"-"`
	RefreshToken    string     `json:"-"`
	TokenType       string     `json:"token_type"`
	Scope           string     `json:"scope"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

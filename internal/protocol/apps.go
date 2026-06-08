package protocol

import "encoding/json"

const (
	CommandAppsList            = "apps.list"
	CommandAppsPackagePreview  = "apps.package_preview"
	CommandAppsPackageActivate = "apps.package_activate"
	CommandAppsConfigStatus    = "apps.config_status"
	CommandAppsConfigApply     = "apps.config_apply"
	CommandAppsInvoke          = "apps.invoke"

	AppSchemaVersion = "hank.app.v1"
	AppRuntimeStdio  = "stdio"
)

type AppSummary struct {
	ID              string              `json:"id"`
	Name            string              `json:"name"`
	Version         string              `json:"version"`
	Publisher       string              `json:"publisher,omitempty"`
	Description     string              `json:"description,omitempty"`
	Enabled         bool                `json:"enabled"`
	Status          string              `json:"status"`
	LastError       string              `json:"last_error,omitempty"`
	Capabilities    []string            `json:"capabilities,omitempty"`
	SlashCommands   []AppSlashCommand   `json:"slash_commands,omitempty"`
	Commands        []AppCommandSummary `json:"commands,omitempty"`
	PublicConfig    json.RawMessage     `json:"public_config,omitempty"`
	SecretFieldsSet map[string]bool     `json:"secret_fields_set,omitempty"`
}

type AppSlashCommand struct {
	Command     string `json:"command"`
	CommandID   string `json:"command_id"`
	Description string `json:"description,omitempty"`
}

type AppCommandSummary struct {
	ID             string `json:"id"`
	Mode           string `json:"mode"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	AdminOnly      bool   `json:"admin_only"`
}

type AppsListRequest struct{}

type AppsListResponse struct {
	Apps []AppSummary `json:"apps"`
}

type AppsPackagePreviewRequest struct {
	StagingID     string `json:"staging_id"`
	DownloadURL   string `json:"download_url"`
	DownloadToken string `json:"download_token"`
}

type AppsPackagePreviewResponse struct {
	StagingID string     `json:"staging_id"`
	App       AppSummary `json:"app"`
	Warnings  []string   `json:"warnings,omitempty"`
	Replacing bool       `json:"replacing"`
}

type AppsPackageActivateRequest struct {
	StagingID string `json:"staging_id"`
	Enable    bool   `json:"enable"`
}

type AppsPackageActivateResponse struct {
	App AppSummary `json:"app"`
}

type AppsConfigStatusRequest struct {
	AppID string `json:"app_id,omitempty"`
}

type AppsConfigStatusResponse struct {
	Apps []AppSummary `json:"apps"`
}

type AppsConfigApplyRequest struct {
	AppID        string          `json:"app_id"`
	PublicConfig json.RawMessage `json:"public_config,omitempty"`
	Secrets      json.RawMessage `json:"secrets,omitempty"`
	Enable       *bool           `json:"enable,omitempty"`
}

type AppsConfigApplyResponse struct {
	App AppSummary `json:"app"`
}

type AppsInvokeRequest struct {
	AppID     string          `json:"app_id"`
	CommandID string          `json:"command_id"`
	Input     json.RawMessage `json:"input,omitempty"`
	Context   json.RawMessage `json:"context,omitempty"`
}

type AppsInvokeResponse struct {
	Output json.RawMessage `json:"output,omitempty"`
	JobID  string          `json:"job_id,omitempty"`
}

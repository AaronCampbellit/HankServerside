package protocol

import (
	"encoding/json"
	"time"
)

type ConfigStatusRequest struct {
	ServiceType string `json:"service_type,omitempty"`
}

type ServiceProfileSnapshot struct {
	ServiceType    string          `json:"service_type"`
	PublicConfig   json.RawMessage `json:"public_config,omitempty"`
	SecretVersion  int             `json:"secret_version"`
	AppliedVersion int             `json:"applied_version"`
	Status         string          `json:"status"`
	LastError      string          `json:"last_error,omitempty"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

type ConfigStatusResponse struct {
	Profiles []ServiceProfileSnapshot `json:"profiles"`
}

type ConfigApplyRequest struct {
	ServiceType   string          `json:"service_type"`
	PublicConfig  json.RawMessage `json:"public_config,omitempty"`
	Secrets       json.RawMessage `json:"secrets,omitempty"`
	SecretVersion int             `json:"secret_version"`
	Persist       bool            `json:"persist"`
}

type ConfigApplyResponse struct {
	Profile ServiceProfileSnapshot `json:"profile"`
}

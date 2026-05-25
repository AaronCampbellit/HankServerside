package cloud

import (
	"errors"
	"net/http"

	"github.com/dropfile/hankremote/internal/domain"
	"github.com/dropfile/hankremote/internal/protocol"
)

type assistantMediaSettingsResponse struct {
	Online             bool                              `json:"online"`
	CanEdit            bool                              `json:"can_edit"`
	Settings           protocol.MediaSettings            `json:"settings"`
	DestinationOptions []protocol.MediaDestinationOption `json:"destination_options,omitempty"`
	Jobs               []protocol.MediaDownloadJobStatus `json:"jobs"`
	Error              string                            `json:"error,omitempty"`
}

type assistantMediaJobCancelResponse struct {
	Online bool                            `json:"online"`
	Job    protocol.MediaDownloadJobStatus `json:"job"`
}

type assistantMediaJobStatusResponse struct {
	Online bool                            `json:"online"`
	Job    protocol.MediaDownloadJobStatus `json:"job"`
}

func (s *Server) handleAssistantMediaSettings(w http.ResponseWriter, r *http.Request, home domain.Home, membership domain.HomeMembership) {
	switch r.Method {
	case http.MethodGet:
		response, err := s.fetchAssistantMediaSettings(r, home, membership)
		if err != nil {
			response.Online = false
			response.CanEdit = membership.Role == domain.HomeRoleAdmin
			response.Settings = defaultAssistantMediaSettings()
			response.DestinationOptions = defaultAssistantMediaDestinationOptions(response.Settings.DestinationPath)
			response.Error = err.Error()
		}
		writeJSON(w, http.StatusOK, response)
	case http.MethodPut:
		if membership.Role != domain.HomeRoleAdmin {
			http.Error(w, "admin role required", http.StatusForbidden)
			return
		}
		var request protocol.MediaSettingsApplyRequest
		if err := parseJSON(w, r, &request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		request.Persist = true
		envelope, err := s.sendAgentCommand(r.Context(), home.ID, protocol.CommandMediaSettingsApply, request)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if envelope.Error != nil {
			http.Error(w, envelope.Error.Message, http.StatusBadGateway)
			return
		}
		payload, err := protocol.DecodePayload[protocol.MediaSettingsApplyResponse](envelope)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, http.StatusOK, assistantMediaSettingsResponse{
			Online:             true,
			CanEdit:            true,
			Settings:           payload.Settings,
			DestinationOptions: defaultAssistantMediaDestinationOptions(payload.Settings.DestinationPath),
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) fetchAssistantMediaSettings(r *http.Request, home domain.Home, membership domain.HomeMembership) (assistantMediaSettingsResponse, error) {
	envelope, err := s.sendAgentCommand(r.Context(), home.ID, protocol.CommandMediaSettingsStatus, protocol.MediaSettingsStatusRequest{})
	if err != nil {
		return assistantMediaSettingsResponse{}, err
	}
	if envelope.Error != nil {
		return assistantMediaSettingsResponse{}, errors.New(envelope.Error.Message)
	}
	payload, err := protocol.DecodePayload[protocol.MediaSettingsStatusResponse](envelope)
	if err != nil {
		return assistantMediaSettingsResponse{}, err
	}
	return assistantMediaSettingsResponse{
		Online:             true,
		CanEdit:            membership.Role == domain.HomeRoleAdmin,
		Settings:           payload.Settings,
		DestinationOptions: payload.DestinationOptions,
		Jobs:               payload.Jobs,
	}, nil
}

func (s *Server) handleAssistantMediaJobStatus(w http.ResponseWriter, r *http.Request, home domain.Home, _ domain.HomeMembership, jobID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	envelope, err := s.sendAgentCommand(r.Context(), home.ID, protocol.CommandMediaDownloadStatus, protocol.MediaDownloadStatusRequest{JobID: jobID})
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if envelope.Error != nil {
		http.Error(w, envelope.Error.Message, http.StatusBadGateway)
		return
	}
	payload, err := protocol.DecodePayload[protocol.MediaDownloadStatusResponse](envelope)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, assistantMediaJobStatusResponse{Online: true, Job: payload.Job})
}

func (s *Server) handleAssistantMediaJobCancel(w http.ResponseWriter, r *http.Request, home domain.Home, membership domain.HomeMembership, jobID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if membership.Role != domain.HomeRoleAdmin {
		http.Error(w, "admin role required", http.StatusForbidden)
		return
	}
	envelope, err := s.sendAgentCommand(r.Context(), home.ID, protocol.CommandMediaDownloadCancel, protocol.MediaDownloadCancelRequest{JobID: jobID})
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if envelope.Error != nil {
		http.Error(w, envelope.Error.Message, http.StatusBadGateway)
		return
	}
	payload, err := protocol.DecodePayload[protocol.MediaDownloadCancelResponse](envelope)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, assistantMediaJobCancelResponse{Online: true, Job: payload.Job})
}

func defaultAssistantMediaSettings() protocol.MediaSettings {
	return protocol.MediaSettings{
		BaseURL:             "https://gramaton.io",
		PreferredQuality:    "1080p",
		RequireConfirmation: true,
	}
}

func defaultAssistantMediaDestinationOptions(current string) []protocol.MediaDestinationOption {
	options := []protocol.MediaDestinationOption{{Value: "", Label: "Media root"}}
	if current != "" {
		options = append(options, protocol.MediaDestinationOption{
			Value: current,
			Label: "Media root/" + current,
		})
	}
	return options
}

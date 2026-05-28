package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	agentfiles "github.com/dropfile/hankremote/internal/agent/files"
	agentha "github.com/dropfile/hankremote/internal/agent/homeassistant"
	agentmedia "github.com/dropfile/hankremote/internal/agent/media"
	agentnotes "github.com/dropfile/hankremote/internal/agent/notes"
	"github.com/dropfile/hankremote/internal/protocol"
)

type commandDispatcher struct {
	ha     *agentha.Client
	files  *agentfiles.Service
	media  *agentmedia.Service
	notes  *agentnotes.Service
	config *configManager
}

func (d *commandDispatcher) dispatch(ctx context.Context, command protocol.RoutedCommand) (any, *protocol.ErrorPayload) {
	switch command.Command {
	case protocol.CommandSystemPing:
		request, err := decodeBody[protocol.SystemPingRequest](command.Body)
		if err != nil {
			return nil, badRequest("invalid_ping_request", err)
		}
		message := "pong"
		if request.Message != "" {
			message = "pong: " + request.Message
		}
		return protocol.SystemPingResponse{
			Message: message,
			Time:    time.Now().UTC(),
		}, nil

	case "homeassistant.health":
		if err := d.ha.Health(ctx); err != nil {
			return nil, mapError(err)
		}
		return protocol.HomeAssistantHealthResponse{OK: true, CheckedAt: time.Now().UTC()}, nil

	case "homeassistant.fetch_states":
		states, err := d.ha.FetchStates(ctx)
		if err != nil {
			return nil, mapError(err)
		}
		return protocol.HomeAssistantFetchStatesResponse{States: states}, nil

	case "homeassistant.fetch_state":
		request, err := decodeBody[protocol.HomeAssistantFetchStateRequest](command.Body)
		if err != nil {
			return nil, badRequest("invalid_homeassistant_request", err)
		}
		state, err := d.ha.FetchState(ctx, request.EntityID)
		if err != nil {
			return nil, mapError(err)
		}
		return protocol.HomeAssistantFetchStateResponse{State: state}, nil

	case "homeassistant.call_service":
		request, err := decodeBody[protocol.HomeAssistantCallServiceRequest](command.Body)
		if err != nil {
			return nil, badRequest("invalid_homeassistant_request", err)
		}
		result, err := d.ha.CallService(ctx, request.Domain, request.Service, request.Body)
		if err != nil {
			return nil, mapError(err)
		}
		return protocol.HomeAssistantCallServiceResponse{Result: result}, nil

	case "files.list":
		request, err := decodeBody[protocol.FilesListRequest](command.Body)
		if err != nil {
			return nil, badRequest("invalid_file_request", err)
		}
		items, err := d.files.ListSource(ctx, request.SourceID, request.Path)
		if err != nil {
			return nil, mapError(err)
		}
		return protocol.FilesListResponse{Items: items}, nil

	case "files.stat":
		request, err := decodeBody[protocol.FilesStatRequest](command.Body)
		if err != nil {
			return nil, badRequest("invalid_file_request", err)
		}
		item, err := d.files.StatSource(ctx, request.SourceID, request.Path)
		if err != nil {
			return nil, mapError(err)
		}
		return protocol.FilesStatResponse{Item: item}, nil

	case "files.search":
		request, err := decodeBody[protocol.FilesSearchRequest](command.Body)
		if err != nil {
			return nil, badRequest("invalid_file_request", err)
		}
		items, err := d.files.SearchSource(ctx, request.SourceID, request.Query, request.Limit)
		if err != nil {
			return nil, mapError(err)
		}
		return protocol.FilesSearchResponse{Items: items}, nil

	case "files.create_directory":
		request, err := decodeBody[protocol.FilesCreateDirectoryRequest](command.Body)
		if err != nil {
			return nil, badRequest("invalid_file_request", err)
		}
		if err := d.files.CreateDirectorySource(ctx, request.SourceID, request.Path); err != nil {
			return nil, mapError(err)
		}
		return protocol.EmptyResponse{OK: true}, nil

	case "files.rename":
		request, err := decodeBody[protocol.FilesRenameRequest](command.Body)
		if err != nil {
			return nil, badRequest("invalid_file_request", err)
		}
		if err := d.files.RenameSource(ctx, request.SourceID, request.From, request.To); err != nil {
			return nil, mapError(err)
		}
		return protocol.EmptyResponse{OK: true}, nil

	case "files.delete":
		request, err := decodeBody[protocol.FilesDeleteRequest](command.Body)
		if err != nil {
			return nil, badRequest("invalid_file_request", err)
		}
		if err := d.files.DeleteSource(ctx, request.SourceID, request.Path, request.IsDirectory); err != nil {
			return nil, mapError(err)
		}
		return protocol.EmptyResponse{OK: true}, nil

	case "files.download":
		request, err := decodeBody[protocol.FilesDownloadRequest](command.Body)
		if err != nil {
			return nil, badRequest("invalid_file_request", err)
		}
		contentBase64, err := d.files.DownloadSource(ctx, request.SourceID, request.Path)
		if err != nil {
			return nil, mapError(err)
		}
		return protocol.FilesDownloadResponse{Path: request.Path, ContentBase64: contentBase64}, nil

	case "files.upload":
		request, err := decodeBody[protocol.FilesUploadRequest](command.Body)
		if err != nil {
			return nil, badRequest("invalid_file_request", err)
		}
		if err := d.files.UploadSource(ctx, request.SourceID, request.Path, request.ContentBase64); err != nil {
			return nil, mapError(err)
		}
		return protocol.EmptyResponse{OK: true}, nil

	case protocol.CommandMediaSearch:
		if d.media == nil || !d.media.Enabled() {
			return nil, mapError(fmt.Errorf("media source is not configured"))
		}
		request, err := decodeBody[protocol.MediaSearchRequest](command.Body)
		if err != nil {
			return nil, badRequest("invalid_media_request", err)
		}
		response, err := d.media.Search(ctx, request.Query, request.Limit)
		if err != nil {
			return nil, mapError(err)
		}
		return response, nil

	case protocol.CommandMediaPlanDownload:
		if d.media == nil || !d.media.Enabled() {
			return nil, mapError(fmt.Errorf("media source is not configured"))
		}
		request, err := decodeBody[protocol.MediaPlanDownloadRequest](command.Body)
		if err != nil {
			return nil, badRequest("invalid_media_request", err)
		}
		response, err := d.media.PlanDownload(ctx, request)
		if err != nil {
			return nil, mapError(err)
		}
		return response, nil

	case protocol.CommandMediaDownloadStart:
		if d.media == nil || !d.media.Enabled() {
			return nil, mapError(fmt.Errorf("media source is not configured"))
		}
		request, err := decodeBody[protocol.MediaDownloadStartRequest](command.Body)
		if err != nil {
			return nil, badRequest("invalid_media_request", err)
		}
		response, err := d.media.StartDownload(ctx, request)
		if err != nil {
			return nil, mapError(err)
		}
		return response, nil

	case protocol.CommandMediaDownloadStatus:
		if d.media == nil || !d.media.Enabled() {
			return nil, mapError(fmt.Errorf("media source is not configured"))
		}
		request, err := decodeBody[protocol.MediaDownloadStatusRequest](command.Body)
		if err != nil {
			return nil, badRequest("invalid_media_request", err)
		}
		response, err := d.media.Status(ctx, request.JobID)
		if err != nil {
			return nil, mapError(err)
		}
		return response, nil

	case protocol.CommandMediaImageFetch:
		if d.media == nil || !d.media.Enabled() {
			return nil, mapError(fmt.Errorf("media source is not configured"))
		}
		request, err := decodeBody[protocol.MediaImageFetchRequest](command.Body)
		if err != nil {
			return nil, badRequest("invalid_media_image_request", err)
		}
		response, err := d.media.FetchImage(ctx, request.URL)
		if err != nil {
			return nil, mapError(err)
		}
		return response, nil

	case protocol.CommandMediaSettingsStatus:
		if d.media == nil {
			return nil, mapError(fmt.Errorf("media source is not configured"))
		}
		return d.media.Settings(ctx), nil

	case protocol.CommandMediaSettingsApply:
		if d.media == nil {
			return nil, mapError(fmt.Errorf("media source is not configured"))
		}
		request, err := decodeBody[protocol.MediaSettingsApplyRequest](command.Body)
		if err != nil {
			return nil, badRequest("invalid_media_settings_request", err)
		}
		response, err := d.media.ApplySettings(ctx, request)
		if err != nil {
			return nil, mapError(err)
		}
		return response, nil

	case protocol.CommandMediaDownloadJobs:
		if d.media == nil {
			return nil, mapError(fmt.Errorf("media source is not configured"))
		}
		return d.media.Jobs(ctx), nil

	case protocol.CommandMediaDownloadCancel:
		if d.media == nil {
			return nil, mapError(fmt.Errorf("media source is not configured"))
		}
		request, err := decodeBody[protocol.MediaDownloadCancelRequest](command.Body)
		if err != nil {
			return nil, badRequest("invalid_media_cancel_request", err)
		}
		response, err := d.media.Cancel(ctx, request.JobID)
		if err != nil {
			return nil, mapError(err)
		}
		return response, nil

	case "notes.list":
		notes, err := d.notes.List(ctx)
		if err != nil {
			return nil, mapError(err)
		}
		return protocol.NotesListResponse{Notes: notes}, nil

	case "notes.fetch":
		request, err := decodeBody[protocol.NotesFetchRequest](command.Body)
		if err != nil {
			return nil, badRequest("invalid_note_request", err)
		}
		response, err := d.notes.Fetch(ctx, request.NoteID)
		if err != nil {
			return nil, mapError(err)
		}
		return response, nil

	case "notes.save":
		request, err := decodeBody[protocol.NotesSaveRequest](command.Body)
		if err != nil {
			return nil, badRequest("invalid_note_request", err)
		}
		response, err := d.notes.Save(ctx, request.NoteID, request.Title, request.Content, request.ExpectedRevision, request.PageType, request.Board)
		if err != nil {
			return nil, mapError(err)
		}
		return response, nil

	case "notes.rename":
		request, err := decodeBody[protocol.NotesRenameRequest](command.Body)
		if err != nil {
			return nil, badRequest("invalid_note_request", err)
		}
		if err := d.notes.Rename(ctx, request.NoteID, request.Title); err != nil {
			return nil, mapError(err)
		}
		return protocol.EmptyResponse{OK: true}, nil

	case "notes.delete":
		request, err := decodeBody[protocol.NotesDeleteRequest](command.Body)
		if err != nil {
			return nil, badRequest("invalid_note_request", err)
		}
		if err := d.notes.Delete(ctx, request.NoteID); err != nil {
			return nil, mapError(err)
		}
		return protocol.EmptyResponse{OK: true}, nil

	case "notes.sync":
		notes, err := d.notes.Sync(ctx)
		if err != nil {
			return nil, mapError(err)
		}
		return protocol.NotesSyncResponse{Notes: notes}, nil

	case "notes.search":
		request, err := decodeBody[protocol.NotesSearchRequest](command.Body)
		if err != nil {
			return nil, badRequest("invalid_note_request", err)
		}
		results, err := d.notes.Search(ctx, request.Query, request.Limit)
		if err != nil {
			return nil, mapError(err)
		}
		return protocol.NotesSearchResponse{Results: results}, nil

	case "notes.tags":
		tags, err := d.notes.Tags(ctx)
		if err != nil {
			return nil, mapError(err)
		}
		return protocol.NotesTagsResponse{Tags: tags}, nil

	case "notes.tag_rollup":
		request, err := decodeBody[protocol.NotesTagRollupRequest](command.Body)
		if err != nil {
			return nil, badRequest("invalid_note_request", err)
		}
		items, err := d.notes.TagRollup(ctx, request.Tag)
		if err != nil {
			return nil, mapError(err)
		}
		return protocol.NotesTagRollupResponse{Items: items}, nil

	case "config.status":
		request, err := decodeBody[protocol.ConfigStatusRequest](command.Body)
		if err != nil {
			return nil, badRequest("invalid_config_request", err)
		}
		profiles, err := d.config.Status(ctx, request.ServiceType)
		if err != nil {
			return nil, mapError(err)
		}
		return protocol.ConfigStatusResponse{Profiles: profiles}, nil

	case "config.apply":
		request, err := decodeBody[protocol.ConfigApplyRequest](command.Body)
		if err != nil {
			return nil, badRequest("invalid_config_request", err)
		}
		profile, err := d.config.Apply(ctx, request)
		if err != nil {
			return nil, mapError(err)
		}
		return protocol.ConfigApplyResponse{Profile: profile}, nil

	default:
		return nil, &protocol.ErrorPayload{
			Code:    "unsupported_command",
			Message: fmt.Sprintf("unsupported command %q", command.Command),
		}
	}
}

func decodeBody[T any](body json.RawMessage) (T, error) {
	var out T
	if len(body) == 0 {
		return out, nil
	}
	err := json.Unmarshal(body, &out)
	return out, err
}

func badRequest(code string, err error) *protocol.ErrorPayload {
	return &protocol.ErrorPayload{
		Code:    code,
		Message: err.Error(),
	}
}

func mapError(err error) *protocol.ErrorPayload {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, agentha.ErrDisabled):
		return &protocol.ErrorPayload{Code: "homeassistant_not_configured", Message: err.Error()}
	case errors.Is(err, agentfiles.ErrDisabled):
		return &protocol.ErrorPayload{Code: "files_not_configured", Message: err.Error()}
	case errors.Is(err, agentnotes.ErrDisabled):
		return &protocol.ErrorPayload{Code: "notes_not_configured", Message: err.Error()}
	case errors.Is(err, errUnsupportedServiceType):
		return &protocol.ErrorPayload{Code: "unsupported_service_type", Message: err.Error()}
	case errors.Is(err, agentnotes.ErrConflict):
		conflict := &agentnotes.ConflictError{}
		payload := &protocol.ErrorPayload{Code: "note_conflict", Message: err.Error()}
		if errors.As(err, &conflict) {
			payload.Details = map[string]any{
				"current": conflict.Current,
			}
		}
		return payload
	case errors.Is(err, context.DeadlineExceeded):
		return &protocol.ErrorPayload{Code: "request_timeout", Message: err.Error()}
	case errors.Is(err, context.Canceled):
		return &protocol.ErrorPayload{Code: "request_canceled", Message: err.Error()}
	case errors.Is(err, os.ErrNotExist):
		return &protocol.ErrorPayload{Code: "not_found", Message: err.Error()}
	default:
		return &protocol.ErrorPayload{Code: "upstream_error", Message: err.Error()}
	}
}

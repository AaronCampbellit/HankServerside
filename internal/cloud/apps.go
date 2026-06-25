package cloud

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/dropfile/hankremote/internal/domain"
	"github.com/dropfile/hankremote/internal/protocol"
	storepkg "github.com/dropfile/hankremote/internal/store"
)

const (
	maxAppPackageBytes = 32 << 20
	appPackageTTL      = 15 * time.Minute
)

var (
	errAppInvokeBadPayload   = errors.New("invalid apps.invoke payload")
	errAppInvokeUnavailable  = errors.New("app is not installed, enabled, or command-capable")
	errAppInvokeAccessDenied = errors.New("app is not available to this home member")
)

type appPackageStagingRecord struct {
	HomeID        string
	StagingID     string
	DownloadToken string
	Bytes         []byte
	CreatedAt     time.Time
	ExpiresAt     time.Time
}

type appPackageStagingRegistry struct {
	mu       sync.Mutex
	packages map[string]appPackageStagingRecord
}

func newAppPackageStagingRegistry() *appPackageStagingRegistry {
	return &appPackageStagingRegistry{packages: make(map[string]appPackageStagingRecord)}
}

func (r *appPackageStagingRegistry) Put(homeID string, data []byte) appPackageStagingRecord {
	now := time.Now().UTC()
	record := appPackageStagingRecord{
		HomeID:        homeID,
		StagingID:     newID("appstage"),
		DownloadToken: newToken(),
		Bytes:         append([]byte(nil), data...),
		CreatedAt:     now,
		ExpiresAt:     now.Add(appPackageTTL),
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneLocked(now)
	r.packages[record.StagingID] = record
	return record
}

func (r *appPackageStagingRegistry) Get(stagingID string) (appPackageStagingRecord, bool) {
	now := time.Now().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneLocked(now)
	record, ok := r.packages[stagingID]
	if !ok || !record.ExpiresAt.After(now) {
		if ok {
			delete(r.packages, stagingID)
		}
		return appPackageStagingRecord{}, false
	}
	return record, true
}

func (r *appPackageStagingRegistry) Delete(stagingID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.packages, stagingID)
}

func (r *appPackageStagingRegistry) pruneLocked(now time.Time) {
	for stagingID, record := range r.packages {
		if !record.ExpiresAt.After(now) {
			delete(r.packages, stagingID)
		}
	}
}

func (s *Server) handleHomeAppPackageDownload(w http.ResponseWriter, r *http.Request, parts []string) bool {
	if len(parts) != 3 || parts[0] != "apps" || parts[1] != "packages" || parts[2] == "" {
		return false
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return true
	}
	stagingID := strings.TrimSpace(parts[2])
	record, ok := s.appPackages.Get(stagingID)
	if !ok {
		http.NotFound(w, r)
		return true
	}
	if _, ok := s.authenticateAgentPackageDownload(r, record.HomeID); !ok {
		http.Error(w, "unauthorized agent", http.StatusUnauthorized)
		return true
	}
	token := strings.TrimSpace(r.Header.Get("X-Hank-App-Package-Token"))
	if token == "" || token != record.DownloadToken {
		http.Error(w, "unauthorized package download", http.StatusUnauthorized)
		return true
	}
	w.Header().Set("Content-Type", "application/vnd.hank.app-package")
	w.Header().Set("Content-Disposition", `attachment; filename="`+stagingID+`.hankapp"`)
	http.ServeContent(w, r, stagingID+".hankapp", record.CreatedAt, bytes.NewReader(record.Bytes))
	return true
}

func (s *Server) handleHomeApps(w http.ResponseWriter, r *http.Request, home domain.Home, auth authContext, membership domain.HomeMembership, parts []string) bool {
	if len(parts) == 1 && parts[0] == "apps" && r.Method == http.MethodGet {
		if _, ok := s.router.GetAgent(home.ID); ok {
			envelope, err := s.sendAgentCommand(r.Context(), home.ID, protocol.CommandAppsList, protocol.AppsListRequest{})
			if err == nil && envelope.Error == nil {
				response, err := protocol.DecodePayload[protocol.AppsListResponse](envelope)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadGateway)
					return true
				}
				for _, app := range response.Apps {
					if err := s.persistAgentApp(r, home.ID, auth.User.ID, app); err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
						return true
					}
				}
			}
		}
		apps, err := s.store.ListHomeApps(r.Context(), home.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return true
		}
		summaries, err := persistedAppSummaries(apps)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return true
		}
		summaries = filterAppSummariesForMembership(summaries, membership)
		writeJSON(w, http.StatusOK, protocol.AppsListResponse{Apps: summaries})
		return true
	}

	if len(parts) == 3 && parts[0] == "apps" && parts[1] == "import" && parts[2] == "preview" && r.Method == http.MethodPost {
		if membership.Role != domain.HomeRoleAdmin {
			http.Error(w, errAdminRoleRequired.Error(), http.StatusForbidden)
			return true
		}
		if _, ok := s.router.GetAgent(home.ID); !ok {
			writeJSON(w, http.StatusConflict, map[string]any{"error": "agent_offline"})
			return true
		}
		upload, err := readAppPackageUpload(w, r)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, errAppPackageTooLarge) {
				status = http.StatusRequestEntityTooLarge
			}
			http.Error(w, err.Error(), status)
			return true
		}
		staged := s.appPackages.Put(home.ID, upload.Bytes)
		downloadURL := absoluteRequestURL(r, "/v1/home/apps/packages/"+staged.StagingID)
		envelope, err := s.sendAgentCommand(r.Context(), home.ID, protocol.CommandAppsPackagePreview, protocol.AppsPackagePreviewRequest{
			StagingID:     staged.StagingID,
			DownloadURL:   downloadURL,
			DownloadToken: staged.DownloadToken,
			PackageKind:   upload.PackageKind,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return true
		}
		if envelope.Error != nil {
			http.Error(w, envelope.Error.Message, http.StatusBadGateway)
			return true
		}
		response, err := protocol.DecodePayload[protocol.AppsPackagePreviewResponse](envelope)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return true
		}
		writeJSON(w, http.StatusOK, response)
		return true
	}

	if len(parts) == 3 && parts[0] == "apps" && parts[1] == "import" && parts[2] == "activate" && r.Method == http.MethodPost {
		if membership.Role != domain.HomeRoleAdmin {
			http.Error(w, errAdminRoleRequired.Error(), http.StatusForbidden)
			return true
		}
		var body protocol.AppsPackageActivateRequest
		if err := parseJSON(w, r, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return true
		}
		envelope, err := s.sendAgentCommand(r.Context(), home.ID, protocol.CommandAppsPackageActivate, body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return true
		}
		if envelope.Error != nil {
			http.Error(w, envelope.Error.Message, http.StatusBadGateway)
			return true
		}
		response, err := protocol.DecodePayload[protocol.AppsPackageActivateResponse](envelope)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return true
		}
		response.App.UserAccess = domain.HomeAgentAppUserAccessAdminsOnly
		if err := s.persistAgentApp(r, home.ID, auth.User.ID, response.App); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return true
		}
		s.appPackages.Delete(body.StagingID)
		writeJSON(w, http.StatusOK, response)
		return true
	}

	if len(parts) == 3 && parts[0] == "apps" && parts[2] == "config" && r.Method == http.MethodPut {
		if membership.Role != domain.HomeRoleAdmin {
			http.Error(w, errAdminRoleRequired.Error(), http.StatusForbidden)
			return true
		}
		var body protocol.AppsConfigApplyRequest
		if err := parseJSON(w, r, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return true
		}
		body.AppID = parts[1]
		requestedUserAccess := ""
		if strings.TrimSpace(body.UserAccess) != "" {
			requestedUserAccess = normalizeAppUserAccess(body.UserAccess)
			if requestedUserAccess == "" {
				http.Error(w, "invalid app user_access", http.StatusBadRequest)
				return true
			}
			body.UserAccess = requestedUserAccess
		}
		envelope, err := s.sendAgentCommand(r.Context(), home.ID, protocol.CommandAppsConfigApply, body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return true
		}
		if envelope.Error != nil {
			http.Error(w, envelope.Error.Message, http.StatusBadGateway)
			return true
		}
		response, err := protocol.DecodePayload[protocol.AppsConfigApplyResponse](envelope)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return true
		}
		if requestedUserAccess != "" {
			response.App.UserAccess = requestedUserAccess
		}
		if err := s.persistAgentApp(r, home.ID, auth.User.ID, response.App); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return true
		}
		writeJSON(w, http.StatusOK, response)
		return true
	}

	return false
}

func (s *Server) authenticateAgentPackageDownload(r *http.Request, homeID string) (domain.Agent, bool) {
	agentID := strings.TrimSpace(r.Header.Get("X-Hank-Agent-ID"))
	token, err := bearerToken(r.Header.Get("Authorization"))
	if agentID == "" || err != nil {
		return domain.Agent{}, false
	}
	record, err := s.store.ValidateAgentToken(r.Context(), hashToken(token))
	if err != nil || record.Agent.ID != agentID || record.Home.ID != homeID {
		return domain.Agent{}, false
	}
	return record.Agent, true
}

func (s *Server) persistAgentApp(r *http.Request, homeID string, updatedBy string, app protocol.AppSummary) error {
	now := time.Now().UTC()
	publicConfig := strings.TrimSpace(string(app.PublicConfig))
	if publicConfig == "" {
		publicConfig = "{}"
	}
	secretFieldsSet, err := marshalAppMetadataObject(app.SecretFieldsSet)
	if err != nil {
		return err
	}
	settingsSchema, err := marshalAppMetadataObject(app.SettingsSchema)
	if err != nil {
		return err
	}
	capabilities, err := marshalAppMetadataArray(app.Capabilities)
	if err != nil {
		return err
	}
	slashCommands, err := marshalAppMetadataArray(app.SlashCommands)
	if err != nil {
		return err
	}
	commands, err := marshalAppMetadataArray(app.Commands)
	if err != nil {
		return err
	}
	userAccess := normalizeAppUserAccess(app.UserAccess)
	if userAccess == "" {
		if existing, err := s.store.GetHomeApp(r.Context(), homeID, app.ID); err == nil {
			userAccess = normalizeAppUserAccess(existing.UserAccess)
		} else if !errors.Is(err, storepkg.ErrNotFound) {
			return err
		}
	}
	if userAccess == "" {
		userAccess = domain.HomeAgentAppUserAccessAdminsOnly
	}
	status := strings.TrimSpace(app.Status)
	if status == "" {
		status = domain.SyncStatusPending
	}
	return s.store.UpsertHomeApp(r.Context(), domain.HomeAgentApp{
		HomeID:              homeID,
		AppID:               app.ID,
		Name:                app.Name,
		Version:             app.Version,
		Enabled:             app.Enabled,
		PublicConfigJSON:    publicConfig,
		SecretFieldsSetJSON: secretFieldsSet,
		SettingsSchemaJSON:  settingsSchema,
		CapabilitiesJSON:    capabilities,
		SlashCommandsJSON:   slashCommands,
		CommandsJSON:        commands,
		UserAccess:          userAccess,
		Status:              status,
		LastError:           app.LastError,
		UpdatedAt:           now,
		UpdatedBy:           updatedBy,
	})
}

func persistedAppSummaries(apps []domain.HomeAgentApp) ([]protocol.AppSummary, error) {
	summaries := make([]protocol.AppSummary, 0, len(apps))
	for _, app := range apps {
		summary := protocol.AppSummary{
			ID:           app.AppID,
			Name:         app.Name,
			Version:      app.Version,
			Enabled:      app.Enabled,
			Status:       app.Status,
			LastError:    app.LastError,
			UserAccess:   normalizeAppUserAccess(app.UserAccess),
			PublicConfig: json.RawMessage(defaultJSONObject(app.PublicConfigJSON)),
		}
		if err := unmarshalAppMetadata(defaultJSONObject(app.SecretFieldsSetJSON), &summary.SecretFieldsSet); err != nil {
			return nil, fmt.Errorf("decode %s secret fields: %w", app.AppID, err)
		}
		if err := unmarshalAppMetadata(defaultJSONObject(app.SettingsSchemaJSON), &summary.SettingsSchema); err != nil {
			return nil, fmt.Errorf("decode %s settings schema: %w", app.AppID, err)
		}
		if err := unmarshalAppMetadata(defaultJSONArray(app.CapabilitiesJSON), &summary.Capabilities); err != nil {
			return nil, fmt.Errorf("decode %s capabilities: %w", app.AppID, err)
		}
		if err := unmarshalAppMetadata(defaultJSONArray(app.SlashCommandsJSON), &summary.SlashCommands); err != nil {
			return nil, fmt.Errorf("decode %s slash commands: %w", app.AppID, err)
		}
		if err := unmarshalAppMetadata(defaultJSONArray(app.CommandsJSON), &summary.Commands); err != nil {
			return nil, fmt.Errorf("decode %s commands: %w", app.AppID, err)
		}
		summaries = append(summaries, summary)
	}
	return summaries, nil
}

func filterAppSummariesForMembership(apps []protocol.AppSummary, membership domain.HomeMembership) []protocol.AppSummary {
	if membership.Role == domain.HomeRoleAdmin {
		return apps
	}
	filtered := make([]protocol.AppSummary, 0, len(apps))
	for _, app := range apps {
		if !app.Enabled {
			continue
		}
		if normalizeAppUserAccess(app.UserAccess) != domain.HomeAgentAppUserAccessHomeMembers {
			continue
		}
		filtered = append(filtered, app)
	}
	return filtered
}

func canUseHomeAgentApp(app domain.HomeAgentApp, membership domain.HomeMembership) bool {
	if !app.Enabled {
		return false
	}
	if membership.Role == domain.HomeRoleAdmin {
		return true
	}
	return normalizeAppUserAccess(app.UserAccess) == domain.HomeAgentAppUserAccessHomeMembers
}

func (s *Server) authorizeAppsInvokeCommand(ctx context.Context, homeID string, membership domain.HomeMembership, command protocol.RoutedCommand) error {
	if command.Command != protocol.CommandAppsInvoke {
		return nil
	}
	var request protocol.AppsInvokeRequest
	if err := json.Unmarshal(command.Body, &request); err != nil {
		return fmt.Errorf("%w: %v", errAppInvokeBadPayload, err)
	}
	if strings.TrimSpace(request.AppID) == "" || strings.TrimSpace(request.CommandID) == "" {
		return fmt.Errorf("%w: app_id and command_id are required", errAppInvokeBadPayload)
	}
	app, err := s.store.GetHomeApp(ctx, homeID, request.AppID)
	if errors.Is(err, storepkg.ErrNotFound) {
		return fmt.Errorf("%w: %s", errAppInvokeUnavailable, request.AppID)
	}
	if err != nil {
		return err
	}
	if !canUseHomeAgentApp(app, membership) {
		if !app.Enabled {
			return fmt.Errorf("%w: %s", errAppInvokeUnavailable, request.AppID)
		}
		return fmt.Errorf("%w: %s", errAppInvokeAccessDenied, request.AppID)
	}
	if !homeAgentAppHasCommand(app, request.CommandID) {
		return fmt.Errorf("%w: %s.%s", errAppInvokeUnavailable, request.AppID, request.CommandID)
	}
	return nil
}

func homeAgentAppHasCommand(app domain.HomeAgentApp, commandID string) bool {
	commandID = strings.TrimSpace(commandID)
	if commandID == "" {
		return false
	}
	var commands []protocol.AppCommandSummary
	if err := json.Unmarshal([]byte(defaultJSONArray(app.CommandsJSON)), &commands); err != nil {
		return false
	}
	for _, command := range commands {
		if command.ID == commandID {
			return true
		}
	}
	return false
}

func normalizeAppUserAccess(value string) string {
	switch strings.TrimSpace(value) {
	case "", "admins_only":
		return domain.HomeAgentAppUserAccessAdminsOnly
	case "home_members":
		return domain.HomeAgentAppUserAccessHomeMembers
	default:
		return ""
	}
}

func marshalAppMetadataObject(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	if string(raw) == "null" {
		return "{}", nil
	}
	return string(raw), nil
}

func marshalAppMetadataArray(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	if string(raw) == "null" {
		return "[]", nil
	}
	return string(raw), nil
}

func defaultJSONObject(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "{}"
	}
	return value
}

func defaultJSONArray(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "[]"
	}
	return value
}

func unmarshalAppMetadata(raw string, target any) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return json.Unmarshal([]byte(raw), target)
}

var errAppPackageTooLarge = errors.New("app package too large")

type appPackageUpload struct {
	Bytes       []byte
	PackageKind string
}

func readAppPackageUpload(w http.ResponseWriter, r *http.Request) (appPackageUpload, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAppPackageBytes+1<<20)
	contentType := r.Header.Get("Content-Type")
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if mediaType == "multipart/form-data" {
		if err := r.ParseMultipartForm(maxAppPackageBytes); err != nil {
			return appPackageUpload{}, err
		}
		if files := r.MultipartForm.File["source_files"]; len(files) > 0 {
			data, err := zipAppSourceUpload(files)
			if err != nil {
				return appPackageUpload{}, err
			}
			return appPackageUpload{Bytes: data, PackageKind: protocol.AppPackageKindSourceArchive}, nil
		}
		file, _, err := r.FormFile("package")
		if err != nil {
			file, _, err = r.FormFile("file")
		}
		if err != nil {
			return appPackageUpload{}, fmt.Errorf("package file is required")
		}
		defer file.Close()
		data, err := readBoundedAppPackage(file)
		if err != nil {
			return appPackageUpload{}, err
		}
		return appPackageUpload{Bytes: data, PackageKind: protocol.AppPackageKindArchive}, nil
	}
	data, err := readBoundedAppPackage(r.Body)
	if err != nil {
		return appPackageUpload{}, err
	}
	return appPackageUpload{Bytes: data, PackageKind: protocol.AppPackageKindArchive}, nil
}

func readBoundedAppPackage(reader io.Reader) ([]byte, error) {
	var buf bytes.Buffer
	written, err := io.Copy(&buf, io.LimitReader(reader, maxAppPackageBytes+1))
	if err != nil {
		return nil, err
	}
	if written == 0 {
		return nil, fmt.Errorf("package body is required")
	}
	if written > maxAppPackageBytes {
		return nil, errAppPackageTooLarge
	}
	return buf.Bytes(), nil
}

func zipAppSourceUpload(files []*multipart.FileHeader) ([]byte, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("source folder is empty")
	}
	var buf bytes.Buffer
	archive := zip.NewWriter(&buf)
	seen := make(map[string]struct{}, len(files))
	total := int64(0)
	for _, header := range files {
		cleaned, err := cleanAppSourceUploadPath(header.Filename)
		if err != nil {
			_ = archive.Close()
			return nil, err
		}
		if _, ok := seen[cleaned]; ok {
			_ = archive.Close()
			return nil, fmt.Errorf("duplicate source path %q", cleaned)
		}
		seen[cleaned] = struct{}{}
		file, err := header.Open()
		if err != nil {
			_ = archive.Close()
			return nil, err
		}
		writer, err := archive.CreateHeader(&zip.FileHeader{
			Name:   cleaned,
			Method: zip.Deflate,
		})
		if err != nil {
			_ = file.Close()
			_ = archive.Close()
			return nil, err
		}
		limited := io.LimitReader(file, maxAppPackageBytes-total+1)
		written, err := io.Copy(writer, limited)
		_ = file.Close()
		if err != nil {
			_ = archive.Close()
			return nil, err
		}
		total += written
		if total > maxAppPackageBytes {
			_ = archive.Close()
			return nil, errAppPackageTooLarge
		}
	}
	if err := archive.Close(); err != nil {
		return nil, err
	}
	if buf.Len() == 0 {
		return nil, fmt.Errorf("source folder is empty")
	}
	if buf.Len() > maxAppPackageBytes {
		return nil, errAppPackageTooLarge
	}
	return buf.Bytes(), nil
}

func cleanAppSourceUploadPath(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" || strings.Contains(value, "\x00") {
		return "", fmt.Errorf("unsafe source path %q", value)
	}
	cleaned := path.Clean(value)
	if cleaned == "." || strings.HasPrefix(cleaned, "../") || cleaned == ".." || path.IsAbs(cleaned) {
		return "", fmt.Errorf("unsafe source path %q", value)
	}
	parts := strings.Split(cleaned, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("unsafe source path %q", value)
		}
	}
	return cleaned, nil
}

func absoluteRequestURL(r *http.Request, path string) string {
	scheme := "http"
	if requestIsHTTPS(r) {
		scheme = "https"
	}
	host := r.Host
	if forwardedHost := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); forwardedHost != "" {
		host = forwardedHost
	}
	if forwardedProto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwardedProto == "http" || forwardedProto == "https" {
		scheme = forwardedProto
	}
	return scheme + "://" + host + path
}

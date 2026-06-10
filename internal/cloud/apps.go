package cloud

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dropfile/hankremote/internal/domain"
	"github.com/dropfile/hankremote/internal/protocol"
)

const (
	maxAppPackageBytes = 32 << 20
	appPackageTTL      = 15 * time.Minute
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
		apps, err := s.store.ListHomeApps(r.Context(), home.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return true
		}
		writeJSON(w, http.StatusOK, map[string]any{"apps": apps})
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
		data, err := readAppPackageUpload(r)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, errAppPackageTooLarge) {
				status = http.StatusRequestEntityTooLarge
			}
			http.Error(w, err.Error(), status)
			return true
		}
		staged := s.appPackages.Put(home.ID, data)
		downloadURL := absoluteRequestURL(r, "/v1/home/apps/packages/"+staged.StagingID)
		envelope, err := s.sendAgentCommand(r.Context(), home.ID, protocol.CommandAppsPackagePreview, protocol.AppsPackagePreviewRequest{
			StagingID:     staged.StagingID,
			DownloadURL:   downloadURL,
			DownloadToken: staged.DownloadToken,
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
	secretFieldsSet := "{}"
	if len(app.SecretFieldsSet) > 0 {
		raw, err := json.Marshal(app.SecretFieldsSet)
		if err != nil {
			return err
		}
		secretFieldsSet = string(raw)
	}
	settingsSchema := "{}"
	if len(app.SettingsSchema.Fields) > 0 {
		raw, err := json.Marshal(app.SettingsSchema)
		if err != nil {
			return err
		}
		settingsSchema = string(raw)
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
		Status:              status,
		LastError:           app.LastError,
		UpdatedAt:           now,
		UpdatedBy:           updatedBy,
	})
}

var errAppPackageTooLarge = errors.New("app package too large")

func readAppPackageUpload(r *http.Request) ([]byte, error) {
	contentType := r.Header.Get("Content-Type")
	mediaType, _, _ := mime.ParseMediaType(contentType)
	if mediaType == "multipart/form-data" {
		if err := r.ParseMultipartForm(maxAppPackageBytes); err != nil {
			return nil, err
		}
		file, _, err := r.FormFile("package")
		if err != nil {
			file, _, err = r.FormFile("file")
		}
		if err != nil {
			return nil, fmt.Errorf("package file is required")
		}
		defer file.Close()
		return readBoundedAppPackage(file)
	}
	return readBoundedAppPackage(r.Body)
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

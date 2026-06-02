package cloud

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/dropfile/hankremote/internal/domain"
	"github.com/dropfile/hankremote/internal/store"
)

func (s *Server) handleAPNSDeviceRegistration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	auth, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	var body struct {
		DeviceID          string   `json:"device_id"`
		Token             string   `json:"token"`
		Environment       string   `json:"environment"`
		BundleID          string   `json:"bundle_id"`
		EnabledCategories []string `json:"enabled_categories"`
	}
	if err := parseJSON(w, r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	body.DeviceID = strings.TrimSpace(body.DeviceID)
	body.Token = strings.TrimSpace(body.Token)
	body.Environment = normalizeAPNSEnvironment(body.Environment)
	body.BundleID = strings.TrimSpace(body.BundleID)
	if body.DeviceID == "" || body.Token == "" {
		http.Error(w, "device_id and token are required", http.StatusBadRequest)
		return
	}
	if body.Environment == "" {
		body.Environment = "sandbox"
	}
	categories := sanitizeNotificationCategories(body.EnabledCategories)
	encodedCategories, err := json.Marshal(categories)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	device, err := s.store.UpsertAPNSDevice(r.Context(), domain.APNSDevice{
		UserID:            auth.User.ID,
		SessionID:         auth.Session.ID,
		DeviceID:          body.DeviceID,
		Token:             body.Token,
		Environment:       body.Environment,
		BundleID:          body.BundleID,
		EnabledCategories: encodedCategories,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"device_id":  device.DeviceID,
		"categories": categories,
	})
}

func (s *Server) handleWebPushVAPIDPublicKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, ok := s.requireAuth(w, r); !ok {
		return
	}
	publicKey := strings.TrimSpace(os.Getenv("HANK_REMOTE_WEB_PUSH_VAPID_PUBLIC_KEY"))
	writeJSON(w, http.StatusOK, map[string]any{
		"public_key": publicKey,
		"available":  publicKey != "",
	})
}

func (s *Server) handleWebPushDeviceRegistration(w http.ResponseWriter, r *http.Request) {
	auth, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodPost:
		var body struct {
			DeviceID          string   `json:"device_id"`
			Endpoint          string   `json:"endpoint"`
			P256DH            string   `json:"p256dh"`
			Auth              string   `json:"auth"`
			EnabledCategories []string `json:"enabled_categories"`
		}
		if err := parseJSON(w, r, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		body.DeviceID = strings.TrimSpace(body.DeviceID)
		body.Endpoint = strings.TrimSpace(body.Endpoint)
		body.P256DH = strings.TrimSpace(body.P256DH)
		body.Auth = strings.TrimSpace(body.Auth)
		if body.DeviceID == "" || body.Endpoint == "" || body.P256DH == "" || body.Auth == "" {
			http.Error(w, "device_id, endpoint, p256dh, and auth are required", http.StatusBadRequest)
			return
		}
		if !strings.HasPrefix(body.Endpoint, "https://") {
			http.Error(w, "web push endpoint must use https", http.StatusBadRequest)
			return
		}
		categories := sanitizeNotificationCategories(body.EnabledCategories)
		encodedCategories, err := json.Marshal(categories)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		device, err := s.store.UpsertWebPushDevice(r.Context(), domain.WebPushDevice{
			UserID:            auth.User.ID,
			SessionID:         auth.Session.ID,
			DeviceID:          body.DeviceID,
			Endpoint:          body.Endpoint,
			P256DH:            body.P256DH,
			Auth:              body.Auth,
			EnabledCategories: encodedCategories,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":          true,
			"device_id":   device.DeviceID,
			"categories":  categories,
			"deliverable": strings.TrimSpace(os.Getenv("HANK_REMOTE_WEB_PUSH_VAPID_PRIVATE_KEY")) != "",
		})
	case http.MethodDelete:
		deviceID := strings.TrimSpace(r.URL.Query().Get("device_id"))
		if deviceID == "" {
			http.Error(w, "device_id is required", http.StatusBadRequest)
			return
		}
		if err := s.store.DeleteWebPushDevice(r.Context(), auth.User.ID, deviceID); err != nil && !errors.Is(err, store.ErrNotFound) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAPNSDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	auth, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/me/devices/"), "/")
	deviceID, suffix, ok := strings.Cut(path, "/")
	if !ok || suffix != "apns" || strings.TrimSpace(deviceID) == "" {
		http.NotFound(w, r)
		return
	}
	if err := s.store.DeleteAPNSDevice(r.Context(), auth.User.ID, deviceID); err != nil && !errors.Is(err, store.ErrNotFound) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleNotificationSettings(w http.ResponseWriter, r *http.Request) {
	auth, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, notificationSettingsPayload(s.notificationSettingsOrDefault(r.Context(), auth.User.ID)))
	case http.MethodPut:
		var body struct {
			Storage           *bool `json:"storage"`
			Notes             *bool `json:"notes"`
			DashboardEntities *bool `json:"dashboard_entities"`
		}
		if err := parseJSON(w, r, &body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		settings := s.notificationSettingsOrDefault(r.Context(), auth.User.ID)
		if body.Storage != nil {
			settings.StorageEnabled = *body.Storage
		}
		if body.Notes != nil {
			settings.NotesEnabled = *body.Notes
		}
		if body.DashboardEntities != nil {
			settings.DashboardEntitiesEnabled = *body.DashboardEntities
		}
		saved, err := s.store.SaveNotificationSettings(r.Context(), settings)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, notificationSettingsPayload(saved))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func notificationSettingsPayload(settings domain.NotificationSettings) map[string]any {
	return map[string]any{
		"user_id":            settings.UserID,
		"storage":            settings.StorageEnabled,
		"notes":              settings.NotesEnabled,
		"dashboard_entities": settings.DashboardEntitiesEnabled,
		"updated_at":         settings.UpdatedAt,
	}
}

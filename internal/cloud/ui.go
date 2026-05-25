package cloud

import (
	"embed"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/dropfile/hankremote/internal/domain"
)

//go:embed ui/*
var uiAssets embed.FS

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if _, err := s.appAuthFromRequest(r); err == nil {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}
	serveUIFile(w, r, "login.html", "text/html; charset=utf-8")
}

func (s *Server) handleDashboardPage(w http.ResponseWriter, r *http.Request) {
	s.serveHomeMemberUIPage(w, r, "/dashboard", "dashboard.html")
}

func (s *Server) handleHomeUsersPage(w http.ResponseWriter, r *http.Request) {
	if isSettingsPaneRequest(r) {
		s.serveHomeMemberUIPage(w, r, "/dashboard/home-users", "home-users.html")
		return
	}
	s.redirectHomeMemberUIPage(w, r, "/dashboard/home-users", "/dashboard/settings#people")
}

func (s *Server) handleServiceProfilesPage(w http.ResponseWriter, r *http.Request) {
	if isSettingsPaneRequest(r) {
		s.serveHomeMemberUIPage(w, r, "/dashboard/service-profiles", "service-profiles.html")
		return
	}
	s.redirectHomeMemberUIPage(w, r, "/dashboard/service-profiles", "/dashboard/settings#connections")
}

func (s *Server) handleSyncStatusPage(w http.ResponseWriter, r *http.Request) {
	s.redirectHomeMemberUIPage(w, r, "/dashboard/sync-status", "/dashboard#health")
}

func (s *Server) handleStoragePage(w http.ResponseWriter, r *http.Request) {
	if isSettingsPaneRequest(r) {
		s.serveAdminUIPage(w, r, "/dashboard/storage", "storage.html")
		return
	}
	s.redirectAdminUIPage(w, r, "/dashboard/storage", "/dashboard/settings#backups")
}

func (s *Server) handleHankPage(w http.ResponseWriter, r *http.Request) {
	s.serveHomeMemberUIPage(w, r, "/dashboard/hank", "hank.html")
}

func (s *Server) handleHomeAssistantPage(w http.ResponseWriter, r *http.Request) {
	s.serveHomeMemberUIPage(w, r, "/dashboard/home-assistant", "home-assistant.html")
}

func (s *Server) handleAssistantSettingsPage(w http.ResponseWriter, r *http.Request) {
	if isSettingsPaneRequest(r) {
		s.serveHomeMemberUIPage(w, r, "/dashboard/assistant-settings", "assistant-settings.html")
		return
	}
	s.redirectHomeMemberUIPage(w, r, "/dashboard/assistant-settings", "/dashboard/settings#ai")
}

func (s *Server) handleProfileNotesPage(w http.ResponseWriter, r *http.Request) {
	s.serveHomeMemberUIPage(w, r, "/dashboard/profile-notes", "profile-notes.html")
}

func (s *Server) handleFileServerPage(w http.ResponseWriter, r *http.Request) {
	s.serveHomeMemberUIPage(w, r, "/dashboard/file-server", "file-server.html")
}

func (s *Server) handleFileTransfersPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/dashboard/file-transfers" {
		http.NotFound(w, r)
		return
	}
	auth, err := s.appAuthFromRequest(r)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if _, _, err := s.requireSingletonHomeMembership(r.Context(), auth.User.ID); err != nil {
		http.Error(w, "home membership required", http.StatusForbidden)
		return
	}
	target := "/dashboard/file-server"
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (s *Server) handleSettingsPage(w http.ResponseWriter, r *http.Request) {
	s.serveAuthenticatedUIPage(w, r, "/dashboard/settings", "settings.html")
}

func (s *Server) handleSettingsConnectionsPane(w http.ResponseWriter, r *http.Request) {
	s.serveHomeMemberUIPage(w, r, "/dashboard/settings/connections-pane", "settings-connections.html")
}

func (s *Server) handleAcceptInvitationPage(w http.ResponseWriter, r *http.Request) {
	if isSettingsPaneRequest(r) {
		s.serveAuthenticatedUIPage(w, r, "/dashboard/accept-invitation", "accept-invitation.html")
		return
	}
	target := "/dashboard/settings#join-home"
	if token := r.URL.Query().Get("token"); token != "" {
		target = "/dashboard/settings?token=" + url.QueryEscape(token) + "#join-home"
	}
	s.redirectAuthenticatedUIPage(w, r, "/dashboard/accept-invitation", target)
}

func serveDeploymentGuide(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/docs/deployment" {
		http.NotFound(w, r)
		return
	}
	serveUIFile(w, r, "deployment.html", "text/html; charset=utf-8")
}

func serveUIFavicon(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/favicon.ico" {
		http.NotFound(w, r)
		return
	}
	serveUIFile(w, r, "favicon.ico", "image/png")
}

func serveUIAsset(w http.ResponseWriter, r *http.Request) {
	name := path.Clean(strings.TrimPrefix(r.URL.Path, "/assets/"))
	switch name {
	case "styles.css":
		serveUIFile(w, r, name, "text/css; charset=utf-8")
	case "login.js", "dashboard.js", "home-assistant.js", "settings.js", "settings-connections.js", "home-users.js", "service-profiles.js", "sync-status.js", "storage.js", "hank.js", "assistant-settings.js", "profile-notes.js", "file-server.js", "accept-invitation.js", "admin-nav.js":
		serveUIFile(w, r, name, "application/javascript; charset=utf-8")
	case "site.webmanifest":
		serveUIFile(w, r, name, "application/manifest+json; charset=utf-8")
	case "favicon.ico", "favicon.png", "hank-icon.png", "hank-icon-192.png", "hank-icon-512.png", "apple-touch-icon.png":
		serveUIFile(w, r, name, "image/png")
	default:
		http.NotFound(w, r)
	}
}

func isSettingsPaneRequest(r *http.Request) bool {
	return r.URL.Query().Get("pane") == "1"
}

func (s *Server) serveAuthenticatedUIPage(w http.ResponseWriter, r *http.Request, expectedPath string, assetName string) {
	if r.URL.Path != expectedPath {
		http.NotFound(w, r)
		return
	}
	if _, err := s.appAuthFromRequest(r); err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	serveUIFile(w, r, assetName, "text/html; charset=utf-8")
}

func (s *Server) redirectAuthenticatedUIPage(w http.ResponseWriter, r *http.Request, expectedPath string, target string) {
	if r.URL.Path != expectedPath {
		http.NotFound(w, r)
		return
	}
	if _, err := s.appAuthFromRequest(r); err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (s *Server) serveHomeMemberUIPage(w http.ResponseWriter, r *http.Request, expectedPath string, assetName string) {
	if r.URL.Path != expectedPath {
		http.NotFound(w, r)
		return
	}
	auth, err := s.appAuthFromRequest(r)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if _, _, err := s.requireSingletonHomeMembership(r.Context(), auth.User.ID); err != nil {
		http.Error(w, "home membership required", http.StatusForbidden)
		return
	}
	serveUIFile(w, r, assetName, "text/html; charset=utf-8")
}

func (s *Server) redirectHomeMemberUIPage(w http.ResponseWriter, r *http.Request, expectedPath string, target string) {
	if r.URL.Path != expectedPath {
		http.NotFound(w, r)
		return
	}
	auth, err := s.appAuthFromRequest(r)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if _, _, err := s.requireSingletonHomeMembership(r.Context(), auth.User.ID); err != nil {
		http.Error(w, "home membership required", http.StatusForbidden)
		return
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (s *Server) serveAdminUIPage(w http.ResponseWriter, r *http.Request, expectedPath string, assetName string) {
	if r.URL.Path != expectedPath {
		http.NotFound(w, r)
		return
	}
	auth, err := s.appAuthFromRequest(r)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	_, membership, err := s.requireSingletonHomeMembership(r.Context(), auth.User.ID)
	if err != nil || membership.Role != domain.HomeRoleAdmin {
		http.Error(w, "admin role required", http.StatusForbidden)
		return
	}
	serveUIFile(w, r, assetName, "text/html; charset=utf-8")
}

func (s *Server) redirectAdminUIPage(w http.ResponseWriter, r *http.Request, expectedPath string, target string) {
	if r.URL.Path != expectedPath {
		http.NotFound(w, r)
		return
	}
	auth, err := s.appAuthFromRequest(r)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	_, membership, err := s.requireSingletonHomeMembership(r.Context(), auth.User.ID)
	if err != nil || membership.Role != domain.HomeRoleAdmin {
		http.Error(w, "admin role required", http.StatusForbidden)
		return
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func serveUIFile(w http.ResponseWriter, r *http.Request, name string, contentType string) {
	data, err := fs.ReadFile(uiAssets, "ui/"+name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", contentType)
	if strings.HasSuffix(name, ".html") {
		w.Header().Set("Cache-Control", "private, max-age=30")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

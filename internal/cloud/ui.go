package cloud

import (
	"embed"
	"io/fs"
	"net/http"
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
	if auth, err := s.appAuthFromRequest(r); err == nil {
		if auth.User.PasswordChangeRequired {
			http.Redirect(w, r, "/password-change", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}
	serveUIFile(w, r, "login.html", "text/html; charset=utf-8")
}

func (s *Server) handleJoinPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/join" {
		http.NotFound(w, r)
		return
	}
	serveUIFile(w, r, "join.html", "text/html; charset=utf-8")
}

func (s *Server) handlePasswordChangePage(w http.ResponseWriter, r *http.Request) {
	s.serveAuthenticatedUIPage(w, r, "/password-change", "password-change.html")
}

func (s *Server) handleDashboardPage(w http.ResponseWriter, r *http.Request) {
	s.serveHomeMemberUIPage(w, r, "/dashboard", "dashboard.html")
}

func (s *Server) handleHankPage(w http.ResponseWriter, r *http.Request) {
	s.serveHomeMemberUIPage(w, r, "/dashboard/hank", "hank.html")
}

func (s *Server) handleHomeAssistantPage(w http.ResponseWriter, r *http.Request) {
	s.serveHomeMemberUIPage(w, r, "/dashboard/home-assistant", "home-assistant.html")
}

func (s *Server) handleProfileNotesPage(w http.ResponseWriter, r *http.Request) {
	s.serveHomeMemberUIPage(w, r, "/dashboard/profile-notes", "profile-notes.html")
}

func (s *Server) handleFileServerPage(w http.ResponseWriter, r *http.Request) {
	s.serveHomeMemberUIPage(w, r, "/dashboard/file-server", "file-server.html")
}

func (s *Server) handleSettingsPage(w http.ResponseWriter, r *http.Request) {
	s.serveAuthenticatedUIPage(w, r, "/dashboard/settings", "settings.html")
}

func (s *Server) handleSettingsPeoplePane(w http.ResponseWriter, r *http.Request) {
	s.serveHomeMemberUIPage(w, r, "/dashboard/settings/people-pane", "home-users.html")
}

func (s *Server) handleSettingsConnectionsPane(w http.ResponseWriter, r *http.Request) {
	s.serveHomeMemberUIPage(w, r, "/dashboard/settings/connections-pane", "settings-connections.html")
}

func (s *Server) handleSettingsAIPane(w http.ResponseWriter, r *http.Request) {
	s.serveHomeMemberUIPage(w, r, "/dashboard/settings/ai-pane", "assistant-settings.html")
}

func (s *Server) handleSettingsBackupsPane(w http.ResponseWriter, r *http.Request) {
	s.serveAdminUIPage(w, r, "/dashboard/settings/backups-pane", "storage.html")
}

func (s *Server) handleSettingsJoinHomePane(w http.ResponseWriter, r *http.Request) {
	s.serveAuthenticatedUIPage(w, r, "/dashboard/settings/join-home-pane", "accept-invitation.html")
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
	case "api-client.js", "login.js", "join.js", "password-change.js", "dashboard.js", "home-assistant.js", "settings.js", "settings-connections.js", "home-users.js", "service-profiles.js", "sync-status.js", "storage.js", "hank.js", "assistant-settings.js", "profile-notes.js", "file-server.js", "accept-invitation.js", "admin-nav.js":
		serveUIFile(w, r, name, "application/javascript; charset=utf-8")
	case "favicon.ico", "favicon.png", "hank-icon.png", "hank-icon-192.png", "hank-icon-512.png", "apple-touch-icon.png":
		serveUIFile(w, r, name, "image/png")
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) serveAuthenticatedUIPage(w http.ResponseWriter, r *http.Request, expectedPath string, assetName string) {
	if r.URL.Path != expectedPath {
		http.NotFound(w, r)
		return
	}
	auth, err := s.appAuthFromRequest(r)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if r.URL.Path != "/password-change" && auth.User.PasswordChangeRequired {
		http.Redirect(w, r, "/password-change", http.StatusSeeOther)
		return
	}
	serveUIFile(w, r, assetName, "text/html; charset=utf-8")
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
	if auth.User.PasswordChangeRequired {
		http.Redirect(w, r, "/password-change", http.StatusSeeOther)
		return
	}
	if _, _, err := s.requireSingletonHomeMembership(r.Context(), auth.User.ID); err != nil {
		http.Error(w, "home membership required", http.StatusForbidden)
		return
	}
	serveUIFile(w, r, assetName, "text/html; charset=utf-8")
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
	if auth.User.PasswordChangeRequired {
		http.Redirect(w, r, "/password-change", http.StatusSeeOther)
		return
	}
	_, membership, err := s.requireSingletonHomeMembership(r.Context(), auth.User.ID)
	if err != nil || membership.Role != domain.HomeRoleAdmin {
		http.Error(w, "admin role required", http.StatusForbidden)
		return
	}
	serveUIFile(w, r, assetName, "text/html; charset=utf-8")
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

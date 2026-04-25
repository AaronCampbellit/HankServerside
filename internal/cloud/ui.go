package cloud

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
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
	s.serveAuthenticatedUIPage(w, r, "/dashboard", "dashboard.html")
}

func (s *Server) handleHomeUsersPage(w http.ResponseWriter, r *http.Request) {
	s.serveAuthenticatedUIPage(w, r, "/dashboard/home-users", "home-users.html")
}

func (s *Server) handleServiceProfilesPage(w http.ResponseWriter, r *http.Request) {
	s.serveAuthenticatedUIPage(w, r, "/dashboard/service-profiles", "service-profiles.html")
}

func (s *Server) handleSyncStatusPage(w http.ResponseWriter, r *http.Request) {
	s.serveAuthenticatedUIPage(w, r, "/dashboard/sync-status", "sync-status.html")
}

func (s *Server) handleProfileNotesPage(w http.ResponseWriter, r *http.Request) {
	s.serveAuthenticatedUIPage(w, r, "/dashboard/profile-notes", "profile-notes.html")
}

func (s *Server) handleFileTransfersPage(w http.ResponseWriter, r *http.Request) {
	s.serveAuthenticatedUIPage(w, r, "/dashboard/file-transfers", "file-transfers.html")
}

func (s *Server) handleAcceptInvitationPage(w http.ResponseWriter, r *http.Request) {
	s.serveAuthenticatedUIPage(w, r, "/dashboard/accept-invitation", "accept-invitation.html")
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
	case "login.js", "dashboard.js", "home-users.js", "service-profiles.js", "sync-status.js", "profile-notes.js", "file-transfers.js", "accept-invitation.js":
		serveUIFile(w, r, name, "application/javascript; charset=utf-8")
	case "site.webmanifest":
		serveUIFile(w, r, name, "application/manifest+json; charset=utf-8")
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
	if _, err := s.appAuthFromRequest(r); err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
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
		w.Header().Set("Cache-Control", "no-store")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

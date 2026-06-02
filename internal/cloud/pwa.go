package cloud

import (
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

//go:embed pwa/* pwa/assets/* pwa/icons/*
var pwaAssets embed.FS

func servePWA(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/pwa/")
	if r.URL.Path == "/pwa" || name == "" {
		servePWAFile(w, r, "index.html")
		return
	}
	name = path.Clean(name)
	if strings.HasPrefix(name, "../") || strings.HasPrefix(name, "/") {
		http.NotFound(w, r)
		return
	}
	if _, err := fs.Stat(pwaAssets, "pwa/"+name); err == nil {
		servePWAFile(w, r, name)
		return
	}
	if path.Ext(name) == "" {
		servePWAFile(w, r, "index.html")
		return
	}
	http.NotFound(w, r)
}

func servePWAFile(w http.ResponseWriter, r *http.Request, name string) {
	data, err := fs.ReadFile(pwaAssets, "pwa/"+name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", pwaContentType(name))
	if name == "index.html" {
		w.Header().Set("Cache-Control", "private, max-age=30")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func pwaContentType(name string) string {
	switch path.Ext(name) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "application/javascript; charset=utf-8"
	case ".webmanifest":
		return "application/manifest+json; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	default:
		if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
			return contentType
		}
		return "application/octet-stream"
	}
}

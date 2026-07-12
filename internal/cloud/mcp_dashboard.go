package cloud

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/dropfile/hankremote/internal/store"
)

// handleProfileMCP backs the "MCP Connector" panel on the AI settings page.
// GET  /v1/me/mcp                      -> connector status + the user's connected apps
// DELETE /v1/me/mcp/connections/{id}   -> disconnect one of the user's apps
//
// Per-user by design: a user sees and revokes only their own MCP grants.
func (s *Server) handleProfileMCP(w http.ResponseWriter, r *http.Request) {
	if !s.mcpEnabled {
		http.NotFound(w, r)
		return
	}
	auth, ok := s.requireAuth(w, r)
	if !ok {
		return
	}

	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/me/mcp"), "/")
	if rest == "context-sources" || strings.HasPrefix(rest, "context-sources/") {
		s.handleMCPContextSources(w, r, auth, rest)
		return
	}

	if strings.HasPrefix(rest, "connections/") && r.Method == http.MethodDelete {
		tokenID := strings.TrimSpace(strings.TrimPrefix(rest, "connections/"))
		if tokenID == "" {
			http.Error(w, "connection id is required", http.StatusBadRequest)
			return
		}
		if err := s.store.RevokeMCPTokenForUser(r.Context(), tokenID, auth.User.ID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.audit(r.Context(), "mcp_oauth.connection_revoked", auditSeverityInfo, auth.User.ID, "", "", requestIDFromContext(r.Context()), "mcp_oauth_token", tokenID, nil)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}

	if rest != "" || r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tokens, err := s.store.ListMCPTokensByUser(r.Context(), auth.User.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	contextSources, err := s.store.ListMCPContextSourcesByUser(r.Context(), auth.User.ID, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	clientNames := map[string]string{}
	connections := make([]map[string]any, 0, len(tokens))
	for _, t := range tokens {
		name, resolved := clientNames[t.ClientID]
		if !resolved {
			if client, err := s.store.GetMCPOAuthClient(r.Context(), t.ClientID); err == nil {
				name = client.ClientName
			}
			clientNames[t.ClientID] = name
		}
		// A grant counts as "connected" while it can still be used: either the
		// access token is unexpired, or it can be refreshed (no/future refresh expiry).
		now := time.Now()
		connected := t.AccessExpiresAt.After(now) ||
			t.RefreshExpiresAt == nil || t.RefreshExpiresAt.After(now)
		connections = append(connections, map[string]any{
			"id":           t.ID,
			"client_id":    t.ClientID,
			"client_name":  name,
			"scopes":       t.Scopes,
			"created_at":   t.CreatedAt,
			"last_used_at": t.LastUsedAt,
			"connected":    connected,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":          true,
		"resource_url":     s.mcpResourceURL(r),
		"scopes_supported": mcpSupportedScopes,
		"connections":      connections,
		"context_sources":  contextSources,
	})
}

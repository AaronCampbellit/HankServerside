package cloud

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dropfile/hankremote/internal/domain"
	"github.com/dropfile/hankremote/internal/store"
)

type openAITokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}

func (s *Server) handleOpenAIOAuthStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	auth, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	if strings.TrimSpace(s.openAIClientID) == "" || strings.TrimSpace(s.openAIRedirectURI) == "" {
		http.Error(w, "openai oauth is not configured", http.StatusNotImplemented)
		return
	}
	stateRaw := newToken()
	codeVerifier := newToken() + newToken()
	hash := sha256.Sum256([]byte(codeVerifier))
	codeChallenge := base64.RawURLEncoding.EncodeToString(hash[:])
	now := time.Now().UTC()
	if err := s.store.UpsertOpenAIOAuthState(r.Context(), domain.OpenAIOAuthState{StateHash: hashToken(stateRaw), UserID: auth.User.ID, CodeVerifier: codeVerifier, CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", s.openAIClientID)
	q.Set("redirect_uri", s.openAIRedirectURI)
	q.Set("scope", defaultString(strings.TrimSpace(s.openAIScopes), "openid profile email"))
	q.Set("state", stateRaw)
	q.Set("code_challenge", codeChallenge)
	q.Set("code_challenge_method", "S256")
	writeJSON(w, http.StatusOK, map[string]any{"authorization_url": "https://auth.openai.com/oauth/authorize?" + q.Encode()})
}

func (s *Server) handleOpenAIOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if strings.TrimSpace(s.openAIClientID) == "" || strings.TrimSpace(s.openAIClientSecret) == "" || strings.TrimSpace(s.openAIRedirectURI) == "" {
		http.Error(w, "openai oauth is not configured", http.StatusNotImplemented)
		return
	}
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if state == "" || code == "" {
		http.Error(w, "missing state or code", http.StatusBadRequest)
		return
	}
	oauthState, err := s.store.ConsumeOpenAIOAuthState(r.Context(), hashToken(state))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "invalid state", http.StatusBadRequest)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tok, err := s.exchangeOpenAICode(r.Context(), code, oauthState.CodeVerifier)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	now := time.Now().UTC()
	expiresAt := now.Add(time.Duration(tok.ExpiresIn) * time.Second)
	if err := s.store.UpsertOpenAIAccount(r.Context(), domain.OpenAIAccount{UserID: oauthState.UserID, ProviderUserID: "", AccessToken: tok.AccessToken, RefreshToken: tok.RefreshToken, Scope: tok.Scope, TokenType: tok.TokenType, ExpiresAt: &expiresAt, CreatedAt: now, UpdatedAt: now}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "linked": true, "expires_at": expiresAt})
}

func (s *Server) exchangeOpenAICode(ctx context.Context, code string, codeVerifier string) (openAITokenResponse, error) {
	values := url.Values{}
	values.Set("grant_type", "authorization_code")
	values.Set("client_id", s.openAIClientID)
	values.Set("client_secret", s.openAIClientSecret)
	values.Set("redirect_uri", s.openAIRedirectURI)
	values.Set("code", code)
	values.Set("code_verifier", codeVerifier)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://auth.openai.com/oauth/token", strings.NewReader(values.Encode()))
	if err != nil {
		return openAITokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return openAITokenResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return openAITokenResponse{}, errors.New("openai token exchange failed")
	}
	var parsed openAITokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return openAITokenResponse{}, err
	}
	return parsed, nil
}

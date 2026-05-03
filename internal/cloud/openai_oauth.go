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
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}

type openAIAccountStatusResponse struct {
	Configured      bool                             `json:"configured"`
	Linked          bool                             `json:"linked"`
	Missing         []string                         `json:"missing,omitempty"`
	AuthMode        string                           `json:"auth_mode,omitempty"`
	AuthProvider    string                           `json:"auth_provider,omitempty"`
	ChatGPTPlanType string                           `json:"chatgpt_plan_type,omitempty"`
	Pending         *chatGPTDeviceAuthStatusResponse `json:"pending,omitempty"`
	Scopes          string                           `json:"scopes,omitempty"`
	RedirectURI     string                           `json:"redirect_uri,omitempty"`
	TokenType       string                           `json:"token_type,omitempty"`
	Scope           string                           `json:"scope,omitempty"`
	ExpiresAt       *time.Time                       `json:"expires_at,omitempty"`
	UpdatedAt       *time.Time                       `json:"updated_at,omitempty"`
}

func (s *Server) handleOpenAIOAuthStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	auth, ok := s.requireAuth(w, r)
	if !ok {
		return
	}

	useChatGPT := s.shouldUseChatGPTOAuth()
	missing := s.openAIOAuthMissingConfig()
	authMode := "authorization_url"
	if useChatGPT {
		missing = s.chatGPTOAuthMissingConfig()
		authMode = chatGPTDeviceAuthMode
	}
	response := openAIAccountStatusResponse{
		Configured:   len(missing) == 0,
		Missing:      missing,
		AuthMode:     authMode,
		Scopes:       defaultString(strings.TrimSpace(s.openAIScopes), "openid profile email"),
		RedirectURI:  strings.TrimSpace(s.openAIRedirectURI),
		AuthProvider: openAIAccountProviderLegacyOpenAI,
	}
	if useChatGPT {
		response.AuthProvider = openAIAccountProviderChatGPTCodex
		response.Pending = s.chatGPTDeviceAuths.status(auth.User.ID)
		response.Scopes = ""
		response.RedirectURI = ""
	}
	account, err := s.store.GetOpenAIAccount(r.Context(), auth.User.ID)
	if err == nil {
		if (!useChatGPT && account.AuthProvider != openAIAccountProviderChatGPTCodex) || (useChatGPT && account.AuthProvider == openAIAccountProviderChatGPTCodex) {
			response.Linked = true
			response.AuthProvider = account.AuthProvider
			response.ChatGPTPlanType = account.ChatGPTPlanType
			response.TokenType = account.TokenType
			response.Scope = account.Scope
			response.ExpiresAt = account.ExpiresAt
			response.UpdatedAt = &account.UpdatedAt
		}
	} else if !errors.Is(err, store.ErrNotFound) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, response)
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
	if s.shouldUseChatGPTOAuth() {
		if len(s.chatGPTOAuthMissingConfig()) > 0 {
			http.Error(w, "chatgpt oauth is not configured", http.StatusNotImplemented)
			return
		}
		response, err := s.beginChatGPTDeviceAuth(r.Context(), auth.User.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		writeJSON(w, http.StatusOK, response)
		return
	}
	if len(s.openAIOAuthMissingConfig()) > 0 {
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
	if len(s.openAIOAuthMissingConfig()) > 0 {
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
	if err := s.store.UpsertOpenAIAccount(r.Context(), domain.OpenAIAccount{UserID: oauthState.UserID, ProviderUserID: "", AuthProvider: openAIAccountProviderLegacyOpenAI, AccessToken: tok.AccessToken, RefreshToken: tok.RefreshToken, Scope: tok.Scope, TokenType: tok.TokenType, ExpiresAt: &expiresAt, CreatedAt: now, UpdatedAt: now}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "linked": true, "expires_at": expiresAt})
}

func (s *Server) openAIOAuthMissingConfig() []string {
	missing := []string{}
	if strings.TrimSpace(s.openAIClientID) == "" {
		missing = append(missing, "HANK_REMOTE_OPENAI_CLIENT_ID")
	}
	if strings.TrimSpace(s.openAIClientSecret) == "" {
		missing = append(missing, "HANK_REMOTE_OPENAI_CLIENT_SECRET")
	}
	if strings.TrimSpace(s.openAIRedirectURI) == "" {
		missing = append(missing, "HANK_REMOTE_OPENAI_REDIRECT_URI")
	}
	return missing
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

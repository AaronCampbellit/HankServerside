package cloud

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"golang.org/x/crypto/bcrypt"

	"github.com/dropfile/hankremote/internal/domain"
	"github.com/dropfile/hankremote/internal/observability"
	"github.com/dropfile/hankremote/internal/protocol"
	"github.com/dropfile/hankremote/internal/storageops"
	"github.com/dropfile/hankremote/internal/store"
)

const maxHTTPBodyBytes = 1 << 20
const maxWSMessageBytes = 2 << 20

type Server struct {
	addr               string
	store              *store.Store
	router             *Router
	logger             *slog.Logger
	http               *http.Server
	metrics            *observability.Metrics
	limiter            *rateLimiter
	transfers          *transferRegistry
	appTickets         *appWebSocketTicketRegistry
	notes              *cloudNotesService
	collaboration      *noteCollaborationHub
	agentRequests      *agentRequestRegistry
	syncs              *homeSyncController
	storage            *storageops.Service
	storageEvents      map[string]struct{}
	realtimeCancel     context.CancelFunc
	sessionTTL         time.Duration
	requestTimeout     time.Duration
	openAIClientID     string
	openAIClientSecret string
	openAIRedirectURI  string
	openAIScopes       string
	assistantAI        AssistantAIConfig
	chatGPTDeviceAuths *chatGPTDeviceAuthRegistry
}

type authContext struct {
	User    domain.User
	Session domain.AppSession
}

func NewServer(addr string, db *store.Store, sessionTTL time.Duration, requestTimeout time.Duration, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	if sessionTTL <= 0 {
		sessionTTL = 7 * 24 * time.Hour
	}
	if requestTimeout <= 0 {
		requestTimeout = 30 * time.Second
	}

	realtimeCtx, realtimeCancel := context.WithCancel(context.Background())
	server := &Server{
		addr:               addr,
		store:              db,
		router:             NewRouter(),
		logger:             logger,
		metrics:            observability.NewMetrics(),
		limiter:            newRateLimiter(),
		transfers:          newTransferRegistry(),
		appTickets:         newAppWebSocketTicketRegistry(),
		notes:              newCloudNotesService(db),
		collaboration:      newNoteCollaborationHub(db),
		agentRequests:      newAgentRequestRegistry(),
		syncs:              newHomeSyncController(),
		storage:            storageops.NewService("", "", ""),
		storageEvents:      make(map[string]struct{}),
		realtimeCancel:     realtimeCancel,
		sessionTTL:         sessionTTL,
		requestTimeout:     requestTimeout,
		chatGPTDeviceAuths: newChatGPTDeviceAuthRegistry(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", server.handleLoginPage)
	mux.HandleFunc("/dashboard", server.handleDashboardPage)
	mux.HandleFunc("/dashboard/home-users", server.handleHomeUsersPage)
	mux.HandleFunc("/dashboard/service-profiles", server.handleServiceProfilesPage)
	mux.HandleFunc("/dashboard/sync-status", server.handleSyncStatusPage)
	mux.HandleFunc("/dashboard/storage", server.handleStoragePage)
	mux.HandleFunc("/dashboard/hank", server.handleHankPage)
	mux.HandleFunc("/dashboard/assistant-settings", server.handleAssistantSettingsPage)
	mux.HandleFunc("/dashboard/profile-notes", server.handleProfileNotesPage)
	mux.HandleFunc("/dashboard/file-transfers", server.handleFileTransfersPage)
	mux.HandleFunc("/dashboard/accept-invitation", server.handleAcceptInvitationPage)
	mux.HandleFunc("/docs/deployment", serveDeploymentGuide)
	mux.HandleFunc("/favicon.ico", serveUIFavicon)
	mux.HandleFunc("/assets/", serveUIAsset)
	mux.HandleFunc("/healthz", server.handleHealthz)
	mux.HandleFunc("/readyz", server.handleReadyz)
	mux.HandleFunc("/metrics", server.handleMetrics)
	mux.HandleFunc("/v1/auth/register", server.handleAuthRegister)
	mux.HandleFunc("/v1/auth/login", server.handleAuthLogin)
	mux.HandleFunc("/v1/auth/logout", server.handleAuthLogout)
	mux.HandleFunc("/v1/me", server.handleMe)
	mux.HandleFunc("/v1/oauth/openai/status", server.handleOpenAIOAuthStatus)
	mux.HandleFunc("/v1/oauth/openai/start", server.handleOpenAIOAuthStart)
	mux.HandleFunc("/v1/oauth/openai/callback", server.handleOpenAIOAuthCallback)
	mux.HandleFunc("/v1/me/notes", server.handleProfileNotesHTTP)
	mux.HandleFunc("/v1/me/notes/", server.handleProfileNotesHTTP)
	mux.HandleFunc("/v1/me/profile", server.handleProfileSettingsHTTP)
	mux.HandleFunc("/v1/me/profile-secret-vault", server.handleProfileSecretVaultHTTP)
	mux.HandleFunc("/v1/me/profile-backup", server.handleProfileBackupHTTP)
	mux.HandleFunc("/v1/ws/app-ticket", server.handleAppWebSocketTicket)
	mux.HandleFunc("/v1/home", server.handleHome)
	mux.HandleFunc("/v1/home/invitations/accept", server.handleHomeInvitationAccept)
	mux.HandleFunc("/v1/home/", server.handleHomeSubroutes)
	mux.HandleFunc("/v1/file-transfers/", server.handleFileTransfer)
	mux.HandleFunc("/ws/agent", server.handleAgentWebSocket)
	mux.HandleFunc("/ws/app", server.handleAppWebSocket)

	server.http = &http.Server{
		Addr:              addr,
		Handler:           securityHeadersMiddleware(requestIDMiddleware(mux)),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go server.forwardStoreNotifications(realtimeCtx)
	go server.forwardStorageEvents(realtimeCtx)

	return server
}

func (s *Server) ConfigureOpenAI(clientID, clientSecret, redirectURI, scopes string) {
	s.openAIClientID = strings.TrimSpace(clientID)
	s.openAIClientSecret = strings.TrimSpace(clientSecret)
	s.openAIRedirectURI = strings.TrimSpace(redirectURI)
	s.openAIScopes = strings.TrimSpace(scopes)
}

func (s *Server) ConfigureAssistantAI(cfg AssistantAIConfig) {
	cfg.normalize()
	s.assistantAI = cfg
}

func (s *Server) ConfigureStorageOps(stateDir, logDir, intentSecret string) {
	s.storage = storageops.NewService(stateDir, logDir, intentSecret)
}
func (s *Server) ListenAndServe() error {
	s.logger.Info("starting Hank Remote cloud service", "addr", s.addr)
	err := s.http.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.realtimeCancel != nil {
		s.realtimeCancel()
	}
	return s.http.Shutdown(ctx)
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"service": "hank-remote-cloud",
		"time":    time.Now().UTC(),
	})
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Ping(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":               true,
		"storage":          "ready",
		"online_agents":    s.router.AgentCount(),
		"online_apps":      s.router.AppCount(),
		"pending_requests": s.router.PendingCount(),
	})
}

func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = io.WriteString(w, s.metrics.RenderPrometheus())
	if s.storage != nil {
		if status, err := s.storage.Status(); err == nil {
			_, _ = io.WriteString(w, storageops.RenderMetrics(status))
		}
	}
}

func (s *Server) handleAuthRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	type request struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	var body request
	if err := parseJSON(w, r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	body.Email = strings.TrimSpace(strings.ToLower(body.Email))
	if body.Email == "" || len(body.Password) < 8 {
		http.Error(w, "email and password are required; password must be at least 8 characters", http.StatusBadRequest)
		return
	}

	if _, err := s.store.GetSingletonHome(r.Context()); err == nil {
		http.Error(w, "registration is disabled after first setup", http.StatusForbidden)
		return
	} else if !errors.Is(err, store.ErrNotFound) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if !s.limiter.Allow("register:"+clientIP(r), 10, time.Minute) {
		s.metrics.IncAuthFailure("register_rate_limited")
		s.logger.Warn("registration rate limited", "request_id", requestIDFromContext(r.Context()), "client_ip", clientIP(r))
		http.Error(w, "too many registration attempts", http.StatusTooManyRequests)
		return
	}

	if _, err := s.store.GetUserByEmail(r.Context(), body.Email); err == nil {
		http.Error(w, "user already exists", http.StatusConflict)
		return
	} else if !errors.Is(err, store.ErrNotFound) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	now := time.Now().UTC()
	user := domain.User{
		ID:           newID("usr"),
		Email:        body.Email,
		PasswordHash: string(passwordHash),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.store.CreateUser(r.Context(), user); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if _, _, err := s.store.BootstrapSingletonHome(r.Context(), user, "Home"); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	session, rawToken, err := s.createSession(r.Context(), user.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.logger.Info("app user registered", "request_id", requestIDFromContext(r.Context()), "user_id", user.ID, "email", user.Email, "session_id", session.ID)
	setSessionCookie(w, r, rawToken, session.ExpiresAt)

	writeJSON(w, http.StatusCreated, map[string]any{
		"user":          sanitizeUser(user),
		"session_id":    session.ID,
		"session_token": rawToken,
		"expires_at":    session.ExpiresAt,
	})
}

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	type request struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	var body request
	if err := parseJSON(w, r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	body.Email = strings.TrimSpace(strings.ToLower(body.Email))
	if !s.limiter.Allow("login:"+clientIP(r), 20, time.Minute) {
		s.metrics.IncAuthFailure("login_rate_limited")
		s.logger.Warn("login rate limited", "request_id", requestIDFromContext(r.Context()), "client_ip", clientIP(r), "email", body.Email)
		http.Error(w, "too many login attempts", http.StatusTooManyRequests)
		return
	}

	user, err := s.store.GetUserByEmail(r.Context(), body.Email)
	if err != nil {
		s.metrics.IncAuthFailure("login_unknown_user")
		s.logger.Warn("login failed for unknown user", "request_id", requestIDFromContext(r.Context()), "client_ip", clientIP(r), "email", body.Email)
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(body.Password)); err != nil {
		s.metrics.IncAuthFailure("login_bad_password")
		s.logger.Warn("login failed with bad password", "request_id", requestIDFromContext(r.Context()), "client_ip", clientIP(r), "user_id", user.ID, "email", body.Email)
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	session, rawToken, err := s.createSession(r.Context(), user.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.logger.Info("app user logged in", "request_id", requestIDFromContext(r.Context()), "user_id", user.ID, "session_id", session.ID)
	setSessionCookie(w, r, rawToken, session.ExpiresAt)

	writeJSON(w, http.StatusOK, map[string]any{
		"user":          sanitizeUser(user),
		"session_id":    session.ID,
		"session_token": rawToken,
		"expires_at":    session.ExpiresAt,
	})
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	auth, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	if err := s.store.RevokeSession(r.Context(), auth.Session.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	clearSessionCookie(w, r)
	s.logger.Info("app session revoked", "request_id", requestIDFromContext(r.Context()), "user_id", auth.User.ID, "session_id", auth.Session.ID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	auth, ok := s.requireAuth(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user":       sanitizeUser(auth.User),
		"session_id": auth.Session.ID,
		"expires_at": auth.Session.ExpiresAt,
	})
}

func (s *Server) handleAppWebSocketTicket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	auth, ok := s.requireAuth(w, r)
	if !ok {
		return
	}

	rawTicket, expiresAt := s.appTickets.Issue(auth.Session.ID, auth.User.ID, 90*time.Second)
	s.logger.Info("issued app websocket ticket", "request_id", requestIDFromContext(r.Context()), "user_id", auth.User.ID, "session_id", auth.Session.ID, "expires_at", expiresAt)
	writeJSON(w, http.StatusCreated, map[string]any{
		"ticket":         rawTicket,
		"expires_at":     expiresAt,
		"websocket_path": "/ws/app?app_ticket=" + rawTicket,
	})
}

func (s *Server) handleAgentWebSocket(w http.ResponseWriter, r *http.Request) {
	if err := enforceSameOriginIfPresent(r); err != nil {
		s.metrics.IncAuthFailure("agent_bad_origin")
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("agent_id"))
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if agentID == "" || token == "" {
		s.metrics.IncAuthFailure("agent_missing_credentials")
		s.logger.Warn("agent websocket missing credentials", "request_id", requestIDFromContext(r.Context()), "client_ip", clientIP(r))
		http.Error(w, "unauthorized agent", http.StatusUnauthorized)
		return
	}

	if !s.limiter.Allow("agent:"+clientIP(r), 40, time.Minute) {
		s.metrics.IncAuthFailure("agent_rate_limited")
		s.logger.Warn("agent websocket rate limited", "request_id", requestIDFromContext(r.Context()), "client_ip", clientIP(r), "agent_id", agentID)
		http.Error(w, "too many agent auth attempts", http.StatusTooManyRequests)
		return
	}

	record, err := s.store.ValidateAgentToken(r.Context(), hashToken(token))
	if err != nil || record.Agent.ID != agentID {
		s.metrics.IncAuthFailure("agent_bad_token")
		s.logger.Warn("agent websocket rejected", "request_id", requestIDFromContext(r.Context()), "client_ip", clientIP(r), "agent_id", agentID)
		http.Error(w, "unauthorized agent", http.StatusUnauthorized)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		s.logger.Error("failed to accept agent websocket", "error", err)
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "connection closed")
	conn.SetReadLimit(maxWSMessageBytes)

	ctx := r.Context()
	peer := newWSPeer(conn)

	s.logger.Info("agent websocket connected", "request_id", requestIDFromContext(r.Context()), "agent_id", record.Agent.ID, "home_id", record.Home.ID)
	defer func() {
		s.router.UnregisterAgent(record.Home.ID)
		s.metrics.SetOnlineAgents(s.router.AgentCount())
		now := time.Now().UTC()
		_ = s.store.SetAgentStatus(context.Background(), record.Agent.ID, domain.AgentStatusOffline, &now)
		s.markHomeSyncOffline(record.Home.ID, record.Agent.ID)
		s.emitHomeStatus(context.Background(), record.Home.ID, map[string]any{"home_id": record.Home.ID, "agent_id": record.Agent.ID, "status": domain.AgentStatusOffline})
		s.logger.Info("agent websocket disconnected", "request_id", requestIDFromContext(r.Context()), "agent_id", record.Agent.ID, "home_id", record.Home.ID)
	}()

	for {
		var envelope protocol.Envelope
		if err := wsjson.Read(ctx, conn, &envelope); err != nil {
			if closeStatus(err) != websocket.StatusNormalClosure {
				s.logger.Warn("agent websocket read failed", "agent_id", record.Agent.ID, "error", err)
			}
			return
		}

		switch envelope.Type {
		case protocol.TypeAgentRegister:
			payload, err := protocol.DecodePayload[protocol.AgentRegister](envelope)
			if err != nil {
				s.writePeerError(ctx, peer, protocol.TypeAgentError, "", record.Agent.ID, record.Home.ID, "bad_register_payload", err.Error(), nil)
				continue
			}
			if payload.AgentID != "" && payload.AgentID != record.Agent.ID {
				s.writePeerError(ctx, peer, protocol.TypeAgentError, "", record.Agent.ID, record.Home.ID, "agent_id_mismatch", "register payload agent ID does not match token agent ID", nil)
				continue
			}

			now := time.Now().UTC()
			agent := record.Agent
			agent.Status = domain.AgentStatusOnline
			agent.LastSeenAt = &now
			agent.UpdatedAt = now
			if payload.HomeName != "" {
				record.Home.Name = payload.HomeName
			}
			if err := s.store.UpsertAgent(ctx, agent); err != nil {
				s.logger.Error("failed to mark agent online", "agent_id", agent.ID, "error", err)
				return
			}
			s.router.RegisterAgent(record.Home.ID, agent, peer, nil)
			s.metrics.SetOnlineAgents(s.router.AgentCount())
			s.emitHomeStatus(ctx, record.Home.ID, map[string]any{"home_id": record.Home.ID, "agent_id": agent.ID, "status": domain.AgentStatusOnline})

			reply, err := protocol.NewEnvelope(protocol.TypeAgentRegistered, "", agent.ID, record.Home.ID, protocol.AgentRegistered{
				AcceptedAt: now,
				HomeID:     record.Home.ID,
				Message:    "agent registered",
			})
			if err != nil {
				s.logger.Error("failed to encode registration reply", "agent_id", agent.ID, "error", err)
				return
			}
			if err := peer.Write(ctx, reply); err != nil {
				s.logger.Warn("failed to write registration reply", "agent_id", agent.ID, "error", err)
				return
			}

		case protocol.TypeAgentHeartbeat:
			payload, err := protocol.DecodePayload[protocol.AgentHeartbeat](envelope)
			if err != nil {
				s.writePeerError(ctx, peer, protocol.TypeAgentError, envelope.RequestID, record.Agent.ID, record.Home.ID, "bad_heartbeat_payload", err.Error(), nil)
				continue
			}
			now := time.Now().UTC()
			_ = s.store.SetAgentStatus(ctx, record.Agent.ID, domain.AgentStatusOnline, &now)
			s.router.UpdateAgentCapabilities(record.Home.ID, payload.Capabilities)
			if slices.Contains(payload.Capabilities, "notes.sync") {
				if state, err := s.store.GetHomeNoteSyncState(ctx, record.Home.ID); err == nil {
					if state.LastManifestAt == nil || now.Sub(*state.LastManifestAt) > time.Minute {
						s.scheduleHomeSync(record.Home, record.Agent.ID)
					}
				} else if errors.Is(err, store.ErrNotFound) {
					s.scheduleHomeSync(record.Home, record.Agent.ID)
				}
			}

		case protocol.TypeAgentEvent:
			s.handleAgentEvent(ctx, record.Home.ID, envelope)

		case protocol.TypeCloudResponse:
			s.handleAgentResponse(ctx, envelope)

		case protocol.TypeFileTransferReady:
			s.handleTransferReady(envelope)

		case protocol.TypeFileTransferData:
			s.handleTransferData(envelope)

		case protocol.TypeFileTransferComplete:
			s.handleTransferComplete(envelope)

		case protocol.TypeFileTransferError:
			s.handleTransferError(envelope)

		default:
			s.writePeerError(ctx, peer, protocol.TypeAgentError, envelope.RequestID, record.Agent.ID, record.Home.ID, "unsupported_message", "unsupported envelope type", nil)
		}
	}
}

func (s *Server) handleAppWebSocket(w http.ResponseWriter, r *http.Request) {
	if err := enforceSameOriginIfPresent(r); err != nil {
		s.metrics.IncAuthFailure("app_ws_bad_origin")
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	auth, err := s.appAuthFromRequest(r)
	if err != nil {
		s.metrics.IncAuthFailure("app_ws_unauthorized")
		s.logger.Warn("app websocket unauthorized", "request_id", requestIDFromContext(r.Context()), "client_ip", clientIP(r))
		http.Error(w, "unauthorized app session", http.StatusUnauthorized)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		s.logger.Error("failed to accept app websocket", "error", err)
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "connection closed")
	conn.SetReadLimit(maxWSMessageBytes)

	ctx := r.Context()
	appPeer := newWSPeer(conn)
	appConn := s.router.RegisterApp(auth.Session.ID, auth.User.ID, appPeer)
	s.metrics.SetOnlineApps(s.router.AppCount())
	s.logger.Info("app websocket connected", "request_id", requestIDFromContext(r.Context()), "user_id", auth.User.ID, "session_id", auth.Session.ID)
	defer func() {
		s.collaboration.removeApp(auth.Session.ID)
		s.router.UnregisterApp(auth.Session.ID)
		s.metrics.SetOnlineApps(s.router.AppCount())
		s.logger.Info("app websocket disconnected", "request_id", requestIDFromContext(r.Context()), "user_id", auth.User.ID, "session_id", auth.Session.ID)
	}()

	for {
		var envelope protocol.Envelope
		if err := wsjson.Read(ctx, conn, &envelope); err != nil {
			if closeStatus(err) != websocket.StatusNormalClosure {
				s.logger.Warn("app websocket read failed", "session_id", auth.Session.ID, "error", err)
			}
			return
		}

		if envelope.Type != protocol.TypeAppCommand {
			s.writePeerError(ctx, appPeer, protocol.TypeAppError, envelope.RequestID, "", envelope.HomeID, "unsupported_message", "app websocket only accepts app.command messages", nil)
			continue
		}

		command, err := protocol.DecodePayload[protocol.RoutedCommand](envelope)
		if err != nil {
			s.writePeerError(ctx, appPeer, protocol.TypeAppError, envelope.RequestID, "", envelope.HomeID, "bad_command_payload", err.Error(), nil)
			continue
		}
		if envelope.RequestID == "" {
			s.writePeerError(ctx, appPeer, protocol.TypeAppError, envelope.RequestID, "", envelope.HomeID, "invalid_request", "request_id is required", nil)
			continue
		}

		home, membership, err := s.requireSingletonHomeMembership(ctx, auth.User.ID)
		if err != nil {
			s.metrics.IncRouteFailure("home_not_found")
			s.writePeerError(ctx, appPeer, protocol.TypeAppError, envelope.RequestID, "", envelope.HomeID, "home_not_found", "home not found for this user", nil)
			continue
		}
		envelope.HomeID = home.ID

		if s.handleRealtimeCommand(ctx, appConn, appPeer, envelope, command) {
			continue
		}

		if feature := featureForCommand(command.Command); feature != "" {
			if err := s.requireHomeFeature(ctx, home, membership, auth.User.ID, feature); err != nil {
				code := "permission_denied"
				if errors.Is(err, errFeaturePermissionDenied) {
					s.writePeerError(ctx, appPeer, protocol.TypeAppError, envelope.RequestID, "", envelope.HomeID, code, err.Error(), nil)
					continue
				}
				s.writePeerError(ctx, appPeer, protocol.TypeAppError, envelope.RequestID, "", envelope.HomeID, "permission_check_failed", err.Error(), nil)
				continue
			}
		}

		if strings.HasPrefix(command.Command, "notes.") {
			if err := s.handleCloudNotesCommand(ctx, appPeer, envelope, auth, command); err != nil {
				s.logger.Warn("cloud notes command failed", "request_id", envelope.RequestID, "home_id", envelope.HomeID, "command", command.Command, "error", err)
			}
			continue
		}

		agentConn, ok := s.router.GetAgent(home.ID)
		if !ok {
			s.metrics.IncRouteFailure("agent_offline")
			s.writePeerError(ctx, appPeer, protocol.TypeAppError, envelope.RequestID, "", envelope.HomeID, "agent_offline", "target home agent is offline", nil)
			continue
		}

		if _, err := s.router.AddPending(context.Background(), envelope.RequestID, envelope.HomeID, command.Command, appConn, s.requestTimeout, s.handlePendingTimeout); err != nil {
			code := "request_rejected"
			statusMessage := err.Error()
			if errors.Is(err, ErrTooManyInFlight) {
				code = "too_many_in_flight_requests"
			}
			s.metrics.IncRouteFailure(code)
			s.writePeerError(ctx, appPeer, protocol.TypeAppError, envelope.RequestID, "", envelope.HomeID, code, statusMessage, nil)
			continue
		}

		s.logger.Info("routing app command", "request_id", envelope.RequestID, "session_id", auth.Session.ID, "user_id", auth.User.ID, "home_id", envelope.HomeID, "agent_id", agentConn.agent.ID, "command", command.Command)

		relay, err := protocol.NewEnvelope(protocol.TypeCloudCommand, envelope.RequestID, agentConn.agent.ID, envelope.HomeID, command)
		if err != nil {
			if _, ok := s.router.ResolvePending(envelope.RequestID); ok {
				s.metrics.IncRouteFailure("encoding_failed")
			}
			s.writePeerError(ctx, appPeer, protocol.TypeAppError, envelope.RequestID, "", envelope.HomeID, "encoding_failed", err.Error(), nil)
			continue
		}

		if err := agentConn.peer.Write(ctx, relay); err != nil {
			if _, ok := s.router.ResolvePending(envelope.RequestID); ok {
				s.metrics.IncRouteFailure("route_write_failed")
			}
			s.writePeerError(ctx, appPeer, protocol.TypeAppError, envelope.RequestID, "", envelope.HomeID, "route_write_failed", err.Error(), nil)
			continue
		}
	}
}

func (s *Server) handleAgentResponse(ctx context.Context, envelope protocol.Envelope) {
	if s.agentRequests.Resolve(envelope) {
		return
	}

	pending, ok := s.router.ResolvePending(envelope.RequestID)
	if !ok {
		return
	}

	duration := time.Since(pending.startedAt)
	failed := envelope.Error != nil
	s.metrics.RecordCommand(pending.command, duration, failed)
	s.logger.Info("agent response relayed", "request_id", envelope.RequestID, "home_id", envelope.HomeID, "agent_id", envelope.AgentID, "command", pending.command, "duration", duration.String(), "failed", failed)

	if envelope.Error != nil {
		_ = pending.app.peer.Write(ctx, protocol.NewErrorEnvelope(protocol.TypeAppError, envelope.RequestID, envelope.AgentID, envelope.HomeID, envelope.Error.Code, envelope.Error.Message, envelope.Error.Details))
		return
	}

	response := protocol.Envelope{
		Version:   protocol.Version,
		Type:      protocol.TypeAppResponse,
		RequestID: envelope.RequestID,
		AgentID:   envelope.AgentID,
		HomeID:    envelope.HomeID,
		Timestamp: time.Now().UTC(),
		Payload:   envelope.Payload,
	}
	_ = pending.app.peer.Write(ctx, response)
	s.emitCommandSideEffect(ctx, pending.command, envelope.Payload)
}

func (s *Server) handlePendingTimeout(ctx context.Context, pending *pendingRequest) {
	duration := time.Since(pending.startedAt)
	s.metrics.IncRouteFailure("request_timeout")
	s.metrics.RecordCommand(pending.command, duration, true)
	s.logger.Warn("app command timed out", "request_id", pending.requestID, "home_id", pending.homeID, "command", pending.command, "duration", duration.String())
	_ = pending.app.peer.Write(ctx, protocol.NewErrorEnvelope(protocol.TypeAppError, pending.requestID, "", pending.homeID, "request_timeout", "agent did not respond before timeout", nil))
}

func (s *Server) handleFileTransferSetup(w http.ResponseWriter, r *http.Request, home domain.Home, operation string) {
	type request struct {
		Path string `json:"path"`
	}

	var body request
	if err := parseJSON(w, r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	body.Path = strings.TrimSpace(body.Path)
	if body.Path == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}

	agentConn, ok := s.router.GetAgent(home.ID)
	if !ok {
		http.Error(w, "target home agent is offline", http.StatusBadGateway)
		return
	}

	transfer, rawToken := s.transfers.Create(home.ID, agentConn.agent.ID, operation, body.Path, 10*time.Minute)

	method := http.MethodGet
	if operation == protocol.FileTransferOperationUpload {
		method = http.MethodPut
	}

	payload := transfer.Snapshot()
	payload["transfer_token"] = rawToken
	payload["method"] = method
	payload["url"] = "/v1/file-transfers/" + transfer.ID + "?token=" + rawToken
	writeJSON(w, http.StatusCreated, payload)
}

func (s *Server) handleFileTransfer(w http.ResponseWriter, r *http.Request) {
	transferID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/file-transfers/"), "/")
	if transferID == "" {
		http.NotFound(w, r)
		return
	}

	rawToken := strings.TrimSpace(r.URL.Query().Get("token"))
	if rawToken == "" {
		http.Error(w, "transfer token is required", http.StatusUnauthorized)
		return
	}

	switch r.Method {
	case http.MethodGet:
		transfer, err := s.transfers.Authorize(transferID, rawToken, protocol.FileTransferOperationDownload)
		if err != nil {
			http.Error(w, "transfer not found", http.StatusNotFound)
			return
		}
		offset, err := offsetParam(r, 0)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		attempt, err := s.transfers.BeginAttempt(transfer, offset)
		if err != nil {
			s.writeTransferAttemptError(w, transfer, err)
			return
		}
		defer s.transfers.EndAttempt(attempt.ID)

		agentConn, ok := s.router.GetAgent(transfer.HomeID)
		if !ok {
			http.Error(w, "target home agent is offline", http.StatusBadGateway)
			return
		}

		open, err := protocol.NewEnvelope(protocol.TypeFileTransferOpen, attempt.ID, agentConn.agent.ID, transfer.HomeID, protocol.FileTransferOpen{
			Operation: protocol.FileTransferOperationDownload,
			Path:      transfer.Path,
			Offset:    offset,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := agentConn.peer.Write(r.Context(), open); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		ready, protocolErr, err := s.waitTransferReady(r.Context(), attempt)
		if err != nil {
			http.Error(w, err.Error(), http.StatusGatewayTimeout)
			return
		}
		if protocolErr != nil {
			transfer.Fail(protocolErr)
			http.Error(w, protocolErr.Message, http.StatusBadGateway)
			return
		}
		transfer.MarkReady(ready)

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(transfer.Path)+`"`)
		if remaining := ready.Size - offset; remaining >= 0 {
			w.Header().Set("Content-Length", int64ToString(remaining))
		}

		flusher, _ := w.(http.Flusher)
		currentOffset := offset
		var pendingComplete *protocol.FileTransferComplete
		for {
			select {
			case <-r.Context().Done():
				transfer.Advance(currentOffset, ready.Size)
				return
			case frame := <-attempt.DataCh:
				if frame.Error != nil {
					transfer.Fail(frame.Error)
					return
				}
				if frame.Offset != currentOffset {
					transfer.Fail(&protocol.ErrorPayload{Code: "transfer_offset_mismatch", Message: "download stream offset mismatch"})
					return
				}
				if len(frame.Data) == 0 {
					continue
				}
				if _, err := w.Write(frame.Data); err != nil {
					transfer.Advance(currentOffset, ready.Size)
					return
				}
				currentOffset += int64(len(frame.Data))
				transfer.Advance(currentOffset, ready.Size)
				if pendingComplete != nil && currentOffset >= pendingComplete.Offset {
					transfer.Complete(*pendingComplete)
					return
				}
				if flusher != nil {
					flusher.Flush()
				}
			case result := <-attempt.CompleteCh:
				protocolErr := result.Error
				if protocolErr != nil {
					transfer.Fail(protocolErr)
					return
				}
				complete := result.Complete
				if currentOffset >= complete.Offset {
					transfer.Complete(complete)
					return
				}
				pendingComplete = &complete
			}
		}

	case http.MethodPut:
		transfer, err := s.transfers.Authorize(transferID, rawToken, protocol.FileTransferOperationUpload)
		if err != nil {
			http.Error(w, "transfer not found", http.StatusNotFound)
			return
		}
		offset, err := offsetParam(r, transfer.NextOffset())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		attempt, err := s.transfers.BeginAttempt(transfer, offset)
		if err != nil {
			s.writeTransferAttemptError(w, transfer, err)
			return
		}
		defer s.transfers.EndAttempt(attempt.ID)

		agentConn, ok := s.router.GetAgent(transfer.HomeID)
		if !ok {
			http.Error(w, "target home agent is offline", http.StatusBadGateway)
			return
		}

		open, err := protocol.NewEnvelope(protocol.TypeFileTransferOpen, attempt.ID, agentConn.agent.ID, transfer.HomeID, protocol.FileTransferOpen{
			Operation: protocol.FileTransferOperationUpload,
			Path:      transfer.Path,
			Offset:    offset,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := agentConn.peer.Write(r.Context(), open); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		ready, protocolErr, err := s.waitTransferReady(r.Context(), attempt)
		if err != nil {
			http.Error(w, err.Error(), http.StatusGatewayTimeout)
			return
		}
		if protocolErr != nil {
			transfer.Fail(protocolErr)
			http.Error(w, protocolErr.Message, http.StatusBadGateway)
			return
		}
		transfer.MarkReady(ready)
		if ready.Offset != offset {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":       "transfer_offset_mismatch",
				"next_offset": ready.Offset,
				"size":        ready.Size,
			})
			return
		}

		buffer := make([]byte, 32*1024)
		currentOffset := offset
		for {
			n, err := r.Body.Read(buffer)
			if n > 0 {
				envelope, envelopeErr := protocol.NewEnvelope(protocol.TypeFileTransferData, attempt.ID, agentConn.agent.ID, transfer.HomeID, protocol.FileTransferChunk{
					Offset:        currentOffset,
					ContentBase64: base64.StdEncoding.EncodeToString(buffer[:n]),
				})
				if envelopeErr != nil {
					http.Error(w, envelopeErr.Error(), http.StatusInternalServerError)
					return
				}
				if err := agentConn.peer.Write(r.Context(), envelope); err != nil {
					http.Error(w, err.Error(), http.StatusBadGateway)
					return
				}
				currentOffset += int64(n)
				transfer.Advance(currentOffset, currentOffset)
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}

		complete, err := protocol.NewEnvelope(protocol.TypeFileTransferComplete, attempt.ID, agentConn.agent.ID, transfer.HomeID, protocol.FileTransferComplete{
			Operation: protocol.FileTransferOperationUpload,
			Path:      transfer.Path,
			Offset:    currentOffset,
			Size:      currentOffset,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := agentConn.peer.Write(r.Context(), complete); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}

		done, protocolErr, err := s.waitTransferComplete(r.Context(), attempt)
		if err != nil {
			http.Error(w, err.Error(), http.StatusGatewayTimeout)
			return
		}
		if protocolErr != nil {
			transfer.Fail(protocolErr)
			http.Error(w, protocolErr.Message, http.StatusBadGateway)
			return
		}
		transfer.Complete(done)

		writeJSON(w, http.StatusOK, map[string]any{
			"ok":          true,
			"path":        done.Path,
			"size":        done.Size,
			"next_offset": done.Offset,
			"resumable":   true,
		})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleTransferReady(envelope protocol.Envelope) {
	attempt, ok := s.transfers.GetAttempt(envelope.RequestID)
	if !ok {
		return
	}
	if envelope.Error != nil {
		attempt.ReadyCh <- transferReadyResult{Error: envelope.Error}
		return
	}
	ready, err := protocol.DecodePayload[protocol.FileTransferReady](envelope)
	if err != nil {
		attempt.ReadyCh <- transferReadyResult{Error: &protocol.ErrorPayload{Code: "invalid_transfer_ready", Message: err.Error()}}
		return
	}
	attempt.ReadyCh <- transferReadyResult{Ready: ready}
}

func (s *Server) handleTransferData(envelope protocol.Envelope) {
	attempt, ok := s.transfers.GetAttempt(envelope.RequestID)
	if !ok {
		return
	}
	chunk, err := protocol.DecodePayload[protocol.FileTransferChunk](envelope)
	if err != nil {
		attempt.DataCh <- transferDataFrame{Error: &protocol.ErrorPayload{Code: "invalid_transfer_chunk", Message: err.Error()}}
		return
	}
	data, err := base64.StdEncoding.DecodeString(chunk.ContentBase64)
	if err != nil {
		attempt.DataCh <- transferDataFrame{Error: &protocol.ErrorPayload{Code: "invalid_transfer_chunk", Message: err.Error()}}
		return
	}
	attempt.DataCh <- transferDataFrame{Offset: chunk.Offset, Data: data}
}

func (s *Server) handleTransferComplete(envelope protocol.Envelope) {
	attempt, ok := s.transfers.GetAttempt(envelope.RequestID)
	if !ok {
		return
	}
	if envelope.Error != nil {
		attempt.CompleteCh <- transferCompleteResult{Error: envelope.Error}
		return
	}
	complete, err := protocol.DecodePayload[protocol.FileTransferComplete](envelope)
	if err != nil {
		attempt.CompleteCh <- transferCompleteResult{Error: &protocol.ErrorPayload{Code: "invalid_transfer_complete", Message: err.Error()}}
		return
	}
	attempt.CompleteCh <- transferCompleteResult{Complete: complete}
}

func (s *Server) handleTransferError(envelope protocol.Envelope) {
	attempt, ok := s.transfers.GetAttempt(envelope.RequestID)
	if !ok {
		return
	}
	if envelope.Error == nil {
		attempt.ReadyCh <- transferReadyResult{Error: &protocol.ErrorPayload{Code: "transfer_failed", Message: "unknown transfer failure"}}
		attempt.CompleteCh <- transferCompleteResult{Error: &protocol.ErrorPayload{Code: "transfer_failed", Message: "unknown transfer failure"}}
		return
	}
	attempt.ReadyCh <- transferReadyResult{Error: envelope.Error}
	attempt.CompleteCh <- transferCompleteResult{Error: envelope.Error}
}

func (s *Server) createSession(ctx context.Context, userID string) (domain.AppSession, string, error) {
	rawToken := newToken()
	now := time.Now().UTC()
	session := domain.AppSession{
		ID:        newID("sess"),
		UserID:    userID,
		TokenHash: hashToken(rawToken),
		ExpiresAt: now.Add(s.sessionTTL),
		CreatedAt: now,
	}
	if err := s.store.CreateSession(ctx, session); err != nil {
		return domain.AppSession{}, "", err
	}
	return session, rawToken, nil
}

func (s *Server) requireAuth(w http.ResponseWriter, r *http.Request) (authContext, bool) {
	auth, err := s.appAuthFromRequest(r)
	if err != nil {
		s.metrics.IncAuthFailure("app_http_unauthorized")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return authContext{}, false
	}
	return auth, true
}

func (s *Server) appAuthFromRequest(r *http.Request) (authContext, error) {
	rawToken := sessionTokenFromCookie(r)
	if rawToken == "" {
		token, err := bearerToken(r.Header.Get("Authorization"))
		if err == nil {
			rawToken = token
		}
	}
	if rawToken == "" {
		appTicket := strings.TrimSpace(r.URL.Query().Get("app_ticket"))
		if appTicket != "" {
			ticket, err := s.appTickets.Consume(appTicket)
			if err != nil {
				return authContext{}, err
			}
			session, err := s.store.GetSessionByID(r.Context(), ticket.SessionID)
			if err != nil {
				return authContext{}, err
			}
			user, err := s.store.GetUserByID(r.Context(), session.UserID)
			if err != nil {
				return authContext{}, err
			}
			return authContext{User: user, Session: session}, nil
		}
	}
	if rawToken == "" {
		rawToken = strings.TrimSpace(r.URL.Query().Get("session_token"))
		if rawToken == "" {
			return authContext{}, errors.New("missing session token")
		}
	}

	session, err := s.store.GetSessionByHash(r.Context(), hashToken(rawToken))
	if err != nil {
		return authContext{}, err
	}
	user, err := s.store.GetUserByID(r.Context(), session.UserID)
	if err != nil {
		return authContext{}, err
	}
	return authContext{User: user, Session: session}, nil
}

func (s *Server) writePeerError(ctx context.Context, peer *wsPeer, messageType string, requestID string, agentID string, homeID string, code string, message string, details map[string]any) {
	_ = peer.Write(ctx, protocol.NewErrorEnvelope(messageType, requestID, agentID, homeID, code, message, details))
}

func sanitizeUser(user domain.User) map[string]any {
	return map[string]any{
		"id":         user.ID,
		"email":      user.Email,
		"created_at": user.CreatedAt,
		"updated_at": user.UpdatedAt,
	}
}

func parseJSON(w http.ResponseWriter, r *http.Request, out any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxHTTPBodyBytes)
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(out)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if requestID == "" {
			requestID = newID("req")
		}
		w.Header().Set("X-Request-ID", requestID)
		ctx := context.WithValue(r.Context(), requestIDContextKey{}, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type requestIDContextKey struct{}

func requestIDFromContext(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDContextKey{}).(string)
	return requestID
}

func clientIP(r *http.Request) string {
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		return strings.TrimSpace(parts[0])
	}
	return r.RemoteAddr
}

func closeStatus(err error) websocket.StatusCode {
	var closeErr websocket.CloseError
	if errors.As(err, &closeErr) {
		return closeErr.Code
	}
	return websocket.StatusInternalError
}

func int64ToString(value int64) string {
	return strconv.FormatInt(value, 10)
}

func offsetParam(r *http.Request, fallback int64) (int64, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("offset"))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 0, errors.New("offset must be a non-negative integer")
	}
	return value, nil
}

func (s *Server) waitTransferReady(ctx context.Context, attempt *transferAttempt) (protocol.FileTransferReady, *protocol.ErrorPayload, error) {
	select {
	case <-ctx.Done():
		return protocol.FileTransferReady{}, nil, ctx.Err()
	case result := <-attempt.ReadyCh:
		return result.Ready, result.Error, nil
	}
}

func (s *Server) waitTransferComplete(ctx context.Context, attempt *transferAttempt) (protocol.FileTransferComplete, *protocol.ErrorPayload, error) {
	select {
	case <-ctx.Done():
		return protocol.FileTransferComplete{}, nil, ctx.Err()
	case result := <-attempt.CompleteCh:
		return result.Complete, result.Error, nil
	}
}

func (s *Server) writeTransferAttemptError(w http.ResponseWriter, transfer *transferSession, err error) {
	switch {
	case errors.Is(err, ErrTransferBusy):
		http.Error(w, "transfer is already active", http.StatusConflict)
	case errors.Is(err, ErrTransferOffsetInvalid):
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":       "transfer_offset_mismatch",
			"next_offset": transfer.NextOffset(),
		})
	case errors.Is(err, ErrTransferNotFound):
		http.Error(w, "transfer not found", http.StatusNotFound)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

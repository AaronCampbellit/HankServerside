package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/dropfile/hankremote/internal/domain"
)

var ErrTooManyInFlight = errors.New("too many in-flight requests")

const (
	AgentTypePrimary = "primary"
	AgentTypeWorker  = "worker"
)

type agentConnection struct {
	connectionID string
	agent        domain.Agent
	homeID       string
	agentType    string
	peer         *wsPeer
	capabilities []string
	metadata     map[string]string
	metrics      json.RawMessage
}

func (c *agentConnection) isPrimary() bool {
	return c.agentType == "" || c.agentType == AgentTypePrimary
}

type appConnection struct {
	connectionID  string
	sessionID     string
	userID        string
	peer          *wsPeer
	mu            sync.Mutex
	inFlight      int
	subscriptions map[string]struct{}
}

type pendingRequest struct {
	requestID string
	homeID    string
	command   string
	fileJobID string
	app       *appConnection
	startedAt time.Time
	timer     *time.Timer
}

type AgentSnapshot struct {
	AgentID      string          `json:"agent_id"`
	HomeID       string          `json:"home_id"`
	HomeName     string          `json:"home_name,omitempty"`
	AgentType    string          `json:"agent_type,omitempty"`
	Status       string          `json:"status"`
	Capabilities []string        `json:"capabilities,omitempty"`
	LastSeenAt   *time.Time      `json:"last_seen_at,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	Metrics      json.RawMessage `json:"metrics,omitempty"`
}

type Router struct {
	mu sync.RWMutex
	// Per home: every connected agent by agent ID. Exactly one may be the
	// primary (the home agent that owns HA, notes sync, SMB); the rest are
	// workers (desktops/laptops exposing their own capabilities).
	agentsByHomeID       map[string]map[string]*agentConnection
	primaryAgentByHomeID map[string]string
	appsByConnectionID   map[string]*appConnection
	pendingByID          map[string]*pendingRequest
}

func NewRouter() *Router {
	return &Router{
		agentsByHomeID:       make(map[string]map[string]*agentConnection),
		primaryAgentByHomeID: make(map[string]string),
		appsByConnectionID:   make(map[string]*appConnection),
		pendingByID:          make(map[string]*pendingRequest),
	}
}

func (r *Router) RegisterAgent(homeID string, agent domain.Agent, peer *wsPeer, capabilities []string, agentType string, metadata map[string]string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	connectionID := newID("agentconn")
	connection := &agentConnection{
		connectionID: connectionID,
		agent:        agent,
		homeID:       homeID,
		agentType:    agentType,
		peer:         peer,
		capabilities: append([]string(nil), capabilities...),
		metadata:     metadata,
	}
	if r.agentsByHomeID[homeID] == nil {
		r.agentsByHomeID[homeID] = make(map[string]*agentConnection)
	}
	r.agentsByHomeID[homeID][agent.ID] = connection
	if connection.isPrimary() {
		// A reconnecting or replacement primary takes over primary routing;
		// any previous primary connection entry for a different agent ID is
		// left in place as a plain agent until it disconnects.
		r.primaryAgentByHomeID[homeID] = agent.ID
	}
	return connectionID
}

func (r *Router) UpdateAgentCapabilities(homeID string, agentID string, capabilities []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if agent := r.agentsByHomeID[homeID][agentID]; agent != nil {
		agent.capabilities = append([]string(nil), capabilities...)
	}
}

func (r *Router) UpdateAgentMetrics(homeID string, agentID string, metrics json.RawMessage) {
	if len(metrics) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if agent := r.agentsByHomeID[homeID][agentID]; agent != nil {
		agent.metrics = append(json.RawMessage(nil), metrics...)
	}
}

func (r *Router) UnregisterAgent(homeID string, agentID string, connectionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	agents := r.agentsByHomeID[homeID]
	current := agents[agentID]
	if current == nil || (connectionID != "" && current.connectionID != connectionID) {
		return
	}
	delete(agents, agentID)
	if len(agents) == 0 {
		delete(r.agentsByHomeID, homeID)
	}
	if r.primaryAgentByHomeID[homeID] == agentID {
		delete(r.primaryAgentByHomeID, homeID)
	}
}

// GetAgent returns the home's primary agent — the pre-multi-agent behavior
// every untargeted command and relay path still relies on.
func (r *Router) GetAgent(homeID string) (*agentConnection, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	primaryID, ok := r.primaryAgentByHomeID[homeID]
	if !ok {
		return nil, false
	}
	agent, ok := r.agentsByHomeID[homeID][primaryID]
	return agent, ok
}

// ResolveAgent picks the primary when agentID is blank, otherwise the exact
// connected agent. Used by paths that support explicit agent targeting.
func (r *Router) ResolveAgent(homeID string, agentID string) (*agentConnection, bool) {
	if agentID == "" {
		return r.GetAgent(homeID)
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	agent, ok := r.agentsByHomeID[homeID][agentID]
	return agent, ok
}

// AgentsForHome snapshots every connected agent for the home.
func (r *Router) AgentsForHome(homeID string) []AgentSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	agents := r.agentsByHomeID[homeID]
	snapshots := make([]AgentSnapshot, 0, len(agents))
	for _, connection := range agents {
		agentType := connection.agentType
		if agentType == "" {
			agentType = AgentTypePrimary
		}
		snapshots = append(snapshots, AgentSnapshot{
			AgentID:      connection.agent.ID,
			HomeID:       homeID,
			AgentType:    agentType,
			Status:       domain.AgentStatusOnline,
			Capabilities: append([]string(nil), connection.capabilities...),
			LastSeenAt:   connection.agent.LastSeenAt,
			Metadata:     connection.metadata,
			Metrics:      connection.metrics,
		})
	}
	return snapshots
}

func (r *Router) RegisterApp(sessionID string, userID string, peer *wsPeer) *appConnection {
	r.mu.Lock()
	defer r.mu.Unlock()
	connection := &appConnection{
		connectionID:  newID("appconn"),
		sessionID:     sessionID,
		userID:        userID,
		peer:          peer,
		subscriptions: make(map[string]struct{}),
	}
	r.appsByConnectionID[connection.connectionID] = connection
	return connection
}

func (r *Router) AppsForTopic(topic string) []*appConnection {
	r.mu.RLock()
	defer r.mu.RUnlock()

	apps := make([]*appConnection, 0)
	for _, app := range r.appsByConnectionID {
		if app.isSubscribed(topic) {
			apps = append(apps, app)
		}
	}
	return apps
}

func (r *Router) UnregisterApp(connectionID string) {
	r.mu.Lock()
	app, ok := r.appsByConnectionID[connectionID]
	delete(r.appsByConnectionID, connectionID)
	if ok {
		for requestID, pending := range r.pendingByID {
			if pending.app.connectionID == app.connectionID {
				if pending.timer != nil {
					pending.timer.Stop()
				}
				pending.app.release()
				delete(r.pendingByID, requestID)
			}
		}
	}
	r.mu.Unlock()
}

func (r *Router) AddPending(ctx context.Context, requestID string, homeID string, command string, fileJobID string, app *appConnection, timeout time.Duration, onTimeout func(context.Context, *pendingRequest)) (*pendingRequest, error) {
	if !app.acquire() {
		return nil, ErrTooManyInFlight
	}

	r.mu.Lock()
	if _, exists := r.pendingByID[requestID]; exists {
		r.mu.Unlock()
		app.release()
		return nil, errors.New("duplicate request_id")
	}

	pending := &pendingRequest{
		requestID: requestID,
		homeID:    homeID,
		command:   command,
		fileJobID: fileJobID,
		app:       app,
		startedAt: time.Now().UTC(),
	}
	pending.timer = time.AfterFunc(timeout, func() {
		r.mu.Lock()
		current, ok := r.pendingByID[requestID]
		if ok {
			delete(r.pendingByID, requestID)
		}
		r.mu.Unlock()
		if ok {
			current.app.release()
			onTimeout(ctx, current)
		}
	})

	r.pendingByID[requestID] = pending
	r.mu.Unlock()
	return pending, nil
}

func (r *Router) ResolvePending(requestID string) (*pendingRequest, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	pending, ok := r.pendingByID[requestID]
	if !ok {
		return nil, false
	}
	delete(r.pendingByID, requestID)
	if pending.timer != nil {
		pending.timer.Stop()
	}
	pending.app.release()
	return pending, true
}

func (r *Router) PendingCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.pendingByID)
}

func (r *Router) AgentCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	total := 0
	for _, agents := range r.agentsByHomeID {
		total += len(agents)
	}
	return total
}

func (r *Router) AppCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.appsByConnectionID)
}

func (r *Router) Snapshots(homeNames map[string]string, agents map[string]domain.Agent) []AgentSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()

	snapshots := make([]AgentSnapshot, 0, len(r.agentsByHomeID))
	for homeID, connections := range r.agentsByHomeID {
		for _, connection := range connections {
			agent := agents[connection.agent.ID]
			snapshots = append(snapshots, AgentSnapshot{
				AgentID:      connection.agent.ID,
				HomeID:       homeID,
				HomeName:     homeNames[homeID],
				AgentType:    connection.agentType,
				Status:       agent.Status,
				Capabilities: append([]string(nil), connection.capabilities...),
				LastSeenAt:   agent.LastSeenAt,
			})
		}
	}
	return snapshots
}

func (c *appConnection) acquire() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.inFlight >= 32 {
		return false
	}
	c.inFlight++
	return true
}

func (c *appConnection) release() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.inFlight > 0 {
		c.inFlight--
	}
}

func (c *appConnection) subscribe(topics []string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.subscriptions == nil {
		c.subscriptions = make(map[string]struct{})
	}
	for _, topic := range topics {
		if topic != "" {
			c.subscriptions[topic] = struct{}{}
		}
	}
	return c.subscriptionListLocked()
}

func (c *appConnection) unsubscribe(topics []string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, topic := range topics {
		delete(c.subscriptions, topic)
	}
	return c.subscriptionListLocked()
}

func (c *appConnection) isSubscribed(topic string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.subscriptions[topic]
	return ok
}

func (c *appConnection) subscriptionListLocked() []string {
	topics := make([]string, 0, len(c.subscriptions))
	for topic := range c.subscriptions {
		topics = append(topics, topic)
	}
	return topics
}

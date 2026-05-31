package cloud

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/dropfile/hankremote/internal/domain"
)

var ErrTooManyInFlight = errors.New("too many in-flight requests")

type agentConnection struct {
	connectionID string
	agent        domain.Agent
	homeID       string
	peer         *wsPeer
	capabilities []string
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
	AgentID      string     `json:"agent_id"`
	HomeID       string     `json:"home_id"`
	HomeName     string     `json:"home_name,omitempty"`
	Status       string     `json:"status"`
	Capabilities []string   `json:"capabilities,omitempty"`
	LastSeenAt   *time.Time `json:"last_seen_at,omitempty"`
}

type Router struct {
	mu                 sync.RWMutex
	agentsByHomeID     map[string]*agentConnection
	appsByConnectionID map[string]*appConnection
	pendingByID        map[string]*pendingRequest
}

func NewRouter() *Router {
	return &Router{
		agentsByHomeID:     make(map[string]*agentConnection),
		appsByConnectionID: make(map[string]*appConnection),
		pendingByID:        make(map[string]*pendingRequest),
	}
}

func (r *Router) RegisterAgent(homeID string, agent domain.Agent, peer *wsPeer, capabilities []string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	connectionID := newID("agentconn")
	r.agentsByHomeID[homeID] = &agentConnection{
		connectionID: connectionID,
		agent:        agent,
		homeID:       homeID,
		peer:         peer,
		capabilities: append([]string(nil), capabilities...),
	}
	return connectionID
}

func (r *Router) UpdateAgentCapabilities(homeID string, capabilities []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if agent := r.agentsByHomeID[homeID]; agent != nil {
		agent.capabilities = append([]string(nil), capabilities...)
	}
}

func (r *Router) UnregisterAgent(homeID string, connectionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if current := r.agentsByHomeID[homeID]; current != nil && (connectionID == "" || current.connectionID == connectionID) {
		delete(r.agentsByHomeID, homeID)
	}
}

func (r *Router) GetAgent(homeID string) (*agentConnection, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	agent, ok := r.agentsByHomeID[homeID]
	return agent, ok
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
	return len(r.agentsByHomeID)
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
	for homeID, connection := range r.agentsByHomeID {
		agent := agents[connection.agent.ID]
		snapshots = append(snapshots, AgentSnapshot{
			AgentID:      connection.agent.ID,
			HomeID:       homeID,
			HomeName:     homeNames[homeID],
			Status:       agent.Status,
			Capabilities: append([]string(nil), connection.capabilities...),
			LastSeenAt:   agent.LastSeenAt,
		})
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

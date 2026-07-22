package observability

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

type Metrics struct {
	mu                       sync.Mutex
	onlineAgents             int
	onlineApps               int
	authFailures             map[string]int64
	commandCounts            map[string]int64
	commandFailures          map[string]int64
	commandLatency           map[string]time.Duration
	routeFailures            map[string]int64
	httpCounts               map[string]int64
	httpLatency              map[string]time.Duration
	assistantCounts          map[string]int64
	assistantErrors          map[string]int64
	relayLatency             time.Duration
	relayCount               int64
	relayFailures            int64
	desktopSessions          map[string]int64
	desktopJoin              map[string]int64
	desktopReconnect         map[string]int64
	desktopTerminated        map[string]int64
	desktopRelayBytes        map[string]int64
	desktopBackpressure      map[string]int64
	desktopReadiness         map[string]int64
	desktopReadinessReported map[string]int64
}

func NewMetrics() *Metrics {
	metrics := &Metrics{
		authFailures:    make(map[string]int64),
		commandCounts:   make(map[string]int64),
		commandFailures: make(map[string]int64),
		commandLatency:  make(map[string]time.Duration),
		routeFailures:   make(map[string]int64),
		httpCounts:      make(map[string]int64),
		httpLatency:     make(map[string]time.Duration),
		assistantCounts: map[string]int64{"unknown": 0},
		assistantErrors: map[string]int64{"unknown": 0},
		desktopSessions: make(map[string]int64), desktopJoin: make(map[string]int64), desktopReconnect: make(map[string]int64),
		desktopTerminated: make(map[string]int64), desktopRelayBytes: make(map[string]int64), desktopBackpressure: make(map[string]int64), desktopReadiness: make(map[string]int64), desktopReadinessReported: make(map[string]int64),
	}
	for _, platform := range []string{"windows", "macos", "unknown"} {
		for _, state := range []string{"requested", "offered", "agent_ready", "joining", "active", "reconnecting", "denied", "expired", "terminated", "failed"} {
			metrics.desktopSessions[platform+"\x00"+state] = 0
		}
	}
	for _, side := range []string{"browser", "agent"} {
		for _, outcome := range []string{"success", "failed", "expired", "replayed"} {
			metrics.desktopJoin[side+"\x00"+outcome] = 0
		}
	}
	for _, outcome := range []string{"success", "failed", "expired"} {
		metrics.desktopReconnect[outcome] = 0
	}
	for _, reason := range []string{"user_ended", "agent_ended", "local_ended", "slow_consumer", "rate_limit", "frame_limit", "idle_timeout", "join_timeout", "hard_expired", "revoked", "transport_closed", "unknown"} {
		metrics.desktopTerminated[reason] = 0
	}
	for _, direction := range []string{"browser_to_agent", "agent_to_browser"} {
		metrics.desktopRelayBytes[direction] = 0
		metrics.desktopBackpressure[direction] = 0
	}
	for _, platform := range []string{"windows", "macos"} {
		for _, check := range []string{"service", "daemon", "host", "indicator", "capture", "control", "identity", "certificate"} {
			key := platform + "\x00" + check
			metrics.desktopReadiness[key] = 0
			metrics.desktopReadinessReported[key] = 0
		}
	}
	return metrics
}

func (m *Metrics) SetDesktopSessions(state, platform string, count int64) {
	if !allowed(state, "requested", "offered", "agent_ready", "joining", "active", "reconnecting", "denied", "expired", "terminated", "failed") || !allowed(platform, "windows", "macos", "unknown") {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.desktopSessions[platform+"\x00"+state] = count
}
func (m *Metrics) IncDesktopJoin(side, outcome string) {
	if !allowed(side, "browser", "agent") || !allowed(outcome, "success", "failed", "expired", "replayed") {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.desktopJoin[side+"\x00"+outcome]++
}
func (m *Metrics) IncDesktopReconnect(outcome string) {
	if allowed(outcome, "success", "failed", "expired") {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.desktopReconnect[outcome]++
	}
}
func (m *Metrics) IncDesktopTerminated(reason string) {
	if !allowed(reason, "user_ended", "agent_ended", "local_ended", "slow_consumer", "rate_limit", "frame_limit", "idle_timeout", "join_timeout", "hard_expired", "revoked", "transport_closed", "unknown") {
		reason = "unknown"
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.desktopTerminated[reason]++
}
func (m *Metrics) AddDesktopRelayBytes(direction string, count int64) {
	if count >= 0 && allowed(direction, "browser_to_agent", "agent_to_browser") {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.desktopRelayBytes[direction] += count
	}
}
func (m *Metrics) IncDesktopRelayBackpressure(direction string) {
	if allowed(direction, "browser_to_agent", "agent_to_browser") {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.desktopBackpressure[direction]++
	}
}
func (m *Metrics) SetDesktopReadiness(platform, check string, ready bool) {
	if !allowed(platform, "windows", "macos") || !allowed(check, "service", "daemon", "host", "indicator", "capture", "control", "identity", "certificate") {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if ready {
		m.desktopReadiness[platform+"\x00"+check] = 1
	} else {
		m.desktopReadiness[platform+"\x00"+check] = 0
	}
	m.desktopReadinessReported[platform+"\x00"+check] = 1
}

func (m *Metrics) SetOnlineAgents(count int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onlineAgents = count
}

func (m *Metrics) SetOnlineApps(count int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onlineApps = count
}

func (m *Metrics) IncAuthFailure(kind string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.authFailures[kind]++
}

func (m *Metrics) RecordCommand(command string, duration time.Duration, failed bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.commandCounts[command]++
	m.commandLatency[command] += duration
	if failed {
		m.commandFailures[command]++
	}
}

func (m *Metrics) IncRouteFailure(code string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.routeFailures[code]++
}

func (m *Metrics) RecordHTTP(route string, method string, status int, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s %s %d", method, route, status)
	m.httpCounts[key]++
	m.httpLatency[key] += duration
}

func (m *Metrics) RecordRelay(duration time.Duration, failed bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.relayCount++
	m.relayLatency += duration
	if failed {
		m.relayFailures++
	}
}

func (m *Metrics) RecordAssistantProvider(provider string, failed bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if provider == "" {
		provider = "unknown"
	}
	m.assistantCounts[provider]++
	if failed {
		m.assistantErrors[provider]++
	}
}

func (m *Metrics) RenderPrometheus() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	var b strings.Builder
	fmt.Fprintf(&b, "hank_remote_online_agents %d\n", m.onlineAgents)
	fmt.Fprintf(&b, "hank_remote_online_apps %d\n", m.onlineApps)

	writeLabelledCounter(&b, "hank_remote_auth_failures_total", "kind", m.authFailures)
	writeLabelledCounter(&b, "hank_remote_command_total", "command", m.commandCounts)
	writeLabelledCounter(&b, "hank_remote_command_failures_total", "command", m.commandFailures)
	writeLabelledCounter(&b, "hank_remote_route_failures_total", "code", m.routeFailures)
	writeLabelledCounter(&b, "hank_remote_http_requests_total", "route", m.httpCounts)
	writeLabelledCounter(&b, "hank_remote_assistant_provider_requests_total", "provider", m.assistantCounts)
	writeLabelledCounter(&b, "hank_remote_assistant_provider_errors_total", "provider", m.assistantErrors)
	writeTwoLabelledGauge(&b, "hank_desktop_sessions", "platform", "state", m.desktopSessions)
	writeTwoLabelledGauge(&b, "hank_desktop_join_total", "side", "outcome", m.desktopJoin)
	writeLabelledCounter(&b, "hank_desktop_reconnect_total", "outcome", m.desktopReconnect)
	writeLabelledCounter(&b, "hank_desktop_terminated_total", "reason", m.desktopTerminated)
	writeLabelledCounter(&b, "hank_desktop_relay_bytes_total", "direction", m.desktopRelayBytes)
	writeLabelledCounter(&b, "hank_desktop_relay_backpressure_total", "direction", m.desktopBackpressure)
	writeTwoLabelledGauge(&b, "hank_desktop_readiness", "platform", "check", m.desktopReadiness)
	writeTwoLabelledGauge(&b, "hank_desktop_readiness_reported", "platform", "check", m.desktopReadinessReported)

	keys := make([]string, 0, len(m.commandLatency))
	for key := range m.commandLatency {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		total := m.commandLatency[key].Seconds()
		fmt.Fprintf(&b, "hank_remote_command_latency_seconds_sum{command=%q} %.6f\n", key, total)
		fmt.Fprintf(&b, "hank_remote_command_latency_seconds_count{command=%q} %d\n", key, m.commandCounts[key])
	}
	keys = keys[:0]
	for key := range m.httpLatency {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(&b, "hank_remote_http_latency_seconds_sum{route=%q} %.6f\n", key, m.httpLatency[key].Seconds())
		fmt.Fprintf(&b, "hank_remote_http_latency_seconds_count{route=%q} %d\n", key, m.httpCounts[key])
	}
	fmt.Fprintf(&b, "hank_remote_relay_latency_seconds_sum %.6f\n", m.relayLatency.Seconds())
	fmt.Fprintf(&b, "hank_remote_relay_latency_seconds_count %d\n", m.relayCount)
	fmt.Fprintf(&b, "hank_remote_relay_failures_total %d\n", m.relayFailures)
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	fmt.Fprintf(&b, "hank_remote_go_alloc_bytes %d\n", mem.Alloc)
	fmt.Fprintf(&b, "hank_remote_go_goroutines %d\n", runtime.NumGoroutine())

	return b.String()
}

func allowed(value string, values ...string) bool {
	for _, candidate := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func writeTwoLabelledGauge(b *strings.Builder, name, firstLabel, secondLabel string, values map[string]int64) {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		parts := strings.SplitN(key, "\x00", 2)
		if len(parts) == 2 {
			fmt.Fprintf(b, "%s{%s=%q,%s=%q} %d\n", name, firstLabel, parts[0], secondLabel, parts[1], values[key])
		}
	}
}

func writeLabelledCounter(b *strings.Builder, name string, label string, counters map[string]int64) {
	keys := make([]string, 0, len(counters))
	for key := range counters {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Fprintf(b, "%s{%s=%q} %d\n", name, label, key, counters[key])
	}
}

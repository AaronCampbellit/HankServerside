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
	mu              sync.Mutex
	onlineAgents    int
	onlineApps      int
	authFailures    map[string]int64
	commandCounts   map[string]int64
	commandFailures map[string]int64
	commandLatency  map[string]time.Duration
	routeFailures   map[string]int64
	httpCounts      map[string]int64
	httpLatency     map[string]time.Duration
	assistantCounts map[string]int64
	assistantErrors map[string]int64
	relayLatency    time.Duration
	relayCount      int64
	relayFailures   int64
}

func NewMetrics() *Metrics {
	return &Metrics{
		authFailures:    make(map[string]int64),
		commandCounts:   make(map[string]int64),
		commandFailures: make(map[string]int64),
		commandLatency:  make(map[string]time.Duration),
		routeFailures:   make(map[string]int64),
		httpCounts:      make(map[string]int64),
		httpLatency:     make(map[string]time.Duration),
		assistantCounts: map[string]int64{"unknown": 0},
		assistantErrors: map[string]int64{"unknown": 0},
	}
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

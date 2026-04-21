package observability

import (
	"fmt"
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
}

func NewMetrics() *Metrics {
	return &Metrics{
		authFailures:    make(map[string]int64),
		commandCounts:   make(map[string]int64),
		commandFailures: make(map[string]int64),
		commandLatency:  make(map[string]time.Duration),
		routeFailures:   make(map[string]int64),
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

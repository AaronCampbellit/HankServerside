package cloud

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/dropfile/hankremote/internal/protocol"
)

const topicAgentsHealth = "agents.health"

// isManagementCommand reports whether a routed command is an RMM management
// action that must be admin-gated (device control, wake-on-LAN, remote shell).
// File and Home Assistant commands have their own dedicated policy checks and
// are intentionally excluded here.
func isManagementCommand(command string) bool {
	return strings.HasPrefix(command, "host.") ||
		strings.HasPrefix(command, "shell.") ||
		strings.HasPrefix(command, "wol.")
}

// agentHealthMonitor watches connected agents and emits alerts on the
// agents.health realtime topic: an agent going offline, or a worker reporting
// low free disk. State is per-agent so each condition fires once per edge.
type agentHealthMonitor struct {
	mu           sync.Mutex
	wasOnline    map[string]bool
	diskAlerted  map[string]bool
	diskFreePct  float64
}

func newAgentHealthMonitor() *agentHealthMonitor {
	return &agentHealthMonitor{
		wasOnline:   make(map[string]bool),
		diskAlerted: make(map[string]bool),
		diskFreePct: 0.10, // alert when free disk drops below 10%
	}
}

func (s *Server) runAgentHealthMonitor(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkAgentHealth(ctx)
		}
	}
}

func (s *Server) checkAgentHealth(ctx context.Context) {
	home, err := s.store.GetSingletonHome(ctx)
	if err != nil || home.ID == "" {
		return
	}
	homeID := home.ID
	snapshots := s.router.AgentsForHome(homeID)
	online := make(map[string]bool, len(snapshots))
	for _, snapshot := range snapshots {
		online[snapshot.AgentID] = true
		s.evaluateDiskAlert(ctx, homeID, snapshot)
	}

	s.health.mu.Lock()
	// Offline transitions: an agent we last saw online is no longer connected.
	var offline []string
	for agentID, was := range s.health.wasOnline {
		if was && !online[agentID] {
			offline = append(offline, agentID)
		}
	}
	for agentID := range online {
		s.health.wasOnline[agentID] = true
	}
	for _, agentID := range offline {
		s.health.wasOnline[agentID] = false
		delete(s.health.diskAlerted, agentID)
	}
	s.health.mu.Unlock()

	for _, agentID := range offline {
		s.emitAgentAlert(ctx, homeID, agentID, "agent.offline", "warning", "An agent disconnected from Hank.", nil)
	}
}

func (s *Server) evaluateDiskAlert(ctx context.Context, homeID string, snapshot AgentSnapshot) {
	if len(snapshot.Metrics) == 0 {
		return
	}
	var metrics protocol.HostMetrics
	if err := json.Unmarshal(snapshot.Metrics, &metrics); err != nil {
		return
	}
	if metrics.DiskTotalBytes <= 0 {
		return
	}
	freePct := float64(metrics.DiskTotalBytes-metrics.DiskUsedBytes) / float64(metrics.DiskTotalBytes)

	s.health.mu.Lock()
	alreadyAlerted := s.health.diskAlerted[snapshot.AgentID]
	low := freePct < s.health.diskFreePct
	if low && !alreadyAlerted {
		s.health.diskAlerted[snapshot.AgentID] = true
	} else if !low && alreadyAlerted {
		delete(s.health.diskAlerted, snapshot.AgentID)
	}
	s.health.mu.Unlock()

	if low && !alreadyAlerted {
		s.emitAgentAlert(ctx, homeID, snapshot.AgentID, "agent.disk_low", "warning",
			"An agent is low on disk space.",
			map[string]any{
				"free_percent":     int(freePct * 100),
				"disk_total_bytes": metrics.DiskTotalBytes,
				"disk_used_bytes":  metrics.DiskUsedBytes,
			})
	}
}

func (s *Server) emitAgentAlert(ctx context.Context, homeID string, agentID string, kind string, severity string, summary string, details map[string]any) {
	payload := map[string]any{
		"home_id":  homeID,
		"agent_id": agentID,
		"kind":     kind,
		"severity": severity,
		"summary":  summary,
		"time":     time.Now().UTC(),
	}
	if details != nil {
		payload["details"] = details
	}
	s.broadcastAppEventOnKey(ctx, scopedHomeTopic(homeID, topicAgentsHealth), topicAgentsHealth, kind, payload)
	s.audit(ctx, "agent.health."+strings.TrimPrefix(kind, "agent."), auditSeverityWarning, "", "", homeID, "", "agent", agentID, details)
}

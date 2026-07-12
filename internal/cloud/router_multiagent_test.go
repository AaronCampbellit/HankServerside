package cloud

import (
	"encoding/json"
	"testing"

	"github.com/dropfile/hankremote/internal/domain"
)

func TestRouterWorkerDoesNotDisplacePrimary(t *testing.T) {
	router := NewRouter()
	primary := domain.Agent{ID: "agent_primary", HomeID: "home_1"}
	worker := domain.Agent{ID: "agent_mac", HomeID: "home_1"}

	primaryConn := router.RegisterAgent("home_1", primary, nil, []string{"notes.sync"}, AgentTypePrimary, nil)
	workerConn := router.RegisterAgent("home_1", worker, nil, []string{"files"}, AgentTypeWorker, map[string]string{"hostname": "mac"})

	if got, ok := router.GetAgent("home_1"); !ok || got.agent.ID != "agent_primary" {
		t.Fatalf("GetAgent should return the primary, got %+v ok=%v", got, ok)
	}
	if router.AgentCount() != 2 {
		t.Fatalf("expected 2 connected agents, got %d", router.AgentCount())
	}

	if got, ok := router.ResolveAgent("home_1", "agent_mac"); !ok || got.agent.ID != "agent_mac" {
		t.Fatalf("ResolveAgent(agent_mac) failed: %+v ok=%v", got, ok)
	}
	if got, ok := router.ResolveAgent("home_1", ""); !ok || got.agent.ID != "agent_primary" {
		t.Fatalf("ResolveAgent(blank) should return primary, got %+v ok=%v", got, ok)
	}
	if _, ok := router.ResolveAgent("home_1", "agent_ghost"); ok {
		t.Fatal("ResolveAgent should miss unknown agents")
	}

	// Worker disconnect leaves the primary routable.
	router.UnregisterAgent("home_1", "agent_mac", workerConn)
	if _, ok := router.GetAgent("home_1"); !ok {
		t.Fatal("primary should survive worker disconnect")
	}
	if router.AgentCount() != 1 {
		t.Fatalf("expected 1 agent after worker disconnect, got %d", router.AgentCount())
	}

	// Primary disconnect clears primary routing entirely.
	router.UnregisterAgent("home_1", "agent_primary", primaryConn)
	if _, ok := router.GetAgent("home_1"); ok {
		t.Fatal("no primary should remain after primary disconnect")
	}
}

func TestRouterEmptyAgentTypeIsPrimary(t *testing.T) {
	router := NewRouter()
	agent := domain.Agent{ID: "agent_legacy", HomeID: "home_1"}
	router.RegisterAgent("home_1", agent, nil, nil, "", nil)
	if got, ok := router.GetAgent("home_1"); !ok || got.agent.ID != "agent_legacy" {
		t.Fatalf("legacy empty agent_type must register as primary, got %+v ok=%v", got, ok)
	}
}

func TestRouterPerAgentCapabilitiesAndMetrics(t *testing.T) {
	router := NewRouter()
	router.RegisterAgent("home_1", domain.Agent{ID: "a1", HomeID: "home_1"}, nil, nil, AgentTypePrimary, nil)
	router.RegisterAgent("home_1", domain.Agent{ID: "a2", HomeID: "home_1"}, nil, nil, AgentTypeWorker, nil)

	router.UpdateAgentCapabilities("home_1", "a2", []string{"files", "host.read"})
	router.UpdateAgentMetrics("home_1", "a2", json.RawMessage(`{"cpu_load_1m":1.5}`))

	snapshots := router.AgentsForHome("home_1")
	if len(snapshots) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(snapshots))
	}
	for _, snapshot := range snapshots {
		switch snapshot.AgentID {
		case "a1":
			if len(snapshot.Capabilities) != 0 || len(snapshot.Metrics) != 0 {
				t.Fatalf("a1 should have no capabilities/metrics, got %+v", snapshot)
			}
		case "a2":
			if len(snapshot.Capabilities) != 2 {
				t.Fatalf("a2 capabilities not applied: %+v", snapshot.Capabilities)
			}
			if string(snapshot.Metrics) != `{"cpu_load_1m":1.5}` {
				t.Fatalf("a2 metrics not applied: %s", snapshot.Metrics)
			}
		}
	}
}

func TestRouterUnregisterIgnoresStaleConnectionID(t *testing.T) {
	router := NewRouter()
	agent := domain.Agent{ID: "a1", HomeID: "home_1"}
	router.RegisterAgent("home_1", agent, nil, nil, AgentTypePrimary, nil)
	fresh := router.RegisterAgent("home_1", agent, nil, nil, AgentTypePrimary, nil) // reconnect

	// A late defer from the OLD connection must not tear down the new one.
	router.UnregisterAgent("home_1", "a1", "agentconn_stale")
	if _, ok := router.GetAgent("home_1"); !ok {
		t.Fatal("stale unregister must not remove the fresh connection")
	}

	router.UnregisterAgent("home_1", "a1", fresh)
	if _, ok := router.GetAgent("home_1"); ok {
		t.Fatal("fresh unregister should remove the agent")
	}
}

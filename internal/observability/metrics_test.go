package observability

import (
	"strings"
	"testing"
)

func TestDesktopMetricsArePayloadFreeAndFixedCardinality(t *testing.T) {
	metrics := NewMetrics()
	metrics.SetDesktopSessions("active", "macos", 2)
	metrics.IncDesktopJoin("browser", "success")
	metrics.IncDesktopReconnect("failed")
	metrics.IncDesktopTerminated("slow_consumer")
	metrics.AddDesktopRelayBytes("agent_to_browser", 4096)
	metrics.IncDesktopRelayBackpressure("agent_to_browser")
	metrics.SetDesktopReadiness("windows", "service", true)
	rendered := metrics.RenderPrometheus()
	for _, required := range []string{"hank_desktop_sessions{platform=\"macos\",state=\"active\"} 2", "hank_desktop_join_total{side=\"browser\",outcome=\"success\"} 1", "hank_desktop_reconnect_total{outcome=\"failed\"} 1", "hank_desktop_terminated_total{reason=\"slow_consumer\"} 1", "hank_desktop_relay_bytes_total{direction=\"agent_to_browser\"} 4096", "hank_desktop_relay_backpressure_total{direction=\"agent_to_browser\"} 1", "hank_desktop_readiness{platform=\"windows\",check=\"service\"} 1"} {
		if !strings.Contains(rendered, required) {
			t.Fatalf("missing %q in %s", required, rendered)
		}
	}
	metrics.IncDesktopTerminated("session-secret-or-user-value")
	if strings.Contains(metrics.RenderPrometheus(), "session-secret") {
		t.Fatal("unbounded reason label accepted")
	}
}

func TestNewMetricsExposeZeroAssistantProviderCounters(t *testing.T) {
	t.Parallel()

	rendered := NewMetrics().RenderPrometheus()
	for _, metric := range []string{
		`hank_remote_assistant_provider_requests_total{provider="unknown"} 0`,
		`hank_remote_assistant_provider_errors_total{provider="unknown"} 0`,
		`hank_desktop_sessions{platform="unknown",state="active"} 0`,
		`hank_desktop_join_total{side="agent",outcome="failed"} 0`,
		`hank_desktop_reconnect_total{outcome="expired"} 0`,
		`hank_desktop_readiness{platform="macos",check="capture"} 0`,
		`hank_desktop_readiness_reported{platform="macos",check="capture"} 0`,
	} {
		if !strings.Contains(rendered, metric) {
			t.Fatalf("RenderPrometheus() missing %q", metric)
		}
	}
}

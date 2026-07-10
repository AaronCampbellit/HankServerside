package observability

import (
	"strings"
	"testing"
)

func TestNewMetricsExposeZeroAssistantProviderCounters(t *testing.T) {
	t.Parallel()

	rendered := NewMetrics().RenderPrometheus()
	for _, metric := range []string{
		`hank_remote_assistant_provider_requests_total{provider="unknown"} 0`,
		`hank_remote_assistant_provider_errors_total{provider="unknown"} 0`,
	} {
		if !strings.Contains(rendered, metric) {
			t.Fatalf("RenderPrometheus() missing %q", metric)
		}
	}
}

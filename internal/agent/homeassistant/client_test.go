package homeassistant

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchStatesAndCallService(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/states":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{
					"entity_id": "light.kitchen",
					"state":     "on",
					"attributes": map[string]any{
						"friendly_name": "Kitchen",
					},
				},
			})

		case r.Method == http.MethodPost && r.URL.Path == "/api/services/light/turn_on":
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := New(server.URL, "test-token", 5*time.Second)

	states, err := client.FetchStates(context.Background())
	if err != nil {
		t.Fatalf("FetchStates: %v", err)
	}
	if len(states) != 1 || states[0].EntityID != "light.kitchen" || states[0].State != "on" {
		t.Fatalf("FetchStates returned %#v", states)
	}

	result, err := client.CallService(context.Background(), "light", "turn_on", json.RawMessage(`{"entity_id":"light.kitchen"}`))
	if err != nil {
		t.Fatalf("CallService: %v", err)
	}
	if string(result) != "{\"ok\":true}\n" {
		t.Fatalf("CallService result = %q", string(result))
	}
}

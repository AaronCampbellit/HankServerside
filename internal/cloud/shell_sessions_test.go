package cloud

import (
	"testing"
	"time"
)

func TestShellSessionRegistryEnforcesOwnerTargetAndLimits(t *testing.T) {
	now := time.Unix(100, 0)
	registry := newShellSessionRegistry(2, 2, 5*time.Minute)
	registry.now = func() time.Time { return now }

	if err := registry.open("term_a", "home", "user", "agent-a"); err != nil {
		t.Fatal(err)
	}
	if err := registry.authorize("term_a", "home", "user", "agent-a"); err != nil {
		t.Fatalf("owner rejected: %v", err)
	}
	if err := registry.authorize("term_a", "home", "other", "agent-a"); err == nil {
		t.Fatal("foreign user authorized")
	}
	if err := registry.authorize("term_a", "home", "user", "agent-b"); err == nil {
		t.Fatal("foreign target authorized")
	}
	if err := registry.open("term_b", "home", "user", "agent-a"); err != nil {
		t.Fatal(err)
	}
	if err := registry.open("term_c", "home", "user", "agent-a"); err == nil {
		t.Fatal("per-user limit was not enforced")
	}

	now = now.Add(6 * time.Minute)
	registry.prune()
	if err := registry.authorize("term_a", "home", "user", "agent-a"); err == nil {
		t.Fatal("expired session remained authorized")
	}
}

func TestShellSessionRegistryAuthorizesOwnedTopic(t *testing.T) {
	registry := newShellSessionRegistry(2, 2, 5*time.Minute)
	if err := registry.open("term_a", "home", "user", "agent-a"); err != nil {
		t.Fatal(err)
	}
	if !registry.ownsTopic("shell.session:term_a", "home", "user") {
		t.Fatal("owner could not subscribe")
	}
	if registry.ownsTopic("shell.session:term_a", "home", "other") {
		t.Fatal("foreign user could subscribe")
	}
	if !registry.allowsAgentEvent("shell.session:term_a", "home", "agent-a") ||
		registry.allowsAgentEvent("shell.session:term_a", "home", "agent-b") {
		t.Fatal("terminal output agent ownership was not enforced")
	}
}

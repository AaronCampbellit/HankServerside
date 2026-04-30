package storageops

import "testing"

func TestSignedIntentRoundTrip(t *testing.T) {
	stateDir := t.TempDir()
	secret := "test-secret"

	intent, err := CreateIntent(stateDir, secret, Intent{
		Type:        IntentTypePrimaryRestore,
		HomeID:      "home_1",
		RequestedBy: "usr_1",
		BackupLabel: "20260430-010101F",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyIntent(secret, intent) {
		t.Fatal("expected intent to verify")
	}
	intent.BackupLabel = "tampered"
	if VerifyIntent(secret, intent) {
		t.Fatal("expected tampered intent to fail verification")
	}

	intents, err := ListIntents(stateDir, secret)
	if err != nil {
		t.Fatal(err)
	}
	if len(intents) != 1 || intents[0].Type != IntentTypePrimaryRestore {
		t.Fatalf("intents = %+v", intents)
	}
}

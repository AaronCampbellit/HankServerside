package store

import "testing"

func TestRebindPlaceholders(t *testing.T) {
	t.Parallel()

	got := rebindPlaceholders(`SELECT * FROM users WHERE id = ? AND email = ?`)
	want := `SELECT * FROM users WHERE id = $1 AND email = $2`
	if got != want {
		t.Fatalf("rebindPlaceholders() = %q, want %q", got, want)
	}
}

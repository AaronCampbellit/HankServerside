package store

import "testing"

func TestRebindPlaceholdersSkipsSQLLiteralsAndComments(t *testing.T) {
	query := `SELECT '?' AS literal, "col?name", $tag$??$tag$, ? AS first
-- ? comment
/* ? block */
WHERE value = ? AND note = 'it''s ? safe'`
	got := rebindPlaceholders(query)
	want := `SELECT '?' AS literal, "col?name", $tag$??$tag$, $1 AS first
-- ? comment
/* ? block */
WHERE value = $2 AND note = 'it''s ? safe'`
	if got != want {
		t.Fatalf("rebindPlaceholders =\n%s\nwant\n%s", got, want)
	}
}

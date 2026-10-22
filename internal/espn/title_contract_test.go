package espn

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestClient_MatchTitleContract freezes the exact shape of model.Match.Title as
// produced by the parser: "<home display name> vs <away display name>".
//
// This string is not presentation-only. It feeds the user's custom_rules
// regexes (internal/catalog), so changing the separator, the order or the
// spacing silently breaks rules people already wrote. Any future change here
// needs a migration note, not just a green suite.
//
// The expected literals come from the recorded fixture, whose own event names
// read "Chelsea at Everton" and "Liverpool at Brentford": the title must be
// built from the competitor display names, never from the event name.
func TestClient_MatchTitleContract(t *testing.T) {
	fixture := readScoreboardFixture(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture)
	}))
	defer server.Close()

	fixedNow := time.Date(2026, 10, 17, 12, 0, 0, 0, time.FixedZone("America/Bogota", -5*3600))
	client := NewClient(
		WithBaseURL(server.URL),
		WithCacheDir(t.TempDir()),
		WithNow(func() time.Time { return fixedNow }),
		WithRetryBackoff(func() time.Duration { return 0 }),
	)

	matches, err := client.FetchMatches(context.Background(), "premier")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := map[string]string{
		"401878771": "Everton vs Chelsea",
		"401879262": "Brentford vs Liverpool",
	}
	if len(matches) != len(want) {
		t.Fatalf("expected %d matches, got %d", len(want), len(matches))
	}
	for _, m := range matches {
		expected, ok := want[m.ID]
		if !ok {
			t.Fatalf("unexpected match id %s", m.ID)
		}
		if m.Title != expected {
			t.Errorf("match %s title: got %q, want %q (custom_rules depend on this format)", m.ID, m.Title, expected)
		}
	}
}

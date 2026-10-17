package parrilla

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

// TestParseHTML_SplitAcrossTables verifies the chunk-by-4 strategy survives
// header drift, heavy inline styles, a schedule split across two <table>
// elements, accented team names, both "vs." and empty channel cells.
func TestParseHTML_SplitAcrossTables(t *testing.T) {
	body := readFixture(t, "two_tables.html")

	snap, err := ParseHTML("2026-10-17", body)
	if err != nil {
		t.Fatalf("ParseHTML: %v", err)
	}
	if snap.Date != "2026-10-17" {
		t.Fatalf("date = %q, want 2026-10-17", snap.Date)
	}
	if got, want := len(snap.Entries), 5; got != want {
		t.Fatalf("entries = %d, want %d (header must be skipped, both tables captured)", got, want)
	}

	want := []Entry{
		{
			Competition: "Premier League",
			HomeTeam:    "Everton",
			AwayTeam:    "Chelsea",
			Raw:         "Everton vs. Chelsea",
			StartLocal:  "6:30 a.m.",
			StartMins:   6*60 + 30,
			Channels:    []string{"Disney+ Premium", "ESPN"},
		},
		{
			Competition: "La Liga EA Sports",
			HomeTeam:    "Espanyol",
			AwayTeam:    "Atlético Madrid",
			Raw:         "Espanyol vs. Atlético Madrid",
			StartLocal:  "7:00 a.m.",
			StartMins:   7 * 60,
			Channels:    []string{"DGO", "DSports", "Paramount+", "Amazon Prime Video", "DAZN", "Win Play"},
		},
		{
			Competition: "Liga Betplay",
			HomeTeam:    "Boyaca Chicó",
			AwayTeam:    "Atlético Nacional",
			Raw:         "Boyaca Chicó vs. Atlético Nacional",
			StartLocal:  "3:00 p.m.",
			StartMins:   15 * 60,
			Channels:    []string{"Win Sports"},
		},
		{
			Competition: "Liga Betplay",
			HomeTeam:    "Aplazado FC",
			AwayTeam:    "Por Definir FC",
			Raw:         "Aplazado FC vs. Por Definir FC",
			StartLocal:  "5:00 p.m.",
			StartMins:   17 * 60,
			Channels:    []string{},
		},
		{
			Competition: "Serie A Italiana",
			HomeTeam:    "Venezia FC",
			AwayTeam:    "Napoli",
			Raw:         "Venezia FC vs. Napoli",
			StartLocal:  "8:10 p.m.",
			StartMins:   20*60 + 10,
			Channels:    []string{"ESPN 2", "Win Sports Más", "DSports+"},
		},
	}

	for i := range want {
		got := snap.Entries[i]
		if !reflect.DeepEqual(got, want[i]) {
			t.Errorf("entry %d:\n got  %+v\n want %+v", i, got, want[i])
		}
	}

	// The postponed match must keep an empty (non-nil) channel slice, never an
	// invented channel.
	empty := snap.Entries[3].Channels
	if empty == nil {
		t.Errorf("postponed entry channels = nil, want empty non-nil slice")
	}
	if len(empty) != 0 {
		t.Errorf("postponed entry channels = %v, want empty", empty)
	}
}

// TestParseHTML_HeaderDrift verifies a different header wording does not break
// the positional parse and that "vs" without a period splits correctly.
func TestParseHTML_HeaderDrift(t *testing.T) {
	body := readFixture(t, "alt_header.html")

	snap, err := ParseHTML("2026-03-05", body)
	if err != nil {
		t.Fatalf("ParseHTML: %v", err)
	}
	if got, want := len(snap.Entries), 2; got != want {
		t.Fatalf("entries = %d, want %d", got, want)
	}

	first := snap.Entries[0]
	if first.Raw != "Bayern Múnich vs Borussia Dortmund" {
		t.Fatalf("raw = %q", first.Raw)
	}
	if first.HomeTeam != "Bayern Múnich" || first.AwayTeam != "Borussia Dortmund" {
		t.Fatalf("split = %q / %q", first.HomeTeam, first.AwayTeam)
	}
	if first.StartMins != 13*60+45 {
		t.Fatalf("start mins = %d, want %d", first.StartMins, 13*60+45)
	}
	if !reflect.DeepEqual(first.Channels, []string{"ESPN 3", "Apple TV"}) {
		t.Fatalf("channels = %v", first.Channels)
	}

	second := snap.Entries[1]
	if second.StartMins != 21*60+30 {
		t.Fatalf("start mins = %d, want %d", second.StartMins, 21*60+30)
	}
}

func TestParseHTML_NoSchedule(t *testing.T) {
	_, err := ParseHTML("2026-10-17", []byte("<html><body><p>sin tablas</p></body></html>"))
	if err == nil {
		t.Fatal("expected ErrNoSchedule, got nil")
	}
}

func TestBuildURL(t *testing.T) {
	tests := []struct {
		date time.Time
		want string
	}{
		{
			date: time.Date(2026, time.October, 17, 0, 0, 0, 0, time.UTC),
			want: "https://www.noticiascaracol.com/golcaracol/partidos-en-vivo-hoy-17-de-octubre-del-2026-horarios-y-programacion-por-tv-so35",
		},
		{
			date: time.Date(2026, time.March, 5, 0, 0, 0, 0, time.UTC),
			want: "https://www.noticiascaracol.com/golcaracol/partidos-en-vivo-hoy-5-de-marzo-del-2026-horarios-y-programacion-por-tv-so35",
		},
	}
	for _, tt := range tests {
		if got := BuildURL(tt.date); got != tt.want {
			t.Errorf("BuildURL(%s) = %q, want %q", tt.date.Format("2006-01-02"), got, tt.want)
		}
	}
}

func TestParseStartMinutes(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"6:30 a.m.", 390},
		{"7:00 a.m.", 420},
		{"1:45 p.m.", 825},
		{"12:30 a.m.", 30},
		{"12:00 p.m.", 720},
		{"9:30 p.m.", 1290},
		{"6:30 pm", 1110},
		{"Hora", -1},
		{"", -1},
	}
	for _, tt := range tests {
		if got := parseStartMinutes(tt.in); got != tt.want {
			t.Errorf("parseStartMinutes(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestCanonicalChannel(t *testing.T) {
	tests := []struct {
		name string
		key  string
		p    Playability
		ok   bool
	}{
		{"ESPN", "espn1", Linear, true},
		{"ESPN 2", "espn2", Linear, true},
		{"ESPN2", "espn2", Linear, true},
		{"espn.2", "espn2", Linear, true},
		{"ESPN 4", "espn4", Linear, true},
		{"ESPN4", "espn4", Linear, true},
		{"DSports", "dsports", Linear, true},
		{"D Sports", "dsports", Linear, true},
		{"DSports 2", "dsports2", Linear, true},
		{"DSports 4", "dsports4", Linear, true},
		{"DSports+", "dsports_plus", Streaming, true},
		{"DSPORTS+", "dsports_plus", Streaming, true},
		{"Win", "win", Linear, true},
		{"Win Sports", "win", Linear, true},
		{"Win Sports Más", "win_sports_mas", Linear, true},
		{"Win Sports Mas", "win_sports_mas", Linear, true},
		{"Win+", "win_plus", Linear, true},
		{"Win Plus", "win_plus", Linear, true},
		{"Win+ Fútbol", "win_plus", Linear, true},
		{"TyC Sports Internacional", "tyc", Linear, true},
		{"Claro Sports", "claro_sports", Linear, true},
		{"Gol Caracol", "gol_caracol", Linear, true},
		{"Caracol TV", "caracol", Linear, true},
		{"RCN", "rcn", Linear, true},
		// Obsolete keys must no longer resolve. ESPN Extra was renamed ESPN 4 in
		// Colombia (the Fox Sports brand was discontinued in LatAm on 2024-02-15),
		// so espn_extra is gone and its old aliases must not resurrect it.
		{"ESPN Extra", "", Unknown, false},
		{"ESPN+", "", Unknown, false},
		{"espn_extra", "", Unknown, false},
		// Streaming brands without a stream key in channels.json.example are
		// unknown here; the parser keeps their names verbatim instead of guessing.
		{"Disney+ Premium", "", Unknown, false},
		{"Apple TV", "", Unknown, false},
		{"Nope TV", "", Unknown, false},
		{"", "", Unknown, false},
	}
	for _, tt := range tests {
		key, p, ok := CanonicalChannel(tt.name)
		if key != tt.key || p != tt.p || ok != tt.ok {
			t.Errorf("CanonicalChannel(%q) = (%q, %v, %v), want (%q, %v, %v)",
				tt.name, key, p, ok, tt.key, tt.p, tt.ok)
		}
	}
}

// TestChannelSpecs_MatchExampleKeys enforces the invariant that every key the
// parrilla mapping can emit exists in channels.json.example, the source of truth
// for playable streams. It pins the exact expected key set so a future edit
// cannot silently reintroduce an obsolete key (espn_extra) or register a key
// with no stream behind it.
func TestChannelSpecs_MatchExampleKeys(t *testing.T) {
	const examplePath = "../../channels.json.example"

	data, err := os.ReadFile(examplePath)
	if err != nil {
		t.Skipf("channels.json.example not readable (%v); skipping cross-check", err)
	}

	var example struct {
		Streams map[string]string `json:"streams"`
	}
	if err := json.Unmarshal(data, &example); err != nil {
		t.Fatalf("parse %s: %v", examplePath, err)
	}

	emitted := make(map[string]bool)
	for _, spec := range channelSpecs {
		if spec.Key == "" {
			t.Errorf("channel spec %q has an empty key", spec.Display)
			continue
		}
		if _, ok := example.Streams[spec.Key]; !ok {
			t.Errorf("channel key %q (%s) is not present in %s", spec.Key, spec.Display, examplePath)
		}
		if emitted[spec.Key] {
			t.Errorf("channel key %q is emitted by more than one spec", spec.Key)
		}
		emitted[spec.Key] = true
	}

	want := []string{
		"espn1", "espn2", "espn3", "espn4", "espn5", "espn6", "espn7",
		"dsports", "dsports2", "dsports4", "dsports_plus",
		"win", "win_plus", "win_sports_mas",
		"tyc", "claro_sports", "gol_caracol", "caracol", "rcn",
	}
	for _, key := range want {
		if !emitted[key] {
			t.Errorf("expected the parrilla mapping to emit key %q, but it does not", key)
		}
	}
	if len(emitted) != len(want) {
		t.Errorf("emitted %d distinct keys, want exactly %d (%v)", len(emitted), len(want), want)
	}
}

// TestFetch_UsesBaseURLAndParses proves the fetcher builds the deterministic URL,
// sends a browser User-Agent and parses the response, without touching the
// network.
func TestFetch_UsesBaseURLAndParses(t *testing.T) {
	fixture := readFixture(t, "two_tables.html")

	var gotPath, gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(fixture)
	}))
	defer srv.Close()

	f := NewGolCaracol(WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	date := time.Date(2026, time.October, 17, 0, 0, 0, 0, time.UTC)

	snap, err := f.Fetch(context.Background(), date)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got, want := len(snap.Entries), 5; got != want {
		t.Fatalf("entries = %d, want %d", got, want)
	}
	if !strings.Contains(gotPath, "partidos-en-vivo-hoy-17-de-octubre-del-2026-horarios-y-programacion-por-tv-so35") {
		t.Errorf("request path = %q, want the deterministic 17-octubre slug", gotPath)
	}
	if !strings.Contains(gotUA, "Mozilla/5.0") {
		t.Errorf("User-Agent = %q, want a browser UA", gotUA)
	}
}

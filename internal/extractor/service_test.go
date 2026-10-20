package extractor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/e12jassir/kickoff-cli/internal/model"
)

func TestParseAgendaHTML(t *testing.T) {
	fixture, err := os.ReadFile("testdata/agenda.html")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	events, err := ParseAgendaHTML(string(fixture), "https://pelotalibre.org")
	if err != nil {
		t.Fatalf("ParseAgendaHTML failed: %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	e1 := events[0]
	if e1.HomeTeam != "Real Madrid" || e1.AwayTeam != "Barcelona" {
		t.Errorf("unexpected event 1 teams: %s vs %s", e1.HomeTeam, e1.AwayTeam)
	}
	if e1.TimeStr != "15:00" {
		t.Errorf("expected time 15:00, got %s", e1.TimeStr)
	}
	if len(e1.Streams) != 2 {
		t.Fatalf("expected 2 streams for event 1, got %d", len(e1.Streams))
	}
	if e1.Streams[0].ChannelName != "DSports" {
		t.Errorf("expected channel DSports, got %s", e1.Streams[0].ChannelName)
	}
	if e1.Streams[0].PlayerURL != "https://pelotalibre.org/canal/dsports.php" {
		t.Errorf("expected resolved player url, got %s", e1.Streams[0].PlayerURL)
	}

	e2 := events[1]
	if e2.HomeTeam != "Boca Juniors" || e2.AwayTeam != "River Plate" {
		t.Errorf("unexpected event 2 teams: %s vs %s", e2.HomeTeam, e2.AwayTeam)
	}

	e3 := events[2]
	if e3.HomeTeam != "Junior" || e3.AwayTeam != "Atletico Nacional" {
		t.Errorf("unexpected event 3 teams: %s vs %s", e3.HomeTeam, e3.AwayTeam)
	}
}

func TestParseAgendaJSON(t *testing.T) {
	sampleJSON := []byte(`{
		"data": [
			{
				"attributes": {
					"diary_description": "LaLiga: Real Madrid vs Sevilla",
					"diary_hour": "14:00:00",
					"embeds": {
						"data": [
							{
								"attributes": {
									"embed_name": "ESPN Deportes",
									"embed_iframe": "/embed/eventos.html?r=aHR0cHM6Ly9sYTE4aGQuc3Uvdml2by9jYW5hbC5waHA/c3RyZWFtPWVzcG5kZXBvcnRlcw=="
								}
							}
						]
					}
				}
			}
		]
	}`)

	events, err := ParseAgendaJSON(sampleJSON, "https://agenda18.com")
	if err != nil {
		t.Fatalf("ParseAgendaJSON failed: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	ev := events[0]
	if ev.HomeTeam != "Real Madrid" || ev.AwayTeam != "Sevilla" {
		t.Errorf("expected Real Madrid vs Sevilla, got %s vs %s", ev.HomeTeam, ev.AwayTeam)
	}
	if len(ev.Streams) != 1 {
		t.Fatalf("expected 1 stream, got %d", len(ev.Streams))
	}
	if ev.Streams[0].ChannelName != "ESPN Deportes" {
		t.Errorf("expected ESPN Deportes, got %s", ev.Streams[0].ChannelName)
	}
	expectedPlayer := "https://la18hd.su/vivo/canal.php?stream=espndeportes"
	if ev.Streams[0].PlayerURL != expectedPlayer {
		t.Errorf("expected decoded player %s, got %s", expectedPlayer, ev.Streams[0].PlayerURL)
	}
}

func TestMatchEventPairing(t *testing.T) {
	s := NewService()

	events := []ExtractedEvent{
		{
			HomeTeam: "Real Madrid",
			AwayTeam: "Barcelona",
			RawTitle: "Real Madrid vs Barcelona",
			Streams: []ExtractedStream{
				{ChannelName: "DSports", PlayerURL: "https://example.com/dsports"},
			},
		},
		{
			HomeTeam: "Boca Juniors",
			AwayTeam: "River Plate",
			RawTitle: "Boca Juniors vs River Plate",
			Streams: []ExtractedStream{
				{ChannelName: "TNT Sports", PlayerURL: "https://example.com/tnt"},
			},
		},
	}

	espnMatch := model.Match{
		Title: "Real Madrid vs. Barcelona",
		HomeTeam: model.Competitor{
			DisplayName: "Real Madrid",
		},
		AwayTeam: model.Competitor{
			DisplayName: "FC Barcelona",
		},
	}

	matchedEvent, score := s.MatchEvent(espnMatch, events)
	if matchedEvent == nil {
		t.Fatal("expected match to be paired")
	}
	if score <= 0 {
		t.Errorf("expected positive score, got %d", score)
	}
	if matchedEvent.HomeTeam != "Real Madrid" {
		t.Errorf("expected Real Madrid match, got %s", matchedEvent.HomeTeam)
	}

	// Negative match test
	unrelatedMatch := model.Match{
		Title: "Arsenal vs. Chelsea",
		HomeTeam: model.Competitor{
			DisplayName: "Arsenal",
		},
		AwayTeam: model.Competitor{
			DisplayName: "Chelsea",
		},
	}

	unmatched, score := s.MatchEvent(unrelatedMatch, events)
	if unmatched != nil || score != 0 {
		t.Errorf("expected no match, got %v with score %d", unmatched, score)
	}
}

func TestResolveManifest_Direct(t *testing.T) {
	fixture, err := os.ReadFile("testdata/player_direct.html")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fixture)
	}))
	defer ts.Close()

	s := NewService(WithHTTPClient(ts.Client()))
	stream := &ExtractedStream{
		PlayerURL: ts.URL + "/dsports",
	}

	err = s.ResolveManifest(context.Background(), stream)
	if err != nil {
		t.Fatalf("ResolveManifest failed: %v", err)
	}

	expectedManifest := "https://edge-live.cdn.stream/dsports/live.m3u8?token=secure123"
	if stream.StreamURL != expectedManifest {
		t.Errorf("expected manifest %s, got %s", expectedManifest, stream.StreamURL)
	}
	if stream.Headers["Referer"] != ts.URL+"/dsports" {
		t.Errorf("expected Referer header set, got %s", stream.Headers["Referer"])
	}
}

func TestResolveManifest_Iframe(t *testing.T) {
	iframeFixture, err := os.ReadFile("testdata/embed_player.html")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	playerFixture, err := os.ReadFile("testdata/player_iframe.html")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.URL.Path == "/embed/player.php" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(iframeFixture)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(playerFixture)
	}))
	defer ts.Close()

	s := NewService(WithHTTPClient(ts.Client()))
	stream := &ExtractedStream{
		PlayerURL: ts.URL + "/canal/tnt",
	}

	err = s.ResolveManifest(context.Background(), stream)
	if err != nil {
		t.Fatalf("ResolveManifest with iframe failed: %v", err)
	}

	expectedManifest := "https://live-origin.media/hls/tnt_sports.m3u8"
	if stream.StreamURL != expectedManifest {
		t.Errorf("expected manifest %s, got %s", expectedManifest, stream.StreamURL)
	}
}

func TestProbeHLS(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/valid.m3u8" {
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-STREAM-INF:BANDWIDTH=3000000\nlive.m3u8"))
			return
		}
		if r.URL.Path == "/404.m3u8" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Path == "/invalid.m3u8" {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<html>Error 403 Forbidden</html>"))
			return
		}
	}))
	defer ts.Close()

	s := NewService(WithHTTPClient(ts.Client()))

	// Valid stream test
	validStream := &ExtractedStream{
		StreamURL: ts.URL + "/valid.m3u8",
	}
	if !s.ProbeHLS(context.Background(), validStream) {
		t.Error("expected valid stream to pass probe")
	}
	if !validStream.Alive {
		t.Error("expected stream.Alive to be true")
	}

	// 404 stream test
	notFoundStream := &ExtractedStream{
		StreamURL: ts.URL + "/404.m3u8",
	}
	if s.ProbeHLS(context.Background(), notFoundStream) {
		t.Error("expected 404 stream to fail probe")
	}

	// Invalid HTML content stream test
	invalidStream := &ExtractedStream{
		StreamURL: ts.URL + "/invalid.m3u8",
	}
	if s.ProbeHLS(context.Background(), invalidStream) {
		t.Error("expected HTML content to fail probe")
	}
}

func TestFetchAgenda_DomainRotation(t *testing.T) {
	agendaFixture, err := os.ReadFile("testdata/agenda.html")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	// Mirror 1: Fails with 500
	m1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer m1.Close()

	// Mirror 2: Succeeds with agenda
	m2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(agendaFixture)
	}))
	defer m2.Close()

	s := NewService(
		WithHTTPClient(m2.Client()),
		WithBaseDomains(m1.URL, m2.URL),
		WithCacheTTL(10*time.Second),
	)

	events, err := s.FetchAgenda(context.Background())
	if err != nil {
		t.Fatalf("FetchAgenda failed during fallback: %v", err)
	}

	if len(events) != 3 {
		t.Errorf("expected 3 events from mirror 2, got %d", len(events))
	}

	// Test cache hit
	eventsCached, err := s.FetchAgenda(context.Background())
	if err != nil {
		t.Fatalf("FetchAgenda cached hit failed: %v", err)
	}
	if len(eventsCached) != 3 {
		t.Errorf("expected 3 cached events, got %d", len(eventsCached))
	}
}

func TestResolveMatchStreams(t *testing.T) {
	agendaFixture, err := os.ReadFile("testdata/agenda.html")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	playerFixture, err := os.ReadFile("testdata/player_direct.html")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(agendaFixture)
		case "/canal/dsports.php":
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			// Replace edge-live with local server url
			modified := strings.ReplaceAll(string(playerFixture), "https://edge-live.cdn.stream/dsports/live.m3u8?token=secure123", "/stream/dsports.m3u8")
			_, _ = w.Write([]byte(modified))
		case "/canal/dsports2.php":
			w.WriteHeader(http.StatusNotFound)
		case "/stream/dsports.m3u8":
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("#EXTM3U\n#EXT-X-VERSION:3\nsegment1.ts"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	s := NewService(
		WithHTTPClient(ts.Client()),
		WithBaseDomains(ts.URL),
	)

	match := model.Match{
		Title: "Real Madrid vs Barcelona",
		HomeTeam: model.Competitor{
			DisplayName: "Real Madrid",
		},
		AwayTeam: model.Competitor{
			DisplayName: "FC Barcelona",
		},
	}

	streams, err := s.ResolveMatchStreams(context.Background(), match)
	if err != nil {
		t.Fatalf("ResolveMatchStreams failed: %v", err)
	}

	if len(streams) != 1 {
		t.Fatalf("expected 1 live stream, got %d", len(streams))
	}

	st := streams[0]
	if st.ChannelName != "DSports" {
		t.Errorf("expected channel name DSports, got %s", st.ChannelName)
	}
	if !st.Alive {
		t.Errorf("expected stream to be marked alive")
	}
	if !strings.HasSuffix(st.StreamURL, "/stream/dsports.m3u8") {
		t.Errorf("expected stream url ending with /stream/dsports.m3u8, got %s", st.StreamURL)
	}
	if st.Headers["Referer"] == "" {
		t.Errorf("expected Referer header to be populated")
	}

	// Test un-paired match
	unrelatedMatch := model.Match{
		Title: "Juventus vs Inter",
		HomeTeam: model.Competitor{
			DisplayName: "Juventus",
		},
		AwayTeam: model.Competitor{
			DisplayName: "Inter",
		},
	}
	none, err := s.ResolveMatchStreams(context.Background(), unrelatedMatch)
	if err != nil {
		t.Fatalf("unexpected error for unrelated match: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("expected 0 streams for unmatched game, got %d", len(none))
	}
}

func TestFetchAgenda_StaleAndDiskFallback(t *testing.T) {
	statusCode := http.StatusOK
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if statusCode != http.StatusOK {
			w.WriteHeader(statusCode)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":1,"attributes":{"diary_description":"PSG vs Barcelona","diary_hour":"14:00:00","embeds":{"data":[{"attributes":{"embed_name":"ESPN","embed_iframe":"https://example.com/stream"}}]}}}]}`))
	}))
	defer ts.Close()

	tempDir := t.TempDir()
	s1 := NewService(
		WithBaseDomains(ts.URL),
		WithCacheDir(tempDir),
		WithCacheTTL(1*time.Millisecond),
	)

	// Step 1: Successful initial fetch persists to disk
	events, err := s1.FetchAgenda(context.Background())
	if err != nil {
		t.Fatalf("initial FetchAgenda failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	// Wait for async disk write
	time.Sleep(50 * time.Millisecond)

	// Step 2: Server now returns 503 Service Unavailable (Cloudflare outage under load)
	statusCode = http.StatusServiceUnavailable

	// Memory fallback on same instance (stale-while-revalidate)
	staleEvents, err := s1.FetchAgenda(context.Background())
	if err != nil {
		t.Fatalf("expected stale memory fallback to succeed, got error: %v", err)
	}
	if len(staleEvents) != 1 {
		t.Fatalf("expected 1 event from stale fallback, got %d", len(staleEvents))
	}

	// Step 3: Fresh service instance (restarted app) recovers from disk cache
	s2 := NewService(
		WithBaseDomains(ts.URL),
		WithCacheDir(tempDir),
		WithCacheTTL(1*time.Millisecond),
	)
	diskEvents, err := s2.FetchAgenda(context.Background())
	if err != nil {
		t.Fatalf("expected disk cache fallback to succeed on fresh service, got error: %v", err)
	}
	if len(diskEvents) != 1 || diskEvents[0].HomeTeam != "PSG" {
		t.Fatalf("expected 1 recovered event from disk cache, got %+v", diskEvents)
	}
}

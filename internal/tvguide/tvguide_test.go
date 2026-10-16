package tvguide

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/e12jassir/kickoff-cli/internal/model"
)

const mockESPN1HTML = `<!DOCTYPE html>
<html lang="es-CO">
<head><title>Canal ESPN (Colombia)</title></head>
<body>
<div class="div_MainHeaderTop"></div>
<table>
  <tr>
    <td colspan="5" class="noticiero">
      <div class="tbl_EPG_ProgramsColumn noticiero">
        <div class="div_program_title_on_channel">
          <a itemprop="url" href="https://www.gatotv.com/noticiero/sportscenter" title="SportsCenter">
            <span>SportsCenter</span>
          </a>
        </div>
      </div>
    </td>
  </tr>
  <tr>
    <td colspan="6" class="deporte">
      <div class="tbl_EPG_ProgramsColumn deporte">
        <div class="div_program_title_on_channel">
          <span>LASK vs. Liverpool</span>
        </div>
        <div class="div_episode_deporte_on_channel">
          Fútbol UEFA Champions League
        </div>
      </div>
    </td>
  </tr>
  <tr>
    <td colspan="5" class="deporte">
      <div class="tbl_EPG_ProgramsColumn deporte">
        <div class="div_program_title_on_channel">
          <a itemprop="url" href="https://www.gatotv.com/deporte/roma_vs_real_madrid" title="Roma vs. Real Madrid">
            <span>Roma vs. Real Madrid</span>
          </a>
        </div>
        <div class="div_episode_deporte_on_channel">
          Fútbol UEFA Champions League
        </div>
      </div>
    </td>
  </tr>
</table>
</body>
</html>`

const mockESPN2HTML = `<!DOCTYPE html>
<html lang="es-CO">
<head><title>Canal ESPN 2 (Colombia)</title></head>
<body>
<table>
  <tr>
    <td colspan="6" class="deporte">
      <div class="tbl_EPG_ProgramsColumn deporte">
        <div class="div_program_title_on_channel">
          <span>Feyenoord vs. Como</span>
        </div>
      </div>
    </td>
  </tr>
  <tr>
    <td colspan="6" class="deporte">
      <div class="tbl_EPG_ProgramsColumn deporte">
        <div class="div_program_title_on_channel">
          <a itemprop="url" href="https://www.gatotv.com/deporte/manchester_city_vs_psg" title="Manchester City vs. Paris Saint-Germain">
            <span>Manchester City vs. Paris Saint-Germain</span>
          </a>
        </div>
      </div>
    </td>
  </tr>
</table>
</body>
</html>`

func TestHTMLParsing(t *testing.T) {
	t.Run("ESPN 1 titles extraction", func(t *testing.T) {
		titles := ParseHTML(mockESPN1HTML)
		if len(titles) != 3 {
			t.Fatalf("expected 3 titles, got %d: %v", len(titles), titles)
		}
		expected := []string{"SportsCenter", "LASK vs. Liverpool", "Roma vs. Real Madrid"}
		for i, exp := range expected {
			if titles[i] != exp {
				t.Errorf("titles[%d] expected %q, got %q", i, exp, titles[i])
			}
		}
	})

	t.Run("ESPN 2 titles extraction", func(t *testing.T) {
		titles := ParseHTML(mockESPN2HTML)
		if len(titles) != 2 {
			t.Fatalf("expected 2 titles, got %d: %v", len(titles), titles)
		}
		expected := []string{"Feyenoord vs. Como", "Manchester City vs. Paris Saint-Germain"}
		for i, exp := range expected {
			if titles[i] != exp {
				t.Errorf("titles[%d] expected %q, got %q", i, exp, titles[i])
			}
		}
	})

	t.Run("Fallback extraction via tbl_EPG_ProgramsColumn", func(t *testing.T) {
		htmlFallback := `
		<div class="tbl_EPG_ProgramsColumn deporte">
			<span>Aston Villa vs. Fenerbahce</span>
		</div>
		`
		titles := ParseHTML(htmlFallback)
		if len(titles) != 1 || titles[0] != "Aston Villa vs. Fenerbahce" {
			t.Fatalf("expected ['Aston Villa vs. Fenerbahce'], got %v", titles)
		}
	})
}

func TestNormalizeAndAccents(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"FC Barcelona", "barcelona"},
		{"Real Madrid C.F.", "real madrid"},
		{"LASK vs. Liverpool", "lask vs liverpool"},
		{"Fenerbahçe S.K.", "fenerbahce sk"},
		{"Atlético de Madrid", "atletico de madrid"},
		{"Bayern de Múnich", "bayern de munich"},
		{"Feyenoord Rotterdam &amp; Como", "feyenoord rotterdam como"},
		{"Bodo/Glimt", "bodo glimt"},
	}

	for _, tt := range tests {
		got := Normalize(tt.input)
		if got != tt.expected {
			t.Errorf("Normalize(%q) = %q, expected %q", tt.input, got, tt.expected)
		}
	}
}

func TestMatchScore(t *testing.T) {
	tests := []struct {
		name        string
		guideTitle  string
		match       model.Match
		shouldMatch bool
		minScore    int
	}{
		{
			name:       "LASK vs Liverpool exact team match",
			guideTitle: "LASK vs. Liverpool",
			match: model.Match{
				HomeTeam: model.Competitor{DisplayName: "LASK Linz"},
				AwayTeam: model.Competitor{DisplayName: "Liverpool"},
				Title:    "LASK Linz vs Liverpool",
			},
			shouldMatch: true,
			minScore:    5,
		},
		{
			name:       "Feyenoord vs Como keyword match",
			guideTitle: "Feyenoord vs. Como",
			match: model.Match{
				HomeTeam: model.Competitor{DisplayName: "Feyenoord Rotterdam"},
				AwayTeam: model.Competitor{DisplayName: "Como"},
				Title:    "Feyenoord Rotterdam vs Como",
			},
			shouldMatch: true,
			minScore:    5,
		},
		{
			name:       "Roma vs Real Madrid match",
			guideTitle: "Roma vs. Real Madrid",
			match: model.Match{
				HomeTeam: model.Competitor{DisplayName: "AS Roma"},
				AwayTeam: model.Competitor{DisplayName: "Real Madrid"},
				Title:    "AS Roma vs Real Madrid",
			},
			shouldMatch: true,
			minScore:    5,
		},
		{
			name:       "Accent folding match: Fenerbahce",
			guideTitle: "Aston Villa vs. Fenerbahce",
			match: model.Match{
				HomeTeam: model.Competitor{DisplayName: "Aston Villa"},
				AwayTeam: model.Competitor{DisplayName: "Fenerbahçe"},
			},
			shouldMatch: true,
			minScore:    5,
		},
		{
			name:       "Negative match: Unrelated team on Real Madrid entry",
			guideTitle: "Roma vs. Real Madrid",
			match: model.Match{
				HomeTeam: model.Competitor{DisplayName: "Real Betis"},
				AwayTeam: model.Competitor{DisplayName: "FC Porto"},
				Title:    "Real Betis vs FC Porto",
			},
			shouldMatch: false,
			minScore:    0,
		},
		{
			name:       "Negative match: SportsCenter",
			guideTitle: "SportsCenter",
			match: model.Match{
				HomeTeam: model.Competitor{DisplayName: "Liverpool"},
				AwayTeam: model.Competitor{DisplayName: "Chelsea"},
			},
			shouldMatch: false,
			minScore:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched, score := MatchScore(tt.guideTitle, tt.match)
			if matched != tt.shouldMatch {
				t.Fatalf("expected matched=%v, got %v (score %d)", tt.shouldMatch, matched, score)
			}
			if matched && score < tt.minScore {
				t.Errorf("expected score >= %d, got %d", tt.minScore, score)
			}
		})
	}
}

func TestService_ResolveMatch(t *testing.T) {
	svc := NewService()
	svc.SetGuides([]ChannelGuide{
		{
			Name:      "ESPN 1 HD",
			Slug:      "espn_colombia",
			StreamURL: "http://example.com/espn1.m3u8",
			Programs:  []string{"SportsCenter", "LASK vs. Liverpool", "Roma vs. Real Madrid"},
		},
		{
			Name:      "ESPN 2 HD",
			Slug:      "espn_2_colombia",
			StreamURL: "http://example.com/espn2.m3u8",
			Programs:  []string{"Feyenoord vs. Como", "Manchester City vs. Paris Saint-Germain"},
		},
	})

	tests := []struct {
		name            string
		match           model.Match
		expectedChannel string
		expectFound     bool
	}{
		{
			name: "LASK Linz vs Liverpool maps to ESPN 1 HD",
			match: model.Match{
				HomeTeam: model.Competitor{DisplayName: "LASK Linz"},
				AwayTeam: model.Competitor{DisplayName: "Liverpool"},
				Title:    "LASK Linz vs Liverpool",
			},
			expectedChannel: "ESPN 1 HD",
			expectFound:     true,
		},
		{
			name: "Feyenoord Rotterdam vs Como maps to ESPN 2 HD",
			match: model.Match{
				HomeTeam: model.Competitor{DisplayName: "Feyenoord Rotterdam"},
				AwayTeam: model.Competitor{DisplayName: "Como"},
				Title:    "Feyenoord Rotterdam vs Como",
			},
			expectedChannel: "ESPN 2 HD",
			expectFound:     true,
		},
		{
			name: "Manchester City vs Paris Saint-Germain maps to ESPN 2 HD",
			match: model.Match{
				HomeTeam: model.Competitor{DisplayName: "Manchester City"},
				AwayTeam: model.Competitor{DisplayName: "Paris Saint-Germain"},
				Title:    "Manchester City vs Paris Saint-Germain",
			},
			expectedChannel: "ESPN 2 HD",
			expectFound:     true,
		},
		{
			name: "Bayern Munich vs Viking FK not in guides returns not found",
			match: model.Match{
				HomeTeam: model.Competitor{DisplayName: "Bayern Munich"},
				AwayTeam: model.Competitor{DisplayName: "Viking FK"},
				Title:    "Bayern Munich vs Viking FK",
			},
			expectedChannel: "",
			expectFound:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, found := svc.ResolveMatch(tt.match)
			if found != tt.expectFound {
				t.Fatalf("expected found=%v, got %v", tt.expectFound, found)
			}
			if found && target.Name != tt.expectedChannel {
				t.Errorf("expected channel %q, got %q", tt.expectedChannel, target.Name)
			}
		})
	}
}

func TestCacheSerialization_FreshAndExpired(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tvguide_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	fixedTime := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	nowFunc := func() time.Time { return fixedTime }

	sampleGuides := []ChannelGuide{
		{
			Name:      "ESPN 1 HD",
			Slug:      "espn_colombia",
			StreamURL: "http://example.com/espn1.m3u8",
			Programs:  []string{"LASK vs. Liverpool"},
		},
	}

	// 1. Initialize service and save cache
	svc1 := NewService(
		WithCacheDir(tempDir),
		WithNow(nowFunc),
		WithCacheTTL(4*time.Hour),
	)
	if err := svc1.saveCache(sampleGuides); err != nil {
		t.Fatalf("failed to save cache: %v", err)
	}

	expectedFile := filepath.Join(tempDir, "tvguide_2026-10-14.json")
	if _, err := os.Stat(expectedFile); err != nil {
		t.Fatalf("cache file not created at %s: %v", expectedFile, err)
	}

	// 2. Initialize a new service instance: should load from disk with zero network delay
	svc2 := NewService(
		WithCacheDir(tempDir),
		WithNow(nowFunc),
		WithCacheTTL(4*time.Hour),
	)
	guides := svc2.Guides()
	if len(guides) != 1 || guides[0].Name != "ESPN 1 HD" {
		t.Fatalf("failed to load guides from disk cache: %v", guides)
	}

	// 3. Test TTL expiration: simulate 5 hours later
	expiredTime := fixedTime.Add(5 * time.Hour)
	svc3 := NewService(
		WithCacheDir(tempDir),
		WithNow(func() time.Time { return expiredTime }),
		WithCacheTTL(4*time.Hour),
	)
	if len(svc3.Guides()) != 0 {
		t.Errorf("expected empty guides due to TTL expiration, got %v", svc3.Guides())
	}
}

func TestCrawler_WorkerPool_And_OfflineFallback(t *testing.T) {
	// Set up mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "espn_colombia"):
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(mockESPN1HTML))
		case strings.Contains(r.URL.Path, "espn_2_colombia"):
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(mockESPN2HTML))
		case strings.Contains(r.URL.Path, "win_sports"):
			// Simulate 302 or 404
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	targets := []ChannelTarget{
		{Name: "ESPN 1 HD", Slug: "espn_colombia", StreamURL: "http://example.com/espn1.m3u8"},
		{Name: "ESPN 2 HD", Slug: "espn_2_colombia", StreamURL: "http://example.com/espn2.m3u8"},
		{Name: "Win Sports HD", Slug: "win_sports", StreamURL: "http://example.com/winsports.m3u8"},
		{Name: "Broken Channel", Slug: "broken_slug", StreamURL: "http://example.com"},
	}

	crawler := NewCrawler(server.Client(), server.URL)
	guides, err := crawler.Crawl(context.Background(), targets)
	if err != nil {
		t.Fatalf("crawler error: %v", err)
	}

	// ESPN 1 and ESPN 2 should succeed, Win Sports and Broken Channel should be skipped gracefully
	if len(guides) != 2 {
		t.Fatalf("expected 2 successful guides, got %d", len(guides))
	}

	guideMap := make(map[string]ChannelGuide)
	for _, g := range guides {
		guideMap[g.Name] = g
	}

	if _, ok := guideMap["ESPN 1 HD"]; !ok {
		t.Errorf("missing ESPN 1 HD from crawled guides")
	}
	if _, ok := guideMap["ESPN 2 HD"]; !ok {
		t.Errorf("missing ESPN 2 HD from crawled guides")
	}

	// Test Offline Fallback in Service
	tempDir, _ := os.MkdirTemp("", "tvguide_offline_*")
	defer os.RemoveAll(tempDir)

	svc := NewService(
		WithCacheDir(tempDir),
		WithCrawler(crawler),
		WithTargets(targets),
	)

	// Refresh with mock server
	if err := svc.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh failed: %v", err)
	}
	if len(svc.Guides()) != 2 {
		t.Fatalf("expected 2 guides after refresh, got %d", len(svc.Guides()))
	}

	// Now point crawler to a broken endpoint to simulate network down
	brokenCrawler := NewCrawler(http.DefaultClient, "http://127.0.0.1:54321")
	svcBroken := NewService(
		WithCacheDir(tempDir),
		WithCrawler(brokenCrawler),
		WithTargets(targets),
	)
	// Refresh with broken crawler should gracefully fallback to cached data without panicking
	_ = svcBroken.Refresh(context.Background())
	if len(svcBroken.Guides()) != 2 {
		t.Errorf("offline fallback failed to restore cached guides: %v", svcBroken.Guides())
	}
}

func TestParseProgramEntries(t *testing.T) {
	htmlSample := `
	<table>
		<tr class="tbl_EPG_row">
			<td><div class="tbl_EPG_TimesColumn"><time datetime="13:40">1:40 PM</time></div></td>
			<td><div class="tbl_EPG_TimesColumn"><time datetime="16:00">4:00 PM</time></div></td>
			<td>
				<div class="tbl_EPG_ProgramsColumn deporte">
					<div class="div_program_title_on_channel">
						<span>Aston Villa vs. Fenerbahce</span>
					</div>
				</div>
			</td>
		</tr>
		<tr class="tbl_EPG_row_selected">
			<td><div class="tbl_EPG_TimesColumn"><time datetime="22:00">10:00 PM</time></div></td>
			<td><div class="tbl_EPG_TimesColumn"><time datetime="00:00">12:00 AM</time></div></td>
			<td>
				<div class="tbl_EPG_ProgramsColumn deporte">
					<div class="div_program_title_on_channel">
						<span>Slovan Bratislava vs. Stuttgart</span>
					</div>
				</div>
			</td>
		</tr>
	</table>`

	entries := ParseProgramEntries(htmlSample)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	if entries[0].Title != "Aston Villa vs. Fenerbahce" || entries[0].StartTime != "13:40" || entries[0].IsLiveNow {
		t.Errorf("unexpected entry 0: %+v", entries[0])
	}
	if entries[1].Title != "Slovan Bratislava vs. Stuttgart" || entries[1].StartTime != "22:00" || !entries[1].IsLiveNow {
		t.Errorf("unexpected entry 1: %+v", entries[1])
	}
}

func TestResolveMatch_TimeAwareReplayDisqualification(t *testing.T) {
	svc := NewService()

	// Channel 3 has Slovan Bratislava as a replay at 22:00, and Aston Villa LIVE at 13:40
	// Channel 4 has Slovan Bratislava LIVE at 13:40, and Aston Villa as a replay at 22:00
	svc.SetGuides([]ChannelGuide{
		{
			Name:      "ESPN 3 HD",
			Slug:      "espn_3_colombia",
			StreamURL: "http://example.com/espn3.m3u8",
			Entries: []ProgramEntry{
				{Title: "Aston Villa vs. Fenerbahce", StartTime: "13:40", EndTime: "16:00"},
				{Title: "Slovan Bratislava vs. Stuttgart", StartTime: "22:00", EndTime: "00:00"},
			},
		},
		{
			Name:      "ESPN 4 HD",
			Slug:      "espn_4_colombia",
			StreamURL: "http://example.com/espn4.m3u8",
			Entries: []ProgramEntry{
				{Title: "Slovan Bratislava vs. Stuttgart", StartTime: "13:40", EndTime: "16:00"},
				{Title: "Aston Villa vs. Fenerbahce", StartTime: "22:00", EndTime: "00:00"},
			},
		},
	})

	loc, _ := time.LoadLocation("America/Bogota")
	matchTime := time.Date(2026, 10, 14, 14, 0, 0, 0, loc)

	slovanMatch := model.Match{
		Title:     "Slovan Bratislava vs VfB Stuttgart",
		StartTime: matchTime,
		HomeTeam:  model.Competitor{DisplayName: "Slovan Bratislava"},
		AwayTeam:  model.Competitor{DisplayName: "VfB Stuttgart"},
	}

	target, found := svc.ResolveMatch(slovanMatch)
	if !found {
		t.Fatalf("expected to resolve Slovan match, got not found")
	}
	if target.Name != "ESPN 4 HD" {
		t.Errorf("expected Slovan to resolve to ESPN 4 HD (live at 13:40), but got %s (replay was on ESPN 3)", target.Name)
	}

	villaMatch := model.Match{
		Title:     "Aston Villa vs Fenerbahce",
		StartTime: matchTime,
		HomeTeam:  model.Competitor{DisplayName: "Aston Villa"},
		AwayTeam:  model.Competitor{DisplayName: "Fenerbahce"},
	}

	targetVilla, foundVilla := svc.ResolveMatch(villaMatch)
	if !foundVilla {
		t.Fatalf("expected to resolve Villa match, got not found")
	}
	if targetVilla.Name != "ESPN 3 HD" {
		t.Errorf("expected Aston Villa to resolve to ESPN 3 HD (live at 13:40), but got %s", targetVilla.Name)
	}
}


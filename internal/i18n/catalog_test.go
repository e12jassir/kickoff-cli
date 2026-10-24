package i18n

import (
	"reflect"
	"strings"
	"testing"
)

// TestCatalogParity enforces the rule that both compiled catalogs are complete.
// A field left untranslated is detectable here instead of surfacing as an empty
// gap in the middle of the interface.
func TestCatalogParity(t *testing.T) {
	catalogs := []struct {
		name string
		cat  *Catalog
	}{
		{"es", New(ES)},
		{"en", New(EN)},
	}

	typ := reflect.TypeOf(*catalogs[0].cat)
	texts := 0
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}
		texts++
		if field.Type.Kind() != reflect.String {
			t.Errorf("catalog field %s has type %s, want a plain string text", field.Name, field.Type)
			continue
		}
		for _, c := range catalogs {
			value := reflect.ValueOf(c.cat).Elem().Field(i).String()
			if strings.TrimSpace(value) == "" {
				t.Errorf("catalog %s field %s is empty; every domain text needs a value", c.name, field.Name)
			}
		}
	}

	if texts == 0 {
		t.Fatal("Catalog exposes no text fields")
	}
}

// TestCatalogFieldsUseNamedPlaceholdersOnly pins the interpolation contract:
// texts are filled through named placeholders such as {league}, never through
// positional verbs, because argument order changes with the target language.
func TestCatalogFieldsUseNamedPlaceholdersOnly(t *testing.T) {
	catalogs := []struct {
		name string
		cat  *Catalog
	}{
		{"es", New(ES)},
		{"en", New(EN)},
	}

	typ := reflect.TypeOf(*catalogs[0].cat)
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() || field.Type.Kind() != reflect.String {
			continue
		}
		for _, c := range catalogs {
			value := reflect.ValueOf(c.cat).Elem().Field(i).String()
			if strings.Contains(value, "%") {
				t.Errorf("catalog %s field %s uses a positional verb: %q", c.name, field.Name, value)
			}
		}
	}
}

// TestInterpolationNamedArgs renders every interpolated text and compares it
// against the exact Spanish wording the interface shows today, so the migration
// in the following tasks cannot drift from the current output.
func TestInterpolationNamedArgs(t *testing.T) {
	cat := New(ES)

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"league header", cat.Header("LaLiga"), "KICKOFF · LaLiga"},
		{"offline badge", cat.OfflineBadge("14:05"), "[OFFLINE · cached at 14:05]"},
		{"startup loader", cat.StartupLoader("⠋"), "⠋ Cargando Kickoff..."},
		{"menu help with update", cat.MenuHelpUpdate("1.2.0"), "↑/k ↓/j: navegar  •  enter: entrar  •  u: actualizar (1.2.0)  •  q: salir"},
		{"update banner", cat.UpdateBanner("*", "1.2.0"), "* ¡Actualización disponible (1.2.0)! Presiona [u] para actualizar"},
		{"connecting pulse", cat.ConnectingPulse("⠋", "ESPN", "~~", "auto • 1080p"), "⠋ Conectando con ESPN... ~~\nBitrate: auto • 1080p"},
		{"error body", cat.ErrorBody("boom"), "  ✖ Error: boom"},
		{"no upcoming matches", cat.NoUpcomingMatches("LaLiga"), "No hay próximos partidos programados para LaLiga."},
		{"matches today", cat.MatchesToday(4), "PARTIDOS DE HOY (4)"},
		{"upcoming dated", cat.UpcomingMatches("02/01", 3), "PRÓXIMOS PARTIDOS · 02/01 (3)"},
		{"upcoming undated", cat.UpcomingMatches("", 3), "PRÓXIMOS PARTIDOS (3)"},
		{"position counter", cat.PositionCounter(2, 7), "  •  [2/7]"},
		{"league leader", cat.LeagueLeader("Atlético", "30", "12", "+8"), "[1] Líder de Liga: Atlético (30 pts • 12 PJ • DG +8)"},
		{"recent results last round", cat.RecentResultsLastRound(0), "RESULTADOS RECIENTES / JORNADA PASADA (0)"},
		{"recent results", cat.RecentResults(9, 3), "RESULTADOS RECIENTES (9)  •  [3/9]"},
		{"standings loading", cat.StandingsLoading("⠋"), "  ⠋ Cargando tabla de posiciones..."},
		{"standings header", cat.StandingsHeader("LaLiga"), "TABLA DE POSICIONES · LaLiga"},
		{"group navigation", cat.GroupNavigation("Grupo A", 1, 2), "◄ Grupo A (1/2) ►  [ [ / ] : grupo ]"},
		{"standings group header", cat.StandingsGroupHeader("nav"), "POSICIONES · nav"},
		{"scorers loading", cat.ScorersLoading("⠋"), "  ⠋ Cargando máximos goleadores y asistentes..."},
		{"live status", cat.LiveStatus("45'"), "EN VIVO 45'"},
		{"today marker", cat.StatusToday("18:30"), "18:30 (Hoy)"},
		{"tomorrow marker", cat.StatusTomorrow("18:30"), "18:30 (Mañana)"},
		{"yesterday marker", cat.StatusYesterday("18:30"), "18:30 (Ayer)"},
		{"referee", cat.Referee("Ana"), "Árbitro: Ana"},
		{"coach", cat.Coach("Pep"), "DT: Pep"},
		{"penalty marker", cat.WithPenalty("Luis"), "Luis (pen.)"},
		{"own goal marker", cat.WithOwnGoal("Luis"), "Luis (AG)"},
		{"now playing", cat.NowPlaying("ESPN"), "✓ Reproduciendo: ESPN • x: parar señal"},
		{"playback error", cat.PlaybackError("boom"), "Error de reproducción: boom"},
		{"update error", cat.UpdateError("boom"), "Error al actualizar: boom"},
		{"update succeeded", cat.UpdateSucceeded("v1.2.0"), "✓ kickoff actualizado a v1.2.0. ¡Reinicia la app!"},
		{"loading matches", cat.LoadingMatches("LaLiga"), "Consultando partidos de hoy (LaLiga)..."},
		{"reloading data", cat.ReloadingData("LaLiga"), "Recargando datos (LaLiga)..."},
		{"connecting", cat.Connecting("ESPN"), "Conectando con ESPN..."},
		{"goal penalty title", cat.GoalPenaltyTitle("Luis", "45'"), "¡GOL de Luis! (pen. 45')"},
		{"goal own goal title", cat.GoalOwnGoalTitle("Luis", "45'"), "¡GOL en propia puerta de Luis! (45')"},
		{"goal title", cat.GoalTitle("Luis", "45'"), "¡GOL de Luis! (45')"},
		{"goal score title", cat.GoalScoreTitle("A 1 - 0 B"), "¡GOL! A 1 - 0 B"},
		{"assist with name", cat.AssistWith("Juan"), "Asistencia: Juan"},
		{"goal body", cat.GoalBody("A 1 - 0 B", "Asistencia: Juan"), "A 1 - 0 B\nAsistencia: Juan"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("got %q, want %q", tc.got, tc.want)
			}
			if strings.ContainsAny(tc.got, "{}") {
				t.Errorf("rendered text kept a placeholder: %q", tc.got)
			}
		})
	}
}

// TestInterpolatePrefersLongestPlaceholder pins the one ambiguity named
// placeholders can hit: {n} must not eat the prefix of {name}.
func TestInterpolatePrefersLongestPlaceholder(t *testing.T) {
	got := interpolate("{name}|{n}", map[string]string{"name": "equipo", "n": "3"})
	if want := "equipo|3"; got != want {
		t.Errorf("interpolate() = %q, want %q", got, want)
	}
}

// TestPluralOneOther pins the one/other selection every countable text needs.
func TestPluralOneOther(t *testing.T) {
	cat := New(ES)

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"one older match", cat.ScrollOlder(1), "▲  1 partido anterior  ▲"},
		{"several older matches", cat.ScrollOlder(4), "▲  4 partidos anteriores  ▲"},
		{"one newer match", cat.ScrollNewer(1), "▼  1 partido más abajo  ▼"},
		{"several newer matches", cat.ScrollNewer(2), "▼  2 partidos más abajo  ▼"},
		{"zero uses the other form", cat.ScrollNewer(0), "▼  0 partidos más abajo  ▼"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("got %q, want %q", tc.got, tc.want)
			}
		})
	}

	// The English catalog must answer for the same counts without an empty text.
	if got := New(EN).ScrollOlder(1); strings.TrimSpace(got) == "" {
		t.Error("en catalog rendered an empty plural text")
	}
}

// TestMissingKeyVisible keeps the deliberate "!" + key marker honest: it must be
// reachable for a key the catalog does not know, and unreachable for every text
// a production catalog does know.
func TestMissingKeyVisible(t *testing.T) {
	cat := New(ES)

	if got, want := cat.Text("NoSuchKey"), "!NoSuchKey"; got != want {
		t.Errorf("missing key rendered %q, want %q", got, want)
	}

	// An English catalog without a translation falls back to the Spanish base.
	untranslated := &Catalog{lang: EN}
	if got, want := untranslated.Text("WindowTooSmall"), cat.WindowTooSmall; got != want {
		t.Errorf("untranslated en field rendered %q, want the es fallback %q", got, want)
	}

	typ := reflect.TypeOf(*cat)
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() || field.Type.Kind() != reflect.String {
			continue
		}
		for _, lang := range []Lang{ES, EN} {
			if got := New(lang).Text(field.Name); strings.HasPrefix(got, "!") {
				t.Errorf("catalog %s reports known field %s as missing: %q", lang, field.Name, got)
			}
		}
	}
}

// TestDefaultReturnsCompleteCatalog guards the exported process-wide accessor:
// whichever language the environment resolves, the result is a full catalog.
func TestDefaultReturnsCompleteCatalog(t *testing.T) {
	cat := Default()
	if cat == nil {
		t.Fatal("Default returned nil")
	}

	typ := reflect.TypeOf(*cat)
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() || field.Type.Kind() != reflect.String {
			continue
		}
		if strings.TrimSpace(reflect.ValueOf(cat).Elem().Field(i).String()) == "" {
			t.Errorf("Default catalog field %s is empty", field.Name)
		}
	}
}

// TestNewUnknownLanguageFallsBackToSpanish keeps New total: an unsupported code
// still yields a usable catalog instead of one full of empty texts.
func TestNewUnknownLanguageFallsBackToSpanish(t *testing.T) {
	unknown := New(Lang("fr"))
	if unknown == nil {
		t.Fatal("New returned nil for an unknown language")
	}
	if got, want := unknown.WindowTooSmall, New(ES).WindowTooSmall; got != want {
		t.Errorf("unknown language rendered %q, want the es base %q", got, want)
	}
}

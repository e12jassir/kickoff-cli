package i18n

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/e12jassir/kickoff-cli/internal/model"
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
		{"setup title", cat.SetupTitle("v1.0"), "Kickoff v1.0 — Experiencia de fútbol CLI y centro de transmisión"},
		{"setup option language", cat.SetupOptionLanguage("Español"), "Idioma / Language: Español"},
		{"setup update prompt", cat.SetupUpdatePrompt("v1.0", "v1.1"), "    Versión actual:   v1.0\n    Última versión:   v1.1\n\n    ¿Deseas descargar e instalar esta actualización ahora?"},
		{"setup status updating", cat.SetupStatusUpdating("v1.1"), "Actualizando Kickoff a v1.1..."},
		{"setup status updated", cat.SetupStatusUpdated("v1.1"), "¡Kickoff actualizado a v1.1! Reinicia la app para aplicar."},
		{"setup status update failed", cat.SetupStatusUpdateFailed("network"), "Error al actualizar: network"},
		{"setup status up to date", cat.SetupStatusUpToDate("v1.0"), "Kickoff está actualizado (v1.0)."},
		{"setup status cache cleared", cat.SetupStatusCacheCleared(5), "Caché de API limpiada (5 archivos temporales eliminados)."},
		{"setup step os detected", cat.SetupStepOSDetected("linux", "amd64"), "linux/amd64 detectado"},
		{"setup step mpv installed", cat.SetupStepMpvInstalled("v0.38"), "Instalado: v0.38"},
		{"setup step channels imported", cat.SetupStepChannelsImported(12, "channels.json"), "Se importaron 12 canales en channels.json"},
		{"setup mpv operational", cat.SetupMpvOperational("v0.38"), "mpv operativo: v0.38"},
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

// TestFormatError proves domain errors map to localized catalog strings,
// support wrapped errors via errors.Is, fall back to err.Error() when unrecognized,
// and return "" when nil.
func TestFormatError(t *testing.T) {
	cat := New(ES)

	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "nil error returns empty string",
			err:  nil,
			want: "",
		},
		{
			name: "ErrPlayerTimeout",
			err:  model.ErrPlayerTimeout,
			want: "el reproductor tardó demasiado en responder",
		},
		{
			name: "wrapped ErrPlayerTimeout",
			err:  fmt.Errorf("mpv launch failed: %w", model.ErrPlayerTimeout),
			want: "el reproductor tardó demasiado en responder",
		},
		{
			name: "ErrPlayerExited",
			err:  model.ErrPlayerExited,
			want: "el reproductor se cerró de forma inesperada",
		},
		{
			name: "wrapped ErrPlayerExited",
			err:  fmt.Errorf("subsystem failure: %w", model.ErrPlayerExited),
			want: "el reproductor se cerró de forma inesperada",
		},
		{
			name: "ErrInvalidURL",
			err:  model.ErrInvalidURL,
			want: "la URL de la transmisión no es válida",
		},
		{
			name: "wrapped ErrInvalidURL",
			err:  fmt.Errorf("parse error: %w", model.ErrInvalidURL),
			want: "la URL de la transmisión no es válida",
		},
		{
			name: "ErrCorruptedPayload",
			err:  model.ErrCorruptedPayload,
			want: "datos corruptos o inválidos del servidor",
		},
		{
			name: "wrapped ErrCorruptedPayload",
			err:  fmt.Errorf("json decoding: %w", model.ErrCorruptedPayload),
			want: "datos corruptos o inválidos del servidor",
		},
		{
			name: "ErrUpdateFailed",
			err:  model.ErrUpdateFailed,
			want: "no se pudo instalar la actualización",
		},
		{
			name: "wrapped ErrUpdateFailed",
			err:  fmt.Errorf("checksum mismatch: %w", model.ErrUpdateFailed),
			want: "no se pudo instalar la actualización",
		},
		{
			name: "ErrNetwork",
			err:  model.ErrNetwork,
			want: "error en la solicitud de red",
		},
		{
			name: "wrapped ErrNetwork",
			err:  fmt.Errorf("dial tcp timeout: %w", model.ErrNetwork),
			want: "error en la solicitud de red",
		},
		{
			name: "unrecognized error falls back to err.Error()",
			err:  errors.New("unrecognized custom failure"),
			want: "unrecognized custom failure",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := cat.FormatError(tc.err)
			if got != tc.want {
				t.Errorf("FormatError(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

// TestFormatPlaybackError tests that domain errors are localized and formatted
// through PlaybackErrorTemplate.
func TestFormatPlaybackError(t *testing.T) {
	cat := New(ES)

	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "nil error returns empty string",
			err:  nil,
			want: "",
		},
		{
			name: "domain error",
			err:  model.ErrPlayerTimeout,
			want: "Error de reproducción: el reproductor tardó demasiado en responder",
		},
		{
			name: "unrecognized error",
			err:  errors.New("mpv crashed"),
			want: "Error de reproducción: mpv crashed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := cat.FormatPlaybackError(tc.err)
			if got != tc.want {
				t.Errorf("FormatPlaybackError(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

// TestFormatUpdateError tests that update errors are localized and formatted
// through UpdateErrorTemplate.
func TestFormatUpdateError(t *testing.T) {
	cat := New(ES)

	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "nil error returns empty string",
			err:  nil,
			want: "",
		},
		{
			name: "domain error",
			err:  model.ErrUpdateFailed,
			want: "Error al actualizar: no se pudo instalar la actualización",
		},
		{
			name: "unrecognized error",
			err:  errors.New("permission denied"),
			want: "Error al actualizar: permission denied",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := cat.FormatUpdateError(tc.err)
			if got != tc.want {
				t.Errorf("FormatUpdateError(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

// TestEnglishCatalogTranslations tests that New(EN) returns natural, idiomatic
// English translations for all key fields and that it does not return Spanish text.
func TestEnglishCatalogTranslations(t *testing.T) {
	cat := New(EN)
	esCat := New(ES)

	// Distinct translations must not match Spanish base.
	distinctFields := []struct {
		name string
		en   string
		es   string
		want string
	}{
		{"TabResumen", cat.TabResumen, esCat.TabResumen, "1 Summary"},
		{"TabPartidos", cat.TabPartidos, esCat.TabPartidos, "2 Matches"},
		{"TabPosiciones", cat.TabPosiciones, esCat.TabPosiciones, "3 Standings"},
		{"TabGoleadores", cat.TabGoleadores, esCat.TabGoleadores, "4 Top Scorers"},
		{"FallbackHome", cat.FallbackHome, esCat.FallbackHome, "Home"},
		{"FallbackAway", cat.FallbackAway, esCat.FallbackAway, "Away"},
		{"FallbackVenue", cat.FallbackVenue, esCat.FallbackVenue, "Main Stadium"},
		{"FallbackPlayer", cat.FallbackPlayer, esCat.FallbackPlayer, "Player"},
		{"FallbackCoachStaff", cat.FallbackCoachStaff, esCat.FallbackCoachStaff, "Coaching Staff"},
		{"FallbackOwnGoal", cat.FallbackOwnGoal, esCat.FallbackOwnGoal, "Own Goal"},
		{"FallbackGoal", cat.FallbackGoal, esCat.FallbackGoal, "Goal"},
		{"FallbackChannel", cat.FallbackChannel, esCat.FallbackChannel, "Channel"},
		{"StatusPre", cat.StatusPre, esCat.StatusPre, "Upcoming"},
		{"StatusPost", cat.StatusPost, esCat.StatusPost, "FT"},
		{"StatusFinished", cat.StatusFinished, esCat.StatusFinished, "FINISHED"},
		{"StatusScheduled", cat.StatusScheduled, esCat.StatusScheduled, "Scheduled"},
		{"ColumnTeam", cat.ColumnTeam, esCat.ColumnTeam, "Team"},
		{"ColumnPlayed", cat.ColumnPlayed, esCat.ColumnPlayed, "P"},
		{"ColumnWon", cat.ColumnWon, esCat.ColumnWon, "W"},
		{"ColumnDrawn", cat.ColumnDrawn, esCat.ColumnDrawn, "D"},
		{"ColumnLost", cat.ColumnLost, esCat.ColumnLost, "L"},
		{"ColumnGoalDiff", cat.ColumnGoalDiff, esCat.ColumnGoalDiff, "GD"},
		{"ColumnPlayer", cat.ColumnPlayer, esCat.ColumnPlayer, "Player"},
		{"ColumnGoals", cat.ColumnGoals, esCat.ColumnGoals, "Goals"},
		{"ColumnAssists", cat.ColumnAssists, esCat.ColumnAssists, "Assists"},
		{"ModalTabSummary", cat.ModalTabSummary, esCat.ModalTabSummary, "Summary"},
		{"ModalTabStats", cat.ModalTabStats, esCat.ModalTabStats, "Statistics"},
		{"ModalTabLineups", cat.ModalTabLineups, esCat.ModalTabLineups, "Lineups"},
		{"SignalsSection", cat.SignalsSection, esCat.SignalsSection, "BROADCAST SIGNALS"},
		{"Recommended", cat.Recommended, esCat.Recommended, "★ Recommended"},
		{"Possession", cat.Possession, esCat.Possession, "Possession"},
		{"StatShots", cat.StatShots, esCat.StatShots, "Shots (On target)"},
		{"StatPasses", cat.StatPasses, esCat.StatPasses, "Passes (Accuracy)"},
		{"StatFouls", cat.StatFouls, esCat.StatFouls, "Fouls"},
		{"StatCorners", cat.StatCorners, esCat.StatCorners, "Corners"},
		{"StatOffsides", cat.StatOffsides, esCat.StatOffsides, "Offsides"},
		{"StatSaves", cat.StatSaves, esCat.StatSaves, "Saves"},
		{"LineupStarters", cat.LineupStarters, esCat.LineupStarters, "STARTERS"},
		{"LineupSubstitutes", cat.LineupSubstitutes, esCat.LineupSubstitutes, "SUBSTITUTES"},
		{"TopScorersHeader", cat.TopScorersHeader, esCat.TopScorersHeader, "TOP SCORERS (TOP 5)"},
		{"TopAssistsHeader", cat.TopAssistsHeader, esCat.TopAssistsHeader, "TOP ASSISTS (TOP 5)"},
		{"StreamStopped", cat.StreamStopped, esCat.StreamStopped, "⏹ Stream stopped"},
		{"ErrPlayerTimeout", cat.ErrPlayerTimeout, esCat.ErrPlayerTimeout, "player timed out waiting to respond"},
		{"ErrPlayerExited", cat.ErrPlayerExited, esCat.ErrPlayerExited, "player exited unexpectedly"},
		{"ErrInvalidURL", cat.ErrInvalidURL, esCat.ErrInvalidURL, "invalid streaming URL"},
		{"ErrCorruptedPayload", cat.ErrCorruptedPayload, esCat.ErrCorruptedPayload, "corrupted or invalid payload from server"},
		{"ErrUpdateFailed", cat.ErrUpdateFailed, esCat.ErrUpdateFailed, "failed to install update"},
		{"ErrNetwork", cat.ErrNetwork, esCat.ErrNetwork, "network request failed"},
	}

	for _, tc := range distinctFields {
		t.Run(tc.name, func(t *testing.T) {
			if tc.en == tc.es {
				t.Errorf("%s in English equals Spanish: %q", tc.name, tc.en)
			}
			if tc.en != tc.want {
				t.Errorf("%s got %q, want %q", tc.name, tc.en, tc.want)
			}
		})
	}

	// Verify formatted helpers and domain errors under English catalog.
	if got, want := cat.FormatError(model.ErrPlayerTimeout), "player timed out waiting to respond"; got != want {
		t.Errorf("FormatError(ErrPlayerTimeout) = %q, want %q", got, want)
	}
	if got, want := cat.FormatPlaybackError(model.ErrPlayerTimeout), "Playback error: player timed out waiting to respond"; got != want {
		t.Errorf("FormatPlaybackError(ErrPlayerTimeout) = %q, want %q", got, want)
	}
	if got, want := cat.FormatUpdateError(model.ErrUpdateFailed), "Update error: failed to install update"; got != want {
		t.Errorf("FormatUpdateError(ErrUpdateFailed) = %q, want %q", got, want)
	}
	if got, want := cat.Referee("Oliver"), "Referee: Oliver"; got != want {
		t.Errorf("Referee = %q, want %q", got, want)
	}
	if got, want := cat.Coach("Arteta"), "Manager: Arteta"; got != want {
		t.Errorf("Coach = %q, want %q", got, want)
	}
	if got, want := cat.WithPenalty("Saka"), "Saka (pen.)"; got != want {
		t.Errorf("WithPenalty = %q, want %q", got, want)
	}
	if got, want := cat.WithOwnGoal("Saka"), "Saka (OG)"; got != want {
		t.Errorf("WithOwnGoal = %q, want %q", got, want)
	}
	if got, want := cat.MatchesToday(3), "MATCHES TODAY (3)"; got != want {
		t.Errorf("MatchesToday = %q, want %q", got, want)
	}
	if got, want := cat.UpcomingMatches("02/01", 3), "UPCOMING MATCHES · 02/01 (3)"; got != want {
		t.Errorf("UpcomingMatches = %q, want %q", got, want)
	}
	if got, want := cat.ScrollOlder(1), "▲  1 previous match  ▲"; got != want {
		t.Errorf("ScrollOlder(1) = %q, want %q", got, want)
	}
	if got, want := cat.ScrollOlder(3), "▲  3 previous matches  ▲"; got != want {
		t.Errorf("ScrollOlder(3) = %q, want %q", got, want)
	}
	if got, want := cat.ScrollNewer(1), "▼  1 more match below  ▼"; got != want {
		t.Errorf("ScrollNewer(1) = %q, want %q", got, want)
	}
	if got, want := cat.ScrollNewer(3), "▼  3 more matches below  ▼"; got != want {
		t.Errorf("ScrollNewer(3) = %q, want %q", got, want)
	}
	if got, want := cat.LeagueLeader("Arsenal", "30", "12", "+8"), "[1] League Leader: Arsenal (30 pts • 12 MP • GD +8)"; got != want {
		t.Errorf("LeagueLeader = %q, want %q", got, want)
	}
	if got, want := cat.NowPlaying("Sky"), "✓ Now playing: Sky • x: stop stream"; got != want {
		t.Errorf("NowPlaying = %q, want %q", got, want)
	}
	if got, want := cat.GoalPenaltyTitle("Saka", "45'"), "GOAL by Saka! (pen. 45')"; got != want {
		t.Errorf("GoalPenaltyTitle = %q, want %q", got, want)
	}
	if got, want := cat.GoalOwnGoalTitle("Saka", "45'"), "OWN GOAL by Saka! (45')"; got != want {
		t.Errorf("GoalOwnGoalTitle = %q, want %q", got, want)
	}
	if got, want := cat.GoalTitle("Saka", "45'"), "GOAL by Saka! (45')"; got != want {
		t.Errorf("GoalTitle = %q, want %q", got, want)
	}
	if got, want := cat.GoalScoreTitle("ARS 1 - 0 CHE"), "GOAL! ARS 1 - 0 CHE"; got != want {
		t.Errorf("GoalScoreTitle = %q, want %q", got, want)
	}
	if got, want := cat.AssistWith("Odegaard"), "Assist: Odegaard"; got != want {
		t.Errorf("AssistWith = %q, want %q", got, want)
	}
	if got, want := cat.GoalBody("ARS 1 - 0 CHE", "Assist: Odegaard"), "ARS 1 - 0 CHE\nAssist: Odegaard"; got != want {
		t.Errorf("GoalBody = %q, want %q", got, want)
	}
}

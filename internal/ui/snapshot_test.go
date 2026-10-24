package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/e12jassir/kickoff-cli/internal/i18n"
	"github.com/e12jassir/kickoff-cli/internal/model"
)

// TestRender_NeutralSpanishUnchanged pins the invariant that every view and tab
// rendered under the default Spanish catalog produces the exact, byte-identical
// neutral Spanish strings expected by Kickoff CLI (S2, S9).
func TestRender_NeutralSpanishUnchanged(t *testing.T) {
	cat := i18n.New(i18n.ES)

	// 1. Startup view (StateStartup)
	t.Run("Startup view", func(t *testing.T) {
		m := NewModel(&mockClient{}, &mockCatalog{}, &mockPlayer{}).WithInitialState(StateStartup)
		m.width = 100
		m.height = 30
		view := m.View()

		assertContains(t, view, "Terminal Live Football")
		assertContains(t, view, "Cargando Competiciones...")
		assertContains(t, view, "Presiona cualquier tecla para continuar...")
	})

	// 2. Main menu view (StateMainMenu)
	t.Run("Main menu view", func(t *testing.T) {
		m := NewModel(&mockClient{}, &mockCatalog{}, &mockPlayer{}).WithInitialState(StateMainMenu)
		m.width = 100
		m.height = 30
		view := m.View()

		assertContains(t, view, "Terminal Live Football")
		assertContains(t, view, "navegar")
		assertContains(t, view, "desplegar/entrar")
		assertContains(t, view, "salir")
	})

	// 3. TabPartidos (TabPartidos) with sample matches (both populated and fallbacks)
	t.Run("TabPartidos with sample match and fallbacks", func(t *testing.T) {
		now := time.Now()
		m := NewModel(&mockClient{}, &mockCatalog{}, &mockPlayer{}).WithInitialState(StateBrowsing)
		m.width = 100
		m.height = 30
		m.leagueTab = TabPartidos
		m.matches = []model.Match{
			{
				ID:        "m-today",
				StartTime: now,
				HomeTeam:  model.Competitor{DisplayName: "Real Madrid", Score: "2"},
				AwayTeam:  model.Competitor{DisplayName: "Barcelona", Score: "1"},
				Venue:     "Santiago Bernabéu",
				Status:    model.MatchStatus{State: "post", Description: "Finalizado", Completed: true},
			},
			{
				ID:        "m-fallback",
				StartTime: now.Add(24 * time.Hour),
				HomeTeam:  model.Competitor{DisplayName: ""},
				AwayTeam:  model.Competitor{DisplayName: ""},
				Venue:     "",
				Status:    model.MatchStatus{State: "pre", Description: ""},
			},
		}
		view := m.View()

		assertContains(t, view, "PARTIDOS DE HOY")
		assertContains(t, view, "Partidos")
		assertContains(t, view, "enter: detalles")
		// Assert fallback texts when competitor names/venues are empty
		assertContains(t, view, "Local")
		assertContains(t, view, "Visitante")
		assertContains(t, view, "Por jugar")
		assertContains(t, view, "Estadio Principal")
	})

	// 4. TabResumen (TabResumen) with sample leader and past matches
	t.Run("TabResumen with leader and past matches", func(t *testing.T) {
		m := NewModel(&mockClient{}, &mockCatalog{}, &mockPlayer{}).WithInitialState(StateBrowsing)
		m.width = 100
		m.height = 30
		m.leagueTab = TabResumen
		m.standings = &model.LeagueStandings{
			LeagueKey: "laliga",
			Entries: []model.StandingsEntry{
				{
					Rank:        1,
					TeamName:    "Atlético Madrid",
					Points:      "30",
					GamesPlayed: "12",
					PointDiff:   "+8",
				},
			},
		}
		m.matches = []model.Match{
			{
				ID:        "m-past",
				StartTime: time.Now().Add(-24 * time.Hour),
				HomeTeam:  model.Competitor{DisplayName: "Real Madrid", Score: "3"},
				AwayTeam:  model.Competitor{DisplayName: "Sevilla", Score: "0"},
				Status:    model.MatchStatus{State: "post", Completed: true},
			},
		}
		view := m.View()

		assertContains(t, view, "Resumen")
		assertContains(t, view, "Líder de Liga")
		assertContains(t, view, "PJ")
		assertContains(t, view, "DG")
		assertContains(t, view, "RESULTADOS RECIENTES")
		assertContains(t, view, "Enter: detalles")
	})

	// 5. TabPosiciones (TabPosiciones) with sample standings table
	t.Run("TabPosiciones with standings table", func(t *testing.T) {
		m := NewModel(&mockClient{}, &mockCatalog{}, &mockPlayer{}).WithInitialState(StateBrowsing)
		m.width = 100
		m.height = 30
		m.leagueTab = TabPosiciones
		m.standings = &model.LeagueStandings{
			LeagueKey: "laliga",
			Entries: []model.StandingsEntry{
				{
					Rank:        1,
					TeamName:    "Barcelona",
					GamesPlayed: "10",
					Wins:        "8",
					Ties:        "1",
					Losses:      "1",
					PointDiff:   "+15",
					Points:      "25",
				},
			},
		}
		view := m.View()

		assertContains(t, view, "Posiciones")
		assertContains(t, view, "TABLA DE POSICIONES")
		assertContains(t, view, "Equipo")
		assertContains(t, view, "PJ")
		assertContains(t, view, "PTS")
		assertContains(t, view, "DG")
	})

	// 6. TabGoleadores (TabGoleadores) with sample statistics
	t.Run("TabGoleadores with statistics", func(t *testing.T) {
		m := NewModel(&mockClient{}, &mockCatalog{}, &mockPlayer{}).WithInitialState(StateBrowsing)
		m.width = 100
		m.height = 30
		m.leagueTab = TabGoleadores
		m.statistics = &model.LeagueStatistics{
			LeagueKey: "laliga",
			Scorers: []model.ScorerEntry{
				{Rank: 1, PlayerName: "R. Lewandowski", TeamName: "Barcelona", Value: "10"},
			},
			Assists: []model.ScorerEntry{
				{Rank: 1, PlayerName: "Lamine Yamal", TeamName: "Barcelona", Value: "6"},
			},
		}
		view := m.View()

		assertContains(t, view, "Goleadores")
		assertContains(t, view, "MÁXIMOS GOLEADORES")
		assertContains(t, view, "MÁXIMOS ASISTENTES")
		assertContains(t, view, "Jugador")
		assertContains(t, view, "Goles")
		assertContains(t, view, "Asist.")
	})

	// 7. Channel modal (renderChannelModal)
	t.Run("Channel modal with signals and fallbacks", func(t *testing.T) {
		// Test modal with fallbacks for empty team names, empty venue, and scheduled status
		modalMatch := model.Match{
			HomeTeam: model.Competitor{DisplayName: ""},
			AwayTeam: model.Competitor{DisplayName: ""},
			Venue:    "",
			Status:   model.MatchStatus{State: "pre", Description: ""},
		}
		channels := []model.Channel{
			{Name: "ESPN Deportes", DisplayName: "ESPN Deportes", BitrateMode: "1080p"},
		}
		modalView := renderChannelModal(modalMatch, time.Now(), channels, 0, 0, false, "", true, false, false, 70, cat)

		assertContains(t, modalView, "Local")
		assertContains(t, modalView, "Visitante")
		assertContains(t, modalView, "Estadio Principal")
		assertContains(t, modalView, "Programado")
		assertContains(t, modalView, "SEÑALES DE TRANSMISIÓN")
		assertContains(t, modalView, "Resumen")
		assertContains(t, modalView, "Estadísticas")
		assertContains(t, modalView, "Alineaciones")
	})

	// 8. Error state (StateError)
	t.Run("Error state", func(t *testing.T) {
		m := NewModel(&mockClient{}, &mockCatalog{}, &mockPlayer{}).WithInitialState(StateBrowsing)
		m.width = 100
		m.height = 30
		m.state = StateError
		m.errorMsg = cat.FormatError(model.ErrPlayerTimeout)
		view := m.View()

		assertContains(t, view, "✖ Error: el reproductor tardó demasiado en responder")
		assertContains(t, view, "recargar")
		assertContains(t, view, "volver")
		assertContains(t, view, "salir")
	})
}

// TestModel_DomainErrorMapping verifies that Update properly formats domain errors
// using the model's catalog on matchesLoadedMsg, playbackErrMsg, and updateFinishedMsg.
func TestModel_DomainErrorMapping(t *testing.T) {
	m := NewModel(&mockClient{}, &mockCatalog{}, &mockPlayer{})

	// 1. matchesLoadedMsg with model.ErrCorruptedPayload
	newM, _ := m.Update(matchesLoadedMsg{
		gen:       m.loadGen,
		leagueKey: m.CurrentLeague().Key,
		err:       model.ErrCorruptedPayload,
	})
	m = newM.(Model)
	if m.state != StateError {
		t.Fatalf("expected StateError, got %d", m.state)
	}
	if got, want := m.errorMsg, "datos corruptos o inválidos del servidor"; got != want {
		t.Errorf("expected errorMsg %q, got %q", want, got)
	}
	viewStr := m.View()
	assertContains(t, viewStr, "datos corruptos o inválidos del servidor")

	// 2. playbackErrMsg with model.ErrPlayerTimeout
	newM, _ = m.Update(playbackErrMsg{err: model.ErrPlayerTimeout})
	m = newM.(Model)
	wantPlaybackErr := "Error de reproducción: el reproductor tardó demasiado en responder"
	if m.errorMsg != wantPlaybackErr {
		t.Errorf("expected errorMsg %q, got %q", wantPlaybackErr, m.errorMsg)
	}

	// 3. updateFinishedMsg with model.ErrUpdateFailed
	newM, _ = m.Update(updateFinishedMsg{err: model.ErrUpdateFailed})
	m = newM.(Model)
	wantUpdateErr := "Error al actualizar: no se pudo instalar la actualización"
	if m.errorMsg != wantUpdateErr {
		t.Errorf("expected errorMsg %q, got %q", wantUpdateErr, m.errorMsg)
	}
}

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected rendered output to contain %q, but it was missing.\nRendered output:\n%s", substr, s)
	}
}

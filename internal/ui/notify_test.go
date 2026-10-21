package ui

import (
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/e12jassir/kickoff-cli/internal/bar"
	"github.com/e12jassir/kickoff-cli/internal/model"
)

// fakeNotifier records the goal notifications emitted by the model so tests can
// assert on them without spawning a real desktop notification.
type fakeNotifier struct {
	mu    sync.Mutex
	goals []string
}

func (f *fakeNotifier) NotifyGoal(match model.Match, homeScore, awayScore string, event *model.MatchEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.goals = append(f.goals, match.ID+" "+homeScore+"-"+awayScore)
	return nil
}

func (f *fakeNotifier) recorded() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.goals))
	copy(out, f.goals)
	return out
}

func liveMatch(id, home, away string) model.Match {
	return model.Match{
		ID:       id,
		Title:    "Home vs Away",
		HomeTeam: model.Competitor{ID: "h-" + id, DisplayName: "Home", Score: home, IsHome: true},
		AwayTeam: model.Competitor{ID: "a-" + id, DisplayName: "Away", Score: away},
		Status:   model.MatchStatus{State: "in"},
	}
}

func finishedMatch(id, home, away string) model.Match {
	m := liveMatch(id, home, away)
	m.Status = model.MatchStatus{State: "post", Completed: true}
	return m
}

// goalModel builds a model wired to a recording notifier.
func goalModel(t *testing.T) (Model, *fakeNotifier) {
	t.Helper()
	m := NewModel(&mockClient{}, &mockCatalog{}, &mockPlayer{})
	notifier := &fakeNotifier{}
	m.notifier = notifier
	m.goalTracker = bar.NewScoreTracker()
	return m, notifier
}

// deliverMatches feeds one silent refresh into the model and runs the commands
// it returns, exactly as the Bubble Tea runtime would.
func deliverMatches(t *testing.T, m Model, matches []model.Match) Model {
	t.Helper()
	updated, cmd := m.Update(matchesLoadedMsg{
		leagueKey: m.CurrentLeague().Key,
		matches:   matches,
		isSilent:  true,
		gen:       m.loadGen,
	})
	runCommand(cmd)
	return updated.(Model)
}

// runCommand executes a command the way the Bubble Tea runtime would, unwrapping
// batches so nested commands (prewarm, notifications) actually run.
func runCommand(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, nested := range batch {
			if nested != nil {
				nested()
			}
		}
	}
}

// TestGoalNotification_FiresOnScoreChange proves a live score change produced by
// a background refresh raises exactly one desktop notification.
func TestGoalNotification_FiresOnScoreChange(t *testing.T) {
	m, notifier := goalModel(t)

	m = deliverMatches(t, m, []model.Match{liveMatch("m1", "1", "0")})
	if got := notifier.recorded(); len(got) != 0 {
		t.Fatalf("first observation must prime the baseline without notifying, got %v", got)
	}

	m = deliverMatches(t, m, []model.Match{liveMatch("m1", "2", "0")})
	got := notifier.recorded()
	if len(got) != 1 {
		t.Fatalf("expected exactly one goal notification, got %v", got)
	}
	if got[0] != "m1 2-0" {
		t.Fatalf("expected notification for m1 2-0, got %q", got[0])
	}

	// A refresh with an unchanged score must stay silent.
	_ = deliverMatches(t, m, []model.Match{liveMatch("m1", "2", "0")})
	if got := notifier.recorded(); len(got) != 1 {
		t.Fatalf("unchanged score must not notify again, got %v", got)
	}
}

// TestGoalNotification_IgnoresFinishedMatches proves completed fixtures never
// raise a goal alert, even when their final score arrives late.
func TestGoalNotification_IgnoresFinishedMatches(t *testing.T) {
	m, notifier := goalModel(t)

	m = deliverMatches(t, m, []model.Match{finishedMatch("m2", "0", "0")})
	m = deliverMatches(t, m, []model.Match{finishedMatch("m2", "1", "0")})

	if got := notifier.recorded(); len(got) != 0 {
		t.Fatalf("completed matches must not notify, got %v", got)
	}
}

// TestGoalNotification_DisabledWithoutNotifier proves a nil notifier or tracker
// disables the feature without panicking.
func TestGoalNotification_DisabledWithoutNotifier(t *testing.T) {
	m := NewModel(&mockClient{}, &mockCatalog{}, &mockPlayer{})
	m.notifier = nil

	m = deliverMatches(t, m, []model.Match{liveMatch("m3", "0", "0")})
	_ = deliverMatches(t, m, []model.Match{liveMatch("m3", "1", "0")})
}

// TestAutoSync_PollsMatchesInMainMenu proves the app keeps polling the selected
// league while the user sits on the main menu, so goals are announced even when
// no match has been opened.
func TestAutoSync_PollsMatchesInMainMenu(t *testing.T) {
	m := NewModel(&mockClient{}, &mockCatalog{}, &mockPlayer{})
	if m.state != StateMainMenu {
		t.Fatalf("expected the model to start on the main menu, got %v", m.state)
	}

	cmds := m.autoSyncCommands()
	if len(cmds) < 2 {
		t.Fatalf("expected a sync tick plus a matches fetch on the main menu, got %d command(s)", len(cmds))
	}

	narration := NewModel(nil, &mockCatalog{}, &mockPlayer{})
	if got := len(narration.autoSyncCommands()); got != 1 {
		t.Fatalf("expected only the sync tick when no client is configured, got %d command(s)", got)
	}
}

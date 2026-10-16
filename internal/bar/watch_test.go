package bar

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"

	"github.com/e12jassir/kickoff-cli/internal/model"
)

type mockNotifier struct {
	mu            sync.Mutex
	notifications []goalNotification
}

type goalNotification struct {
	Match     model.Match
	HomeScore string
	AwayScore string
	Event     *model.MatchEvent
}

func (m *mockNotifier) NotifyGoal(match model.Match, homeScore, awayScore string, event *model.MatchEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.notifications = append(m.notifications, goalNotification{
		Match:     match,
		HomeScore: homeScore,
		AwayScore: awayScore,
		Event:     event,
	})
	return nil
}

func (m *mockNotifier) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.notifications)
}

func TestScoreTracker_BaselineAndGoalDetection(t *testing.T) {
	tracker := NewScoreTracker()

	match := model.Match{
		ID:        "m1",
		HomeTeam:  model.Competitor{DisplayName: "FC Barcelona", Score: "1"},
		AwayTeam:  model.Competitor{DisplayName: "Real Madrid", Score: "1"},
		LeagueKey: "esp.1",
		Status:    model.MatchStatus{State: "in", Detail: "50'"},
	}

	// 1. Initial observation sets baseline, no change returned
	changes1 := tracker.Update([]model.Match{match})
	if len(changes1) != 0 {
		t.Fatalf("expected 0 changes on baseline init, got %d", len(changes1))
	}

	// 2. Score remains same, no change returned
	match.Status.Detail = "55'"
	changes2 := tracker.Update([]model.Match{match})
	if len(changes2) != 0 {
		t.Fatalf("expected 0 changes when score unchanged, got %d", len(changes2))
	}

	// 3. Goal scored by home team!
	match.HomeTeam.Score = "2"
	match.Status.Detail = "60'"
	match.Events = []model.MatchEvent{
		{
			Type:       "goal",
			PlayerName: "Lamine Yamal",
			Clock:      "60'",
			IsScoring:  true,
		},
	}

	changes3 := tracker.Update([]model.Match{match})
	if len(changes3) != 1 {
		t.Fatalf("expected 1 change on goal, got %d", len(changes3))
	}

	ch := changes3[0]
	if ch.PrevHome != "1" || ch.NewHome != "2" || ch.PrevAway != "1" || ch.NewAway != "1" {
		t.Errorf("unexpected score change values: %+v", ch)
	}
	if ch.GoalEvent == nil || ch.GoalEvent.PlayerName != "Lamine Yamal" {
		t.Errorf("expected goal event for Lamine Yamal, got %+v", ch.GoalEvent)
	}
}

func TestWatch_TerminationOnContextCancel(t *testing.T) {
	client := &mockClient{
		allMatches: []model.Match{
			sampleLiveMatch(),
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	var buf bytes.Buffer

	opts := WatchOptions{
		Options:  Options{Format: FormatPlain},
		Interval: 20 * time.Millisecond,
	}

	done := make(chan error, 1)
	go func() {
		done <- Watch(ctx, client, nil, opts, &buf)
	}()

	// Let it run for a couple of ticks
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("expected clean exit (nil error) on context cancellation, got: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Watch loop did not terminate within timeout after context cancel")
	}

	if buf.Len() == 0 {
		t.Error("expected at least one rendered line in buffer")
	}
}

func TestWatch_DispatchesNotifications(t *testing.T) {
	now := time.Now()
	mInitial := model.Match{
		ID:        "m1",
		HomeTeam:  model.Competitor{DisplayName: "FC Barcelona", Score: "1"},
		AwayTeam:  model.Competitor{DisplayName: "Real Madrid", Score: "1"},
		LeagueKey: "esp.1",
		StartTime: now,
		Status:    model.MatchStatus{State: "in", Detail: "50'"},
	}
	mScored := mInitial
	mScored.HomeTeam.Score = "2"
	mScored.Status.Detail = "60'"
	mScored.Events = []model.MatchEvent{
		{
			Type:       "goal",
			PlayerName: "Pedri",
			Clock:      "60'",
			IsScoring:  true,
		},
	}

	currentMatches := []model.Match{mInitial}

	client := &mockClient{
		matchesByLeague: map[string][]model.Match{
			"esp.1": currentMatches,
		},
	}

	notifier := &mockNotifier{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var buf bytes.Buffer
	opts := WatchOptions{
		Options:  Options{Format: FormatPlain, League: "esp.1"},
		Interval: 15 * time.Millisecond,
		Notify:   true,
		Notifier: notifier,
	}

	done := make(chan error, 1)
	go func() {
		done <- Watch(ctx, client, nil, opts, &buf)
	}()

	// Wait for baseline to be set on first tick
	time.Sleep(30 * time.Millisecond)
	if notifier.count() != 0 {
		t.Fatalf("expected 0 notifications on first tick, got %d", notifier.count())
	}

	// Update score in client
	client.SetLeagueMatches("esp.1", []model.Match{mScored})

	// Wait for tick to detect goal
	time.Sleep(45 * time.Millisecond)
	cancel()
	<-done

	if notifier.count() != 1 {
		t.Errorf("expected 1 goal notification dispatched, got %d", notifier.count())
	}
}

package bar

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/e12jassir/kickoff-cli/internal/model"
)

type matchScore struct {
	homeScore string
	awayScore string
}

// ScoreChange records a change in score for a match.
type ScoreChange struct {
	Match     model.Match
	PrevHome  string
	PrevAway  string
	NewHome   string
	NewAway   string
	GoalEvent *model.MatchEvent
}

// ScoreTracker keeps state of previous scores to detect goals across polling intervals.
type ScoreTracker struct {
	mu     sync.Mutex
	scores map[string]matchScore
}

// NewScoreTracker initializes a fresh ScoreTracker instance.
func NewScoreTracker() *ScoreTracker {
	return &ScoreTracker{
		scores: make(map[string]matchScore),
	}
}

// SetScore allows explicitly setting the baseline score for a match ID (useful for tests).
func (t *ScoreTracker) SetScore(matchID, homeScore, awayScore string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.scores[matchID] = matchScore{homeScore: homeScore, awayScore: awayScore}
}

// Update inspects the provided matches, establishes baselines for newly discovered fixtures,
// and returns any detected score changes / goal events.
func (t *ScoreTracker) Update(matches []model.Match) []ScoreChange {
	t.mu.Lock()
	defer t.mu.Unlock()

	var changes []ScoreChange
	for _, m := range matches {
		curHome := m.HomeTeam.Score
		if curHome == "" {
			curHome = "0"
		}
		curAway := m.AwayTeam.Score
		if curAway == "" {
			curAway = "0"
		}

		prev, exists := t.scores[m.ID]
		if !exists {
			// First observation of this fixture establishes baseline; do not spam past goals.
			t.scores[m.ID] = matchScore{homeScore: curHome, awayScore: curAway}
			continue
		}

		if curHome != prev.homeScore || curAway != prev.awayScore {
			var latestScoringEvent *model.MatchEvent
			for i := len(m.Events) - 1; i >= 0; i-- {
				ev := m.Events[i]
				if ev.IsScoring || ev.Type == "goal" || ev.Type == "penalty" || ev.Type == "owngoal" {
					evCopy := ev
					latestScoringEvent = &evCopy
					break
				}
			}

			changes = append(changes, ScoreChange{
				Match:     m,
				PrevHome:  prev.homeScore,
				PrevAway:  prev.awayScore,
				NewHome:   curHome,
				NewAway:   curAway,
				GoalEvent: latestScoringEvent,
			})

			t.scores[m.ID] = matchScore{homeScore: curHome, awayScore: curAway}
		}
	}

	return changes
}

// WatchOptions configures execution parameters for Watch.
type WatchOptions struct {
	Options
	Interval time.Duration
	Notify   bool
	Notifier Notifier
}

func flushWriter(w io.Writer) {
	if f, ok := w.(interface{ Flush() error }); ok {
		_ = f.Flush()
	} else if f, ok := w.(interface{ Flush() }); ok {
		f.Flush()
	}
}

// Watch continuously monitors matches, renders status lines to out, and triggers goal alerts.
// It terminates cleanly without error when ctx is canceled.
func Watch(ctx context.Context, client model.Client, resolver model.Resolver, opts WatchOptions, out io.Writer) error {
	if opts.Interval <= 0 {
		opts.Interval = 30 * time.Second
	}
	notifier := opts.Notifier
	if notifier == nil {
		notifier = NewNotifier()
	}

	tracker := NewScoreTracker()

	renderTick := func() {
		if ctx.Err() != nil {
			return
		}
		matches, err := FetchAndFilter(ctx, client, opts.Options)
		if err != nil {
			return
		}
		if opts.Notify {
			changes := tracker.Update(matches)
			for _, ch := range changes {
				_ = notifier.NotifyGoal(ch.Match, ch.NewHome, ch.NewAway, ch.GoalEvent)
			}
		}
		outStr, err := FormatMatches(matches, opts.Options, resolver)
		if err != nil {
			return
		}
		_, _ = fmt.Fprintln(out, outStr)
		flushWriter(out)
	}

	// Immediate first tick
	renderTick()

	ticker := time.NewTicker(opts.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			renderTick()
		}
	}
}

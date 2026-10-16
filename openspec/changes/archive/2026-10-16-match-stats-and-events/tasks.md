# Tasks: Match Details, Goal Scorers, Cards, and Team Statistics

- [x] Task 1: Update domain model (`internal/model/types.go`) with `MatchEvent`, `MatchStats`, updated `Match`, and `FetchMatchStats` interface method <!-- id: task-1 -->
- [x] Task 2: Implement match event parsing in `FetchMatches` and match statistics in `FetchMatchStats` (`internal/espn/client.go`) <!-- id: task-2 -->
- [x] Task 3: Implement stats loading message, command, and async state transition in TUI (`internal/ui/update.go`) <!-- id: task-3 -->
- [x] Task 4: Render match events (goals/cards in dual columns) and team statistics in scoreboard modal (`internal/ui/view.go`) <!-- id: task-4 -->
- [x] Task 5: Add tests for ESPN client stats/events and UI modal rendering (`internal/espn/client_test.go`, `internal/ui/ui_test.go`) <!-- id: task-5 -->
- [x] Task 6: Verification and build (`go test -v ./...`, `make build`, `make install`) <!-- id: task-6 -->

# Tasks: Premium Sofascore-Style Interactive Mini-Tabs

- [x] Task 1: Domain models update in `internal/model/types.go` (`Player`, `TeamLineup`, `MatchLineups`, additional `MatchStats` fields, `Match.Lineups`, `Client.FetchMatchDetails`) <!-- id: task-1 -->
- [x] Task 2: ESPN client parser and cache update in `internal/espn/client.go` (`rawRoster`, `rawRosterItem`, `FetchMatchDetails` extracting stats and lineups with defensive fallback) <!-- id: task-2 -->
- [x] Task 3: TUI state & keymaps update in `internal/ui/model.go` and `internal/ui/update.go` (`modalTab`, tab navigation keys `left/right/tab/shift+tab/1/2/3`, cursor gating, stats & lineups loading) <!-- id: task-3 -->
- [x] Task 4: UI styles & layout in `internal/ui/styles.go` and `internal/ui/view.go` (modal width, segmented pill tabs, Tab 0 Resumen with clean channels/empty state, Tab 1 Estadísticas with Unicode progress bar and 6 stat rows, Tab 2 Alineaciones with formations and 11 starters, footer) <!-- id: task-4 -->
- [x] Task 5: Unit and integration tests in `internal/espn/client_test.go` and `internal/ui/ui_test.go` (lineups parsing, stats caching, tab switching 0/1/2, modal views rendering) <!-- id: task-5 -->
- [x] Task 6: Verification, build, and installation (`go test -v ./...`, `make build`, `make install`) <!-- id: task-6 -->

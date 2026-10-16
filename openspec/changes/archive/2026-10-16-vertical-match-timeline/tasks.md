# Tasks: Centered Vertical Match Timeline & Game Info

- [x] Task 1: Domain models update in `internal/model/types.go` (`MatchEvent.ClockValue`, `Match.Referee`, `Match.Attendance`) <!-- id: task-1 -->
- [x] Task 2: ESPN client parser and game info in `internal/espn/client.go` (`rawGameInfo`, `rawOfficial`, `ClockValue`, player name fallbacks, `FetchMatchDetails` referee/attendance extraction) <!-- id: task-2 -->
- [x] Task 3: Centered vertical timeline & bottom game info in `internal/ui/view.go` and `internal/ui/update.go` (chronological sorting, dedicated line per event, spine badges, halftime divider `──┤ E.T. ├──`, referee/attendance display) <!-- id: task-3 -->
- [x] Task 4: Unit and integration tests in `internal/espn/client_test.go` and `internal/ui/ui_test.go` (chronological timeline, referee/attendance, fallbacks) <!-- id: task-4 -->
- [x] Task 5: Verification, build, installation and git commit <!-- id: task-5 -->

# Proposal: Migrate to Bubble Tea

## Intent
Replace legacy Bash, fzf, and Python scripts with a native Go binary powered by Charm's Bubble Tea TUI framework. This eliminates subshell process overhead, brittle parsing pipelines, and terminal stdin contention with background media players.

## Scope
### In Scope
- Go module initialization (Go 1.27) and project layout.
- Domain models: `Match`, `Channel`, `League`, and `Stream`.
- Native ESPN API client with JSON parsing.
- Per-match channel catalog engine with quality constraints (720p/1080p, bitrate bounds).
- External MPV process lifecycle and Unix domain socket IPC controller.
- Interactive Bubble Tea TUI: League tabs, match browser, and channel selector modal.
- Unit tests for ESPN client, catalog, and player controller.

### Out of Scope
- Web or Electron interfaces.
- In-process video decoding (delegated to external hardware-accelerated MPV).
- Custom stream proxying or transcoding.

## Capabilities
### New Capabilities
- `bubbletea-tui`: Interactive terminal user interface using Bubble Tea, Bubbles, and Lip Gloss for state rendering and navigation.
- `espn-client`: Native HTTP client fetching and parsing ESPN scoreboard endpoints.
- `stream-catalog`: Channel mapping and fixture matching engine with quality filtering.
- `player-controller`: MPV execution manager and Unix domain socket IPC telemetry controller.

### Modified Capabilities
- None.

## Approach
Structure the application into clean modular packages (`internal/model`, `internal/espn`, `internal/catalog`, `internal/player`, `internal/ui`). The Bubble Tea model orchestrates asynchronous `tea.Cmd` tasks for network requests and player supervision within a non-blocking event loop.

## Affected Areas
- `bin/kickoff`: Replaced by compiled Go executable.
- `bin/kickoff-preview`: Deprecated in favor of native TUI components.
- `Makefile`: Updated for Go build and test targets (`go build`, `go test`).
- Repository: Introduces `go.mod`, `go.sum`, `cmd/kickoff`, and `internal/`.

## Risks
- **Terminal raw mode contention**: MPV capturing stdin could disrupt TUI state. *Mitigation*: Launch MPV with `--input-terminal=no` and communicate solely via Unix IPC socket.
- **Upstream API shifts**: ESPN schema changes could break parsing. *Mitigation*: Resilient JSON decoding with defensive defaults and mocked tests.

## Rollback Plan
Legacy shell scripts remain preserved in Git history. If regressions occur, revert `Makefile` install targets back to `bin/kickoff`.

## Dependencies
- Go (>= 1.27)
- `github.com/charmbracelet/bubbletea`, `bubbles`, `lipgloss`
- External runtime: `mpv`

## Success Criteria
- Native binary compiles cleanly without external script dependencies.
- Fixture view renders under 200ms without spawning child processes.
- MPV launches seamlessly via IPC without terminal flicker.
- Unit test suite passes for ESPN client, catalog, and player controller.

# Tasks: Migrate to Bubble Tea

## Review Workload Forecast
Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: feature-branch-chain
400-line budget risk: Medium

Delivery tracked on feature branch `feat/migrate-bubbletea` as an approved size-exception (~750 LoC total).

---

### Phase 1: Foundation (go.mod, internal/model/types.go)
- [x] 1.1 Initialize Go 1.27 module (`github.com/e12jassir/kickoff`) and install Charm dependencies (`bubbletea`, `bubbles`, `lipgloss`) in `go.mod`.
- [x] 1.2 Define domain types (`Match`, `Competitor`, `Channel`, `League`, `PlayRequest`) in `internal/model/types.go`.
- [x] 1.3 Define domain error types and interface contracts (`espn.Client`, `catalog.Resolver`, `player.Controller`) in `internal/model/types.go`.

### Phase 2: ESPN Client & Testing (internal/espn/client.go, client_test.go)
- [x] 2.1 Implement native ESPN HTTP client with context support in `internal/espn/client.go`.
- [x] 2.2 Add defensive JSON unmarshaling for scoreboard events, gracefully skipping incomplete payloads.
- [x] 2.3 Implement file-based 300-second TTL cache in `$XDG_CACHE_HOME/kickoff/`.
- [x] 2.4 Add unit tests in `internal/espn/client_test.go` covering valid scoreboards, empty fixtures, and corrupted JSON using `net/http/httptest`.

### Phase 3: Stream Catalog Engine (internal/catalog/catalog.go, catalog_test.go)
- [x] 3.1 Implement channel registry and club regex matching in `internal/catalog/catalog.go`.
- [x] 3.2 Add channel resolution with 720p/1080p profiles and fallback channel selection for unknown leagues.
- [x] 3.3 Implement BoldMSS CDN protocol sanitizer (HTTPS to HTTP) and URL safety validation.
- [x] 3.4 Add table-driven unit tests in `internal/catalog/catalog_test.go` for regex matching and URL sanitization.

### Phase 4: Player Controller & Threat Matrix Tests (internal/player/player.go, player_test.go)
- [x] 4.1 Implement MPV process supervisor in `internal/player/player.go` spawning detached processes with `stdin` bound to `/dev/null` and `--no-terminal`.
- [x] 4.2 Implement secure Unix IPC domain socket allocation in `0700` temporary directories (`os.MkdirTemp`).
- [x] 4.3 Implement IPC client polling for `vo-configured` with a 12-second timeout and process exit detection.
- [x] 4.4 Implement cleanup handlers for unlinking sockets and terminating orphaned MPV instances.
- [x] 4.5 Add unit tests in `internal/player/player_test.go` using mock Unix socket listeners and exit path verification.

### Phase 5: Bubble Tea TUI (internal/ui/model.go, update.go, view.go, styles.go)
- [x] 5.1 Implement Bubble Tea model and state transitions (`Loading`, `Browsing`, `ChannelSelect`, `Error`) in `internal/ui/model.go`.
- [x] 5.2 Implement event loop update logic and keymaps in `internal/ui/update.go` handling async `tea.Cmd` messages.
- [x] 5.3 Implement view rendering in `internal/ui/view.go` for league tabs, match lists, empty states, and channel modal.
- [x] 5.4 Define Lip Gloss terminal styles, borders, and color palettes in `internal/ui/styles.go`.

### Phase 6: CLI Binary & Makefile Integration (cmd/kickoff/main.go, Makefile)
- [x] 6.1 Implement CLI entrypoint in `cmd/kickoff/main.go` wiring dependencies and running `tea.NewProgram`.
- [x] 6.2 Update `Makefile` with Go targets (`build`, `test`, `run`, `clean`) pointing to `bin/kickoff`.
- [x] 6.3 Run full test suite and verify end-to-end HLS playback across leagues.
- [x] 6.4 Deprecate legacy Bash/Python scripts (`bin/kickoff`, `bin/kickoff-preview`).

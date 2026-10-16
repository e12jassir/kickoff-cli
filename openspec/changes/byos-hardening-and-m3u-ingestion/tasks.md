# Tasks: BYOS Hardening, Audit Fixes & Native M3U Ingestion

## Phase 1: Audit Fixes & Codebase Hardening
- [x] Task 1.1: Remove `boldmss.com` vendor check from `SanitizeURL` in `internal/catalog/catalog.go`, and update `catalog_test.go` and `player_test.go` <!-- id: task-1-1 -->
- [x] Task 1.2: Add flexible JSON unmarshaling for `Channel` (`bitrate_mode` and `bitrateMode`) in `internal/model/types.go` <!-- id: task-1-2 -->
- [x] Task 1.3: Support ESPN league codes (`eng.1`, `esp.1`, `col.1`, etc.) as aliases in `loadExternalConfig` (`internal/catalog/catalog.go`) <!-- id: task-1-3 -->
- [x] Task 1.4: Purge `initChampionsRules` and hardcoded team matchings in `internal/catalog/catalog.go` and update related unit tests <!-- id: task-1-4 -->
- [x] Task 1.5: Fix `setup-channels` target in `Makefile` to copy from `channels.json.example` <!-- id: task-1-5 -->
- [x] Task 1.6: Verify tests pass with `go test -v ./...` <!-- id: task-1-6 -->

## Phase 2: Native M3U / M3U8 Playlist Ingestion
- [x] Task 2.1: Implement M3U parser in `internal/catalog/m3u.go` extracting `#EXTINF` tags (`tvg-name`, `tvg-id`, channel title) and stream URLs <!-- id: task-2-1 -->
- [x] Task 2.2: Implement dynamic channel matching using `tvguide.Normalize` to link EPG broadcasts to M3U channels <!-- id: task-2-2 -->
- [x] Task 2.3: Support `playlist_url` and `playlist_path` in `ExternalConfig` (`internal/catalog/catalog.go`) <!-- id: task-2-3 -->
- [x] Task 2.4: Write unit tests for M3U parsing and broadcaster fuzzy matching (`internal/catalog/m3u_test.go`) <!-- id: task-2-4 -->

## Phase 3: CLI Config & Playlist Importer (`kickoff config import`)
- [x] Task 3.1: Add `config import <url|path>` subcommand in `cmd/kickoff/main.go` <!-- id: task-3-1 -->
- [x] Task 3.2: Implement download, disclaimer verification, schema validation, and automatic backup (`channels.json.bak`) <!-- id: task-3-2 -->
- [x] Task 3.3: Verify end-to-end import flow and update documentation in `docs/configuration.md` <!-- id: task-3-3 -->


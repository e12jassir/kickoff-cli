# Change: byos-hardening-and-m3u-ingestion

## Purpose
Apply legal and technical audit corrections (purge IPTV vendor fingerprints, resolve JSON tag & league code mapping inconsistencies, remove static Champions rules, fix Makefile setup), and implement the native M3U playlist parser with dynamic nomitative EPG matching and CLI configuration import.

## Requirements

### Requirement: Purge IPTV Vendor Fingerprints
The catalog and URL sanitization logic MUST be 100% agnostic and MUST NOT contain hardcoded third-party IPTV vendor domains (e.g. `boldmss.com`).

#### Scenario: Generic HTTP / HTTPS validation
- **GIVEN** a stream URL with `http://` or `https://`
- **WHEN** `SanitizeURL` validates the endpoint
- **THEN** it accepts standard HTTP/HTTPS schemes without vendor-specific host rewrites.

### Requirement: Unified Configuration Tags and League Keys
The configuration loader MUST seamlessly accept both snake_case and camelCase field representations, and both human-friendly keys (`premier`, `colombia`) and ESPN league codes (`eng.1`, `col.1`).

#### Scenario: Deserializing Channel BitrateMode
- **GIVEN** a JSON channel configuration containing `"bitrate_mode": "max"` or `"bitrateMode": "max"`
- **WHEN** unmarshaling the channel
- **THEN** `Channel.BitrateMode` is correctly populated with `"max"`.

#### Scenario: Resolving League Overrides by ESPN Code or Key
- **GIVEN** an external configuration defining `"eng.1"` or `"premier"`
- **WHEN** loading league overrides
- **THEN** matches for the Premier League correctly resolve to the specified channels.

### Requirement: Dynamic TV Guide and EPG Broadcaster Resolution
The catalog MUST NOT rely on static fixture pairings. It MUST dynamically match broadcast signals from ESPN metadata (`match.Broadcasts`) and TV Guide crawlers (`tvguide.Service`) against user-provided streams.

#### Scenario: Dynamic fixture channel resolution
- **GIVEN** any match fixture with broadcaster names in `match.Broadcasts`
- **WHEN** `GetChannels` is called
- **THEN** candidate channels are resolved using normalized broadcaster comparisons without static team rules.

### Requirement: Native M3U / M3U8 Playlist Parser
The catalog MUST support loading user M3U/M3U8 playlists from local file paths or remote URLs, extracting `#EXTINF` channel attributes (`tvg-name`, `tvg-id`, channel title), and dynamically linking channels via `tvguide.Normalize`.

#### Scenario: Parsing M3U playlist and fuzzy matching broadcaster
- **GIVEN** an M3U playlist containing `#EXTINF:-1 tvg-name="ESPN" ...`
- **WHEN** the playlist is parsed
- **THEN** channels are normalized and registered into available user streams.

### Requirement: CLI Remote Config and Playlist Importer (`kickoff config import`)
Kickoff CLI MUST provide a command to import a remote `channels.json` or `.m3u` playlist from a URL or local file into the user's config directory, displaying a legal disclaimer and creating an automatic backup.

#### Scenario: Importing remote configuration
- **GIVEN** a valid URL to a JSON configuration or M3U playlist
- **WHEN** `kickoff config import <url>` is executed
- **THEN** the existing `channels.json` is backed up to `channels.json.bak`, the new configuration is saved, and a success confirmation is displayed.

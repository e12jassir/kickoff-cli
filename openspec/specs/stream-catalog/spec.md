# Capability: stream-catalog

## Purpose
Resolves verified streaming signals, applies team broadcast rules, and configures stream quality.

## Requirements

### Requirement: Channel Resolution and Protocol Sanitization
The catalog MUST resolve streaming channels by league and team, assign bitrate profiles, and sanitize stream endpoints.

#### Scenario: Resolving broadcast channels (Happy Path)
- **GIVEN** a fixture with league and team information
- **WHEN** GetChannels is invoked
- **THEN** the catalog MUST return verified channels prioritizing club-specific signals with 720p and 1080p profiles.

#### Scenario: Unknown league fallback (Edge Case)
- **GIVEN** an unrecognized league key
- **WHEN** resolving channels
- **THEN** the catalog MUST return default fallback channels.

#### Scenario: Incompatible TLS sanitization (Error)
- **GIVEN** a stream URL on a TLS-incompatible host
- **WHEN** resolving stream parameters
- **THEN** the catalog MUST rewrite the URL to plain HTTP.

# Capability: stream-catalog

## Purpose
Resolves verified, playable broadcast channels for a fixture, applies user
configuration, and reports honestly when no source can be verified. A missing
channel is preferred over a wrong one.

## Requirements

### Requirement: Evidence-Only Channel Resolution
`GetChannels` MUST resolve channels exclusively from verified evidence. It MUST
NOT fabricate a channel from a league identity, a country, a match slot or
de-duplication counter, or any other heuristic. When no evidence resolves to a
playable channel, `GetChannels` MUST return an empty result.

#### Scenario: Unknown league (Edge Case)
- **GIVEN** a match whose league is not recognized and that carries no broadcast metadata and no TV-guide entry
- **WHEN** `GetChannels` is invoked
- **THEN** the catalog MUST return no channels and MUST NOT return default fallback channels.

#### Scenario: Simultaneous matches without evidence (Edge Case)
- **GIVEN** two matches that kick off at the same time and carry only a slot/de-duplication index
- **WHEN** `GetChannels` is invoked
- **THEN** the catalog MUST return no channels, because a slot index is not broadcast evidence.

#### Scenario: Colombia or Argentina match without evidence (Edge Case)
- **GIVEN** a match in a Colombian or Argentinian league with no broadcast metadata and no TV-guide entry
- **WHEN** `GetChannels` is invoked
- **THEN** the catalog MUST return no channels and MUST NOT emit a country-wide default line-up.

### Requirement: Evidence Precedence
The catalog MUST evaluate evidence in the following order and MUST stop at the
first tier that yields channels:

1. user `custom_rules`;
2. user `league_channels` overrides;
3. ESPN per-match broadcast metadata;
4. the Gol Caracol daily parrilla (T1);
5. TV-guide corroboration.

#### Scenario: User override wins (Happy Path)
- **GIVEN** a `custom_rules` entry or a `league_channels` override that matches the fixture
- **WHEN** `GetChannels` is invoked
- **THEN** the catalog MUST return the user channels and MUST NOT consult later tiers.

#### Scenario: Broadcast metadata wins over the TV guide (Happy Path)
- **GIVEN** a match with ESPN broadcast metadata that maps to a configured stream key, and a TV-guide entry that also matches
- **WHEN** `GetChannels` is invoked
- **THEN** the catalog MUST return the broadcast-resolved channel and MUST NOT return the TV-guide channel.

#### Scenario: Named broadcaster without a configured stream (Edge Case)
- **GIVEN** ESPN broadcast metadata naming a broadcaster whose stream key the user has not configured, and no US-only broadcaster in the metadata
- **WHEN** `GetChannels` is invoked
- **THEN** the catalog MUST return no channels rather than guess a substitute.

#### Scenario: US broadcaster without a configured stream (Edge Case)
- **GIVEN** ESPN broadcast metadata naming a US broadcaster (CBS Sports, Paramount, Golazo, Peacock, NBC, Telemundo, ESPN+, VIX, Fubo, TUDN)
- **WHEN** no configured US stream resolves
- **THEN** the catalog MAY consult the TV guide, which is independent evidence.

### Requirement: Gol Caracol Daily Parrilla
The catalog MUST consult the Gol Caracol daily parrilla — the authoritative
per-match Colombian channel listing, fetched from a deterministic per-date URL —
as an evidence tier between ESPN broadcast metadata and the TV guide. Parrilla
entries MUST be matched with the same both-teams rule as the TV guide, so a
single team never matches. When several entries match, the entry whose published
start time overlaps the fixture window MUST win; when none overlaps and the
match is unambiguous, the single matching entry MAY be used. Channel names MUST
be canonicalized to stream keys, and only channels with a configured stream URL
MUST be emitted. An entry with no published channels (still "por definir") MUST
yield nothing. A fetch failure MUST degrade gracefully and MUST allow lower
tiers to run. The snapshot MUST be cached per local calendar day and fetched at
most once per day.

#### Scenario: Parrilla resolves a channel the TV guide cannot (Happy Path)
- **GIVEN** a DSports fixture that carries no ESPN broadcast metadata and is absent from the TV guide
- **WHEN** the Gol Caracol parrilla lists DSports for the fixture and the user has configured the `dsports` key
- **THEN** the catalog MUST resolve the configured DSports stream from the parrilla.

#### Scenario: Channels not yet defined (Edge Case)
- **GIVEN** a parrilla entry whose channel cell is empty ("por definir")
- **WHEN** the catalog evaluates the entry
- **THEN** the parrilla tier MUST yield nothing and MUST NOT guess a channel.

#### Scenario: Parrilla unavailable (Edge Case)
- **GIVEN** the Gol Caracol schedule cannot be fetched for the fixture date
- **WHEN** `GetChannels` is invoked
- **THEN** the parrilla tier MUST yield nothing and resolution MUST continue with the TV guide.

### Requirement: Playability Gate
The catalog MUST emit only channels whose stream URL is a valid http(s) URL
without embedded credentials. It MUST drop any candidate that fails
sanitization, MUST de-duplicate identical URL/resolution pairs, MUST cap the
result at eight channels, MUST order healthy or unprobed channels ahead of
unverified ones, and MUST mark the first returned channel as recommended.

#### Scenario: Unplayable-only match (Edge Case)
- **GIVEN** a match aired only on a channel key the user has not configured
- **WHEN** `GetChannels` is invoked
- **THEN** the catalog MUST return no channels.

#### Scenario: Playable-source check (Happy Path)
- **GIVEN** a resolved channel with a valid stream URL
- **WHEN** `HasPlayableSource` is invoked
- **THEN** it MUST report true; it MUST report false when no channel resolves.

### Requirement: DRM Streaming Platforms Are Not Playable
A channel classified as an over-the-top streaming service (for example Disney+
Premium, DGO, Amazon Prime Video, Paramount+, DAZN, Apple TV, VIX, Fanatiz,
OneFootball) MUST NOT be emitted as a playable channel unless the user maps a
stream key for it. kickoff MUST NOT add DRM/CDM support.

#### Scenario: Streaming-only match (Edge Case)
- **GIVEN** a match aired only on a DRM-protected streaming platform with no user-configured stream
- **WHEN** `GetChannels` is invoked
- **THEN** the catalog MUST return no playable channels and the platform MUST NOT receive a play action.

### Requirement: Broadcast Resolver Acceptance
The TV-guide resolver MUST require both teams of the fixture to be identified in
a guide entry, or the full fixture title to match. It MUST NOT accept a
single-team match. Team comparison MUST be exact at token level, so a prefix
such as `bayer` MUST NOT match `bayern`. Timing MUST gate eligibility: a guide
programme that does not overlap the fixture window MUST be discarded, and timing
MUST NOT add or weight a score. When two distinct channels tie for the top
score, the resolver MUST decline instead of picking one.

#### Scenario: Single-team entry (Edge Case)
- **GIVEN** a guide entry that names only one of the two clubs
- **WHEN** the resolver evaluates the fixture
- **THEN** the entry MUST be rejected and MUST NOT resolve a channel.

#### Scenario: Prefix collision (Edge Case)
- **GIVEN** a guide entry for a club whose name shares a prefix with, but is not equal to, a fixture club token
- **WHEN** the resolver evaluates the fixture
- **THEN** the entry MUST NOT resolve a channel.

#### Scenario: Non-overlapping schedule (Edge Case)
- **GIVEN** a guide entry whose programme window does not overlap the fixture window
- **WHEN** the resolver evaluates the fixture
- **THEN** the entry MUST be discarded.

#### Scenario: Ambiguous tie (Error)
- **GIVEN** two distinct channels with the same top match score
- **WHEN** the resolver evaluates the fixture
- **THEN** the resolver MUST decline and return no channel.

### Requirement: Canonical Stream Keys
The catalog MUST resolve these canonical stream keys: `espn1`–`espn7`,
`dsports`, `dsports2`, `dsports4`, `dsports_plus`, `win`, `win_plus`,
`win_sports_mas`, `gol_caracol`, `caracol`, `caracol_hd2`, `rcn`, `rcn_hd2`,
`claro_sports`, `tyc`, `realmadrid`, `teledeporte`. The retired keys `fox1`,
`fox2`, `fox3`, and `espn_extra` MUST NOT resolve to an endpoint.

#### Scenario: DSports broadcast (Happy Path)
- **GIVEN** a match whose broadcast metadata names DSports
- **WHEN** the user has configured the `dsports` key
- **THEN** the catalog MUST resolve the configured DSports stream.

#### Scenario: Retired Fox key (Edge Case)
- **GIVEN** a match whose broadcast metadata names Fox Sports
- **WHEN** `GetChannels` is invoked
- **THEN** the catalog MUST NOT resolve a `fox*` endpoint and MUST return no channels for that broadcaster.

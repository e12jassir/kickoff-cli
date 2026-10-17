# Capability: espn-client

## Purpose
Fetches, parses, and caches daily soccer fixtures from the ESPN API, preserving
per-match broadcast metadata for the channel catalog.

## Requirements

### Requirement: Fixture Retrieval and Defensive Parsing
The client MUST query ESPN scoreboard endpoints with `lang=en` and MUST NOT send
a `region` parameter: the `region` combined with `lang=es` returns empty
`broadcasts` and `geoBroadcasts`, which disables the evidence chain. The client
MUST cache responses (30 seconds by default, 5 seconds while a match is live,
and 6 hours for past fixtures) and parse events into Match models.

#### Scenario: Scoreboard retrieval and caching (Happy Path)
- **GIVEN** an expired or missing cache for a league
- **WHEN** FetchMatches is called
- **THEN** the client MUST query ESPN with `lang=en` and no `region`, filter by today's date, and update the cache.

#### Scenario: Broadcast metadata preservation (Happy Path)
- **GIVEN** an ESPN scoreboard response that populates `broadcasts` and `geoBroadcasts`
- **WHEN** the response is parsed
- **THEN** the client MUST retain the broadcast names for the channel catalog.

#### Scenario: Incomplete event payload (Edge Case)
- **GIVEN** an event payload missing competitor fields or timestamps
- **WHEN** unmarshaling fixtures
- **THEN** the client MUST skip invalid events and process valid fixtures.

#### Scenario: Upstream endpoint failure (Error)
- **GIVEN** ESPN returns an HTTP error or malformed JSON
- **WHEN** the request fails
- **THEN** the client MUST return an error and invalidate stale cache.

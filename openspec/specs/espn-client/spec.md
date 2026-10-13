# Capability: espn-client

## Purpose
Fetches, parses, and caches daily soccer fixtures from the ESPN API.

## Requirements

### Requirement: Fixture Retrieval and Defensive Parsing
The client MUST query ESPN scoreboard endpoints, cache responses for 300 seconds, and parse events into Match models.

#### Scenario: Scoreboard retrieval and caching (Happy Path)
- **GIVEN** an expired or missing cache for a league
- **WHEN** FetchMatches is called
- **THEN** the client MUST query ESPN, filter by today's date, and update the cache.

#### Scenario: Incomplete event payload (Edge Case)
- **GIVEN** an event payload missing competitor fields or timestamps
- **WHEN** unmarshaling fixtures
- **THEN** the client MUST skip invalid events and process valid fixtures.

#### Scenario: Upstream endpoint failure (Error)
- **GIVEN** ESPN returns an HTTP error or malformed JSON
- **WHEN** the request fails
- **THEN** the client MUST return an error and invalidate stale cache.

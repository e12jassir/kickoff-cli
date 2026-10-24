# Capability: bubbletea-tui

## Purpose
Provides an interactive Bubble Tea terminal UI for league browsing, channel selection, and async player execution.

## Requirements

### Requirement: Interactive Navigation and Playback Trigger
The TUI MUST render league tabs, match lists, and channel modals while orchestrating asynchronous tasks.

#### Scenario: Browsing fixtures and launching stream (Happy Path)
- **GIVEN** an active league tab with scheduled matches
- **WHEN** the user selects a match and confirms a broadcast channel
- **THEN** the TUI MUST dispatch an asynchronous player launch command without blocking the UI.

#### Scenario: League with zero matches (Edge Case)
- **GIVEN** a league has no fixtures scheduled today
- **WHEN** the tab is selected
- **THEN** the TUI MUST display an empty notice without blank list items.

#### Scenario: Async operation failure (Error)
- **GIVEN** a fixture query or player launch fails
- **WHEN** the error event arrives
- **THEN** the TUI MUST render an inline error and retain user control.

### Requirement: Playability-Gated Visibility
The TUI MUST hide a fixture that has no playable channel for the user, rather
than listing it with an empty or fabricated channel row. When the user has
configured no stream source at all, the TUI MUST show an explicit configuration
notice instead of an empty fixture list.

#### Scenario: Fixture without a playable source (Edge Case)
- **GIVEN** a fixture aired only on channels the user cannot play
- **WHEN** the fixture list is rendered
- **THEN** the TUI MUST NOT show the fixture.

#### Scenario: No stream source configured (Edge Case)
- **GIVEN** the user has configured no stream source
- **WHEN** a league tab is opened
- **THEN** the TUI MUST show a notice pointing to the channels configuration instead of an empty list.

### Requirement: Internationalized User Interface
The TUI MUST render user interface chrome, status messages, and navigation hints according to the resolved locale, supporting English and Spanish with strict fallback to Spanish.

#### Scenario: English locale rendering
- **GIVEN** `KICKOFF_LANG=en` or the environment locale is configured with an English locale (such as `LANG=en_US.UTF-8`)
- **WHEN** the TUI or CLI starts up
- **THEN** all application chrome, navigation keys, tabs, status messages, and help texts MUST render in English.

#### Scenario: Spanish locale fallback
- **GIVEN** an unsupported language code, an invalid language configuration, or no language environment variables set
- **WHEN** the language is resolved
- **THEN** the application MUST fall back to Spanish as the default interface language without terminating or crashing.

#### Scenario: Setup language persistence
- **GIVEN** the user interacts with the language toggle option in `kickoff setup`
- **WHEN** the user switches between Spanish and English
- **THEN** the setup wizard MUST update its active catalog in real time and persist the selected language to `~/.config/kickoff/config.json`.


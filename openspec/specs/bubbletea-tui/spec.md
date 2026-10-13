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

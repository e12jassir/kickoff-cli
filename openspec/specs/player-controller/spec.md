# Capability: player-controller

## Purpose
Supervises external MPV player processes and verifies playback via Unix IPC telemetry.

## Requirements

### Requirement: Process Lifecycle and IPC Synchronization
The controller MUST spawn MPV detached from terminal stdin and confirm video output configuration over Unix IPC.

#### Scenario: Detached playback launch and confirmation (Happy Path)
- **GIVEN** a stream URL and bitrate mode
- **WHEN** PlayStream is called
- **THEN** the controller MUST spawn MPV detached from stdin and verify video initialization via IPC before completing.

#### Scenario: Premature player exit (Edge Case)
- **GIVEN** MPV exits prematurely during startup
- **WHEN** process monitoring detects exit
- **THEN** the controller MUST abort polling, remove the socket file, and report exit.

#### Scenario: IPC telemetry timeout (Error)
- **GIVEN** MPV fails to configure video output within 12 seconds
- **WHEN** the timeout expires
- **THEN** the controller MUST terminate the process, remove the socket file, and return an error.

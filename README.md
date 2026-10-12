# kickoff-cli

A minimalist, high-performance terminal soccer client. Browse daily match fixtures organized by top leagues worldwide (La Liga, Premier League, Serie A, Bundesliga, Ligue 1, UEFA Champions League, Copa Libertadores, Liga Argentina, Liga BetPlay, Brasileirão), preview badges with GPU acceleration in Kitty, and launch live broadcasts directly in `mpv` without opening a browser or enduring ads.

---

## Features

- **League Hierarchy**: Organized cleanly by the Top 5 European leagues + Continental cups + Latin American leagues.
- **Real-Time Daily Fixtures**: Live scores and scheduled matches queried dynamically. No matches scheduled? Clean notice with zero dummy entries.
- **Direct Broadcast Stream Selector**: Instant selection between verified sports signals (ESPN, Fox Sports, TyC Sports, Real Madrid TV, Teledeporte).
- **GPU-Accelerated Visual Previews**: Match thumbnails and team badges rendered via Kitty `icat` memory transfer protocol (with Chafa ANSI fallback).
- **IPC Telemetry Synchronization**: The loading spinner hides terminal cursor and suppresses input echo until the video frame renders on screen.

---

## Keybindings

| Keybinding | Action |
|---|---|
| `Enter` | Open broadcast selector / Launch in fullscreen `mpv` |
| `Ctrl + y` | Copy match details to clipboard (`wl-copy`) |
| `Ctrl + r` | Force refresh fixture cache |

---

## Installation

```bash
cd ~/Proyectos/futbol-cli
make install
```

Make sure `~/.local/bin` is in your `$PATH`.

---

## Usage

Simply run:

```bash
kickoff
```

---

## License

MIT © [Esteban David Marrugo Jassir](https://github.com/e12jassir)

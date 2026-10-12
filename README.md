# futbol-cli

A minimalist, high-performance terminal live sports & football streaming client. Browse open IPTV sports broadcasts worldwide, preview official channel logos with GPU acceleration in Kitty, and stream instantly in `mpv` without opening a browser or enduring ads.

---

## Features

- **Interactive Omnibar Search**: Instant fuzzy-searching (`fzf`) across hundreds of sports channels and live feeds.
- **Logos & Metadata Previews**: Pixel-perfect channel logos rendered in real-time via Kitty `icat` memory transfer mode (with Chafa ANSI fallback).
- **Direct HLS/M3U8 Streaming**: Sends direct streams to `mpv` with hardware decoding (`vaapi`/GPU) — zero ads, zero browser overhead.
- **IPC Telemetry Synchronization**: The loading spinner remains active via IPC socket until the video frame is genuinely rendered on your display.
- **Favorites System**: Bookmark channels with `Ctrl+f` and access your personal shortlist anytime.

---

## Keybindings

| Keybinding | Action |
|---|---|
| `Enter` | Launch live broadcast in fullscreen `mpv` |
| `Ctrl + f` | Toggle channel in favorites (`~/.local/state/futbol/favorites.tsv`) |
| `Ctrl + y` | Copy raw HLS stream URL to clipboard |
| `Ctrl + r` | Force refresh sports channel playlist cache |

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
futbol
```

Or search directly from your terminal arguments:

```bash
futbol "sport"
```

---

## License

MIT © [Esteban David Marrugo Jassir](https://github.com/e12jassir)

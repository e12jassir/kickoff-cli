<div align="center">

# ⚽ kickoff

### An elegant, blazing-fast football TUI & live broadcast launcher for your terminal.

[![Release](https://img.shields.io/github/v/release/e12jassir/kickoff-cli?style=flat-square&color=2bbc8a)](https://github.com/e12jassir/kickoff-cli/releases)
[![AUR](https://img.shields.io/badge/AUR-coming%20soon-blue?style=flat-square&logo=arch-linux)](https://github.com/e12jassir/kickoff-cli/tree/main/aur)
[![License](https://img.shields.io/badge/license-MIT-blue?style=flat-square)](LICENSE)

<br/>

[Features](#-features) • [Screenshots](#-screenshots) • [Installation](#-installation) • [Keybindings](#-keybindings) • [Competitions](#-supported-competitions)

<br/>

</div>

---

## ⚡ Overview

**kickoff** brings the excitement of the stadium right to your terminal. Built with Charm's **Bubble Tea** and **Lip Gloss**, it delivers live scores, real-time match events, in-depth stats, and tactical lineups in a distraction-free keyboard-driven interface.

When it's match time, launch verified high-definition broadcast streams in hardware-accelerated **mpv** with a single keystroke. No browser tabs. No popups. No delays.

---

## 📸 Screenshots

<div align="center">

| 🏆 Competition Hub | 📅 Daily Fixtures & Live Pulse |
| :---: | :---: |
| ![Leagues](assets/screenshots/leagues-menu.png) | ![Fixtures](assets/screenshots/match-fixtures.png) |

| ⏱️ Centered Event Timeline | 📊 Team Stats & Possession Bar |
| :---: | :---: |
| ![Timeline](assets/screenshots/match-modal-timeline.png) | ![Stats](assets/screenshots/match-stats.png) |

| 📋 Tactical Lineups & Starting XI |
| :---: |
| ![Lineups](assets/screenshots/match-lineups.png) |

</div>

---

## ✨ Features

- **🎮 Keyboard-First Interface**: Fluid navigation with `hjkl` or arrow keys, modal overlays, and instant tab jumps.
- **🔴 Live Event Telemetry**: Dynamic match clock, live score indicators, and a centered vertical timeline tracking goals, penalties, cards, and substitutions.
- **📊 In-Depth Analytics**: Live possession bars, shot distribution, passing accuracy, corner kicks, and discipline metrics.
- **🛡️ Starting XI & Formations**: Full tactical rosters, squad numbers, and substitute benches.
- **📺 Zero-Ad Stream Launcher**: Automatically maps fixtures to verified broadcast signals and launches them smoothly in `mpv`.
- **📡 Autonomous TV Guide Crawler**: Dynamically scrapes live sports EPG schedules daily to map matches to their verified broadcast channels (ESPN 1-7, Win Sports, Caracol, RCN) without static rules.
- **⚡ Background Health Prober**: Probes live playlists asynchronously in the background with zero UI blocking.
- **🚀 Ultra-Lightweight & Efficient**: Native compiled Go binary consuming under ~28 MB RAM with sub-20ms cold startup (vs 1.5 GB+ in an ad-ridden browser).

---

## 🚀 Installation

### 🐧 Linux (Universal)

Install with a single command (works on Arch, Ubuntu, Debian, Fedora, Mint, openSUSE):
```bash
curl -fsSL https://raw.githubusercontent.com/e12jassir/kickoff-cli/main/install.sh | bash
```
*(Or build from source with `git clone https://github.com/e12jassir/kickoff-cli.git && cd kickoff-cli && make install`)*

---

### 🪟 Windows (10 / 11)

Download **`kickoff.exe`** (or extract **`kickoff_0.1.0_windows_amd64.zip`**) directly from the [Releases](https://github.com/e12jassir/kickoff-cli/releases) page, then run from PowerShell or Windows Terminal:
```powershell
.\kickoff.exe
```

---

### 🍏 macOS

```bash
curl -fsSL https://raw.githubusercontent.com/e12jassir/kickoff-cli/main/install.sh | bash
```

---

> [!IMPORTANT]
> **Runtime Dependency:** [`mpv`](https://mpv.io/installation/) must be installed on your system to play live broadcasts:
> - **Linux:** `sudo pacman -S mpv` (Arch) / `sudo apt install mpv` (Debian/Ubuntu) / `sudo dnf install mpv` (Fedora)
> - **Windows:** `winget install mpv.net` *(or `choco install mpv`)*
> - **macOS:** `brew install mpv`

---

### 🔄 Updating & Diagnostics

Once installed, **kickoff** can update itself and diagnose your system directly from the terminal:

```bash
# Check and update to the latest release automatically
kickoff update

# Diagnose dependencies (mpv, emoji fonts, configs, cache)
kickoff setup
```

You can also press **`u`** directly in the kickoff main menu whenever an update banner appears!

---

## ⌨️ Keybindings

| Key | Action |
| :--- | :--- |
| `↑` / `k` · `↓` / `j` | Move selection |
| `Enter` | Open match modal / Launch selected channel |
| `1` / `2` / `3` | Switch modal tabs (Timeline / Stats / Lineups) |
| `←` / `h` · `→` / `l` | Previous / Next tab |
| `u` | Self-update kickoff (when an update is available) |
| `r` | Refresh live scores & fixtures |
| `Esc` / `Backspace` | Back to menu / Close modal |
| `q` / `Ctrl+C` | Quit |

---

## 🏆 Supported Competitions

- **International**: UEFA Champions League, Copa Libertadores
- **Europe**: Premier League, La Liga, Serie A, Bundesliga, Ligue 1
- **South America**: Liga BetPlay (Colombia), Liga Profesional (Argentina), Brasileirão (Brazil)

---

## 📄 License

MIT © [Esteban David Marrugo Jassir](https://github.com/e12jassir)

Kickoff is an open-source terminal scoreboard and media client. In accordance with legal safe harbor and open source best practices, **Kickoff does not host, curate, or distribute copyrighted video feeds**.

Instead, Kickoff provides a **Bring Your Own Streams (BYOS)** architecture (similar to Stremio or Kodi). You can map your lawful IPTV streams, cable logins, or personal feeds to canonical broadcast channels using `channels.json`.

---

## Quick Setup & Remote Import

Kickoff provides a built-in importer for both JSON configurations and M3U/M3U8 playlists:

```bash
# Import from a remote URL (JSON configuration or M3U playlist)
kickoff config import https://example.com/playlist.m3u

# Import from a local file
kickoff config import ~/my-streams/channels.json

# Direct shortcut
kickoff import https://example.com/channels.json
```

When importing:
- Existing configurations are automatically backed up to `channels.json.bak`.
- When an M3U playlist is imported, Kickoff saves it to `~/.config/kickoff/playlist.m3u` and automatically registers `playlist_path` in `channels.json`.
- A legal disclaimer is displayed reminding users of lawful usage.

---

## Configuration File

Kickoff reads configuration from:

* **Linux & macOS**: `~/.config/kickoff/channels.json`
* **Windows**: `%APPDATA%\kickoff\channels.json` (or `$XDG_CONFIG_HOME/kickoff/channels.json`)

You can initialize a starter template:
```bash
kickoff init
```
Or copy the example file:
```bash
cp channels.json.example ~/.config/kickoff/channels.json
```

---

## Schema (`channels.json`)

```json
{
  "playlist_path": "/home/user/.config/kickoff/playlist.m3u",
  "streams": {
    "espn1": "http://your-iptv-provider.example/live/espn1.m3u8",
    "espn2": "http://your-iptv-provider.example/live/espn2.m3u8",
    "win_plus": "http://your-iptv-provider.example/live/winplus.m3u8",
    "caracol": "http://your-iptv-provider.example/live/caracol.m3u8"
  },
  "league_channels": {
    "eng.1": [
      {
        "name": "ESPN (Custom Feed)",
        "url": "https://example.com/live/espn.m3u8",
        "bitrate_mode": "max"
      }
    ],
    "esp.1": [
      {
        "name": "LaLiga TV",
        "url": "https://example.com/live/laligatv.m3u8"
      }
    ]
  },
  "custom_rules": [
    {
      "league": "esp.1",
      "pattern": "Real Madrid.*Barcelona",
      "channels": [
        {
          "name": "El Clásico Special 4K",
          "url": "https://example.com/live/clasico.m3u8",
          "bitrate_mode": "max"
        }
      ]
    }
  ]
}
```

### Fields

| Field | Type | Description |
| :--- | :--- | :--- |
| `playlist_path` | `string` *(optional)* | Local filesystem path to an M3U/M3U8 playlist file (supports `~` expansion). |
| `playlist_url` | `string` *(optional)* | Remote URL to an M3U/M3U8 playlist. |
| `streams` | `map[string]string` | Maps a canonical channel key to your own IPTV/HLS URL. Recognized keys: `espn1`–`espn7`, `dsports`, `dsports2`, `dsports4`, `dsports_plus`, `win`, `win_plus`, `win_sports_mas`, `gol_caracol`, `caracol`, `caracol_hd2`, `rcn`, `rcn_hd2`, `claro_sports`, `tyc`, `realmadrid`, `teledeporte`. The retired keys `fox1`–`fox3` and `espn_extra` are no longer recognized. |
| `league_channels` | `map[string][]Channel` | Overrides default channel listings for a specific league slug (e.g. `eng.1`, `esp.1`, `uefa.champions`, `col.1`). |
| `custom_rules` | `[]Rule` | Matches a regular expression against the match title. If matched, these channels are prioritized first. |
| `name` | `string` | Display label shown in the stream launcher selector. |
| `url` | `string` | Stream URL (HTTP/HTTPS `.m3u8` playlist). |
| `bitrate_mode` | `string` *(optional)* | HLS bitrate selection: `"min"`, `"max"`, or a bitrate in bps (e.g. `"4500000"`). Defaults to `"min"`. |

---

## Broadcast Sources and Resolution

kickoff resolves channels from verified evidence only, in this order:

1. Your `custom_rules` and `league_channels` overrides.
2. ESPN per-match broadcast metadata for the fixture.
3. The Gol Caracol daily parrilla: the authoritative per-match Colombian channel
   listing, fetched from a deterministic per-date URL and matched on both teams.
4. Corroboration from the Colombian TV guide.

A broadcaster named by ESPN that has no configured stream and is not a US-only
broadcaster ends resolution with no channels. When no source yields a playable
channel, the match is reported with no channels instead of a guessed one.

The Gol Caracol parrilla covers ESPN 1–7, DSports, DSports 2, DSports 4,
DSports+, Win Sports and the streaming platforms. It is the only tier that can
resolve channels absent from both ESPN metadata and the TV guide (for example
DSports, which the TV guide does not list). Entries still marked "por definir"
resolve to nothing rather than a guess.

A channel is playable only when its stream URL resolves, so a broadcast whose
key you have not mapped is never offered. A fixture with no playable channel is
hidden from the match list; if no stream source is configured at all, kickoff
shows a configuration notice instead of an empty list.

### Built-in free-to-air examples

The starter template written by `kickoff init` maps two keys to public,
official, free-to-air web streams: `realmadrid` (Real Madrid TV, operated by
the club itself) and `teledeporte` (RTVE's public sports channel). They exist as
working examples of the BYOS mapping, not as a curated channel list — every
other key is left empty so you supply your own lawful source. Both can be
overridden or removed from `channels.json` like any other stream.

Streaming platforms (Disney+ Premium, DGO, Amazon Prime Video, Paramount+,
DAZN, Apple TV, VIX, Fanatiz, OneFootball) are DRM-protected. kickoff has no
DRM/CDM support and treats them as metadata only: they never receive a play
action, even when the broadcast metadata names them.

---

## Application Preferences (`config.json`)

Kickoff stores user preferences separate from channel streams in:

* **Linux & macOS**: `~/.config/kickoff/config.json`
* **Windows**: `%APPDATA%\kickoff\config.json` (or `$XDG_CONFIG_HOME/kickoff/config.json`)

This file is automatically created and updated when toggling options in the `kickoff setup` wizard, or it can be configured manually.

### Schema (`config.json`)

```json
{
  "lang": "es"
}
```

| Field | Type | Description |
| :--- | :--- | :--- |
| `lang` | `string` *(optional)* | Preferred user interface language: `"es"` (Spanish) or `"en"` (English). Sits at precedence layer 2, below `KICKOFF_LANG`. |

---

## Environment Variables

For temporary overrides, containerized environments, or advanced workflows:

| Variable | Description |
| :--- | :--- |
| `KICKOFF_LANG` | Preferred UI language (`"es"` or `"en"`). Highest precedence layer. |
| `KICKOFF_TZ` | IANA time zone name for match kick-off time conversions (defaults to `America/Bogota`). |
| `KICKOFF_CHANNELS_PATH` | Custom filesystem path to the `channels.json` stream configuration file. |
| `KICKOFF_SIGNALS_URL` | Remote URL (M3U or JSON) for automated channel provisioning during installation or setup. |
| `KICKOFF_STREAM_CBSSN` | Custom HLS stream URL for CBS Sports Network |
| `KICKOFF_STREAM_GOLAZO` | Custom HLS stream URL for CBS Sports Golazo Network |

### Language Resolution Precedence (`KICKOFF_LANG`)

Kickoff resolves the active interface language following a deterministic, closed precedence chain:

1. **`KICKOFF_LANG` environment variable**: If set to a supported language (`"es"` or `"en"`), this value always wins.
2. **`config.json` configuration file**: The `"lang"` key in `~/.config/kickoff/config.json`.
3. **`LC_ALL` environment variable**: System locale override.
4. **`LC_MESSAGES` environment variable**: Message localization locale.
5. **`LANG` environment variable**: Default system locale.
6. **Default fallback**: Spanish (`"es"`).

#### System Locale Detection (`LANG` / `LC_*`)

When reading system locale variables (`LC_ALL`, `LC_MESSAGES`, `LANG`), Kickoff automatically extracts the two-letter language code from standard locale strings:
- `en_US.UTF-8`, `en_GB`, `en-US` → resolves to `"en"` (English)
- `es_CO.UTF-8`, `es_ES`, `es-419` → resolves to `"es"` (Spanish)

If an invalid or unsupported language code is encountered (for example, `KICKOFF_LANG="fr"` or `LANG=de_DE.UTF-8`), Kickoff prints a single non-fatal warning to `stderr` and falls through gracefully to the next layer in the precedence chain, ultimately defaulting to Spanish. The application never aborts or crashes due to locale resolution.

### Timezone Configuration (`KICKOFF_TZ`)

By default, Kickoff displays all match start times in Colombia Standard Time (`America/Bogota`, UTC-5). You can customize this by setting `KICKOFF_TZ` to any valid IANA Time Zone database name:

```bash
# Display matches in Eastern Time (New York)
export KICKOFF_TZ="America/New_York"

# Display matches in Central European Time (Madrid)
export KICKOFF_TZ="Europe/Madrid"

# Display matches in Coordinated Universal Time
export KICKOFF_TZ="UTC"
```

- If `KICKOFF_TZ` is unset or empty, it defaults to `America/Bogota`.
- If an invalid or unparseable time zone name is supplied (e.g. `KICKOFF_TZ="Invalid/Zone"`), Kickoff logs a single non-fatal warning to `stderr` and falls back to `America/Bogota`.
- `KICKOFF_TZ` operates completely independently of `KICKOFF_LANG` (e.g., choosing `America/New_York` does not change the UI language to English, and choosing `en` does not alter match time zones).

---

## Diagnostic Verification

Verify that your custom configuration is loaded and valid:

```bash
kickoff setup
```

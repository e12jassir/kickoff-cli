# Configuration Guide

Kickoff works out of the box with built-in channels and dynamic TV guide crawlers. You can customize broadcast channels, define match-specific routing rules, or add private streams using a configuration file or environment variables.

---

## Configuration File

Kickoff reads configuration from:

* **Linux & macOS**: `~/.config/kickoff/channels.json`
* **Windows**: `%APPDATA%\kickoff\channels.json` (or `$XDG_CONFIG_HOME/kickoff/channels.json`)

To check your configuration paths and status at any time, run:

```bash
kickoff setup
```

---

## Schema (`channels.json`)

```json
{
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
| `league_channels` | `map[string][]Channel` | Overrides default channel listings for a specific league slug (e.g. `eng.1`, `esp.1`, `uefa.champions`, `col.1`). |
| `custom_rules` | `[]Rule` | Matches a regular expression against the match title. If matched, these channels are prioritized first. |
| `name` | `string` | Display label shown in the stream launcher selector. |
| `url` | `string` | Stream URL (HTTP/HTTPS `.m3u8` playlist). |
| `bitrate_mode` | `string` *(optional)* | HLS bitrate selection: `"min"`, `"max"`, or a bitrate in bps (e.g. `"4500000"`). Defaults to `"min"`. |

---

## Environment Variables

For temporary overrides or containerized environments:

| Variable | Description |
| :--- | :--- |
| `KICKOFF_STREAM_CBSSN` | Custom HLS stream URL for CBS Sports Network |
| `KICKOFF_STREAM_GOLAZO` | Custom HLS stream URL for CBS Sports Golazo Network |

---

## Diagnostic Verification

Verify that your custom configuration is loaded and valid:

```bash
kickoff setup
```

Kickoff is an open-source terminal scoreboard and media client. In accordance with legal safe harbor and open source best practices, **Kickoff does not host, curate, or distribute copyrighted video feeds**.

Instead, Kickoff provides a **Bring Your Own Streams (BYOS)** architecture (similar to Stremio or Kodi). You can map your lawful IPTV streams, cable logins, or personal feeds to canonical broadcast channels using `channels.json`.

---

## Configuration File

Kickoff reads configuration from:

* **Linux & macOS**: `~/.config/kickoff/channels.json`
* **Windows**: `%APPDATA%\kickoff\channels.json` (or `$XDG_CONFIG_HOME/kickoff/channels.json`)

You can copy the example file to start:
```bash
cp channels.json.example ~/.config/kickoff/channels.json
```

---

## Schema (`channels.json`)

```json
{
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

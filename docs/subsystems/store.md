# Subsystem: store

## Paths

| Path | Contents |
|---|---|
| `$HIIK_HOME/state.json` | spaces, tabs, panes, active space, UI widths |
| `$HIIK_HOME/settings.json` | fonts, theme, appearance, keys, core path, privacy |
| `$HIIK_HOME/keys.json` | keymap overrides |
| `$HIIK_HOME/programs.json` | program table overrides |
| `$HIIK_HOME/themes/*.json` | user themes |
| `$HIIK_HOME/feedback/` | drafts, never sent automatically |

`HIIK_HOME` defaults to `~/.config/hiik` on macOS and Linux and
`%APPDATA%\hiik` on Windows, following mygo's user config directory.

## state.json

```json
{
  "version": 1,
  "activeSpace": "s1",
  "spaces": [
    {
      "id": "s1",
      "name": "api",
      "folder": "/Users/me/api",
      "machine": "",
      "activeTab": 0,
      "tabs": [
        {
          "id": "t1",
          "name": "main",
          "focus": "p1",
          "root": {
            "id": "p1",
            "terminal": "t3"
          }
        }
      ]
    }
  ],
  "ui": { "sidebarW": 240, "appearance": "system" }
}
```

Splits add `vertical`, `ratio`, `a` and `b` to a node. Terminal addresses
are core-local; a remote address is `machine/tN`. A pane whose terminal
is gone keeps its node and shows a terminated state.

## Writes

- Atomic: write `state.json.tmp`, sync, rename.
- Debounced 200 ms; a structural change schedules a write, a second
  change resets the timer.
- A fatal write error shows a toast, not a crash.
- Invalid JSON is renamed `state.json.corrupt-<unix>`; defaults are used.

## Migrations

The `version` field is read before the rest. Each migration is a pure
function from the previous shape to the next. A file newer than this app is
kept aside and defaults are used; the app never downgrades a file in place.

## Settings

`settings.json` holds `appearance` (system/light/dark), `theme` (graphite,
ink, moss, atom one dark, paper or a user theme), `layout`
(herdr/terminals), `optionKey` (auto/alt/native), `uiFont`,
`terminalFont`, `terminalSize` (6–72), `corePath`, `keys`, `notifyBlocked`,
`notifyDone`, `sound` and `privacy` (`telemetry`, `clipboardRead`,
`clipboardWrite`).

Unknown fields are preserved when writing. Missing fields get defaults.
A bad value is reported with its key and the default is used; it never
blocks startup.

## Tests

Round-trip every model shape; corrupt file; newer version; concurrent
writes; debounce; a failed rename keeps the old file.

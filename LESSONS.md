# Lessons

Fixes and traps that cost time, with evidence. One entry per lesson; keep it
short and actionable. Add the date, what happened, how we know, and the rule
that prevents a repeat.

## 2026-11-09 — Attach streams need credit

- **Context:** a `terminal.attach` stream sends a first paint and then live
  bytes. The viewer must return credit as it consumes bytes.
- **Evidence:** the core's `AttachResult` describes the stream; without
  credit the core stops sending. mygo's terminal plugin reads `Conn` in a
  goroutine, so credit is sent from `Read`.
- **Rule:** every attachment owns a credit window; send credit for exactly
  the bytes read, and start with 1 MiB. Never buffer an attachment without
  reading it.

## 2026-11-09 — `ResizeByConn` is not optional

- **Context:** a viewer that resizes its local screen before the server has
  resized lays out bytes written for the old size at the new size.
- **Evidence:** mygo's terminal documents `ResizeByConn` for exactly this
  case; it keeps the screen's size until `Terminal.Resize` is called.
- **Rule:** attach with `ResizeByConn: true`; send RESIZE to the core from
  `Conn.Resize`, and adopt the core's size only when it says so. The M0
  probe must prove the acknowledgement path.

## 2026-11-10 — An empty default chord is not an unbind

- **Context:** the status bar showed only "commands" even though the key
  table binds the palette by default, and chord labels called Mod "Cmd+".
- **Evidence:** `core.App.Keymap()` returns `app.palette` with `Chord: ""`
  and `Explicit: false`; `paletteChord` tested `isUnbound("")` first and
  returned before the shell's default. `shortcutMods` binds Ctrl+Shift off
  macOS, while the formatter mapped Mod to Cmd+.
- **Rule:** an empty chord without `Explicit` means "no override"; only an
  explicit empty setting unbinds. Format chords through `api.FormatChord`,
  the same table the key map parses.

## 2026-11-10 — No hand-drawn title-bar overlays

- **Context:** a hand-positioned "new window" plus at `Right(54)` stayed
  after Windows and Linux moved to the system title bar, so it showed up
  as a second plus on top of the Changes button.
- **Evidence:** the screenshot of the tab bar; `ui/window.go` had
  `newWindowButton` as an `mui.Overlay` with `Absolute().Right(54)`.
- **Rule:** chrome controls come from the system title bar or from mygo
  widgets in the normal layout flow. Do not place absolute overlays in the
  chrome; expose extra window actions as commands, menus or settings.

## 2026-11-10 — Drag drop targets exist before the drag

- **Context:** pane grips moved to `Element.Drag`/`ui.Drop`, but the drop
  zones created only after a drag started never received the drop, and
  always-on zones blocked the grip and terminal clicks.
- **Evidence:** the grip tests failed both ways; making the zones exist
  every frame with `IgnoreInput(!dragging)` fixed the drop and kept clicks.
- **Rule:** a native drop target must be built before the drag; keep it
  out of input when idle with `IgnoreInput`.

## 2026-11-10 — Watch caches need an invalidation path

- **Context:** Files and Changes had only a manual Refresh; a watcher or
  timer that changes a panel cache must wake the window.
- **Evidence:** `filesTree.refreshLocked` and `changesState.reloadLocked`
  update package state, and `core.App.Invalidate` is the only shell wake
  from a goroutine.
- **Rule:** caches updated off the frame call the app's invalidator once;
  never redraw per file event.

## 2026-11-10 — An Outline's arrow is its prefix

- **Context:** agent sections had a black band and left-side arrows that
  did not match the rest of the sidebar.
- **Evidence:** `ui.Outline` draws its arrow in `Prefix`; the section
  header set `Background(t.Background)` while the panel sits on
  `t.Surface`, so the pinned header showed as a black strip.
- **Rule:** with a custom outline, set `Prefix` to the indentation and put
  the disclosure control in the row content; a pinned header's background
  must be the surface under it.

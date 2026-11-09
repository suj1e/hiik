# Subsystem: ui

The window, its chrome and the panes. It reads `core.App` snapshots, calls
commands and owns the terminal attachments that are visible.

## Regions

```
title bar        drag region, app name, connection, palette
sidebar          spaces (top), agents (bottom), resizable divider
main             tab strip, split panes, empty state
status bar       space summary, core state
overlays         palette, settings, rename, confirm, peek, toast
```

`Shell` holds only view state: sidebar width, focused pane, zoomed pane,
open overlay. Everything else is in `core.App`.

## Layout

A tab's tree is `Node`: a leaf with a terminal address, or a binary split
with `Vertical`, `Ratio`, `A`, `B`. Rendering is recursive `ui.Split` /
`ui.SplitVertical`; the ratio is the split's `*float32`, so dragging the
divider mutates the model directly.

A pane is a card: border, radius, theme surface, focus border. The header
shows the program, title, folder, agent status and split/close buttons.
The body is `terminal.View`.

## Focus and keys

- A click focuses a terminal; the shell records the pane.
- After an overlay closes, the pane focused before gets `.Focus()` again.
- `.Focus()` is called only on a focus change, not every frame, so the
  palette is not stolen.
- App shortcuts are registered with `c.OnWindowShortcut`; they run before
  the terminal. Everything else reaches the program.
- macOS uses Cmd; Linux and Windows use Ctrl+Shift for the same actions.
  `Mod` names that modifier in a keymap chord.
- The `keys` setting in `settings.json` maps an action ID to a chord and
  replaces that action's default binding (`{"tab.new": "Mod+Shift+T"}`).
  Action IDs are command IDs, plus `space.select.1` to `.9` and
  `pane.focus.left|right|up|down`; `pane.split.down` names the down split.
  Empty, `none` or `-` leaves the action unbound. An invalid chord shows
  a toast and binds nothing. `$HIIK_HOME/keys.json` adds overrides in the
  flat `{"command":"chord"}` or sectioned `{"global":{...}}` shape; the
  settings value wins. Terminal-only, layout and mod sections are left to
  those owners for now.

## Look

`applyLook` runs at the top of `Shell.View`. It chooses `DarkTheme` or
`LightTheme` from `appearance` (`system`, `light`, `dark`), with the
`theme` name picking the family under `system`: `graphite`, `ink` and
`moss` are dark, `paper` is light. An explicit appearance wins over the
name. `ui_font` sets the interface family.

The terminal's colors come from the same `*ui.Theme` (text, background,
cursor and the 16-color palette), not from a fixed terminal theme, so a
theme change reaches an attached terminal. `terminal_font` and
`terminal_size` set its font. The shell keeps the latest theme and font
and passes them to `terminal.New`.

## Terminal lifecycle

The UI attaches only visible terminals. Switching a tab detaches the old
terminals and attaches the new ones; the core keeps the programs and their
screens. Reattaching uses the core's first paint.

- Attach at the pane's last known cell size, or 80×24 before the first
  layout; the first layout resizes.
- `ResizeByConn: true`; send RESIZE from `Conn.Resize`, call
  `Terminal.Resize` when the core acknowledges.
- A terminal that exits shows its code with Restart and Close.
- A lost core shows a banner; reattach after reconnect.

## Redraw

- Terminal output is consumed by the terminal plugin, which invalidates its
  own view at ~40 ms.
- Core metadata events wake the window at most every ~16 ms, once per
  batch.
- Lists build only visible rows; the sidebar uses `ui.List` once it can
  scroll.

## Overlays and mods

The shell provides a panel host: a mod panel ID and props render inside a
pane, the right column or an overlay. The palette lists every command from
`core.App` and every command registered by a mod.

- Sidebar: `api.PanelSpaces` and `api.PanelAgents`.
- Tab strip: `api.PanelTabs`.
- Pane header: `api.PanelPaneHead` with the pane's node, address, focus and
  zoom.
- Status bar: `api.PanelStatus`.
- Right column: `api.PanelFiles` and `api.PanelChanges`, toggled from the
  tab strip, with the active space as props.
- Modals: `api.PanelWorktree`, `api.PanelPeek`, `api.PanelSettings`,
  `api.PanelAbout` and `api.PanelMachines`. Settings receives the
  `Settings()` map so its toggles show live values; it links to Machines
  and About.
- `app.palette`, `app.settings`, `app.about`, `app.whats_new` and
  `app.check_updates` ask the shell to open an overlay through
  `Overlay()`; the shell clears the request after opening it.

## Look and keys

`applyLook` runs at the top of every frame: it chooses a light or dark
`ui.Theme` from `appearance`, maps the named themes (`graphite`, `ink`,
`moss` dark; `paper` light), and applies `ui_font`. `refreshTermLook`
derives a `terminal.Theme` and `terminal.Font` from the same theme and the
terminal settings, and updates attached terminals live.

Bindings are action IDs with default chords; `settings["keys"]` and
`$HIIK_HOME/keys.json` override them, settings first. Theme files in
`$HIIK_HOME/themes/*.json` load live; `atom one dark` has its own palette.

Notifications from the core are queued in `ui/notify.go` and drained once
per frame; the shell installs the notifier when it is created.

## Tests

`ui.NewTester` for the empty shell, sidebar selection, tab strip, palette,
settings, focus return and terminal attach with a scripted `Conn`. No test
opens a real window or touches the user's core.

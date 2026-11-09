# Parity checklist

Frozen comparison against the reference client. `done` means implemented
and covered by a test or the guarded E2E; `partial` means the command or
setting exists but a platform limit keeps full behavior, named in the row.

Status: `done`, `partial`, `wip`, `out`.

## Command surface

`TestReferenceCommandSurface` pins all 70 shipped commands. The one omitted
command is `mod.clone`: hiik mods are Go packages, so there is nothing to
clone at run time.

## Core commands

| ID | Item | Status |
|---|---|---|
| CMD-SETTLE | `tab.settle` / `tab.unsettle` | done |
| CMD-SHOW | `terminal.show` / `terminal.park` | done |
| CMD-SCROLL | `terminal.scroll` | partial — mygo v0.8.3 has no public scrollback-scroll method; the command validates and asks the view, which redraws |
| CMD-PASTE-IMG | `terminal.paste_image` | done |
| CMD-COPY | `terminal.copy` | partial — the terminal view owns its selection and copies on its own key binding; the command reports that limit |
| CMD-MACOS | `app.hide_others` / `app.show_all` | partial — mygo exposes hide and quit, not hide-others/show-all |
| CMD-RCLICK | `terminal.right_click` | partial — policy recorded; live switch needs a reattach |
| CMD-LAYOUT | `layout.*` including `layout.set` | done |
| CMD-HELP | `app.help` | done |
| CMD-ZOOM | `app.zoom_in/out/reset` | done — uses mygo's `Context.SetZoom` |
| CMD-THEME | `app.theme` | done |
| CMD-WELCOME | `app.welcome` | done |
| CMD-CONFIRM | one confirmation path for close commands | done |
| CMD-EXIT | exit code, Restart/Close | done |

## Terminal detail

| ID | Item | Status |
|---|---|---|
| TERM-TITLE | title/cwd from OSC 0/2 and OSC 7 | done |
| TERM-REATTACH | detach/reattach after core loss | done |
| TERM-LIGATURES | font ligatures | done |
| TERM-PADDING | padding | done |
| TERM-OPACITY | opacity | done |
| TERM-CURSOR | cursor shape/blink | done |
| TERM-SCROLLBAR | scrollbar visibility | done |
| TERM-MENU | app actions in the pane menu | done |

## Layouts and mods

| ID | Item | Status |
|---|---|---|
| LAY-JSON | user layouts from `layouts/*.json` | done |
| LAY-CLONE | `layout.clone` / `layout.reset` / `layout.set` | done |
| LAY-FLOAT | floating layout blocks | done |
| MOD-PANE | `pane.split {mod, props}` | done |
| MOD-WINDOW | mod windows with keep/close | done |
| MOD-FLOAT | mod panels in floating regions | done |
| MOD-NAMES | layout names exposed to settings | done |

## Settings, keys, programs

| ID | Item | Status |
|---|---|---|
| SET-KEYS-EDIT | editable keymap | done — global editable |
| SET-KEYS-SECTIONS | `keys.json` sections | partial — parsed and shown; shell applies global only |
| SET-OPTION-5 | five Option choices | partial — left/right both map to one boolean |
| SET-PER-LAYOUT | per-layout theme | done |
| SET-ZOOM | app zoom | done — `Context.SetZoom` |
| PROG-TABLE | ~120 program rows | done (143) |
| PROG-ICONS | program icons | done |

## Machines and agents

| ID | Item | Status |
|---|---|---|
| MACH-PICKER | SSH-config picker and login/pairing | done |
| MACH-TEMP | hide `--temp` machines | done |
| MACH-E2E | remote E2E with a real host | wip — guarded test exists (`HIIK_E2E_MACHINE`), no host available here |

## Release

| ID | Item | Status |
|---|---|---|
| REL-INSTALL | download then install/relaunch (unsigned) | done |
| REL-CORE-OWN | core generation/package-manager ownership | done |
| REL-RUNTIME | Windows/Linux acceptance scripts | done |

## Out of scope

- Feedback.
- Herdr 98.
- Code signing and notarization.
- JS mod runtime and `mod.clone` (mods are Go packages).

## Platform limits (mygo v0.8.3)

- No public terminal scrollback-scroll method.
- No public selection-copy method; the terminal view owns selection and its
  own copy key, so `terminal.copy` reports the limitation.
- No live right-click, padding or cursor setters; reattach applies them.
- `option_as_alt` is one boolean, so Left/Right Option cannot differ.
- Remote E2E needs a second host.

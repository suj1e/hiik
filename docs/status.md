# Status

Updated 2026-11-09.

## Works

- Repository layout: docs, contracts, `apps/desktop` module, `ui`/`core`
  split, `mods/api`, `mods/builtin`.
- Framed herdr-core client: handshake, JSON-RPC, events, attach/credit,
  reconnect, isolated-session E2E.
- Spaces, tabs (settle/unsettle), panes (split/show/park/zoom/focus/go/
  swap/resize), pane grips, tab/space drag, terminal attach/detach/reaﬁach,
  find bar, exit code with Restart/Close.
- Agents: working/blocked/done/idle, parents and sub-agents, `herdr.seen`,
  `herdr.pinned`, notifications, sounds, per-turn dedupe.
- Commands: spaces/tabs/panes/terminals/agents/layouts/app; unified close
  confirmation; help and welcome overlays; app zoom and theme commands.
- Settings: Look, Keyboard, Machines, Alerts, Updates, Privacy; default and
  Terminals layouts plus user layout JSON; theme files and Atom One Dark;
  fonts, sizes, ligatures, padding, opacity, cursor, scrollbars; option key;
  editable global keymap and keys.json sections; per-layout themes.
- Built-in panels: spaces, agents, tabs, pane header, status, files,
  changes, worktrees, peek, settings, about, machines.
- Files/Changes/Worktrees local and remote; worktree create/open/remove.
- Mods: Go compile-time panels/commands, mod panes, mod windows with keep,
  floating mod regions.
- Programs: 143-row table and icons; installed agents detected through the
  login shell; new-tab menu includes omp first.
- Machines: list/add/connect/rename/rm, SSH-config picker, login guidance,
  temporary machines hidden, remote E2E test guarded by `HIIK_E2E_MACHINE`.
- Windows: multi-window, floating inspector, install-update overlay.
- Updates and telemetry: check, download/open, core update with ownership
  checks, local telemetry audit; What's New.
- Ops: script tests, icon generator, CI for macOS/Linux/Windows,
  unsigned release workflow and runtime acceptance step.
- `go test ./...` passes (215 top-level tests plus subtests); `just check`
  green; real-core E2E passes;
  the app launches against an isolated core.

## Platform limits

These are mygo v0.8.3 limits, not missing commands:

- No public terminal scrollback-scroll method; `terminal.scroll` validates
  and asks the view, which only redraws. The wheel and Shift+PageUp/Down
  still scroll.
- No public selection-copy method; the terminal view copies on its own
  key, and `terminal.copy` reports the limit.
- No live right-click policy, padding or cursor setters; the shell
  reattaches the terminal to apply them.
- `option_as_alt` is one boolean, so Left/Right Option cannot differ.
- Remote machine E2E needs a second host; unit tests use a fake core.

## Out of scope

- Feedback.
- Herdr 98.
- Code signing and notarization.
- JS mod runtime (mods are Go packages).

## Decisions

- Built-in panels are ordinary mods; the shell owns the window, layout tree,
  terminal views and overlays.
- Panel IDs are global; `api.Panels()` is keyed by the panel ID.
- Mods are Go packages compiled with the app; there is no JS runtime.
- Layouts are Default, Terminals and user JSON; Herdr 98 is out of scope.

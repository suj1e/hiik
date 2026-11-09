# 0004 — The shell owns terminal attachments

- Status: accepted
- Date: 2026-11-09

## Context

A terminal is visible only while its pane is in the active tab of the
active space. Attaching every terminal in the model would spend memory and
credit on screens nobody sees, and would make tab switches slow to settle.
The model does not know what is on screen; the shell does.

## Decision

- `core/hc` owns the framed connection and produces an attachment with a
  `Conn` (bytes, credit, resize, facts).
- `ui` decides which terminal addresses are visible, attaches them, builds
  `terminal.Terminal` with `ResizeByConn: true`, and detaches them when
  they leave the screen.
- `core` stores the address in the pane; it never opens a terminal stream.

## Consequences

- Tab switches detach and reattach; the core keeps the program and its
  screen, so the first paint restores it.
- A detached terminal's selection and scroll position are lost; that is
  accepted in v1.
- A lost core closes every `Conn`; the shell reattaches after reconnect.
- Tests script `Conn` and never need a real PTY.

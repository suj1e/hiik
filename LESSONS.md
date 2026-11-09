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

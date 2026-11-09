# 0002 — Layout state is local, not a core document

- Status: accepted
- Date: 2026-11-09

## Context

herdr-core stores terminals, labels and opaque JSON documents. hiik needs
spaces, tabs, splits, focus, zoom and UI widths. It could keep them in a
core document (shared by every client on that machine) or in a local file.

The app is a single-user desktop client. Sharing layout with another client
is not a requirement, and a core document would need conflict handling,
schema migration on the server and a way to clean up after a lost core.

## Decision

v1 stores layout and window state in `~/.config/hiik/state.json`, written
atomically and debounced. Terminals are adopted on start by their
`hiik.space` / `hiik.tab` labels. The store is behind a small `Store`
interface so a core-document backend can be added later.

## Consequences

- One writer, simple migrations, easy debugging.
- Two hiik windows on one machine do not share a layout; they share
  terminals. Each window uses its own `HIIK_HOME` when needed.
- A future multi-device mode can replace the store without changing the
  model.

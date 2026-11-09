# Roadmap

Each phase ends with a demo and a `just check` green. Phases are slices of
`docs/requirements.md`; they are not dates.

## P0 — Foundation (W0)

- Repository layout, docs, contracts, module, empty shell.
- Exit: `just check` passes; the window opens; the empty state is tested.

## P1 — Core client (W1)

- Framed `hc connect`, JSON-RPC, events, attach, credit, reconnect.
- Fake-core tests; M0 probe proves resize and credit.
- Exit: a CLI probe attaches a terminal, types, resizes and reattaches.

## P2 — Model and commands (W2)

- Spaces/tabs/panes tree, commands, `state.json`, settings.
- Exit: create spaces/tabs/splits in code; reload restores them.

## P3 — Shell (W3)

- Dynamic sidebar/tabs/split panes, terminal attach/detach, focus, keys.
- Exit: a real terminal in a split, typed into, resized, tab-switched.

## P4 — Built-in mods (W4a/W4b)

- Spaces, agents, tabs, palette, settings, status.
- Files, changes, worktrees, peek.
- Exit: every T1/T2 panel reachable and rendered.

## P5 — Machines and release (W5/W6)

- Machine add/list/rename/remove; remote spaces.
- CI, unsigned dmg/exe/deb, release notes, update check.
- Exit: a tag produces artifacts; a remote space runs a terminal.

## P6 — Polish

- Drag and drop, pane grip, layouts, accessibility, performance pass.
- Exit: 8 panes at 60 fps; keyboard-only path; no P0/P1 bugs.

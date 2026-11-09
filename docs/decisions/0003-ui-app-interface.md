# 0003 — The shell depends on `hc` and `mods/api`, not on `core`

- Status: accepted
- Date: 2026-11-09

## Context

`core.App` is the app's model and command entry point. The shell needs
spaces, agents, commands, UI state and a core client for terminal attach.
If the shell imported `core.App` concretely, tests would need the whole
store and command implementation, and the two packages would compile in a
cycle once `core` needs UI-facing types.

## Decision

`apps/desktop/ui` defines a small `App` interface with exactly what it
needs:

```go
Spaces() []api.Space
Agents() []api.Agent
ActiveSpace() string
Commands() []api.Command
Connection() string
Run(string, map[string]any) error
Core() hc.Client
UIState() api.UIState
SetUIState(api.UIState)
```

`*core.App` implements it. The shell imports `hiik/mods/api` and
`hiik/core/hc`, never `hiik/core`.

## Consequences

- UI tests use a fake App and a fake `hc.Client`; no store, no core.
- `core` may import `ui`? No: the dependency stays one way.
- The interface is a contract; adding a method is a design change.

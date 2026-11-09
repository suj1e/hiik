# 0001 — Native Go mods, no scripting runtime

- Status: accepted
- Date: 2026-11-09

## Context

The product needs panels and commands that can be extended without touching
the shell. A scripting runtime would make hot-reload possible, but mygo is a
Go framework: its views are Go functions, and there is no browser or DOM to
host a script. Building a scripting API would mean maintaining a second
rendering contract, a sandbox and a bridge, none of which the app needs.

## Decision

Mods are Go packages compiled into the app. A mod registers panels,
commands and keys through `mods/api`. The shipped interface is built from
the same API. Data-only customization (themes, keys, programs, settings)
stays in JSON and reloads live.

`mygo dev` rebuilds on save, so the edit loop is still short. A user adds a
package under `apps/desktop/mods/`, imports it from `mods/builtin`, and
rebuilds.

## Consequences

- Type safety and no ABI: the compiler checks every mod.
- No runtime installation, sandbox or version negotiation.
- Adding a mod requires a rebuild; this is documented.
- The API must stay small and stable; changes go through a decision record.
- A future scripting layer can implement the same registry dynamically
  without changing the shell.

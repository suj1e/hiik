# AGENTS.md

hiik is a native desktop client for herdr-core: terminals, coding agents,
spaces and tabs. It is a Go app drawn by mygo; there is no web frontend.

## Route

1. `docs/requirements.md` — the v1 contract; IDs are acceptance.
2. `docs/parity.md` — frozen comparison with the reference client and
   the owner of each remaining row.
3. `docs/tech-design.md` — layers, frozen interfaces, workstreams.
4. `docs/status.md` — what works and what is next.
5. `docs/subsystems/<package>.md` — the contract you are changing.
6. `LESSONS.md` — known traps with evidence.

## Hard rules

- No cgo. `CGO_ENABLED=0 go build ./...` must pass on macOS, Linux and Windows.
- herdr-core is a contract: change `contracts/hc.openrpc.json` and
  `docs/subsystems/core.md` in the same change.
- Never stop, kill or restart a core you did not start. Tests use
  `HC_SESSION=hiik-test` and `HIIK_HOME=<temp>`.
- UI reads the model on the main thread; core events are coalesced. Never
  invalidate the window per terminal byte.
- Views use theme colors; no hardcoded hex.
- Docs describe hiik only; do not name the reference implementation.
- Every behavior change updates `docs/status.md`; every surprising fix adds
  a `LESSONS.md` entry with evidence.

## Commands (from the repo root)

```sh
just setup   # download the modules pinned in go.mod
just dev     # mygo dev in apps/desktop
just check   # vet, tests, scripts, docs, contracts, cross-builds
just build   # packaged app
just fmt
```

## Layout

```
apps/desktop/{core,ui,mods}  the app module (go.mod lives here)
contracts/                   vendored core schema and our state schema
docs/                        requirements, design, status, subsystems
scripts/                     Go checks
```

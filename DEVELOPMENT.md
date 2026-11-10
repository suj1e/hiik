# Development

## Prerequisites

- Go 1.27.3.
- `hc` from herdr-core on `PATH`, and a core running (or startable).
- A POSIX `sh` for `just`: macOS and Linux ship one; on Windows, Git for
  Windows puts `sh` on `PATH`.
- A mygo checkout is optional, for reading its source. The build uses the
  version pinned in `apps/desktop/go.mod`; it does not link a local
  checkout.

The terminal view loads libghostty-vt at run time. Under `go run` and
`go test` mygo downloads it once into the user's cache; packaged apps carry
it in their resources. macOS 13 or later is required for the terminal.

## Setup

```sh
just setup
```

This downloads the Go modules pinned in `apps/desktop/go.mod`. There is no
Go workspace and no path outside this repository is used by the build.

## Run

```sh
just dev
```

For a development instance that does not touch your everyday core or state:

```sh
HIIK_HOME=/tmp/hiik-dev HC_SESSION=hiik-dev just dev
```

- `HIIK_HOME` selects the config/state directory (default
  `~/.config/hiik`).
- `HC_SESSION` selects the herdr-core session (default: the user's).

Never stop or kill a core you did not start. Tests always use an isolated
session and home.

## Check

```sh
just check
```

Runs, in order:

1. `go vet` and `go test` in `apps/desktop`.
2. `go vet` and `go test` in `scripts`.
3. `scripts/check-docs` — relative links, required pages, AGENTS.md length.
4. `scripts/check-contracts` — the vendored core schema and our state schema.
5. `scripts/check-version` — `mygo.json` and `docs/release.md`.
6. `CGO_ENABLED=0` cross-builds for darwin/arm64, linux/amd64 and
   windows/amd64.

Live tests that spawn a real core are opt-in:

```sh
HIIK_E2E=1 HC_SESSION=hiik-test go test ./core/hc -run E2E
```

## Build

```sh
just build
```

Packages land in `apps/desktop/dist/`. Releases are unsigned; macOS asks the
user to allow the app the first time.

## Layout

```
apps/desktop/        the app module
  core/              model, commands, store, herdr-core client
  ui/                window, layout, panes, focus, keys
  mods/api/          the mod contract
  mods/builtin/      shipped panels and commands
contracts/           core OpenRPC schema, state schema
docs/                requirements, design, status, subsystems
scripts/             checking programs (a separate Go module)
```

## Troubleshooting

- **`hc` not found**: install herdr-core, or set `core.binary` in
  `settings.json`. hiik never downloads a core on its own.
- **Terminal does not open**: the app needs macOS 13+; check that
  libghostty-vt is in the app's resources (packaged) or cache (`go run`).
- **The window shows "offline"**: the core went away. hiik reconnects; your
  terminals keep running in the core.
- **Two windows fight over a session**: use `HC_SESSION` for each.

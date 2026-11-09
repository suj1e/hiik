# hiik

hiik is a native desktop client for [herdr-core](https://github.com/herdrdev/herdr-core):
terminals and coding agents in spaces and tabs, with panes that stay alive
when the window closes.

It is written in Go and drawn by [mygo](https://mygo.egoist.dev): no web
frontend, no browser, no cgo.

## What it does

- **Spaces** for work on one machine, each with tabs and split panes.
- **Terminals** that herdr-core keeps running; close the window and reopen
  it, or attach from another machine.
- **Agents** in one list, with working, blocked, done and idle states.
- **Remote machines** over SSH, through herdr-core.
- **A command palette**, rebindable keys and themes.
- **Panels** for files, changes and git worktrees.
- **Go mods**: compile-time extensions that add panels, commands and keys.

## Requirements

- macOS 13 or later, Linux (glibc 2.28+), or Windows 10 1809 or later.
- [herdr-core](https://github.com/herdrdev/herdr-core) and its `hc` command
  on `PATH`; hiik is a client, not a terminal server.
- Go 1.27.3 to build. The terminal view needs no WebKit or WebView2.

## Build and run

```sh
git clone <this repo> hiik
cd hiik
just setup
just dev
```

`just build` writes unsigned packages to `apps/desktop/dist/`.

## Documentation

- [Requirements](docs/requirements.md)
- [Parity checklist](docs/parity.md)
- [Technical design](docs/tech-design.md)
- [Development](DEVELOPMENT.md)
- [Contributing](CONTRIBUTING.md)
- [Status](docs/status.md)

## License

MIT. The vendored herdr-core protocol schema keeps its own license; see
`contracts/README.md`.

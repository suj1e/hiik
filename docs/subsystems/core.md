# Subsystem: core

The app's model, commands, persistence and herdr-core client. It has no
mygo UI imports outside the types in `mods/api`.

## Packages

```
core/
  app.go        App: state, snapshots, Start/Close
  model.go      spaces, tabs, nodes, mutations
  commands.go   the command registry and built-ins
  store/        state.json, settings.json, atomic writes
  hc/           the herdr-core client
```

## herdr-core protocol

`hc connect` speaks JSON-RPC 2.0 on a framed connection. A frame is 9 bytes
of header and a payload:

```
channel  uint32 little-endian
kind     uint8
length   uint32 little-endian
payload  length bytes
```

Channel 0 carries one JSON-RPC message per frame. Other channels carry
terminal streams and events. Before any frame, send:

```json
{"jsonrpc":"2.0","id":0,"method":"hello","params":{"framed":true}}
```

The reply's `result` carries `generation`, `version`, `methods` and
`features`. The client refuses a generation it does not know, or a core
without the methods it uses, and says which side to update.

### Methods used

| Method | Use |
|---|---|
| `hello` | handshake, feature check |
| `terminal.list` | initial terminals, `all` for finished |
| `terminal.run` | start a shell or agent, with labels, `cwd`, `on` |
| `terminal.kill` | end a terminal and its sub-agents |
| `terminal.label` | adopt and mark terminals |
| `terminal.attach` | first paint + live bytes on a channel |
| `terminal.read` | screen text when a panel needs it |
| `events.subscribe` | snapshot + ordered events |
| `machine.list` / `add` / `connect` / `rename` / `rm` | machines |
| `machine.exec` | git and file commands on a remote machine |
| `file.read` / `write` / `edit` | files on a machine |

`doc.get` / `doc.set` are intentionally unused in v1; state is local.

### Events

`events.subscribe` returns `seq` and `terminals`, then lines:

```json
{"method":"event","params":{"seq":7,"type":"status","terminal":"t3","data":{...}}}
```

Types: `created`, `restarted`, `labels`, `removed`, `finished`, `lost`,
`status`, `turn`, `title`, `keep`, `size`, `foreground`, plus `doc`.
A `missed` line replaces the snapshot; the client resets its model and
ignores events at or below the new `seq`.

### Attach

```json
{"terminal":"t3","cols":100,"rows":30,"history":2000,
 "display":{"bg":"#111111","fg":"#eeeeee","cursor":"#ffffff","cell":[8,16]}}
```

The reply has `channel`, `terminal`, `resize_notifications` and
`automatic_input`. The channel carries DATA frames: a first paint, then
live bytes. The viewer:

- sends CREDIT as it consumes bytes (start at 1 MiB);
- sends RESIZE when its view changes size;
- sends FOCUS when the terminal gains/loses the keyboard;
- sends FACTS when its colors or cell size change;
- sends CLOSE to detach.

`Conn` implements `io.ReadWriteCloser`, plus `Resize`, `ResizePixels` and
`UseTheme`, so it plugs into mygo's terminal with `ResizeByConn: true`.

## App

`App` owns the lock and the model. Its public methods return snapshots so
the UI never reads a live map. `Start` dials, subscribes and adopts
terminals; `Close` detaches and closes the client. `Run` validates a
command, queues it, and returns before the work happens.

## Machines

`machine.list` refreshes a cache exposed as `App.Machines()`. The mutating
commands (`machine.add`, `connect`, `rename`, `rm`) call the core and then
refresh. A machine has a name, a target and a state (`ready`, `loading`,
`unavailable`) with the core's message. A space's `Machine` selects where
its terminals run; the core returns their addresses as `machine/tN`.

## Attention

The event loop turns status transitions into `api.Notification`s. A notifier
installed with `App.SetNotifier` receives them off the main thread; the
shell queues them and shows toasts, playing a short tone when the sound
setting is on. Notifications are deduplicated per `(terminal, level, turn)`
and the initial snapshot never notifies.

## Updates and feedback

`App.CheckUpdates` reads the latest GitHub release for `HIIK_UPDATE_REPO`
when set; it never installs. `App.ReleaseNotes` serves the bundled notes,
`app.whats_new` opens them, and `App.SaveFeedback` writes a local draft under
`HIIK_HOME/feedback/`. Telemetry is opt-in; when enabled it writes a local
JSON audit under `HIIK_HOME/telemetry/` and sends nothing. Updates check a
GitHub release only when asked; downloads are opened for the user to install
and never replace the running binary.

## Commands

Commands are the only mutation path. Each has an ID, a title, default keys
and a `Run`. The palette, keymap, menus and mods call the same registry.
Built-ins: `space.*`, `tab.*`, `pane.*`, `terminal.*`, `agent.*`,
`layout.*`, `app.*`, `machine.*`.

## Persistence

See [store](store.md). Invalid state is kept aside and defaults are used;
a corrupt file never prevents the app from starting.

## Tests

- Frame codec: golden bytes, partial reads, channel routing.
- RPC: IDs, timeouts, errors, loss.
- Events: ordering, `missed`, label changes.
- Attach: credit, resize, close, lost.
- Model: split/close/zoom/focus/swap, persistence and migrations.
- Fake core over `net.Pipe`; no real core in unit tests.

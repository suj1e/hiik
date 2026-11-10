# Subsystem: mods

Mods are Go packages compiled into hiik. They add panels, commands and
keys; the shipped interface uses the same API.

## Contract

See `docs/tech-design.md` for the types. In short:

```go
func init() {
    api.Register(api.Mod{
        ID: "spaces",
        Name: "Spaces",
        Panels: []api.Panel{{
            ID: "list",
            Title: "Spaces",
            Render: func(rc *api.RenderContext) { ... },
        }},
        Commands: []api.Command{{
            ID: "space.new",
            Title: "New space",
            Keys: []api.Binding{{Mods: ui.Cmd, Key: ui.KeyN}},
            Run: func(app api.App, args map[string]any) error {
                return app.Run("space.new", args)
            },
        }},
    })
}
```

## Lifecycle

- Registration happens in `init`, before the window exists.
- Duplicate mod IDs, empty panel IDs and duplicate command IDs panic at
  startup.
- A panel renders for one frame; it must not keep a `*ui.Context`.
- A command runs off the build pass; it may call the app, which queues the
  actual mutation.
- A mod is never unloaded. `mygo dev` rebuilds the whole app on save.

## Trust

Mods are trusted code, not a sandbox. They can read the model, run
commands and use the network. The README, the mod guide and the settings
page say so. Installing a mod means adding Go code and rebuilding.

## Shipped mods

| Mod | Panels | Commands |
|---|---|---|
| `spaces` | `spaces/list` | new, rename, close, move |
| `agents` | `agents/list` | next, prev, focus, pin, peek; a click focuses the agent's pane (peek stays a command/menu action) |
| `tabs` | `tabs/strip` | new, rename, close, move, settle |
| `panes` | `panes/header` | split, close, zoom, swap |
| `settings` | `settings/panel` | open |
| `files` | `files/panel` | open, reveal, copy path; `mygo.WatchFiles` refreshes the tree |
| `changes` | `changes/panel` | refresh, open; git status refreshes on a timer |
| `worktree` | `worktree/dialog` | new, open, remove; native `ui.List` |
| `peek` | `peek/terminal` | open, keep, close |
| `status` | `status/bar` | connection, retry |
| `machines` | `machines/panel` | list, add, connect, rename, rm; native `ui.List` |
| `about` | `about/panel` | check updates, what's new |

Panel IDs are global: `api.Panels()` is keyed by the panel ID, not
`mod/panel`, so the shell and a layout name a panel directly. Duplicate
panel IDs are a bug; the registry keeps the last one.

The shell (window, split tree, terminal view, focus) is not a mod: mods
draw content, not the window's own structure.

## Adding a personal mod

1. Create `apps/desktop/mods/mymod/main.go`.
2. `api.Register` from `init`.
3. Import it in `apps/desktop/mods/builtin/builtin.go`.
4. `just dev`; the app rebuilds on save.
5. `just check` before sharing.

# Coding standard

Rules that can be checked are preferred. Reviewers cite the rule number.

## Go

1. `gofmt`; no generated formatting debates.
2. No cgo; `CGO_ENABLED=0` builds on all three platforms.
3. Public methods are safe from any goroutine; document the main-thread
   ones.
4. No lock is held across a blocking call, a UI call or a channel send that
   the UI waits on.
5. Errors are wrapped with context (`fmt.Errorf("attach %s: %w", addr,
   err)`); no `panic` outside `init`/programmer errors.
6. A goroutine has an owner and a stop condition. No fire-and-forget loops.
7. Packages depend one way: `ui -> core`, `core -> hc`, mods -> `api` +
   mygo. No cycles, no `internal` escape hatches.
8. Timeouts on every RPC and every reconnect wait.
9. Tests are deterministic; a test that needs a real core is guarded by
   `HIIK_E2E=1` and uses an isolated session.

## UI

10. Views use `c.Theme()` colors; no hex literals.
11. Text sizes come from the app's scale, not one-off constants.
12. Stable `Key` for anything that keeps state across frames.
13. Lists and trees build only visible rows.
14. A shortcut that steals a terminal key must be listed in the keymap and
    the help.
15. Every empty, loading, error and disconnected state has a view.
16. Focus returns to the pane after an overlay closes.
17. Animations use mygo's native transitions, not timers that invalidate.

## Core and protocol

18. Protocol calls go through `core/hc`, never `os/exec hc` directly from
    UI or mods.
19. A new method or field updates `contracts/hc.openrpc.json` and
    `docs/subsystems/core.md`.
20. The client checks `generation` and the methods it uses at hello.
21. Terminal bytes are forwarded; the app never parses VT for layout.

## Docs and process

22. Requirement IDs are quoted in PR bodies.
23. `docs/status.md` changes with behavior.
24. `LESSONS.md` entries have evidence, not opinions.
25. No document names the reference implementation.

# Review

Review is a gate, not a comment thread. A change is approved only when every
item below is true or explicitly waived with a reason in the PR.

## Correctness

- [ ] The change satisfies the named requirement IDs.
- [ ] Errors are returned or shown; no silent failure.
- [ ] The herdr-core protocol is used exactly as the schema says.
- [ ] Terminal attach/detach, resize and reconnect have a defined state.
- [ ] A terminal that exits or is lost has a visible outcome.

## Concurrency

- [ ] UI reads model on the main thread; mutations come through the app.
- [ ] No goroutine outlives the thing it serves.
- [ ] No lock is held across a UI call or a blocking RPC.
- [ ] Core events are coalesced; no per-byte window invalidation.

## UI

- [ ] Theme colors only; no hardcoded hex.
- [ ] Focus is restored after overlays close.
- [ ] App shortcuts do not steal terminal keys.
- [ ] Lists build only visible rows.
- [ ] Empty, loading, error and disconnected states exist.

## Tests

- [ ] Unit tests cover the changed logic.
- [ ] `ui.NewTester` covers new chrome or interaction.
- [ ] Contract tests cover new protocol fields.
- [ ] No test touches the user's core or `~/.config/hiik`.
- [ ] `just check` passes.

## Docs

- [ ] `docs/status.md` says what now works.
- [ ] The subsystem page matches the code.
- [ ] `LESSONS.md` records any trap found.
- [ ] No document names the reference implementation.

## Stop-review conditions

Stop the review and send it back if:

- the change edits another workstream's files;
- a test uses the real core session;
- the UI has no visual proof;
- a protocol change has no schema update;
- the change hides an error instead of handling it.

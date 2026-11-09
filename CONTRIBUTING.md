# Contributing

## One workstream per change

`docs/tech-design.md` splits the app into workstreams with disjoint file
ownership. Work on one; do not edit files owned by another without asking
the integration owner.

## Every change

1. `just check` passes.
2. Behavior changes update `docs/status.md`.
3. A surprising fix adds a `LESSONS.md` entry with the evidence.
4. A protocol change updates `contracts/hc.openrpc.json` and
   `docs/subsystems/core.md`.
5. A UI change includes a screenshot or a `ui.Tester` test.
6. New dependencies need a decision record in `docs/decisions/`.

## Stop and ask

- The change needs a new herdr-core method or a generation bump.
- The change touches a user's real core, state or terminals.
- A test can only pass by using the everyday core.
- Two workstreams need the same file.
- A view needs a hardcoded color, font size or magic layout constant that
  the theme does not have.
- The documentation would need to name the reference implementation.

## Commits

Conventional commits, lowercase, one line. A body explains the why and
names the requirement IDs it satisfies.

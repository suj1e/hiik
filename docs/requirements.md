# Requirements

hiik is a native desktop client for herdr-core. This page is the v1
contract: every item is either implemented and verified, or listed as a
known gap in `docs/status.md`.

Terms:

- **Core** — a running herdr-core, reached through `hc connect`.
- **Terminal** — a live PTY in the core, addressed `tN` or `machine/tN`.
- **Space** — one machine and folder to work in, with tabs.
- **Tab** — an arrangement of panes in one space.
- **Pane** — one terminal on screen, or a split of two panes.
- **Mod** — Go code compiled into the app that adds panels/commands/keys.

Acceptance is a manual step unless a test is named. Every UI item also
needs a `ui.NewTester` test or a screenshot in the PR.

## T1 — Core

### Connection and core

| ID | Requirement | Acceptance |
|---|---|---|
| CORE-01 | Find `hc` from settings, `PATH`, or the login shell; report a clear error when missing | Settings and first-run screen name the path tried |
| CORE-02 | `hc connect` with framed JSON-RPC; check generation, version and methods | Fake-core tests reject a bad hello |
| CORE-03 | Reconnect after the core restarts; reattach visible terminals | Kill the test core; the app shows offline, then reattaches |
| CORE-04 | Never stop a core the app did not start | Code review: no `hc server stop/kill` outside tests |
| CORE-05 | Show core version and connection state in Settings/status | Status bar updates on connect/loss |
| CORE-06 | Use an isolated session with `HC_SESSION`; honor it everywhere | E2E test creates its own session |

### Spaces

| ID | Requirement | Acceptance |
|---|---|---|
| SPACE-01 | Create a space with name, folder and machine | New space shows in the sidebar and starts a terminal |
| SPACE-02 | Rename a space; empty name returns to the derived name | Rename dialog; reload keeps it |
| SPACE-03 | Close a space after confirmation, ending its terminals | Confirm dialog; terminals gone from `hc ls` |
| SPACE-04 | Reorder spaces; order persists | Drag or command; reload keeps order |
| SPACE-05 | Select space by click, next/previous and number keys | Sidebar and keys agree |
| SPACE-06 | An unnamed space follows its first terminal's folder | `cd` in the first terminal changes the label |
| SPACE-07 | A space may live on another machine | Remote space starts terminals through `on` |
| SPACE-08 | Terminals the app starts carry `hiik.space` / `hiik.tab` labels | `hc ls --json` shows the labels |
| SPACE-09 | Terminals without a space label appear under "Other", not lost | A terminal started by `hc run` shows there |

### Tabs

| ID | Requirement | Acceptance |
|---|---|---|
| TAB-01 | New tab in a space with folder, argv and task | Starts a shell or an agent in the right folder |
| TAB-02 | Rename a tab; empty name returns to its number | Rename dialog; reload keeps it |
| TAB-03 | Close a tab after confirmation; closing the last tab closes the space | Confirm dialogs |
| TAB-04 | Reorder tabs; order persists | Drag or command |
| TAB-05 | Select, next, previous | Keys and clicks agree |
| TAB-06 | Settle/unsettle a tab (put away, still running) | Settled tab leaves lists; agent attention brings it back |

### Panes

| ID | Requirement | Acceptance |
|---|---|---|
| PANE-01 | Split right and down, with a terminal or a panel | New pane shows and gets focus |
| PANE-02 | Close a pane after confirmation; its terminal ends | Terminal gone from `hc ls` |
| PANE-03 | Zoom a pane and restore | Toggle keeps focus |
| PANE-04 | Focus by click and by direction | Accent border follows focus |
| PANE-05 | Swap two panes or a pane and its neighbor | Geometry swaps, focus stays |
| PANE-06 | Drag dividers; sizes persist | Reload keeps sizes |
| PANE-07 | A single pane can still show its frame | Setting respected |
| PANE-08 | Drag a pane by its grip to split/swap | Drop target highlights; Escape cancels |

### Terminals

| ID | Requirement | Acceptance |
|---|---|---|
| TERM-01 | Attach to a live terminal and show its first paint and live output | `hc run` a shell; it shows |
| TERM-02 | Type into the terminal, including Kitty, modifyOtherKeys and IME | Keys match the program's mode |
| TERM-03 | Resize with `ResizeByConn`; no reflow corruption | M0 probe and a live resize test |
| TERM-04 | Scrollback, alternate screen, reflow | Wheel and keys scroll; vim/less work |
| TERM-05 | Find bar over screen and scrollback | Enter/Shift-Enter, Escape |
| TERM-06 | Selection, copy, paste; bracketed paste | Cmd/Ctrl+C/V and context menu |
| TERM-07 | Paste images and files where the program asks | OSC 5522 and file drop |
| TERM-08 | Links open on modifier-click; hover preview | OSC 8 and plain URLs |
| TERM-09 | Kitty graphics and Unicode placeholders | yazi/image test |
| TERM-10 | Clipboard write policy and contrast setting | Settings take effect live |
| TERM-11 | Title and working directory follow the program | Pane header updates |
| TERM-12 | Exit shows code and offers restart/close | Kill the shell; pane shows the result |
| TERM-13 | Detach hidden terminals; reattach with first paint | Tab switch keeps the program, not the selection |
| TERM-14 | Context menu for copy/paste/select all/clear | Right-click and Shift+right-click |

### Agents

| ID | Requirement | Acceptance |
|---|---|---|
| AGENT-01 | One list of agents across spaces and machines | Sidebar shows all live agents |
| AGENT-02 | Working, blocked, done, idle states with colors | Status events update the list |
| AGENT-03 | "Done" turns "idle" when the user looks at it | Focus the agent; state changes |
| AGENT-04 | Pin/unpin; pinned first | Survives core reconnect |
| AGENT-05 | Next/previous/ focus | Keys move through the list order |
| AGENT-06 | Peek at a terminal without a pane; keep as tab | Overlay opens, keep works |
| AGENT-07 | Notify and play a sound on blocked, and on done while away | Settings toggle; toast/sound |
| AGENT-08 | Sub-agents appear under their parent | `hc run` from an agent shows nested |

### Commands, keys, palette

| ID | Requirement | Acceptance |
|---|---|---|
| CMD-01 | One command registry used by palette, keys, menus and mods | `commands` test lists every command once |
| CMD-02 | Command palette with search and grouped results | Cmd/Ctrl+K; arrows and Enter |
| CMD-03 | Help lists every key and command | Palette entry and menu |
| KEY-01 | Default keymap in one table | Docs and palette agree |
| KEY-02 | User keymap overrides without editing Go | `keys.json` reloads live |
| KEY-03 | Conflicts are reported, not silently won | Save an invalid chord; error toast |
| KEY-04 | Terminal keys win unless the app binds them | Ctrl+C reaches the program |

### Settings and themes

| ID | Requirement | Acceptance |
|---|---|---|
| SET-01 | Settings for appearance, fonts, terminal, keys, machines, privacy | Every field applies live or on save |
| SET-02 | Zoom the whole app; reset | Cmd/Ctrl +/-/0 |
| SET-03 | UI and terminal fonts; terminal font size | Live preview |
| SET-04 | Light/dark/system appearance | Follows the OS when chosen |
| SET-05 | Clipboard and contrast policy | Matches the settings doc |
| SET-06 | Telemetry off by default; a page shows what would be sent | Nothing is sent without opt-in |
| THEME-01 | Built-in themes plus user theme files | Theme picker reloads live |
| THEME-02 | Per-layout theme overrides | Switch layouts; colors follow |
| THEME-03 | Terminal palette derived from the app theme | Program colors match |

### Machines and programs

| ID | Requirement | Acceptance |
|---|---|---|
| MACHINE-01 | Add, list, connect, rename and remove machines | Settings → Machines does all |
| MACHINE-02 | Password/host-key/core-missing cases show the command to run | Each state has a message |
| MACHINE-03 | New spaces and terminals can target a machine | Remote `tN` works |
| MACHINE-04 | Unreachable machine keeps its spaces, marked unavailable | Sidebar badge; retry |
| MACHINE-05 | Identify a reachable machine's OS and architecture | Machines panel shows Linux/macOS/Windows; the space group header shows the OS |
| PROG-01 | Built-in program/agent table with names, icons and colors | Pane headers and agent list agree |
| PROG-02 | `programs.json` adds or overrides entries | Reload applies |

## T2 — Built-in surface

| ID | Requirement | Acceptance |
|---|---|---|
| FILE-01 | Files panel: tree for a folder, expand/collapse | Opens from a tab |
| FILE-02 | Preview text, images and Markdown | Click a file |
| FILE-03 | Watch changes; refresh the tree | Edit a file outside the app |
| FILE-04 | File links open the preview | Click a path in a terminal |
| FILE-05 | Large folders stay responsive | 10k-file fixture |
| CHANGE-01 | Changes panel: git status for the space's repo | Staged/unstaged/untracked |
| CHANGE-02 | Diff view, side by side and unified | Click a changed file |
| CHANGE-03 | Open/reveal/copy path actions | Context menu |
| CHANGE-04 | Refresh on git events or a timer | Commit outside the app |
| WORKTREE-01 | List, create, open and remove git worktrees | Same repo, new branch |
| WORKTREE-02 | Worktree spaces nest under their repo | Sidebar grouping |
| PANEL-01 | Right-side Files/Changes panels toggle per space | `workspace.panels` state |
| PANEL-02 | Peek at any live terminal over the tab | Click an agent without a pane |
| PANEL-03 | Floating panel pinned to an edge, draggable | Drop springs to the nearest edge |
| PANEL-04 | Mod windows with a title bar, kept across restarts | Open/close/keep |
| LAYOUT-01 | "Main" layout: spaces/agents left, tabs and panes, status | Default |
| LAYOUT-02 | "Terminals" layout: tabs in the title bar, space picker | Switch from settings/palette |
| DRAG-01 | Drop files onto a terminal as pasted paths | Finder drag |
| DRAG-02 | Reorder tabs and spaces by drag | Reload keeps order |

## T3 — Release and operations

| ID | Requirement | Acceptance |
|---|---|---|
| UPDATE-01 | Check for a newer app, download the release asset and open it for installation | Download lands in Downloads; the running binary is never replaced |
| UPDATE-02 | What's new window with release notes | Opens once per version |
| UPDATE-03 | Offer a newer core of the same generation; never one a package manager owns | Settings → Updates |
| REL-01 | CI runs vet, tests, docs, contracts and cross-builds | Green on macOS, Linux, Windows |
| REL-02 | Tag builds unsigned dmg/exe/deb and a GitHub release | `release.yml` artifact list |
| REL-03 | Version is consistent across `mygo.json`, docs and the release log | `scripts/check-version` |
| REL-04 | Release notes in user words, one line per change | `docs/release.md` |
| TELEM-01 | Telemetry is opt-in, auditable and off by default | Settings → Privacy; local audit file |

## Mods

| ID | Requirement | Acceptance |
|---|---|---|
| MOD-01 | A mod registers panels, commands and keys with `mods/api` | Example mod compiles |
| MOD-02 | Built-in panels use the same API | Code review; no private back door |
| MOD-03 | A bad mod fails at startup with its ID | Duplicate ID test |
| MOD-04 | Mods are trusted code; docs say so | README and mod guide |
| MOD-05 | A user mod is one package added to `apps/desktop/mods/` and imported by `builtin` | Rebuild shows it |
| MOD-06 | Mods never run on the frame's build pass; commands are queued | Command test |

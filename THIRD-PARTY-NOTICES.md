# Third-party notices

hiik is MIT licensed. It depends on and vendors the following:

## mygo

- License: MIT
- Copyright: 2026 MyGo contributors
- Use: the app framework, native UI and terminal plugin.

## libghostty-vt

- License: MIT
- Copyright: Ghostty contributors
- Use: the terminal emulator behind mygo's terminal plugin, loaded at run
  time and placed in packaged apps by mygo.

## herdr-core protocol schema

- License: Apache-2.0
- Copyright: herdrdev
- Use: `contracts/hc.openrpc.json`, a vendored copy of the OpenRPC document
  printed by `hc schema` (herdr-core 0.1.19). The full license text is in
  `contracts/LICENSE-APACHE-2.0`.

## Lucide icons

- License: ISC
- Copyright: Lucide contributors
- Use: the Lucide-shaped SVGs in `apps/desktop/icons/svg` (`search`,
  `panel-left`, `monitor`, `server`, `branch`, `clock`, `layers`, `x`,
  `folder`, `pin`, `terminal`); `mark.svg` is hiik's own.

## Program marks

- Use: the SVGs in `apps/desktop/resources/icons` identify the programs in
  `programs.json`.
- They remain the trademarks of their projects, used nominatively; no
  license is granted by this repository. See that folder's README.

The app talks to herdr-core over its documented API. No source code from
another desktop client is included.

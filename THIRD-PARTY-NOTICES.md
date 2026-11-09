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

The app talks to herdr-core over its documented API. No source code from
another desktop client is included.

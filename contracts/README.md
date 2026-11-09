# Contracts

## `hc.openrpc.json`

Vendored from **herdr-core 0.1.19** with:

```sh
hc schema > contracts/hc.openrpc.json
```

It is the OpenRPC description of the core's JSON-RPC API, including the
framed `hello` handshake and every method, parameter and result. It is
Apache-2.0, copyright herdrdev; see `LICENSE-APACHE-2.0`. Do not edit it by
hand. `scripts/check-contracts` verifies the methods hiik uses exist.

## `state.schema.json`

hiik's own `state.json` (MIT). It is versioned; every migration updates the
schema and `docs/subsystems/store.md`.

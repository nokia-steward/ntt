# Titan wire-protocol deviation

`ntt`'s master-host wire protocol is deliberately **not**
bit-compatible with Eclipse Titan's MC<->HC traffic. This document
records the deviation, the reasoning, and the migration path for
operators of mixed `ntt` / Titan deployments.

## What Titan does

Titan's `mctr_cli` and `titan-host-controller` exchange a custom
binary protocol whose framing and message types are defined in
`titan.core/mctr2/mctr/MC.cc` (and the matching C structs in
`HC.cc`). The wire format consists of fixed-header records ending in
a payload whose layout is per-message-type; the format is documented
neither in `titan.docs` nor in any ETSI standard. The protocol
predates JSON's mainstream adoption and is optimised for the in-LAN
broadcast topology Titan farms originally ran on.

## What `ntt` does

`ntt` ships two equivalent encodings:

| Encoding   | Frame                                    | When to use                       |
| ---------- | ---------------------------------------- | --------------------------------- |
| JSON-line  | one JSON object per `\n`-terminated line | local debugging, REPLs, log mining |
| Binary     | 8-byte header (magic + version + length) + JSON payload | farms, low-latency CI |

Both encodings carry the same `wire.Message` discriminated union.
The binary form's payload is JSON deliberately - the framing
overhead is bounded, the messages are short, and we keep one schema
to test rather than two.

The schema is versioned (the `Version` field on `Hello`); incompatible
changes bump the major version, additive changes only bump the minor.

## Why we don't speak Titan-bytes

Three reasons, in priority order:

1. **No specification, no clean-room path.** Reading Titan's source
   to copy its wire format would put `ntt`'s BSD-3 licence at risk;
   we deliberately avoid Titan source (see also
   `runtime/cpp/CLEAN_ROOM.md`).
2. **JSON-over-stream is debuggable.** `tcpdump`, `socat` and a JSON
   pretty-printer are all you need to triage a farm in production -
   no proprietary decoder needed.
3. **Titan's framing leaks types.** Several message bodies depend on
   ABI choices made by the original C++ compiler (struct packing,
   endianness). Even a faithful re-implementation would have to take
   sides on edge cases Titan itself only handles incidentally.

## Migration path for existing Titan farms

A `ntt` host controller cannot drop in as a Titan host today; the
master would not understand its framing. The supported alternatives:

- **End-to-end `ntt`**: replace both the master and the hosts with
  `ntt-mctr` / `ntt-hc`. This is the cheapest path and works today;
  the conformance and report layers are bit-compatible with the
  Titan equivalents at the JUnit / TAP boundary, so downstream CI
  dashboards do not have to change.
- **Bridge process**: write a process that speaks Titan on one side
  and `ntt`'s `wire.Message` on the other. The translation is
  mechanical: every Titan message maps to one `MessageKind`. We have
  not implemented this bridge - the user demand has not materialised
  - but we will accept patches that add it under
  `cmd/ntt-titan-bridge`.

## Stability promise

The JSON message shapes documented in `wire.go` are stable starting
with `Version = "1.0.0"`. We will not remove fields without a major
version bump; we may add new optional fields without prior notice.
The binary framing's magic / header layout is stable starting with
`MagicTag = 0x6E74` and `Version = 1`; the framing is otherwise
trivial and intentionally over-engineered to a peer that's only ever
been our own client.

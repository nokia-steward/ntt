# `ntt` vs Eclipse Titan - feature matrix

This document tracks the gaps and overlaps between `ntt` (the toolchain
in this repo) and Eclipse Titan, the de-facto reference TTCN-3
implementation. Rows are grouped by feature area; status uses three
markers:

- `done`     - shipped in `ntt` and verified by tests in this repo.
- `partial`  - work landed but limited (see notes column).
- `planned`  - tracked by the ntt-Titan roadmap milestones M1-M9.

The matrix is kept up to date by the same CI job that runs the ETSI
conformance suite (`testdata/conformance-baseline.json`). When a
column flips status the dashboard at `docs/conformance/index.html`
re-renders.

## Language frontend

| Feature                              | `ntt`    | Titan | Notes                                            |
| ------------------------------------ | -------- | ----- | ------------------------------------------------ |
| TTCN-3 v4.11.1 parser                | done     | done  | shared `ttcn3/syntax` + v2 schema AST            |
| ASN.1:2015 frontend                  | done     | done  | `internal/asn1`, pure Go, no external compiler   |
| `with { encode ... }` attribute parse| done     | done  | `ttcn3/attr` (M1)                                |
| Type checker (scope + subtypes)      | done     | done  | `ttcn3/semantic` (M1)                            |
| Cross-module imports                 | done     | done  | covered by M1 conformance gate                   |
| LSP server (`gopls`-equivalent)      | done     | n/a   | `internal/lsp`                                   |

## Runtime

| Feature                              | `ntt`    | Titan | Notes                                            |
| ------------------------------------ | -------- | ----- | ------------------------------------------------ |
| Value model (record/set/union/enum)  | done     | done  | `runtime/object.go`                              |
| Template matching                    | done     | done  | `runtime/template` (M2)                          |
| Component lifecycle (MTC/PTC/system) | done     | done  | `runtime/component` (M2)                         |
| Ports (message + procedure)          | done     | done  | `runtime/port` (M2)                              |
| `alt` scheduler (snapshot)           | done     | done  | `runtime/alt` (M2)                               |
| Timers                               | done     | done  | `runtime/timer` (M2)                             |
| Verdict aggregation                  | done     | done  | `runtime/report` (M4)                            |
| Logging (per `[LOGGING]`)            | partial  | done  | text + JSON streams only; M9 grows HTML log     |

## Codecs

| Codec      | `ntt` status | Titan | Notes                                                |
| ---------- | ------------ | ----- | ---------------------------------------------------- |
| RAW        | done         | done  | full FIELDLENGTH/BYTEORDER (M3)                      |
| TEXT       | partial      | done  | basic separators only; advanced rules planned        |
| JSON       | done         | done  | aliases, compact mode (M3)                           |
| XER        | partial      | done  | BASIC variant only; CANONICAL/EXTENDED planned       |
| BER/DER/CER| partial      | done  | length-prefixed primitive types (M3 stubs)           |
| OER        | partial      | done  | length-prefixed primitive types (M3 stubs)           |
| PER        | partial      | done  | length-prefixed primitive types (M3 stubs)           |

## Execution

| Feature                              | `ntt`    | Titan | Notes                                            |
| ------------------------------------ | -------- | ----- | ------------------------------------------------ |
| Single-process executor              | done     | done  | `ntt exec` (M4)                                  |
| `.cfg` parser                        | done     | done  | `runtime/cfg` (M4)                               |
| Reports (JUnit/TAP/JSON/HTML)        | done     | done  | `runtime/report` (M4)                            |
| Distributed (master + host)          | done     | done  | `ntt mctr`, `ntt hc` (M5)                        |
| Wire protocol                        | partial  | done  | JSON-line; Titan bytes-on-wire planned post-M5   |
| Test port API (Go-native)            | done     | done  | `runtime/port/api` (M5)                          |
| Test port API (C ABI)                | done     | done  | header + cgo bridge + fd loop in `runtime/port/api/cabi[/cgo]` |
| Titan-compat port shim               | done     | done  | `runtime/port/api/cabi/ntt_titan_compat.h` (Handler_Add_Fd_Read, TTCN_Buffer) |
| C ABI bridge wired into interpreter  | done     | n/a   | `-tags cabicgo` enables it; see [docs/cabi-ports.md](cabi-ports.md) |
| Typed-value -> wire encoding (JSON)  | done     | done  | runtime records/lists/maps auto-JSONed before reaching C `send` hook |
| Built-in TCP test port (no user code)| done     | n/a   | `[TESTPORT_PARAMETERS] transport := "tcp"`; newline or length-prefix framing; `runtime/port/tcpport`; see [docs/live-testing-and-profiling.md](live-testing-and-profiling.md) |
| Real-clock live execution            | done     | done  | `ntt exec --live` drives a live SUT on the real clock (same strict engine) |
| Performance profiling                | done     | partial | `ntt exec --profile`: per-port latency percentiles + throughput; `--format=profile` / json |

## Build / integration

| Feature                              | `ntt`    | Titan | Notes                                            |
| ------------------------------------ | -------- | ----- | ------------------------------------------------ |
| Makefile generator                   | done     | done  | `ntt makefilegen` (M6)                           |
| CMake module                         | done     | done  | `cmake/NTTTitan.cmake` (M6)                      |
| `.tpd` -> `package.yml`              | done     | n/a   | `ntt migrate from-titan` (M6)                    |
| `.cfg` validator                     | done     | done  | `ntt cfgvalidate` (M6)                           |
| Drop-in for `ttcn3_makefilegen`      | done     | n/a   | M6 generates flag-compatible targets             |

## Codegen / IR

| Feature                              | `ntt`    | Titan | Notes                                            |
| ------------------------------------ | -------- | ----- | ------------------------------------------------ |
| SSA-ish IR                           | done     | n/a   | `ir` (M7)                                        |
| Go backend                           | done     | n/a   | `backend/golang` (M7)                            |
| C++ backend                          | done     | done  | `backend/cpp` clean-room (M8)                    |
| C++ runtime mirror                   | done     | done  | header-only `runtime/cpp` (M8)                   |
| Perf gate: compiled >=5x interpreted | partial  | n/a   | benchmark scaffold lands with M7; gate in M9     |

## IDE / adoption

| Feature                              | `ntt`    | Titan | Notes                                            |
| ------------------------------------ | -------- | ----- | ------------------------------------------------ |
| VS Code test explorer JSON           | done     | n/a   | `runtime/explorer` (M9)                          |
| DAP adapter                          | partial  | n/a   | `runtime/dap` minimal launch/threads; stepping post-M9 |
| Migration guide                      | done     | n/a   | this document + `docs/ntt-vs-titan.md`           |
| Conformance dashboard                | done     | n/a   | `docs/conformance/index.html`                    |
| Performance dashboard                | planned  | n/a   | M9 follow-on after compiled perf gate            |

## Why a column is missing for some Titan rows

`n/a` means Titan doesn't compete in that row (e.g. `.tpd` migration is
inherently from-Titan-only, and the Go backend is unique to `ntt`).
The matrix's goal is not "ntt > Titan everywhere" - it's "any team
running Titan today can tell at a glance whether ntt covers their use
case yet".

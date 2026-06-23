# `ntt` documentation index

`ntt` is a TTCN-3 toolchain in Go, targeting feature-parity with
Eclipse Titan while staying BSD-3 licensable. The pages below are the
canonical reference for the project.

## For new users

- [Getting started](getting-started.md) - install, write your first
  testcase, run it, and use the editor integration.

## For Titan users

- [`ntt` vs Titan feature matrix](ntt-vs-titan.md) - what's done,
  what's partial, what's planned, milestone-by-milestone.
- [`ntt migrate from-titan`](../README.md#migration) - convert a Titan
  `.tpd` project to a `package.yml` and run `ntt exec` on it.
- [Custom C/C++ test ports](cabi-ports.md) - build a `ntt-mysuite`
  binary that statically links your Titan-style `.cc` / `.hh`
  ports via the cgo bridge.

## For contributors

- [Repository layout](../README.md#repository-layout) - top-level
  packages and where new code goes.
- [Roadmap (live)](../README.md#roadmap) - the active milestone list
  and what's currently in flight.
- [Conformance dashboard](conformance/index.html) - ETSI conformance
  pass-rate per push, gated by CI.

## Per-milestone references

| Milestone                          | Status   | Reference                                              |
| ---------------------------------- | -------- | ------------------------------------------------------ |
| M1 Foundations                     | shipped  | `ttcn3/attr`, `ttcn3/semantic`, `ntt check`            |
| M2 Runtime core                    | shipped  | `runtime/{template,component,port,alt,timer}`          |
| M3 Codec generators                | shipped  | `runtime/codec/{raw,json,text,asn1}`                   |
| M4 Single-process executor         | shipped  | `ntt exec`, `runtime/{cfg,report,exec}`                |
| M5 Distributed executor + ports    | shipped  | `ntt mctr`, `ntt hc`, `runtime/{wire,mctr,hc,port/api}` |
| M6 Build tooling                   | shipped  | `ntt {migrate,makefilegen,cfgvalidate}`, `cmake/`      |
| M7 MIR + Go backend                | shipped  | `ir`, `backend/golang`                                 |
| M8 C++ backend + runtime mirror    | shipped  | `backend/cpp`, `runtime/cpp` (clean-room declaration)  |
| M9 Adoption (IDE + docs)           | shipped  | `runtime/{explorer,dap}`, `docs/`                      |

Each milestone's code ships with a `doc.go` and a runnable example -
follow the `Reference` links above to land directly in the source.

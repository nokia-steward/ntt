# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Releases before 0.24.0 predate this file; see the
[git tags](https://github.com/nokia/ntt/tags) and GitHub releases for their history.

## [0.24.0] - 2026-08-21

Live testing and performance profiling: the same strict TTCN-3 engine can now
drive a real system under test over the network and measure how it behaves —
functional testing *and* profiling on one engine, with the deterministic
conformance path untouched. It realizes the approach from Peuster et al.,
["Joint testing and profiling of microservice-based network services using
TTCN-3"](https://doi.org/10.1016/j.icte.2019.02.001) (ICT Express, 2019): a
single functional test doubles as a performance probe. See
[docs/live-testing-and-profiling.md](docs/live-testing-and-profiling.md).

### Added

- **`ntt exec --live`** — run the strict engine on the real clock with real
  concurrency, so timers pace real I/O against a live SUT. The virtual-clock
  default (reproducible, deterministic) is unchanged; bound a live run with
  `--timeout`.
- **Real-clock `t.read`** — measures actual wall-clock elapsed under `--live`,
  so a testcase can time an operation (`t.start; <request/response>; t.read`).
  Virtual-clock semantics (ETSI 23.4) are unchanged.
- **Built-in TCP test port** (`runtime/port/tcpport`) — bind a TTCN-3 message
  port to a live TCP endpoint with no user C code and no cgo. Newline framing
  (charstring) or length-prefix framing (octetstring, binary-safe).
- **Config-driven test ports** — declare the TCP port in a `.cfg`
  `[TESTPORT_PARAMETERS]` block (`transport := "tcp"`, `host`/`port` or
  `address`, `dial_timeout`, `framing`); no Go code required. A `*`
  (any-component) entry supplies defaults that a specific-component entry
  inherits and overrides, so one port name can reach different SUTs on
  different components.
- **Performance profiling** — `ntt exec --profile` captures per-port send /
  receive counts, send→receive round-trip latency (min / p50 / p90 / p99) and
  throughput. Report it as a table with `--format=profile`, as a metrics
  section in `--format=json`, or as a **Performance profile** table in the
  self-contained `--format=html` report (handy as a CI artefact).
- **Runnable example** — [`examples/live-testing/`](examples/live-testing/):
  a TTCN-3 module plus `.cfg` that drive a real TCP SUT and report its
  latency, with a copy-pasteable stand-in server. Exercised by CI so it
  cannot rot.

### Changed

- **`all component.done` / `.killed` now block** (ETSI 21.3.7/21.3.8) instead
  of answering a non-blocking snapshot, so a forked PTC's body actually runs
  and its verdict is recorded — a failing PTC can no longer leave the testcase
  `pass`.
- Conformance baseline moved **4747 → 4754 (96.53%)**, net **+7 with zero
  per-file regressions**. The baseline is a measurement, not a high-water
  mark; the `--regress` gate is the ratchet.

### Fixed

- An injected message now reaches a **driver-bound PTC** — an external test
  port's inbound traffic is delivered to a PTC's `receive`, not just the MTC's.
- **`alt` and `interleave` take a per-round snapshot** (ETSI 20.2): a message
  arriving mid-evaluation can no longer let a later catch-all clause jump ahead
  of an earlier, more specific one — the race a live socket exposed.
- **`p.send(...) to c`, `p.reply(...) to c` and `p.raise(...) to c`** unicast to
  only the addressed component(s) instead of broadcasting to every connected
  peer, so a sibling PTC no longer receives a value or catches an exception
  meant for another.
- A **bare** `p.getreply` / `p.getcall` / `p.catch` guard reads the running
  PTC's own per-component queue, so a reply/exception routed to it is no longer
  invisible to the PTC's own `catch`.

[0.24.0]: https://github.com/nokia/ntt/compare/v0.23.2...v0.24.0

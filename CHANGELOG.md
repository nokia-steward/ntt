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
- **Built-in HTTP test port** (`runtime/port/httpport`) — drive a REST service
  with no user code: `map` binds a base URL, `p.send` issues the request, and
  the response arrives at `p.receive` as a `{status, body}` record. Wired from
  a `.cfg` with `transport := "http"`, supports per-component base URLs, and
  surfaces a transport failure as `status := 0` with the reason in `body`
  rather than a silent timeout.
- **TLS and mutual TLS for the HTTP port** — an `https://` base URL verifies
  against the system roots by default, or against a supplied `ca_cert`;
  `client_cert` + `client_key` enable mTLS, and `server_name` overrides SNI
  when dialling by IP. Certificate files are read at map time, so a bad path
  fails the map operation naming the file. `insecure_skip_verify` is
  supported for self-signed test endpoints and warns on stderr each run.
  No gRPC.
- **Runnable example** — [`examples/live-testing/`](examples/live-testing/):
  a TTCN-3 module plus `.cfg` that drive a real TCP SUT and report its
  latency, with a copy-pasteable stand-in server. Exercised by CI so it
  cannot rot.

### Changed

- **`ntt exec` exits non-zero when the suite does not pass.** It previously
  exited `0` even for a failing suite, so a CI pipeline treated red as green.
  Severity follows the JUnit mapping already used by the reports: `inconc`,
  `fail` and `error` exit non-zero; `pass` and `none` exit `0`. The report
  itself is written first and is byte-identical — only the exit status
  changed.
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
- **Documentation defects found by running the docs through `ntt check`.**
  The getting-started walkthrough used an invalid `package.yml` field
  (`source_dir` instead of `sources`), a testcase with `runs on system`
  (`system` is a keyword, not a component type), and showed `ntt exec` output
  that the tool never produced; `cabi-ports.md` used the keyword `port` as a
  record field name; and the `t.read` stopwatch snippet started a timer with
  no default duration (invalid per ETSI 12). Every TTCN-3 snippet in the docs
  now passes `ntt check`, and the walkthrough was replayed verbatim to confirm
  its commands and output.

[0.24.0]: https://github.com/nokia/ntt/compare/v0.23.2...v0.24.0

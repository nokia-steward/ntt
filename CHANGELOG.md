# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Releases before 0.24.0 predate this file; see the
[git tags](https://github.com/nokia/ntt/tags) and GitHub releases for their history.

## [0.24.0] - unreleased

Live testing and performance profiling: the same strict TTCN-3 engine can now
drive a real system under test over the network and measure how it behaves —
functional testing *and* profiling on one engine, with the deterministic
conformance path untouched. It realizes the approach from Peuster et al.,
["Joint testing and profiling of microservice-based network services using
TTCN-3"](https://doi.org/10.1016/j.icte.2019.02.001) (ICT Express, 2019): a
single functional test doubles as a performance probe. See
[docs/live-testing-and-profiling.md](docs/live-testing-and-profiling.md).

### Added

- **Structured test logging, as the TTCN-3 standard defines it** — `ntt exec
  --log FILE` records every TTCN-3 operation of a run as an event of the
  TCI-TL logging interface (ETSI ES 201 873-6, clause 7.3.4.1): messages
  sent, arriving, received and failing to match (with the template), timers,
  component lifecycle, port configuration, verdicts and alt steps, each with
  a timestamp, the component and the source line. The file is the
  standard's XML format (Annex B), or JSON Lines with `--log-format=jsonl`.
  113 of the 125 operations are logged: procedure-based communication
  (call, getcall, reply, getreply, raise, catch, and a call's timeout),
  function, altstep and testcase entry and exit with their parameters and
  result, assignments, module parameters, `@lazy` evaluation, `encvalue` /
  `decvalue`, `match`, `rnd`, `action`, and the control part. Logs validate against the Annex B schemas:
  the whole conformance corpus was run with logging on, every event valid
  and every verdict unchanged. The published schemas do not compile as
  printed; the seven corrections needed are documented. See
  [`docs/test-logging.md`](docs/test-logging.md).
- **`ntt log diff`** — compares two test logs and reports, per testcase and
  per component, where the components' actions first differ, exiting 0, 1
  or 2 as `diff(1)` does. Built to check that a suite behaves the same on the virtual and
  the real clock: it compares actions — sends and receives with their
  values and templates, verdicts, timers, component, port and alt
  operations — and leaves out what legitimately differs between correct
  runs, the arrival of messages and the alt rounds that found nothing.
- **`ntt log profile`** — the per-port performance profile of a logged run:
  the sends, receives, throughput and send→receive latency percentiles that
  `ntt exec --profile` measures live, computed from the log's timestamps
  and reported in the same table or JSON.
- **`ntt exec --live`** — run the strict engine on the real clock with real
  concurrency, so timers pace real I/O against a live SUT. The virtual-clock
  default (reproducible, deterministic) is unchanged; bound a live run with
  `--timeout`.
- **Real-clock `t.read`** — measures actual wall-clock elapsed under `--live`,
  so a testcase can time an operation (`t.start; <request/response>; t.read`).
  Virtual-clock semantics (ETSI 23.4) are unchanged.
- **Built-in TCP test port** (`runtime/port/tcpport`) — bind a TTCN-3 message
  port to a live TCP endpoint with no user C code and no cgo. Newline framing
  (charstring) or length-prefix framing (octetstring, binary-safe). A peer
  hang-up mid-test is reported rather than left as a silence a suite cannot
  distinguish from a slow answer: always as a stderr warning, and — opt-in via
  `report_disconnect` — as an inbound `Disconnected` value it can match on.
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
  surfaces a failed request as a separate `TransportError` inbound type with
  a machine-matchable `reason` (`refused`, `unreachable`, `timeout`, `dns`,
  `tls`, `other`, `reset`, `oversize`) plus a `detail` string for logging — so a suite can branch
  on "the pod is restarting" versus "the pod is wedged" instead of
  substring-matching an error message, and a failure is never a silent
  timeout. A 4xx/5xx stays an ordinary response.
- **TLS and mutual TLS for the HTTP port** — an `https://` base URL verifies
  against the system roots by default, or against a supplied `ca_cert`;
  `client_cert` + `client_key` enable mTLS, and `server_name` overrides SNI
  when dialling by IP. Certificate files are read at map time, so a bad path
  fails the map operation naming the file. `insecure_skip_verify` is
  supported for self-signed test endpoints and warns on stderr each run.
  No gRPC.
- **Second runnable example** —
  [`examples/https-testing/`](examples/https-testing/): four testcases
  driving a REST/JSON service over HTTPS, with a self-contained TLS
  stand-in in its README, `TransportError` handling and a per-request
  budget timed with `rtt.read`. Exercised by CI like the TCP one.
- **Runnable example** — [`examples/live-testing/`](examples/live-testing/):
  a TTCN-3 module plus `.cfg` that drive a real TCP SUT and report its
  latency, with a copy-pasteable stand-in server. Exercised by CI so it
  cannot rot.

- **TLS for the built-in TCP port** — `tls := "true"` makes a configured TCP
  port a TLS client over the same framing, verifying the server against the
  system roots or a `ca_cert`, with `client_cert`/`client_key` for mutual
  TLS, `server_name` and `insecure_skip_verify` — the HTTP port's TLS
  settings, now shared by both (`runtime/port/tlsconf`). A hang-up is
  reported as on plain TCP, so a long-lived line stream can run over TLS.

### Changed

- **A `setverdict` reason is reported for every verdict, not just fail and
  error.** `setverdict(inconc, "why")` lost its message even though CI counts
  inconc as a failure, and `setverdict(pass, "label")` lost it too. The reason
  is also no longer rendered as a quoted TTCN-3 literal, so a JUnit
  `<failure message="...">` now reads as written.
- **`ntt exec --pattern` fails when the pattern matches nothing**, instead of
  falling through and running the entire suite. The glob is deliberately
  narrow — `*` matches within one name component and does not cross a `.` —
  so `--pattern "*tc_smoke"` never matches `mod.tc_smoke` and `**tc_smoke`
  is required. The error names the pattern, how many testcases were known,
  and that rule. Running with no selector at all is unchanged.
  **Upgrade note:** a pipeline whose pattern silently matched nothing was
  running everything and will now fail instead; fix the pattern, usually by
  replacing `*` with `**`.
- **`ntt exec` exits non-zero when the suite does not pass.** It previously
  exited `0` even for a failing suite, so a CI pipeline treated red as green.
  Severity follows the JUnit mapping already used by the reports: `inconc`,
  `fail` and `error` exit non-zero; `pass` and `none` exit `0`. The report
  itself is written first and is byte-identical — only the exit status
  changed.
- **`comp.done` / `.killed` block on both clocks** (ETSI 21.3.7/21.3.8)
  instead of answering a non-blocking snapshot, so a forked PTC's body
  actually runs and its verdict is recorded — a failing PTC can no longer
  leave the testcase `pass`.
- **`.done` / `.killed` used as a value is an error.** `if (c.done)`,
  `x := c.killed` and the `all/any component` forms are not TTCN-3: the
  grammar has these operations only as a statement and as an `alt` guard
  (ES 201 873-1 BNF 268/507). They were answered anyway, and the answer
  could be wrong — `false` for a finished component, and for a while `true`
  for one still running. The error says what to use instead.
  **Upgrade note:** write `c.running` / `c.alive` in an expression, or check
  without blocking with `alt { [] c.done {...} [else] {...} }`.
- **A default belongs to the component that activated it** (ETSI 20.5). The
  activated defaults were one list per testcase, so one component's `alt`
  could invoke another component's default, and a bare `deactivate;`
  cleared every component's defaults rather than its own (20.5.3).
- Conformance baseline moved **4747 → 4754 (96.53%)**, net **+7 with zero
  per-file regressions**. The baseline is a measurement, not a high-water
  mark; the `--regress` gate is the ratchet.

### Fixed

Most of what follows is one family: the real clock (`--live`, and anything
that implies it) reached code paths written with only the cooperative
scheduler in mind. The conformance corpus runs on the virtual clock, so no
fixture could catch any of them, and the regression gate stayed green
throughout.

**If you run suites under `--live` — or with a networked test port, which
turns it on for you — read this section.** Two of these could report a
`pass` that nothing earned, so a suite that was green may now correctly
fail. That is the fix working, not a new defect.

- **`p.send(v) to c`, `p.call(...) to (...)`, `p.reply(...) to c` and
  `p.raise(...) to c` no longer broadcast under `--live`.** The
  per-component routing was gated on the scheduler, so on the real clock
  the `to` address was silently ignored and every connected peer received
  the value or caught the exception — a sibling PTC could consume traffic
  meant for another component, with nothing reporting it.
- **A forked PTC's verdict is no longer lost under `--live`.** `comp.done`
  did not block, so the main component ran on and teardown stopped a PTC
  before it reached its `setverdict`. A testcase could report `pass` while
  the component it started would have reported `fail`.
- **A PTC whose body waits on a timer and also uses a port now runs under
  `--live`.** Such bodies were never started at all — not even statements
  before the timer — and the testcase died on its own guard timer with no
  indication why. "Wait, then act" is an ordinary shape for pacing a live
  SUT. A body that waits on a timer and uses *no* port is still not run, on
  either clock; see Known issues.
- **`comp.done` on a component whose body does no port communication waits
  for its modelled completion under `--live`**, rather than reporting it
  as unfinished while the virtual clock reports it as done.
- **A PTC that answers requests runs under `--live`.** A body such as
  `alt { [] p.receive(req) { p.send(rsp) } }` was run inline on the MTC,
  which then waited in the PTC's alt for the request it had not sent, and
  the testcase ended with no verdict. A finished non-alive PTC is also now
  `killed` as well as `done` on both clocks, so `c.killed` no longer waits
  forever.
- **An activated default whose branch re-asserts an already-set verdict is
  detected under `--live`.** Only a verdict *change* was noticed there, so
  the `alt` never learned the default had fired and waited out its guard.

Independent of the two clocks:

- **`p.clear` and `all port.clear` empty the queue** (ETSI 22.5.1). They
  did nothing, so a message they should have discarded was still received
  afterwards.
- **`c.running` is false for a component that was never started** (ETSI
  21.3.5); it answered true from the moment the component was created.
- **The alt guards `[] any port.getcall`, `getreply` and `catch` match**
  (ETSI 22.5). The guard looked in a queue named `any port`, found
  nothing, and waited for good.
- **A receiving operation used as a statement waits** (ETSI 20.1).
  `p.receive(t);` with nothing queued yet returned at once and ended the
  behaviour, so a plain exchange — the MTC sends, a PTC receives and
  answers, the MTC receives — ended with verdict `none`, on either clock.
  It is now the alt with that one alternative the standard defines, and
  the active defaults apply. What this took, each wrong before:
  - a component whose body communicates runs as that component; a
    responder (`getcall; reply`) was replayed inline by its caller, which
    a waiting `getcall` would block;
  - a `call` with no response block is sent — a `noblock` signature's as
    well as one with `nowait` (22.3.1); the former did nothing;
  - `p.check` with no receiving operation sees a call, reply or exception
    at the head of the queue, not only a message (22.4);
  - `any port` is the component's own ports (22.5), not every component's;
  - a default whose timer is its altstep's parameter (`altstep a(timer
    t)`, positional or named) fires: the alt waiting for it looked the
    timer up by the wrong name;
  - `@nodefault p.receive(t)` is the alt `alt @nodefault { ... }`; the
    parser dropped the modifier;
  - a receive in the branch of a default that matched waits, as in any
    behaviour; the default's sweep made it conclude at once;
  - `mtc.stop` from a PTC ends a waiting MTC; nothing woke it.
  A receive in an `interleave` branch body still does not wait: the engine
  does not expand interleave into its alternatives, and waiting there
  would block the branch that could satisfy it.
- **Starting a component whose behaviour still runs is an error** (ETSI
  21.3.2), where it went ahead.
- **A started behaviour runs, on either clock** (ETSI 21.3.2). On the
  default clock a PTC body that waited on a timer and used no port, or ran
  a `while (true)` loop that breaks, was not executed but modelled: none
  of its statements ran, a `setverdict(fail)` in it was lost, and a loop
  that ends was taken for one that does not. Every started body now runs.
  On the virtual clock computing takes no time, so a component in a long
  loop lets the others run now and then — those woken by an event first,
  then the others computing, in turn — and time does not pass while it
  computes. Only a long one takes time: each million or so iterations
  with no wait, and each hundred thousand turns at one instant — about
  fifty thousand message round trips — count as a second of virtual time
  (less when a timer is due sooner, which then fires first), so a
  component that computes for good — a busy wait, a spin, two components
  exchanging messages for good — does not hold every timer, or an
  `execute()` limit, still; nor do two components keeping each other busy
  keep a third from running. A PTC started just before the MTC's behaviour ends
  runs until it waits or finishes, a finite computation included, before
  the PTCs still running are stopped. Passing an object reference to a started
  behaviour, or a value holding one, is an error (ETSI ES 203 790
  5.1.2.2). On `--live`, a started PTC holds up its starter only when it
  maps or sends through an external driver.
- **`c.call(f())` runs f as component c and waits for it** (ETSI 21.3.10),
  on either clock. A body that waited on a timer was modelled, not run,
  and one run inline could not wait for anything another component does.
  `c.call(f(), d) catch(timeout) { ... }` waits at most d seconds: the
  block runs only when f did not end in time, and then c is stopped;
  without the clause, a call that times out is a testcase error. The
  block used to run whether or not the call timed out.
- **`execute(tc(), d)` bounds the testcase in the test system's time**
  (ETSI 26.1): virtual time on the virtual clock, where it was real time,
  so a testcase whose timers ran past d finished in a moment and was not
  stopped. `ntt exec` runs testcases directly and now takes the timeout
  from the control part, as it takes the arguments. A testcase cut off by
  its time limit — the harness budget, `--live --timeout`, an `execute()`
  timeout — ends with `error` (it did not terminate), not the verdict it
  had reached; it terminates when the MTC does, so stopping the PTCs
  afterwards does not count.
- **Values have value semantics** (ETSI 6): an assignment, an
  initialisation, a parameter (its default and a `@lazy` one included), a
  component variable's initial value, a sent message, a redirect and the
  arguments of `start` and `activate` each hold a value of their own. `var R r2 :=
  r; r.a := 3` changed `r2`; a server changing the parameter it received
  changed its caller's argument; one PTC's change to a component variable
  initialised from a constant changed the constant, and every other
  component's copy, and under `--live` could crash the run on a concurrent
  map write.
- **A field of a record initialised positionally can be assigned**
  (`var R r := {1, 2}; r.a := 3` left `r.a` at 1), and `-> param (x)`
  binds the parameter, not the parameter list.
- **Each element of a port array is a port of its own** (ETSI 21.1):
  `connect(self:pa[1], c:p)` connects that element only, what c sends
  arrives on `pa[1]`, and `any from pa.receive(...) -> @index value i`
  binds i. Every element was connected under the array's name.
- **An altstep can be an alternative of an alt** (`[] a()`), with its
  parameters, guards and `[else]`; it deadlocked on the virtual clock.
- **`p.call(...) to c { ... }`** sends the call and waits in its response
  block; the call was never sent.
- **`action()`** (ETSI 22.6) writes its text, free text and values joined
  by `&`, and is logged as `tliAction`; it was not implemented.
- **`system` is a component reference**: what arrives on a mapped port
  from the SUT comes from it, so `from system`, `-> sender s; s == system`
  and `from s` with `s := system` match (22.2.2). None did.
- **Evaluated once:** a port index (`pa[f()].send`) and a `to` clause
  (`send(v) to f()`) ran `f()` twice.
- **`c.done -> value v;` as a statement waits** for c, as `c.done;` does.
- **`any port.getcall(S:{...})` takes the call it matches**, with its
  redirects; a bare `any port.getcall` guard no longer consumes a message.
- **An inline `[false] T.timeout` guard** no longer livelocks the virtual
  clock, as expired default timers did before.
- **A call's signature no longer reaches a PTC** for the unqualified
  `getreply` / `catch` rule (22.3.1 h).
- **A typed template matches only values of its type** (ETSI 22.2.2):
  `p.receive(charstring:?)` took any value — a record the built-in TCP
  port reports a disconnection with, say, so an alt listing the
  charstring alternative first never reached the one for the record —
  and `integer:?` took a charstring. A value of another kind, or a record
  with a field the record type does not declare, now goes to the
  alternative of its own type.
- **`and` and `or` short-circuit** (ETSI 7.1.4): the right operand ran
  even when the left decided the result, so `n > 0 and s[n - 1] == c`
  read `s[-1]` of an empty string and ended in an error.
- **An `inout` or `out` parameter after an `in` one writes back into its
  own argument.** An `in` parameter declared without the word `in` was
  not counted, so `f(R r, inout S s)` wrote `s` into the caller's first
  argument — `r`, or a module parameter passed there — and left the
  actual for `s` unchanged.
- **`?` and `*` in a pattern match a line end too** (ETSI B.1.5), in
  `regexp()` and in the patterns `match()` hands to its regular
  expressions: a value ending in a newline — a JSON body as an HTTP
  service writes it — made every `regexp()` over it return "".
- **Each component has its own variables** (ETSI 6.2.10.4), on either
  clock: a function that runs on a component, however it is reached —
  called from the behaviour a PTC was started with, from a default, from
  another such function — reads and writes that component's variables,
  timers and ports. Only the started function itself used to; anything it
  called shared one module-wide copy with every other component, so two
  PTCs running the same helpers counted into the same variable. A
  component's variables are initialised when it is created, in the MTC
  too, where one initialised from a constant was undefined; an extended
  component type's members are inherited (6.2.10.2); and a function
  running on a PTC no longer sees the names its starter had in scope.
- **A stopped `alive` component can be started again** (ETSI 21.3.3), on
  either clock. The new behaviour did not run: `start` left the component
  `done`, so the next `c.done` was satisfied at once; and the stopped
  behaviour, finishing, finished the new one too. The restart now waits
  for the stopped behaviour to end, and the defaults a behaviour activated
  end with it (21.3.2).

- **`any timer.timeout` / `all timer.timeout` block when used as a
  statement** (ETSI 23.7), as the named `T.timeout` always has. Outside an
  `alt` they previously did nothing at all: no wait, no timeout consumed,
  and the timer left running.
- **A timer-only default can fire** (ETSI 20.5.1). The classic "if this
  hangs longer than N seconds, fail" safety net never fired, because an
  activated default's timer was invisible to the `alt`'s block step.
- **Defaults are not invoked inside a `call` response block** (ETSI
  22.3.1), which was never implemented. Only in that block: an `alt`
  nested in one of its branches, and other components, keep their defaults.
- **Resolving a source position is safe for concurrent use.** The lookup's
  cache could be read half-updated from another goroutine and return the
  wrong line.

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

### Known issues

These are older than this release and are recorded, with their diagnosis,
in [`docs/conformance/remaining-work.md`](docs/conformance/remaining-work.md)
§1s.

- A receive in an `interleave` branch body does not wait: the engine does
  not expand interleave into its alternatives (ETSI 20.4), and two branches
  that depend on each other deadlock.
- Starting a non-alive component again after its behaviour ended is
  accepted, where ETSI 21.3.2 makes it an error.

[0.24.0]: https://github.com/nokia/ntt/compare/v0.23.2...ntt-titan

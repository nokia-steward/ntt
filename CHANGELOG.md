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

- **A test log of lines** — `ntt exec --log run.lines` (or
  `--log-format=lines`) writes the run's events as text, one per line:
  `timestamp|event|component=file:line|fields`, a four-letter event code,
  the fields as TTCN-3 writes values — to read in a pager and split with
  awk. The same events as the TCI-TL XML and JSON Lines logs, which stay
  the standard and complete record.
- **JSON encoding and decoding** — `encvalue`, `encvalue_o` and
  `encvalue_unichar` of a value whose type has `with { encode "JSON" }` (or
  with "JSON" as the dynamic encoding) give its JSON text, and `decvalue*`
  into a variable of such a type decode JSON text by the type (ES 201
  873-11): a record or set as an object, its fields in declaration order and
  an omitted optional left out, a record of as an array, a union as its one
  alternative, an enumerated value by name, an octetstring as hex digits.
  Decoding text from elsewhere — a service's response body — ignores members
  the type does not declare, omits an absent optional field, and returns 1
  for text that is malformed or does not fit the type: more than one value,
  a member given twice, a digit string that is not whole octets. A float's
  special values are `"infinity"`, `"-infinity"` and `"not_a_number"`, and a
  value with an unbound mandatory field cannot be encoded. The type is told
  from a variable, a parameter, a component variable, a function's result,
  `valueof`, or an element or field of one — each as its own declaration
  gives it — and may be qualified (`A.R`); its field types are the
  declaring module's. A value wrapped in an object naming its type,
  `{"M.T": value}`, decodes too. A type with JSON `variant` attributes, its
  own or a field type's, is not handled by this codec yet.
- **`[EXECUTE] Module.control`** — a `.cfg` may name a module's control
  part, as `Module.control` or the module's bare name, and so may
  `--pattern Module.control`; `ntt exec` then runs it as one case: its
  testcases in the order it executes them, with the arguments it computes,
  their verdicts merged. `--timeout` bounds each of those testcases, and
  separately what the control part does itself: a control part that loops
  for good ends with `error` when that time is up.
- **Streaming HTTP responses** — with `stream := "sse"` (or `"lines"`) the
  HTTP port delivers a response as it arrives: its status as soon as the
  headers come, then each server-sent event (or line) as its own inbound
  value, then an end-of-stream value; over https too. The timeout bounds the
  wait for the headers, not the stream, which unmapping the port ends.
- **HTTP headers** — an HTTP port request may carry headers, an
  `Authorization` header say, in an optional `headers` field (a `record of
  { charstring name, charstring val }`); with `response_headers := "true"`
  the response carries its headers too, sorted by name. Suites that declare
  neither are unchanged.
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
- **A block's declarations are its own** (ETSI 5.2.2). An `if` branch or
  a plain `{ }` block declared into the scope around it, so two branches
  of an interleave — running between each other's statements — that each
  declared `var integer v` shared one `v`, and the second's value was the
  first's. A block that declares anything now has a scope of its own —
  so a `do { var integer x ... } while (x < 2)` condition no longer sees
  the body's `x`, which it may not.
  (And a scope makes its table at its first binding: an exchange on the
  real clock runs about twice as fast.)
- **Long charstrings and large parameters cost what they should.** A
  charstring was copied on every assignment and call, and `s := s & c`
  copied the whole of it each time; a record or list passed to a `runs
  on` function was copied on every call. A scanner building a
  100000-character string and calling a helper per character, and per
  element of a 20000-element list, ran past a minute; it now takes under
  one second. A charstring is never changed in place — an element
  assignment makes a new one — so copies share it, and appending to one
  extends it in place when nothing else did; an `in` parameter of a
  function that changes nothing but its own variables, calls none of the
  suite's functions and does not wait is the actual itself.
- **On the real clock, idle components no longer slow an exchange.** A
  message woke one waiting component of all, whichever came first; with
  others parked in an alt on a long timer it was seldom the one it was
  for, which then waited for the 2 ms backstop. An MTC/echo ping-pong ran
  about 29000 round trips a second alone and 640 beside 20 idle PTCs; it
  now runs about 23000. A message wakes the component it is queued for,
  and only it; a stop wakes them all.
- **A record's fields are written in their declaration order** — by
  `log()`, in a verdict's reason and in the test log, where Annex B lists
  them so. A record held its fields in a map and wrote them as the map
  went: `{field2 := 4, field1 := 3}`, and in another order on another run,
  which alone made one in two of the corpus files whose logs differ
  between the two clocks differ.
- **`interleave` interleaves** (ETSI 20.4). A branch body ran to its end
  once its guard matched, and a receive in it did not wait, so two
  branches that depend on each other deadlocked — the first one's body
  waiting for what only the second one's sends. A body that reaches a
  receiving statement, an alt, a timeout or a `done` now waits there while
  the other branches go on, as the standard's expansion of interleave into
  nested alts has it: each started body runs as a coroutine of the
  component, and the interleave waits, as the component, for the earliest
  of what its branches wait for. Interleaves nest, and run in PTCs alike;
  the activated defaults come after the branches not yet taken, as 20.5
  has them. Sem_2004_InterleaveStatement_001 and _002 now pass.
- **A default whose branch ends in `repeat` re-evaluates the alt that
  invoked it** (ETSI 20.5.2). It repeated its own altstep, which then
  found nothing, and the alt ended as if the default had taken over: a
  default skipping keepalives (`[] p.receive(KeepAlive:?) { repeat }`)
  ended the very alt waiting for the answer.
- **`T.running` reads the clock.** It counted how often it was asked, and
  after four times answered false: `while (t_window.running) { ... }`
  around a function waiting on a timer of its own ended after a few
  iterations, the window not half over, on either clock; `any
  timer.running` alike. A timer now runs until its deadline. On the
  virtual clock, where computing takes no time, a loop that polls a timer
  a thousand times at one virtual instant is taken to be busy-waiting, and
  told the timer runs no more, so `while (t.running) {}` still ends; one
  that lets virtual time pass between its polls runs to the deadline. Asking an expired
  timer whether it runs no longer discards its timeout, which a later
  `T.timeout` takes (ETSI 23.6), and an expired timer reads 0 (23.4).
- **A control part's timers run on a clock of its own** on the virtual
  clock, which each testcase it executes moves on by the virtual time that
  testcase took: `while (t.running) { execute(tc()) }` runs as many
  testcases as fit, and a control part's `t.timeout`, or an `alt` on its
  timers, no longer waits in real time or never fires.
- **Choosing another alternative of a union drops the one it carried**:
  `u.i := 1; u.s := "x"` left both, so `u == { s := "x" }` was false — in
  a variable, a parameter, a field, a list element or an `anytype`.
- **A function writing a field of its component's unbound record writes
  the component's**: `cr.n := 5` in a `runs on` function made a record of
  the function's own, and the component's stayed unbound.
- **A module resolves its own names first.** All modules of a run shared
  one flat scope, so a module's constant, function or template could be
  shadowed by a like-named one of another module — imported or not —
  loaded after it: `line == c_closed` compared with another module's
  record. Each module's definitions now resolve in a scope of its own,
  inside the shared one; the test log names a function's variables by the
  function's module, not the testcase's; a component type extends the
  type its own module means, though another module declares a like-named
  one, and its members' initialisers name what that module sees. A
  qualified reference — `A.c_x`, `A.f()` — names module A's definition. A
  module parameter set by its bare name in a `.cfg` is set in every module
  declaring it, and one set by its qualified name too takes that value in
  its module, whichever comes first in the file. And the modules of a run are loaded in the order their
  files were given, where it used to vary from run to run.
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

- Starting a non-alive component again after its behaviour ended is
  accepted, where ETSI 21.3.2 makes it an error.

[0.24.0]: https://github.com/nokia/ntt/compare/v0.23.2...ntt-titan

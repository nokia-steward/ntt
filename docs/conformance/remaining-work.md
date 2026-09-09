# Remaining Conformance Work

State as of 2026-08-20: **4754 / 4948 matched (96.53%)**, 23 skipped
(inconclusive / no-verdict), **194 real misses** in the inventory.

**2026-08-20 (b) — `to <component>` unicast routing, +8, 0 regressions.**
`p.send(v) to c`, `p.reply(...) to c` and `p.raise(S,v) to c` broadcast to
every connected peer under the scheduler — the `to` address only tagged the
sender, it did not restrict delivery — so a sibling PTC received a value or
caught an exception meant for another. And a **bare** `p.getreply` /
`p.getcall` / `p.catch` guard peeked the *unqualified* port name instead of
the current PTC's per-component key, so a reply/exception routed to the
PTC's qualified queue was invisible and its call block hung. Both are fixed
(routeProcEnvelopeTo / the send `to` filter honour the addressed component
set; the bare-guard path qualifies via PortKey). Recovered the 5 fixtures
the all-done change had exposed (SendOperation_005/006, raise_002/003/004)
plus 3 more that were timing out (check_operation_033/034/106). Combined
with the all-done change this session is **net +7 over 4747, zero
regressions**, and the multi-PTC `to`-routing tier described under "Async
multi-PTC message echo" is now largely closed.

**2026-08-20 (a) — `all component.done` now blocks (ETSI 21.3.7), net −1
(superseded by (b) above).** It
answered a non-blocking snapshot the statement context discarded, so under
the cooperative scheduler a started PTC was never granted the token and its
body never ran — its verdict silently lost. Making it park like the
singular `.done` **gained 4** fixtures whose PTC verdict now propagates
(`Sem_160102_predefined_functions_091`,
`Sem_210102_disconnect_and_unmap_operations_001/002/003`) and **exposed 5**
whose old `pass` was hollow: the PTC body was skipped, so nothing could
fail. With the body now running they hit the still-unfixed multi-PTC
send-routing / raise defects and fail honestly
(`Sem_220201_SendOperation_005/006` → fail, `Sem_220305_raise_operation_002/003/004`
→ error). Those five are the same deep tier described under "Async
multi-PTC message echo" below; the change surfaces them rather than
creating them. Net −1 on the number, more correct on the semantics.

Earlier state (2026-08-10): 4747 / 4948 matched (96.39%), 178 real misses.

The rate went **down** on purpose. It read 4813 / 97.73% on 2026-08-04,
and part of that was fiction: two mechanisms in the engine existed to make
fixtures pass, 74 files rested on them, and some of those files tested
nothing at all. Both are gone. See "The correction" below - that section is
the most useful thing in this document.

Baseline and the full miss inventory live in
[`testdata/conformance-baseline.json`](../../testdata/conformance-baseline.json)
and [`current-misses.json`](current-misses.json); regenerate with
[`refresh_artifacts.py`](refresh_artifacts.py). The baseline records the
last measurement and may move down; the ratchet is `--regress` on the
conformance command, enforced per commit by CI.

The incremental "one clean fix per slice" phase is complete: the easy
positive-test bugs have been closed. What remains does **not** yield
isolated low-risk commits. Every change must still pass the full
conformance run (`--regress 0.5`, 0 per-file regressions) and `go test
-race ./...`, which is where the external test-port coverage lives.

## This branch is the product

This work is a fork, not a queue of upstream contributions, and the
document is written from that position. The engine is judged on whether
it implements TTCN-3 correctly for the people running it here; the ETSI
suite is the instrument for measuring that, not the goal.

That distinction decides what gets kept. A change that raises the match
rate while making the semantics less defensible is not worth taking, and a
change that is clearly right but flat on the corpus is. Taken to its
conclusion on 2026-08-10: engine behaviour that existed only to make
fixtures pass was deleted even though it cost 1.34 points, because a number
produced that way is not evidence of anything.

Of the 178 real misses, **102 expect `reject`** (75 reach `pass`, 27 reach
`none`). That is still the largest block and stays out of scope until there
is a reason beyond the number to go after it. Most of the rest are the 74
files the correction stopped fabricating verdicts for.

## One engine (2026-08-03)

There is now a single evaluator: a cooperative discrete-event scheduler
with a virtual clock. The legacy "approximate" evaluator and its
verdict-preferring heuristic are deleted, along with the
`SemanticsProfile` toggle and the `--approximate`, `--profile` and
`--differential` flags. `DeterministicClock` and `DeterministicScheduler`
remain as options, because a driver running real suites wants timers to
pace real I/O; that is the only axis left.

This closed the two features that this document previously listed as the
main path forward — **1a** (strict procedure-payload matching) and **1b**
(async multi-PTC echo) — because concurrent PTCs now genuinely fork,
interleave deterministically and block on real matches instead of being
modelled. Both sections are marked DONE below.

Losing `--differential` removes the built-in way to compare two engines.
The replacement is [`diff_runs.py`](diff_runs.py), which diffs two
`--json` reports file by file and reports per-file provenance moves the
pass-rate gate cannot see. It compares two commits rather than two
engines.

### Match rate versus real execution

The suite match rate is only part of the picture: the harness also
reports a **real-execution** rate (files whose verdict came from actually
executing the testcase rather than from a parse/semantic rejection),
currently **2442 files, 49.58%**.

That number cannot approach 100%, and it is worth being precise about why
before treating it as a target. A file annotated `@verdict pass reject`
expects a rejection, and a completed run always yields a TTCN-3 verdict
(`pass`/`fail`/`inconc`/`none`/`error`) — never the string `reject`. Such
a file can therefore never be counted as `executed` and matching; the best
it reaches is `exec-reject`, which is deliberately excluded from the
real-execution rate. 2200 of the 4925 considered files expect `reject`,
so the ceiling is **2725 / 4925 = 55.33%**.

Total headroom is thus 227 files (+4.61 points), not the ~49 points the
raw figure suggests — and almost none of it is honestly reachable. 206 of
the 207 `parse-only` files carry an explicit ETSI `noexecution`
directive, including all 24 whose control part is the only executable
thing in them, so executing them would mean overriding an instruction not
to. What is genuinely left is the handful of files that fail for real
reasons, which is a match-rate question rather than a real-execution one.

One related caveat: the 23 "skipped" files are not inconclusive tests.
They all carry `@verdict pass reject`, parsed and analyzed cleanly, had no
testcase, and were demoted out of the denominator rather than counted as
misses. They are negative tests the analyzer fails to catch. Fixing that
would *lower* the reported match rate by moving them into the denominator.

## The correction (2026-08-10)

Two mechanisms in the engine existed so that fixtures would pass. Both are
gone, in that order, each with its own commit and baseline update.

**A deadlock is an error.** When every component was parked and no timer
could advance the clock, the scheduler released everyone so their blocked
alts concluded as though nothing had matched. Quiescence is a *provable*
deadlock in the loopback model - there is no outside, so nothing can ever
arrive - and it now produces an `error` verdict naming it. Guarded on no
external port driver being bound, where a real peer may still send and the
inference fails. Measured: **-51, +7** (4813 -> 4769). Six of the seven
gains are `NegSem` fixtures that expect a rejection and now get one.

**An undeclared verdict stays `none`.** ETSI 22.4.1 says the verdict starts
at `none` and setverdict moves it; the engine coerced it to `pass`, which
made a testcase that does nothing look successful. Measured: **-28, +1**
(4769 -> 4744). 26 of the 28 explicitly assert `ttcn3verdict:pass`, so they
are real defects now failing honestly.

The harness was also over-reading the annotation: a bare `@verdict pass
accept` with no `ttcn3verdict:` tag and no `setverdict` in the source
asserts only that the run is acceptable, so `none` satisfies it. Correcting
that recovered exactly 2 files. Applying it to `reject` fixtures as well
would have handed a match to 21 negative files we genuinely fail, which
measurement caught before it landed.

### What the fixtures were actually doing

Some of the 74 files were passing **vacuously** - not "passing for the
wrong reason" but not testing their subject at all.
`Sem_2204_the_check_operation_025` sets `pass` after its alt whether or not
`check(getcall)` matched, so 35 `Sem_2204_*` files were reporting on a
`check` operation that never ran.

The same was true of the Go suite, which matters more, because those tests
were the reason to believe the engine worked. Six were vacuous:

- Four `TestStrictSched_*` procedure tests in which **the PTC bodies never
  ran at all** - not the server's `getcall`, not its safety timer, not the
  client's `call`. Nothing they describe was exercised.
- `TestStrictInterleave_BlockingBodyRunsOnSnapshotEvaluator`, whose nested
  alt never matched.
- `TestAsyncPTC_InjectWakesAltAndBodyRuns`, which hid a **user-facing** bug
  (below).

All six are kept, pointed at what the engine really does, each naming what
must be restored when the gap closes. A green test that asserts nothing is
worse than a red one.

### Where the 74 files went

47 chapter 22 (procedure-based communication and `check`), 14 chapter 21
(configuration operations), 13 spread thinly. Progress since:

- **Procedure responder bodies now run for real** (+3, 4744 -> 4747). The
  syntactic predicate that declared a body containing any procedure
  operation unrunnable predates the scheduler, which can now run and park
  it. One shape stays carved out and the carve-out is measured: a bare
  finite responder started before any call is queued would fall through its
  non-blocking `getcall` and reply to nobody.
- Running the bodies is **necessary but not sufficient**. The four
  `TestStrictSched_*` tripwires still report no verdict, so a client/server
  pair still cannot complete a call/reply round trip. The next layer is
  matching and routing, not scheduling.

### Newly found, not yet fixed

- **An injected message never reaches a driver-bound PTC.** The path a real
  C/C++ test port uses. Diagnosed precisely: the daemon body runs, the
  message lands in the queue under exactly the key its receive reads, it is
  still there unconsumed at the end, the alt re-polls every 2ms for the
  full 500ms, and neither a typed nor a bare `srv.receive` observes it. Not
  a lost wake-up, not a template mismatch - guard evaluation for a
  driver-bound port. Tripwire:
  `TestAsyncPTC_InjectDoesNotReachADriverBoundPTC`.
- **A stopped `alive` component cannot be restarted** (ETSI 21.3.3), which
  is `Sem_210303_Stop_test_component_005..010`. Two layers: `start` does
  not clear the component's `done` flag, so the following `comp.done` is
  satisfied by the previous run and the MTC leaves before the new body is
  scheduled; and with that cleared, the re-forked PTC goroutine still never
  gets scheduled. Tripwire:
  `TestRestartAfterStop_DoesNotRunTheNewBehaviour`.
- **Interleave does not suspend a blocked branch.**
  `Sem_2004_InterleaveStatement_001` needs branch 2 to run while branch 1's
  body is blocked in a nested alt - coroutine-style interleaving inside one
  component. Note the suite treats a nested alt in an interleave body as
  legal: `NegSem_2004_InterleaveStatement_001` has the same construct and
  blames its `for` loop.
- **Timer-only PTC bodies still do not run.** Dropping `timeout` from the
  skip predicate is now possible in principle (the virtual clock removed
  the original excuse) but measured **-7 / +0**, so it is out. Reverted
  rather than absorbed.

## The executed-but-wrong slice (2026-08-04)

Thirteen files executed cleanly and produced a semantically wrong
verdict — the most damning category in the suite, because each one is the
engine confidently getting TTCN-3 wrong. Eleven are fixed; the corpus
moved 4803 → 4813 (the eleventh is offset by `Sem_5010205_OfOperator_001`,
the mislabelled fixture Bucket 2 predicted would flip).

Landed, each its own commit with zero per-file regressions:

- **Assignment notation on a record** leaves unmentioned fields alone
  (ETSI 6.2), **union alternative access under `?`** yields `?`, and
  **template concatenation** expands a `? length(n)` run to n units.
- **An enumerated value beats a clashing local definition** in a
  comparison (ETSI 8.2.3.1).
- **The object cast `=>` and the `of` type test** (ETSI 5.1.2.5/5.1.2.6).
- **A default that stops the component, or breaks, unwinds its caller**
  (ETSI 20.5.1): `runDefaults` discarded the altstep result, so the
  statements after the alt ran anyway.
- **External functions bind through a registry** (see 1d below).
- **`encvalue_o` / `decvalue_o` for bitstrings, records and short input**
  (see 1d below).

Two did not land, and both stopped at the same wall: the engine has
scaffolding that hides broken semantics, and removing the scaffolding
fails dozens of files at once.

- **`Sem_2303_timer_stop_004`** expects `none`. ETSI 22.4.1 says an
  undeclared verdict stays `none`, and the engine coerces it to `pass`
  ([`interpreter/testcase.go`](../../interpreter/testcase.go), the
  comment above the coercion). Removing the coercion costs **64 files** —
  and only 4 of them lack `setverdict` altogether. The other 60 *call*
  `setverdict` on a branch that never executes, so the coercion is
  fabricating the verdict their broken control flow never reached.
  Prerequisite: fix why those branches do not run.
- **`Sem_2601_ExecuteStatement_003`** expects `error` from a testcase
  whose `alt { [] any port.receive { repeat; } }` blocks forever. Leaving
  the scheduler's deadlocked participants parked - which is what the
  standard asks for, since an alt with no match and no `[else]` blocks
  rather than concludes - does produce that `error`. It also turns **51
  other files into timeouts**: their alts cannot fire for reasons of our
  own, and the deadlock release is what lets them fall through to a
  verdict. Recorded at the release in
  [`runtime/scheduler.go`](../../runtime/scheduler.go).

Both are the same finding from two directions, and it is the most useful
thing this slice produced: **the deadlock release and the verdict
coercion are load-bearing for ~115 files between them**, which is a
measurement of how much of the alt / event-delivery model is still
missing. Neither should be removed alone. The order is: make those alts
fire for real, then drop the release, then drop the coercion.

One incidental finding worth its own line: a PTC started with
`comp.start(f)` runs **inline on the parent's goroutine** unless the
deterministic scheduler is on or a narrow syntactic predicate
(`startBodyDoesPortComm` and friends in
[`interpreter/interpreter.go`](../../interpreter/interpreter.go)) decides
to fork it. Under the plain path an MTC cannot feed a PTC that blocks on
`receive`, which is one concrete source of the 51 files above.

## Execution triage 2026-06-17 — the `reject->pass` bulk is NOT low-hanging

A planned push to add semantic-analyzer rejections (106 of 160 misses are
`reject->pass`) was triaged against the live fixtures. **No safe, simple
static check was found** — each candidate is contradictory, needs complex
topology/flow analysis, is a runtime error, or needs a deep model change:

- **`0503_Ordering_002/003`** — CONTRADICTORY (Bucket 2). `NegSem_002` is the
  *identical* decl-after-statement construct as the positive `Sem_0503_003`
  ("declaration in the middle of the block are allowed"); `NegSem_003` ≡
  `Sem_0503_004`. V4 (must precede) vs V5 (relaxed). Rejecting washes.
- **`0901_communication_ports`** (×6) — NOT "port appears twice". It is the
  full ETSI Figure 6/7 connection-scheme matrix: positives `Sem_0901_003/009`
  connect one port to *multiple* peers (allowed); only the two-TSI-port
  subset (`NegSem_002/007`) is a candidate, and it needs static mtc/system
  detection. High risk, ~2 tests.
- **`220201/220202/220203`** send/receive/trigger NegSem — RUNTIME errors
  (`send` on a disconnected port, `@decoded` decode failure, missing `to` on
  a one-to-many connection), not static rejections. Need real codec /
  connection-state modelling.
- **`150605_Referencing_union_alternatives_002`** (`pass->fail`) —
  irreconcilable without **tracking which union alternative is chosen**.
  Making `(?:U).alt` propagate `?` (so `ispresent` is true) forces
  `ischosen((?:U).alt)` false, but `ischosen({alt:=?}.alt)` must stay true;
  the model distinguishes them only by value (`Any` vs `Undefined`), so a
  single rule can't satisfy both. Two attempts (blanket, then type-aware)
  regressed `ischosen` / `ispresent` siblings; both reverted.

**Conclusion:** the safe-win seam is exhausted. Remaining progress requires
a *deep feature* (multi-PTC scheduling, procedure-signature qualification,
union-alternative tracking, a real RAW codec, or connection-topology
analysis) — each a dedicated, higher-risk effort — or leaving the
contradictory/runtime clusters alone. See buckets + slice notes below.

---

## Bucket 1 — Large, dedicated features (the real path forward)

These are worth doing; each is a multi-hour focused slice with real
regression risk on a load-bearing path. Listed by ROI.

### 1y. `comp.done` does not block on the real clock — SCOPED, not started

Found 2026-09-09 by a Windows CI failure in a test of the built-in TCP port,
which is a slower runner losing a race the faster ones win.

`comp.done` and `comp.killed` park properly under the cooperative scheduler
(`blockUntilComponentState`), and `all component.done` was given the same
treatment. Both are gated on `deterministicSchedulerEnabled`. On the
**real-clock path** — `ntt exec --live`, which is what an external transport
forces — they fall through to a non-blocking snapshot. A testcase that forks
PTCs and then waits on `.done` therefore does not wait: it reaches the end
of its body, and teardown stops the PTCs (stop-first, by design) before they
have done their work. ETSI 21.3.7 says the operation blocks.

Anything driving a live SUT from PTCs is exposed, because that is exactly
the shape: fork a worker per node, wait for them, assert. The workaround is
to wait on an event instead — have each worker report completion over a
connected port and receive those reports — which is what
`TestPerComponentAddressesScale` now does, with a comment saying why.

**Why it was not fixed on the spot.** Each forked PTC already has a
`DoneChan` (`TestcaseExec.RegisterPTC`), so the wait itself is easy and needs
no polling. The obstacle is cancellation: `TestcaseExec.Stop()` sets a flag
and signals the message-ready condition; it does **not** close a broadcast
channel. There is nothing safe to `select` on alongside `DoneChan`, so a
naive implementation hangs the interpreter when a PTC never finishes — a far
worse defect than the one it fixes. The honest fix is therefore two steps:
give the exec a close-once stop channel, then make `.done` / `.killed` /
`all component.done` wait on `DoneChan` against it. Both steps are small; the
ordering is what matters.

### 1z. `decvalue` structured decode — SCOPED, not started

Raised 2026-09-09 by a suite driving a REST service through the built-in
HTTP port: its assertions were substring matches against a JSON body, which
cannot show that the *right* element of an array changed. The route from
TTCN-3 to the JSON codec is `decvalue_unichar`, and it is narrower than it
looks. Scoped here so the next attempt starts from the shape rather than
repeating the reconnaissance.

**Do not read `builtins.DecValue`.** It is a stub that returns `Undefined`,
and it is **not the live path** — the interpreter intercepts `decvalue` /
`decvalue_unichar` / `decvalue_o` at the call site (`evalDecValue`) and never
reaches it. Concluding "decvalue is unimplemented" from that stub is the
mistake to avoid; it cost someone an investigation already.

**What already works.** `evalDecValue` has a JSON branch
(`decodeExplicitJSON`) that decodes **scalars and enumerated values**, and
implements the error-behaviour spec (`ET_UNDEF` / `EB_IGNORE`, return code 2
for undecodable input). It requires an explicit third argument —
`decvalue_unichar(body, v, "JSON")` — and does **not** read
`with { encode "JSON" }` off the type.

**The gap is structured decode only**: a JSON object into a record, and a
JSON array into a `record of`.

**Why it is smaller than the stub comment suggests.** That comment ("expects
data shaped types we don't model yet") predates the type machinery:
`TypeDesc.Struct` now carries a record's declared field names and types,
`declaredTypeBinding` already resolves a target's declared type (the enum
path uses it), and `assignToLHS` handles write-back. The work is one
recursive type-directed converter plus tests — afternoon-scale.

**The risk is the gate, not the difficulty.** The ETSI suite exercises
`decvalue` heavily and those fixtures currently fail in a *known* way, so a
partial implementation can move the number in either direction. Measure the
per-file delta before committing, as with any Bucket 1 slice; that is the
reason this wants a dedicated slot rather than being squeezed in.

A useful first slice, from the asking suite: object into a record of
`charstring` / `integer` / nested record / `record of` fields, returning
non-zero when the body does not fit. Encoding, unions, optionality
subtleties and the alias machinery are not needed for it.

### 1a. Strict procedure-payload matching — DONE
**Closed 2026-07-28.** The previously listed fixtures
(`Sem_220302_getcall_operation_020/021`,
`Sem_2204_the_check_operation_090/094`) all match now: procedure receives
honour the signature parameter and value/exception template under the
strict engine instead of firing on "an envelope of this kind arrived".
Chapter 22 retains 22 misses, but every one is a `reject->pass` negative
fixture (a static-rejection question, Bucket 3), not a payload-matching
failure. The history below is kept for context.

**DONE 2026-07-06 (+2): signature qualification.** `call`/`reply`/`raise`
now record `PortMessage.Signature` (the signature identifier peeled from
the `S:{…}` / bare-`S` argument via `procSignatureName`), the blocking
`call(S,…){ }` handler stashes S on the scope (`procCallSigKey`,
save/restore for nesting), and an **unqualified** `getreply`/`catch`
inside that block now skips a queued head whose known signature differs
(the filter lives in `commGuardMatches` for the bare `[] p.getreply`
guard and in `evalPortReceiveInfo` for the `from`-qualified variant).
This fixed `Sem_220301_CallOperation_019/020` (ETSI 22.3.1 h: unqualified
getreply/catch treat only the called procedure's reply/exception) with
0 per-file regressions. It is deliberately scoped to the *implicit*
call-block qualifier and leaves explicit-signature / getcall matching
lenient, so the 119 `Sem_2204` check fixtures are untouched. Guarded by
`interpreter/proc_signature_test.go`.

The remaining ~6 need the harder, coupled change below. The loopback
model still matches those procedure receives **leniently** — a
`getreply`/`catch`/`getcall` fires on "an envelope of this kind arrived",
ignoring the signature/parameter template (see the comment at
`interpreter/interpreter.go` `evalPortReceiveInfo`, the
`payloadOk := isProc || …` line). This is load-bearing: making it strict
in isolation broke 17 check tests (057–088) in a prior attempt because
deferred responders then enqueued replies that lenient `check` wrongly
matched. The two requirements are coupled and must land together:

- **Strict payload/param match** for procedure receives (honour the
  `getreply(S:{…} value ?)` / `catch(S, T)` template), and
- **Deferred-responder timing**: a server PTC whose `getcall;reply/raise`
  body was deferred (caller issued a non-blocking `call` before the
  server was started) must run before the caller's receive. A targeted
  trigger (run pending responders at start when a call is already queued,
  via a `HasPendingCall`-style check) avoids the over-broad
  "run at every receive" that caused the regressions.

The signature is now recorded on every `call`/`reply`/`raise` envelope
(see the 1a DONE note above), so a strict-match attempt can rely on
`PortMessage.Signature` being populated; the remaining risk is purely in
flipping the lenient `payloadOk` for the `check`/getcall paths without
regressing the 119 `Sem_2204` fixtures.

### 1b. Async multi-PTC message echo — DONE
**Closed 2026-07-28** by path (a) below: the strict cooperative scheduler
forks every port-communicating PTC and parks alts on real port traffic,
so an MTC↔PTC round-trip completes. `Sem_060210_ReuseofComponentTypes_002/003`,
`Sem_2004_InterleaveStatement_002/013` and
`Sem_1400_procedure_signatures_004` all match now.
`Sem_200501_the_default_mechanism_008`, the last fixture from this
cluster, was fixed on 2026-08-04 — not by scheduling at all, but because
a `stop` in an activated default was being discarded (see "The
executed-but-wrong slice"). Original analysis retained below — note that it
names `evalAltStmtBestEffort`, `waitForAltPortTraffic` and
`altHasExternalPortGuard`, all deleted on 2026-08-03 with the approximate
engine, so it is history rather than a map of the current code.

**~6 tests**, and **not one mechanism**: `Sem_060210_ReuseofComponentTypes_002/003`
is a server-PTC echo (`while(true){alt{receive->send}}`); `Sem_2004_InterleaveStatement_002/013`
is interleave + self-loop; `Sem_200501_the_default_mechanism_008` is default
+ stop + self-loop; `Sem_1400_procedure_signatures_004` is multi-PTC
`all component.done`. They do **not** share one fix.

A server PTC started with a `receive…send` body must process a message
the MTC sends *after* `start`. **Attempt 2026-06-18 (reverted):** forking
the `while(true)` receive-echo body as a goroutine (relaxing the
`AliveModifier` gate on the existing daemon-style fork) makes it run, but
the round-trip still fails — `evalAltStmtBestEffort` only parks-and-wakes
(`waitForAltPortTraffic`) for ports with an **external driver**
(`altHasExternalPortGuard`); a **loopback** alt falls to the legacy
verdict-preferring heuristic and never blocks for the peer's echo. So the
forked server and the MTC never synchronise.

Two viable paths, both substantial: **(a)** real concurrent scheduling
for loopback alts (extend `waitForAltPortTraffic` parking to loopback
ports under forked PTCs) — touches the most load-bearing path, very high
regression risk; or **(b)** a **synchronous message-responder**
(register the server's alt at `start`; when the MTC's `receive` misses,
run the responder's alt once to drain+echo, then retry) — analogous to
the procedure `RunDeferredResponders`, no concurrency, but it must
interact correctly with the legacy alt heuristic. Neither is a clean
commit.

### 1c. Structural record/list type compatibility — essentially done
**~1 test left:** `Sem_060302_structured_types_010` (needs external
functions; tracked under 1d).

Assigning a value to a variable of a structurally-compatible type with
**different field names** or **constrained dimensions** maps members by
position (ETSI 6.3.2 / 6.2.7). Implemented:

- **Record/set field-name remap** — out/inout-param writeback
  (`coerceWritebackStruct`) and the plain `v2 := v1` assignment
  (`evalAssign`), sharing `remapStructByPosition`: when both sides resolve
  to distinct struct declarations of the same field count with different
  names, the value is relabelled to the target's names. Fixed
  `Sem_050402_actual_parameters_184`, `Sem_060302_structured_types_001`.
- **Constrained array-subtype index offset** — `type integer T[1..2]`
  records its lower index bound on `TypeDesc.IndexOffset` (subtype
  registry); a list assigned to such a variable adopts it (copy-on-write
  so the source keeps its own offset). Fixed
  `Sem_060301_non_structured_types_002`.

Enum-synonym, record-of / set-of, same-alternative union, and
length-restricted string/bitstring/hexstring subtype assignment already
worked. None of this checks field-*type* compatibility — it is scoped to
differently-named or differently-indexed layouts only.

### 1d. Smaller dedicated items — investigated, no clean win
Each item below was scoped against the live fixtures; none yields a
safe, generalisable commit. Captured here so they are not re-attempted.

- **External functions — DONE 2026-08-04** (`Sem_160103_external_functions_001/002`,
  `Sem_060302_structured_types_010`). This entry previously read "gaming —
  do not implement", on the grounds that the return values are
  host-defined. They are, and that is the point: an `external function`
  has no TTCN-3 body because a deployment's SUT adapter supplies one
  (ETSI 16.1.3), so the engine's job is to make that binding possible, not
  to guess. [`runtime/extfunc.go`](../../runtime/extfunc.go) is the
  registry; the harness binds the three fixtures that document their own
  contract in a doc comment, in
  [`conformance_extfuncs.go`](../../conformance_extfuncs.go), so nothing
  fixture-specific sits in the engine. An unbound external function still
  yields Undefined — 472 fixtures declare one, and erroring at the call
  would fail them for an unrelated reason.
- **RAW `encvalue_o` / `decvalue_o` — DONE 2026-08-04**
  (`Sem_160102_predefined_functions_107/110`). A bitstring is
  left-aligned into whole octets behind a 32-bit little-endian bit count,
  so `encvalue_o('011'B)` is `'0300000060'O`; a record encodes its fields
  in order, giving `'74657374546578740005000000'O` for `{"testText", 5}`
  exactly as 107 documents; and `decvalue_o` returns 2 when the input is
  shorter than the output slot's declared field width. Everything else
  keeps the round-trip placeholder rather than an invented encoding, so
  this is still **not** a real RAW codec — item 3 under "Suggested order"
  stands.
- **Template field building / union alt access — DONE 2026-08-04**
  (`Sem_150605_Referencing_union_alternatives_002`,
  `Sem_1511_ConcatenatingTemplatesOfStringAndListTypes_013`). The 2026-06
  attempt described below regressed 6 files because it propagated the
  wildcard blanketly from `left == runtime.Any`, where the type is gone.
  What worked was resolving it where the type is still in hand: the
  selector path returns `AnyOrNone` for an optional field and `Any` for a
  mandatory one, *after* the attribute-access branches, so `ischosen` and
  ordinary field referencing are untouched. History of the failed attempt:
  mirroring the IndexExpr `left == Any` rule into the SelectorExpr path
  fixed the target but broke `Sem_07010802_ischosen_operator_001`,
  `Sem_150602_ReferencingRecordAndSetFields_003/004`,
  `Sem_160102_predefined_functions_022` and `Sem_27010200_general_015`.
- **Control-part conditional selection** (`Sem_2602_TheControlPart_001`):
  the harness runs the first testcase; this file's control part only
  `execute()`s the second under `if(true)`. Honouring control-part
  selection is a harness change affecting ~2883 control-part files — high
  risk; see also the masking note in `current-misses.md`.

---

## Bucket 2 — Contradictory / mislabeled fixtures (intentionally NOT fixed)

The suite ships fixtures whose `@verdict` contradicts a sibling or the
modern spec. The interpreter resolves these toward the modern/positive
reading; "fixing" the negative would just flip which side fails (a wash).
**Do not reject these without re-checking the whole gate.**

- **V4-vs-V5 parametrization** (`050401_formal_parameters` ×8,
  `050402_actual_parameters` ×6): `in/out` timer & port parameters are
  *rejected* by `NegSem_05040103/04_*` but *accepted* by the positive
  `Sem_05040101_026/027/030/031` (identical construct). V5 allows them; we
  allow them. The rule is already coded-but-disabled in
  `ttcn3/semantic/parametrization.go` (`checkParamList`, `_ = dirKind`).
- **`Sem_5010206_Casting_001` vs `OfOperator_001`**: as predicted here,
  implementing the `=>` / `of` class operators correctly (2026-08-04)
  fixed `Casting_001` and broke `Sem_5010205_OfOperator_001`, which is
  mislabelled `@verdict pass reject` over a valid positive body and was
  only matching because `of` used to error. A wash on the corpus, kept
  because the operators are now right.
- **`NegSem_2707_OptionalAttributes_002`**: ETSI's own comment says it is
  not actually forbidden.

---

## Bucket 3 — Risky semantic rejections (would regress positives)

~Several reject→pass clusters want the IUT to *reject* referencing an
uninitialised / `*` value. The loopback model deliberately returns
Undefined for unmodelled/uninitialised reads to keep positive tests
running; erroring would regress many.

- `150603_referencing_record_of_and_set_elements` ×5 — reference
  uninitialised / AnyValueOrNone record-of element.
- `060207_arrays` ×3 — reference uninitialised array element.
- `060212_addressing_entities_inside_sut` ×3, `060215_map_types` ×4,
  `1508_template_restrictions` ×7 (omit/present restriction violations),
  `0901_communication_ports` ×7 (illegal port-type member usage).

These need a precise, narrowly-scoped semantic check per cluster that
fires only on the genuinely-invalid construct — each is its own small,
carefully-gated analysis pass, not a blanket "error on Undefined".

---

## Suggested order

**1a**, **1b** and **1c** are done (`060302_010`, the file 1c was waiting
on, landed with the external-function registry). **1d** is done too: all
four items it listed as unsafe were closed on 2026-08-04, three of them by
rejecting the framing rather than the feature. What remains:

**Engine convergence is done** (2026-08-03) — see "One engine" above.
Both interleave fallbacks and the second, mis-gated altstep fallback are
closed, the approximate engine and the profile toggle are deleted, and
the corpus never moved: every step held 4798 / 4948 with zero per-file
changes. Deleting the blocking-body fallback needed no branch-suspension
machinery, because measurement showed that fallback changed no verdict.

What remains:

1. **Execute `control {}` as a program — largely DONE (2026-08-03).**
   `RunControlWith` runs a module's control part, `execute(TC(args))`
   runs a testcase and yields its verdict, and the module's verdict is
   the worst over the testcases the control part actually ran (ETSI 26.2).
   The harness routes a module through its control part only when that
   part decides something a single-testcase run cannot reproduce —
   several executes, or one carrying a timeout or host operand — so the
   overwhelmingly common `control { execute(TheOnlyTestcase()); }` keeps
   the direct path. +5 fixtures, 0 regressions.

   Two corrections to the scoping this item was originally written from.
   The "24 files whose control part is the only executable thing" are
   **not** a clean slice: every one of them carries an ETSI `noexecution`
   directive, as does all but one of the 207 `parse-only` files. Executing
   them would override an explicit instruction not to, so the
   real-execution rate is effectively not growable through this lever at
   all — the gain here is match rate.

   What is left in this area:
   - **Control-level timers, alts and defaults.** A control part can
     start timers and activate defaults that call `execute`
     (Sem_2601_ExecuteStatement_010); the control scope has no
     TestcaseExec, so those do nothing today.
   - Module-qualified identifiers in a control body
     (Sem_08020305_ImportingAllDefinitionsOfAModule_004) and
     `testcasename()` returning empty under the control path.
   - `Sem_2601_ExecuteStatement_003` is **done**: a deadlock now reports
     itself as `error`, which is what the fixture asked for.
2. **Finish the procedure-communication round trip.** 47 of the 74 files
   the correction exposed, and the four `TestStrictSched_*` tripwires. The
   bodies now run; what still fails is a client/server pair completing a
   call/reply, which is matching and routing. This is the largest single
   block of honest misses in the suite and the highest-value item here.
3. **The three newly-found defects** under "The correction", each with a
   tripwire test carrying its diagnosis: the injected message that never
   reaches a driver-bound PTC (user-facing, the C/C++ test-port path, so
   arguably ahead of item 2), the stopped `alive` component that cannot be
   restarted, and interleave not suspending a blocked branch.
4. **Bucket 3 clusters**, one precise, narrowly-scoped analysis pass at a
   time. 102 of the 178 real misses expect `reject`, so the remaining
   match-rate does live here — but see the 2026-06-17 triage above: none of
   it is low-hanging, spread across 23 clusters, and matching-core changes
   regress easily.
5. **Wire `runtime/codec`.** Filed here rather than under real execution:
   it is worth only ~2 real-execution conversions but up to 13 match-rate
   fixes, all of them the `22_communication_operations` `reject->pass`
   cluster that needs decoding to actually *fail* on malformed input,
   which the current placeholder cache can never do. The codecs exist and
   are tested; the work is the bridge, and the risk is that a real codec
   produces different bytes than the placeholder for the files that
   currently match through it. Note that `encvalue_o` / `decvalue_o` now
   produce real bytes for bitstrings and records (1d) — that is a handful
   of shapes, not the bridge.
6. **Do not** pursue cross-module symbol resolution via `ttcn3/types` as a
   conformance lever. `TypeOf` handles literals and operators only, so
   this means writing a symbol table from scratch, while the existing
   flat-directory hack in `conformance.go` already gets 454 of the 555
   nominally-unresolvable files executing and matching. Measured return:
   one file.
7. Leave **Bucket 2** as-is unless the suite revision is reconciled.

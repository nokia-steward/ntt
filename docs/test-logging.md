# Test logging (TCI-TL)

`ntt exec --log FILE` records every TTCN-3 operation a run performs as a
structured event: each message sent, each arrival in a port queue, each
receive attempt with the template it was matched against, each timer, each
component and port operation, each verdict. Nothing is summarised and
nothing is free text; a run's log can be queried, compared with another
run, or read by other TTCN-3 tooling after the fact.

The events are the ones the TTCN-3 standard itself defines for this: the
TCI-TL logging interface of ETSI ES 201 873-6 (V4.12.1), clause 7.3.4.1.
The file format is the standard's XML mapping of those events, Annex B, and
a log `ntt` writes validates against the Annex B schemas.

```sh
ntt exec --log run.xml suite/            # the standard's XML format
ntt exec --log run.jsonl suite/          # the same events, one per line
ntt exec --log run.log --log-format=jsonl suite/
```

One file covers the whole run, every testcase in order. Logging is off
unless `--log` is given, and costs nothing then.

## What a log looks like

An MTC sending `"ping"` to an echo PTC and matching the reply, abridged, as
JSON Lines (tag, then the attributes and child elements Annex B defines):

```
tliCCreate      mtc   comp=echo  name=echo  alive=false
tliPConnect     mtc   port1=mtc:p  port2=echo:p
tliCStart       mtc   comp=echo  name=M.echo
tliMSend_c      mtc   at=mtc:p  to=echo:p  msgValue="ping"
tliMDetected_c  echo  at=echo:p  from=mtc:p  msgValue="ping"
tliAEnter       echo
tliMReceive_c   echo  at=echo:p  msgValue="ping"  msgTmpl="ping"  from=mtc
tliMSend_c      echo  at=echo:p  to=mtc:p  msgValue="pong"
tliMMismatch_c  mtc   at=mtc:p  msgValue="pong"  msgTmpl="nope"  diffs=.
tliMReceive_c   mtc   at=mtc:p  msgValue="pong"  msgTmpl="pong"  from=echo
tliSetVerdict   mtc   verdict=pass  reason="echoed"
tliCTerminated  echo  verdict=none
tliTcTerminated mtc   tcId=M.tc  verdict=pass  reason="echoed"
```

Every event carries a timestamp, the component that produced it, and where
the test specification performs the operation (file and line).

## What is logged

112 of the 125 TCI-TL operations, covering message- and procedure-based
testing end to end, behaviour, variables, codecs and the control part:

| Area | Operations |
|---|---|
| Testcase | `tliTcStart`, `tliTcStarted`, `tliTcStop` (an `execute()` timeout), `tliTcTerminated` |
| Verdict and log | `tliSetVerdict`, `tliGetVerdict`, `tliLog` |
| Messages to components | `tliMSend_c` (and `_MC` multicast, `_BC` broadcast), `tliMDetected_c`, `tliMReceive_c`, `tliMChecked_c`, `tliMMismatch_c`, `tliCheckedAny_c`, `tliCheckAnyMismatch_c` |
| Messages to the system | `tliMSend_m` (and `_MC` multicast), `tliMDetected_m`, `tliMReceive_m`, `tliMChecked_m`, `tliMMismatch_m`, `tliCheckedAny_m`, `tliCheckAnyMismatch_m` |
| Timers | `tliTStart`, `tliTStop`, `tliTRead`, `tliTRunning`, `tliTTimeoutDetected`, `tliTTimeout`, `tliTTimeoutMismatch` |
| Components | `tliCCreate`, `tliCStart`, `tliCCall`, `tliCCallTerminated`, `tliCStop`, `tliCKill`, `tliCRunning`, `tliCAlive`, `tliCDone`, `tliCDoneMismatch`, `tliCKilled`, `tliCKilledMismatch`, `tliCTerminated` |
| Ports | `tliPConnect`, `tliPDisconnect`, `tliPMap`, `tliPUnmap`, `tliPStart`, `tliPStop`, `tliPHalt`, `tliPClear` |
| Behaviour | `tliSEnter`, `tliSLeave` (testcases, functions and altsteps, with their parameters and result, and the control part) |
| Values | `tliVar` (an assignment), `tliModulePar` (a module parameter read), `tliEvaluate` (a `@lazy` or `@fuzzy` parameter evaluated), `tliRnd`, `tliMatch`, `tliMatchMismatch` |
| Codecs | `tliEncode` (`encvalue`), `tliDecode` (`decvalue`) |
| Alt and defaults | `tliAEnter`, `tliALeave`, `tliANomatch`, `tliARepeat`, `tliADefaults`, `tliAWait`, `tliAActivate`, `tliADeactivate` |
| Procedure calls | `tliPrCall_c` (and `_MC`, `_BC`), `tliPrCall_m`, `tliPrGetCallDetected`, `tliPrGetCall`, `tliPrGetCallChecked`, `tliPrGetCallMismatch` |
| Procedure replies | `tliPrReply_c` (and `_MC`, `_BC`), `tliPrReply_m`, `tliPrGetReplyDetected`, `tliPrGetReply`, `tliPrGetReplyChecked`, `tliPrGetReplyMismatch` |
| Procedure exceptions | `tliPrRaise_c` (and `_MC`, `_BC`), `tliPrRaise_m`, `tliPrCatchDetected`, `tliPrCatch`, `tliPrCatchChecked`, `tliPrCatchMismatch`, `tliPrCatchTimeoutDetected`, `tliPrCatchTimeout` |
| Control part | `tliCtrlStart`, `tliTcExecute`, `tliCtrlTerminated` |
| The log itself | `tliInfo`: an empty log, or a failure of the logging (see below) |

A port mapped to the system logs the `_m` forms, whose peer is an SUT
address; a port connected to other components logs the `_c` forms. The
receiving operations above exist in both forms, and so do call, reply and
raise.

Procedure events carry the signature and its parameters, the reply or
exception value, and the parameter and value templates a receiving
operation matched against. Every event of a call — the call, its arrival
at the server, the getcall, the reply, its arrival back, the getreply — is
logged in that order.

Control-part events are produced by the control part itself, not by a test
component; they appear as the component `control`. `ntt exec` runs
testcases directly, so a log it writes has no control events; they come
from running a control part through the library (`RunControlWith`). A
control part logs its start and end, its own `tliSEnter` and `tliSLeave`,
and each testcase it executes; the functions it calls, its assignments and
the module parameters it reads are not logged.

**Not logged:**

- `tliAction`: the engine does not implement `action()`.
- The control part's parameterised and result forms and its stop
  (`tliCtrlStartWithParameters`, `tliCtrlTerminatedWithResult`,
  `tliCtrlStop`): a control part has no parameters and no result, and the
  engine has no operation that stops one.
- The parameterised map forms (`tliPMapParam`, `tliPUnmapParam`): the
  engine does not evaluate a `param` clause, which only a port driver would
  use, so the operation is logged as `tliPMap` or `tliPUnmap`. Logging the
  parameters would mean evaluating them for the log alone.
- The multicast and broadcast `_m` forms of call, reply and raise, and
  `tliMSend_m_BC`: a procedure operation on a mapped port is logged in its
  unicast `_m` form, without its addresses.

Some operations are logged in a narrower sense than the standard's:

- `tliVar` is logged for an assignment statement, with the variable's whole
  new value (for `r.f[1] := x`, all of `r`). A declaration's initial value,
  a value redirect (`-> value v`) and an `out` parameter written back also
  change a variable, and are not logged as `tliVar`.
- `tliModulePar` is logged when a module parameter is read by name inside
  a testcase, with the module that declares it.
- `tliEncode` and `tliDecode` log what the engine's codec did. It has no
  general encoder: `encvalue` returns a placeholder that a later
  `decvalue` in the same testcase recognises, and the log shows that
  placeholder as the encoded message. The executor knows no codec name, so
  none is given.
- `tliTTimeoutDetected` and `tliPrCatchTimeoutDetected` are logged
  together with the timeout they detect: the executor finds an expired
  timer when it looks at it.
- Also not yet logged: `all timer.stop`, and `disconnect` / `unmap`
  without arguments or over `all component`. An imported behaviour started
  or activated by its unqualified name is reported in the current module.

A PTC whose body the engine models rather than runs — a body that waits on
a timer and uses no port — gets its `tliCStart` and no `tliCTerminated`:
its statements are not executed, and the log does not pretend they were
(see Known issues in the changelog).

These policies keep a log readable without dropping information:

- A mismatch is logged once per message and receiving operation. On the
  real clock an alt re-checks its guards on a short polling backstop; each
  pass would otherwise log the same message failing the same clause again.
  A different message at the head of the queue is logged again, and so is
  every message a `trigger` discards.
- `tliANomatch`, `tliADefaults` and `tliAWait` are logged once per stretch
  of alt rounds that match nothing, for the same reason, and so is a
  `done`, `killed` or `timeout` guard that does not match.
- More generally, an alt that waits evaluates its guards and defaults again
  each time it looks — reading the same module parameter, entering the
  same default altstep. What such a scan repeats is logged once per alt
  round; what it finds new — a receive that now matches, a component whose
  state has changed — is logged when it happens. So the two clocks log the
  same operations.

## Standard compliance

- **Validated against the normative schemas.** The package tests validate
  logs with `xmllint` against the Annex B XSDs, and the whole ETSI
  conformance corpus has been run with logging on: every event valid, and
  every verdict identical to a run with logging off. The XSDs are ETSI's
  and are not shipped; `runtime/tl/gen/README.md` rebuilds them from the
  standard.
- **Where Annex B and clause 7 disagree, the log follows Annex B**, since
  that is what it is validated against: element names such as
  `transmission-failure`, the behaviour of `tliCStart` in an element called
  `name`, and others. `runtime/tl/doc.go` lists each one.
- **The published schemas have errors.** As printed they do not compile.
  `runtime/tl/gen/extract_xsd.py` applies seven minimal corrections, each
  reported as it is applied and listed in `runtime/tl/doc.go`.
- **Choices the standard leaves to the tool:** timestamps are microseconds
  since the Unix epoch, declared in the log header; under the virtual clock
  they are the testcase's start plus the virtual time elapsed. A log's
  timestamps never decrease: a virtual-clock testcase runs ahead of the wall
  clock, so what comes after it — the next testcase, or its control part —
  starts from its last event, not from the wall clock. On the real clock
  components time their events concurrently; an event that reaches the
  log after a later one is given that one's timestamp. Scalar values
  are written in TTCN-3 notation without literal decoration (`5`, `pass`,
  `0A1B` for an octetstring).
- **The log never changes the test.** Values are logged as the engine
  evaluated them; no clause is evaluated a second time for the log, and no
  port driver is created to find out whether a port is mapped. If logging
  itself fails — a value it cannot convert, a logger that errors — the
  failure is recorded in the log as a `tliInfo` event beginning "test
  logging failed", and the test goes on exactly as with logging off.
- **What the executor does not know is left out, not invented.** The
  runtime does not always know a value's declared type, so the optional
  `type` attribute is omitted and the value's element is inferred from its
  content; a value of no known type (an omitted field, a `?` inside a
  structure) is an `anytype`, the schema's type-neutral element. `tliMDetected_m` carries the encoded message, and the executor
  holds decoded values: the octets are given only when the payload is
  itself an octetstring.

## Comparing two runs

`ntt log diff` compares two logs, testcase by testcase and, within a
testcase, component by component, and reports where each component's actions
first differ. Like `diff(1)`, it exits 0 when the runs agree, 1 when they
differ and 2 when a log cannot be read. Its first use is checking
that a suite behaves the same on both clocks:

```sh
ntt exec --log virtual.jsonl suite/
ntt exec --live --log live.jsonl suite/
ntt log diff virtual.jsonl live.jsonl
```

```
M.tc_echo: same (18 events)
M.tc_restart: differs
  PTC, action 1:
    virtual.jsonl: (nothing: the component did no more)
    live.jsonl: tliTStart [PTC, line 10] timer=t dur=1
  mtc, action 8:
    virtual.jsonl: tliTcTerminated [mtc] tcId=M.tc_restart verdict=none
    live.jsonl: tliTcTerminated [mtc] tcId=M.tc_restart verdict=pass
2 testcases: 1 same, 1 differ
```

Either format may be given, in any combination. What is compared, and what
is not:

- **Per component.** Components legitimately interleave differently in real
  time, so each component's own sequence is compared, not the interleaved
  log. Components are matched by the name they were created with —
  numbered in creation order when several share one, so two workers both
  created as `"w"` are told apart, and named under their creator
  (`a/w`) when a component other than the MTC created them, since two
  components may create theirs in either order — and the MTC, the system
  and a control part by their roles. Their numeric ids, which two runs may assign
  differently, are not compared, and a value that refers to a component is
  compared as that component.
- **Actions, not arrivals.** Some events record when something arrived
  relative to what a component was doing, and differ between correct runs:
  arrivals (`tliMDetected_*`, `tliPrGetCallDetected_*` and the other
  `Detected` forms), mismatches, which depend on what was at the head of a
  queue or whether a component or timer had finished yet (every
  `Mismatch` form), and alt rounds that found nothing (`tliANomatch`, `tliADefaults`,
  `tliAWait`). These are left out. So is the elapsed time a timer read
  reports, which measures the clock.
- **Everything else must agree:** every send and receive with its value and
  template (a broadcast's or multicast's destinations as a set), every call, reply and exception, every verdict and reason,
  timer, component and port operation, every alt entered and left, and
  where in the test specification each was performed: the file, by its
  base name since it may sit at different paths, and the line.
- **Either format, in any combination.** XML 1.0 cannot carry every
  character; most control characters become U+FFFD in an XML log. When
  either log is XML, text is compared as XML holds it, so a run compares the
  same with its own two logs, and two values differing only in such
  characters are not told apart.
- **A log cut short** — a killed run's, ending mid-event — is compared up to
  its last complete event, with a warning.

A component whose behaviour outlives its testcase — still running when the
testcase has ended — would otherwise appear to act in the next one. What it
does after its testcase's end is logged as a `tliInfo` naming the operation,
the component and the testcase, and the comparison groups it with the other
events outside any testcase, wherever it falls in the log. The tliInfo
carries a summary of what the component did.

## Profiling a logged run

`ntt log profile` computes from a log the per-port profile that
`ntt exec --profile` measures while a run is live (see
[live testing and profiling](live-testing-and-profiling.md)), and reports it
the same way, as a table or with `--format=json`:

```sh
ntt exec --live --log run.xml suite/
ntt log profile run.xml
```

```
profile "run": pass (1 cases in 68.913ms)

M.tc  [pass]  68.913ms
  port                     sent     recv       recv/s        min        p50        p90        p99
  echo:p                     50       50        725.6      125µs      273µs      370µs    1.811ms
  p                          50       50        725.6      113µs      247µs      378µs    1.215ms
```

The model is the live profiler's. A port's sends and receives are its
`tliMSend_*` and `tliMReceive_*` events, and a send answered by a receive on
the same port, before the next send, is one latency sample. Throughput is
receives per second between the testcase's `tliTcStart` and
`tliTcTerminated`. A port of a component other than the MTC is shown as
`component:port`. Over the same run the two agree: the same counts, and
latencies within the few microseconds between where the engine takes its
time and where it logs the event. Procedure-based operations are not
counted, as the live profiler does not count them.

A log keeps what a live profile summarises, so a run can be profiled after
the fact, and again differently, without being run again. Timestamps are the
log's, in microseconds; a run on the virtual clock gives virtual time.


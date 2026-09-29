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

86 of the 125 TCI-TL operations, covering message- and procedure-based testing
end to end, and the control part:

| Area | Operations |
|---|---|
| Testcase | `tliTcStart`, `tliTcStarted`, `tliTcStop` (an `execute()` timeout), `tliTcTerminated` |
| Verdict and log | `tliSetVerdict`, `tliGetVerdict`, `tliLog` |
| Messages to components | `tliMSend_c` (and `_MC` multicast, `_BC` broadcast), `tliMDetected_c`, `tliMReceive_c`, `tliMChecked_c`, `tliMMismatch_c` |
| Messages to the system | `tliMSend_m`, `tliMDetected_m`, `tliMReceive_m`, `tliMChecked_m`, `tliMMismatch_m` |
| Timers | `tliTStart`, `tliTStop`, `tliTRead`, `tliTRunning`, `tliTTimeout` |
| Components | `tliCCreate`, `tliCStart`, `tliCCall`, `tliCCallTerminated`, `tliCStop`, `tliCKill`, `tliCDone`, `tliCKilled`, `tliCTerminated` |
| Ports | `tliPConnect`, `tliPDisconnect`, `tliPMap`, `tliPUnmap` |
| Alt and defaults | `tliAEnter`, `tliALeave`, `tliANomatch`, `tliARepeat`, `tliADefaults`, `tliAWait`, `tliAActivate`, `tliADeactivate` |
| Procedure calls | `tliPrCall_c` (and `_MC`, `_BC`), `tliPrCall_m`, `tliPrGetCallDetected`, `tliPrGetCall`, `tliPrGetCallChecked`, `tliPrGetCallMismatch` |
| Procedure replies | `tliPrReply_c` (and `_MC`, `_BC`), `tliPrReply_m`, `tliPrGetReplyDetected`, `tliPrGetReply`, `tliPrGetReplyChecked`, `tliPrGetReplyMismatch` |
| Procedure exceptions | `tliPrRaise_c` (and `_MC`, `_BC`), `tliPrRaise_m`, `tliPrCatchDetected`, `tliPrCatch`, `tliPrCatchChecked`, `tliPrCatchMismatch`, `tliPrCatchTimeout` |
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
from running a control part through the library (`RunControlWith`).

**Not logged yet:** the control part's parameterised and result forms and
its stop (`tliCtrlStartWithParameters`, `tliCtrlTerminatedWithResult`,
`tliCtrlStop`); the multicast and broadcast `_m` forms of send and call; codecs (`tliEncode`, `tliDecode`); `match` (`tliMatch`,
`tliMatchMismatch`); function and altstep entry (`tliSEnter`, `tliSLeave`);
and `tliVar`, `tliModulePar`, `tliRnd`, `tliEvaluate`, `tliAction`, `tliCRunning`, `tliCAlive`, the `Mismatch` forms of done,
killed and timeout, `tliPStart`/`Stop`/`Halt`/`Clear`, the parameterised
map forms, and the `check(any)` forms. Also not yet logged: `all
timer.stop`, and `disconnect` / `unmap` without arguments or over `all
component`. An imported behaviour started or activated by its unqualified
name is reported in the current module.

A PTC whose body the engine models rather than runs — a body that waits on
a timer and uses no port — gets its `tliCStart` and no `tliCTerminated`:
its statements are not executed, and the log does not pretend they were
(see Known issues in the changelog).

Two policies keep a log readable without dropping information:

- A mismatch is logged once per message and receiving operation. On the
  real clock an alt re-checks its guards on a short polling backstop; each
  pass would otherwise log the same message failing the same clause again.
  A different message at the head of the queue is logged again, and so is
  every message a `trigger` discards.
- `tliANomatch`, `tliADefaults` and `tliAWait` are logged once per stretch
  of alt rounds that match nothing, for the same reason.

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
  starts from its last event, not from the wall clock. Scalar values
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

A log is the natural way to check that the same testcase behaves the same
on the virtual clock and on the real clock (`--live`), and it is how such
differences have been found. Compare each component's own operations —
events grouped by the component that produced them — rather than the
interleaved log, because components legitimately interleave differently in
real time.

Some events record when an arrival happened relative to what a component
was doing, not what the component did, and differ between correct runs:
arrivals (`tliMDetected_*`), alt rounds that found nothing (`tliANomatch`,
`tliADefaults`, `tliAWait`), and mismatches, which depend on which message
was at the head of a queue. Leave those out and the rest — every send and
receive with its value, every verdict, timer, component and port operation,
every alt entered and left — must agree.

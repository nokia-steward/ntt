# How far can this engine be trusted?

This page is for an engineer deciding whether to rely on `ntt`'s TTCN-3
engine, and it is about the evidence rather than the feature list. For what
exists at all, see the [feature matrix](ntt-vs-titan.md); for the work queue,
[remaining work](conformance/remaining-work.md); for per-push numbers, the
[conformance dashboard](conformance/index.html).

Short answer, as of 2026-08-10: **4747 of 4925 ETSI conformance files match
their expected outcome, 96.39%.** The number went down 1.34 points that day,
deliberately, and the reason it went down is the most informative thing on
this page.

## One engine

There is a single evaluator: a cooperative discrete-event scheduler over a
virtual clock. Components are real goroutines that hold a token, so exactly
one runs at a time and the interleaving is deterministic. Time advances only
at quiescence - when every component is parked - and then jumps to the
soonest timer deadline. A fixture with a 3-second guard timer costs
microseconds and produces the same result every run.

The earlier "approximate" evaluator, which preferred whichever alt branch
produced a verdict, is deleted, along with the profile toggle that selected
between them. There is no mode in which the engine guesses.

## How correctness is measured

Every file in the ETSI suite carries an `@verdict` annotation. The harness
runs the file and compares. A positive fixture must reach the verdict it
declares; a negative fixture (`@verdict pass reject`) must be refused, by
the parser, the semantic analyzer, or at runtime.

Three rules keep that honest:

- **Zero per-file regressions.** Not "the rate went up" - no individual file
  may go from matching to not. `diff_runs.py` compares two JSON reports file
  by file, because a rate can hold steady while ten files break and ten
  others start working.
- **CI gates every commit** against the recorded baseline at
  `--regress 0.5`.
- **Anything that cannot clear the gate is reverted, not absorbed.** On
  2026-08-10 a change that was defensible in principle - running timer-only
  PTC bodies for real, which the virtual clock now makes possible - measured
  -7 files and +0. It is out of the tree, and the diagnosis is recorded in a
  test.

One caveat about a number you will see: the **real-execution rate**
(currently 49.58%) counts only files whose verdict came from executing a
testcase, not from a parse or semantic rejection. It cannot approach 100%.
2200 of the files expect `reject`, and a completed run always yields a TTCN-3
verdict, never the string `reject` - so the ceiling is about 55%. Reading it
as "half the suite doesn't run" is wrong.

## The correction

Two mechanisms in the engine existed so that fixtures would pass.

The scheduler, on reaching a state where every component was blocked and no
timer could fire, released everyone so their blocked alts concluded as
though nothing had matched. And a testcase that never called `setverdict`
was given `pass`.

Both look reasonable in isolation. Together they were holding up **74
files**. Worse, some of those files were not testing their subject at all.
`Sem_2204_the_check_operation_025` sets `pass` after its alt whether or not
`check(getcall)` ever matched - so 35 files nominally covering the `check`
operation were reporting on an operation that never ran.

The same was true of six of our own Go tests, which is the part that should
worry a reader most, because those tests were the reason to believe the
engine worked. Four of them exercised a client/server pair over a procedure
port. The PTC bodies never ran at all - not the server's `getcall`, not its
safety timer, not the client's `call` - and the tests passed anyway, because
nothing set a verdict and the engine supplied one.

So both mechanisms were deleted, in two commits, each with its own
measurement:

| Change | Effect |
| --- | --- |
| A deadlock reports `error` instead of releasing blocked alts | -51, +7 |
| An undeclared verdict stays `none` (ETSI 22.4.1) | -28, +1 |

97.73% became 96.39%. The gains are worth noting too: six of the seven in
the first row are negative fixtures that expect a rejection and now get one.

The engine is now honest about those 74 files, and three of them have since
been earned back properly by making procedure responder bodies actually run.
Every one of the six vacuous Go tests is still in the suite, rewritten to
assert what the engine really does and naming what must be restored when the
gap closes. A green test that asserts nothing is worse than a red one.

## What removing the scaffolding found

Defects that had been sitting behind green tests:

- **An injected message never reaches a PTC whose port has an external
  driver bound.** This is the path a real C or C++ test port uses, so it
  matters more than any conformance fixture. Diagnosed: the daemon body
  runs, the message lands in the queue under exactly the key its receive
  reads, it is still there unconsumed when the testcase ends, and the alt
  re-polls every 2ms throughout. Not a lost wake-up, not a template
  mismatch.
- **A stopped `alive` component cannot be restarted**, which ETSI 21.3.3
  requires. Two layers: `start` does not clear the component's `done` flag,
  so a following `comp.done` is satisfied by the previous run; and with that
  fixed, the re-forked goroutine still never gets scheduled.
- **`interleave` does not suspend a blocked branch**, so two mutually
  dependent branches deadlock instead of completing.

Each has a test carrying its diagnosis, so the next attempt starts from what
was already learned.

## An ending is a value, not a silence

"The engine does not guess" has an outward-facing twin that is easy to
violate while honouring the first: **when nothing happened, say so with a
value the suite can match on.** A testcase cannot distinguish an absence
from a slow answer, and a harness that reports absence by simply not
delivering anything forces every failure through the same channel — the
guard timer — where they all look alike.

It is the same rule each time it comes up, and it has come up repeatedly:

- A **dial failure** on the TCP port fails the `map` operation, naming the
  address, rather than leaving a mapped port that receives nothing.
- A **failed HTTP request** arrives as a `TransportError` with a matchable
  `reason` (`refused` and `reset` are retryable; `timeout` means wedged),
  rather than as no response at all. The reason is an enumeration, not the
  underlying error text, because that text is diagnostic and not API.
- A **scheduler deadlock** is an `error` verdict, not a shrug.
- An **undeclared verdict** stays `none` instead of being coerced to `pass`.
- A **catch-all that also holds the commonest transient failure** is not a
  catch-all but a hole: after `reset` was split out of `other`, `other`
  means "unknown" again, and a suite can branch on it.

The test for a new inbound path is therefore: *can a suite tell "it ended",
"it failed", and "it is still going" apart without waiting for a timer?* If
not, the missing distinction is a value that has not been given a name yet.
This is why the HTTP port grew a second inbound type rather than a sentinel
status, and it is the first question to ask of any streaming or push
mechanism, where "the stream closed" and "the stream is quiet" are otherwise
indistinguishable.

## What this means in practice

The engine is dependable for single-component testcases: templates and
matching, the type system, timers, message-based communication with the
system under test, the control part, and the object-oriented additions. That
is the bulk of the suite and of the passing files.

Be careful with **multi-component procedure-based communication**. A
client/server pair over a procedure port does not reliably complete a
call/reply round trip, and 47 of the honest misses are exactly that. Be
careful with **external test ports** until the inject defect above is fixed.

The largest remaining block is the 102 files expecting a rejection we do not
produce: static checks a compiler-based tool performs and an
interpreter-first one has to be taught. That is a known, bounded gap rather
than a soundness problem.

## Why the drop is the point

Any project can raise a conformance number. We had, partly, by writing
engine behaviour that made fixtures pass - and the tests that were supposed
to catch that had been captured by the same mechanism.

The measurement was corrected before the defects were fixed, deliberately in
that order, and the drop was published rather than absorbed. What is left is
a number that means something: 96.39% of the suite matches for reasons that
survive inspection, and the misses are enumerated in
[remaining work](conformance/remaining-work.md) with a reason each.

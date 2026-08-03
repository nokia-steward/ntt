# Remaining Conformance Work

State as of 2026-07-28: **4798 / 4948 matched (97.42%)**, 23 skipped
(inconclusive / no-verdict), **127 real misses** (was 4790 / 97.26% on
2026-07-06 and 4788 / 97.22% on 2026-06-17). The gate now measures the
**strict** engine — see "Strict engine" below. Baseline and the full miss
inventory live in
[`testdata/conformance-baseline.json`](../../testdata/conformance-baseline.json)
and [`current-misses.json`](current-misses.json); regenerate with
[`refresh_artifacts.py`](refresh_artifacts.py).

The incremental "one clean fix per slice" phase is complete: the easy
positive-test bugs have been closed. What remains does **not** yield
isolated low-risk commits. It falls into three buckets below.
Every change must still pass the full conformance run (`--regress 0.5`,
0 per-file regressions) and an external test-port smoke suite
(17 pass / 0 fail / 2 expected inconc).

## Strict engine (2026-07-28)

Execution now defaults to the **strict** engine: a cooperative
discrete-event scheduler with a virtual clock, replacing the legacy
"approximate" evaluator's verdict-preferring heuristic. The legacy engine
remains available via `--approximate` and is scheduled for deletion.

This closed the two features that this document previously listed as the
main path forward — **1a** (strict procedure-payload matching) and **1b**
(async multi-PTC echo) — because concurrent PTCs now genuinely fork,
interleave deterministically and block on real matches instead of being
modelled. Both sections are marked DONE below.

The suite match rate is only part of the picture: the harness also
reports a **real-execution** rate (files whose verdict came from actually
executing the testcase rather than from a parse/semantic rejection),
currently **2498 files, 50.72%**. Growing that number, not the match
rate, is the meaningful measure of remaining dynamic-semantics work.

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
`Sem_1400_procedure_signatures_004` all match now;
`Sem_200501_the_default_mechanism_008` is the one fixture from this
cluster still missing. Original analysis retained below.

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

- **External functions** (`Sem_160103_external_functions_001/002`):
  **gaming — do not implement.** 003/004/005 pass because they only
  *declare* the function and `setverdict(pass)`; 001 calls
  `xf_…_001()` and asserts `== 1`, 002 asserts `xf_…_002(5) == 6`
  (input+1). Those returns are host-defined and not derivable from the
  signature, so passing them means hard-coding magic values keyed to the
  test name (the bodyless `external function` binds to Undefined at
  `interpreter.go` FuncDecl). The only legitimately special-cased
  external function is `matchFile` (a real utility). This also blocks
  `Sem_060302_structured_types_010` (1c), which calls an external
  function. Leave unless a real host-function plugin mechanism lands.
- **RAW `encvalue_o` length** (`Sem_160102_predefined_functions_107/110`):
  implementation-specific codec bytes. `110` asserts the exact octets
  `'0300000060'O` (a 4-byte little-endian length prefix + the
  left-aligned `'011'B`); `107` needs `decvalue_o` to return the
  failure code `2` for a truncated octet. The loopback model has no real
  RAW codec (it round-trips via `encodeCache`), so matching a specific
  encoder's byte layout is fragile / gaming-adjacent. Needs a real RAW
  codec, not a placeholder.
- **Template field building / union alt access**
  (`Sem_150605_Referencing_union_alternatives_002`,
  `Sem_1511_ConcatenatingTemplatesOfStringAndListTypes_013`): the one
  legitimate feature here, but the fix sits on the load-bearing matching
  core. `150605_002` reduces to a single mechanism — member /
  union-alternative access on a `?` wildcard should propagate the
  wildcard (`(? : My_Union).u1` is `?`), so `ispresent(m.b.u1)` is true.
  Implementing it (mirror the existing IndexExpr `left == Any` rule into
  the SelectorExpr path) fixed the target **but regressed 6**
  (`Sem_07010802_ischosen_operator_001`,
  `Sem_150602_ReferencingRecordAndSetFields_003/004`,
  `Sem_160102_predefined_functions_022`, `Sem_27010200_general_015`) for
  +1, net -5 — reverted. Root cause: at `left == runtime.Any` the type
  is gone, so a union-alternative access can't be told apart from a
  record-field access, and member-access-on-wildcard returning Undefined
  is load-bearing for `ischosen` and field referencing. A safe fix needs
  static type context (is the receiver a union?) at the SelectorExpr,
  not a blanket wildcard-propagation.
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
- **`Sem_5010206_Casting_001` vs `OfOperator_001`**: implementing the
  `=>` / `of` class operators correctly makes `OfOperator_001` (mislabeled
  `@verdict pass reject` on a valid positive body) fail. Wash.
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

**1a** and **1b** are done (strict engine). **1c** is done apart from the
external-function-blocked `060302_010`. **1d** was investigated and still
yields no safe commit (gaming / codec-specific / matching-core
regression — see above). What remains:

1. **Engine convergence** — make the library default strict, then delete
   the approximate engine, the `SemanticsProfile` toggle and the
   `--approximate` / `--profile` flags, and rebaseline. This is cleanup
   of transitional scaffolding, not new coverage, and it is the
   prerequisite for having a single engine.

   *Partly done.* `interleave` no longer falls back to the legacy
   evaluator merely because defaults are active: `runDefaults` reports a
   default that actually took a branch, which is the signal needed to
   leave the interleave (20.5), so the snapshot evaluator now handles
   that case directly. `@nodefault` is honoured on a plain `alt` too,
   which the strict evaluator previously ignored. The one remaining
   fallback is an interleave branch body that may itself block: taking it
   needs cooperative suspension at the blocking point and resumption of a
   sibling, which the snapshot evaluator does not model. Closing that is
   what removes the last caller of `evalAltStmtBestEffort` from the
   strict path.
2. **Bucket 3 clusters**, one precise, narrowly-scoped analysis pass at a
   time. `reject->pass` is now 107 of the 150 unmatched files, so this is
   where the remaining match-rate lives — but see the 2026-06-17 triage
   above: none of it is low-hanging, and matching-core changes regress
   easily (cf. the 150605 attempt in 1d).
3. **Real-execution depth over match rate** — cross-module symbol
   resolution with `ttcn3/types`, wiring `runtime/codec`, executing
   `control {}` as a program, and a host binding for external functions.
   These grow the 50.72% real-execution rate and unblock 1c/1d leftovers;
   several barely move the match rate.
4. Leave **Bucket 2** as-is unless the suite revision is reconciled.

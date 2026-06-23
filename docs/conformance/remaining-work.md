# Remaining Conformance Work

State as of 2026-06-17: **4788 / 4948 matched (97.22%)**, 23 skipped
(inconclusive / no-verdict), **137 real misses**. Baseline and the full
miss inventory live in [`testdata/conformance-baseline.json`](../../testdata/conformance-baseline.json)
and [`current-misses.json`](current-misses.json); regenerate with
[`refresh_artifacts.py`](refresh_artifacts.py).

The incremental "one clean fix per slice" phase is essentially complete:
the easy positive-test bugs have been closed. What remains does **not**
yield isolated low-risk commits. It falls into three buckets below.
Every change must still pass the full conformance run (`--regress 0.5`,
0 per-file regressions) and an external test-port smoke suite
(17 pass / 0 fail / 2 expected inconc).

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

### 1a. Strict procedure-payload matching (recommended first)
**~8 tests.** `Sem_220301_CallOperation_019/020`,
`Sem_220302_getcall_operation_020/021`, `Sem_2204_the_check_operation_090/094`,
plus part of the `220304_getreply` / `220306_catch` reject clusters.

The loopback model matches procedure receives **leniently** — a
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

`Sem_220301_CallOperation_019/020` additionally need the **signature**
recorded on the envelope (`call`/`reply`/`raise` enqueue without setting
`PortMessage.Signature` at `interpreter/interpreter.go:~8395-8426`) so an
unqualified `getreply`/`catch` inside a blocking `call(S2,…){}` block
matches only S2's reply.

### 1b. Async multi-PTC message echo — attempted, blocked on the alt core
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

The cheap single-mechanism seam is now exhausted: **1c is essentially
done** (184 / 001 / 002 landed; only the external-function-blocked
`060302_010` remains), **1d** was investigated and yields no safe commit
(gaming / codec-specific / matching-core regression — see above), and a
scoped **1a** attempt regressed 8 check tests. What remains all carries
real risk or real scope:

1. **1b — async multi-PTC echo** (6 tests, larger infra): the most
   self-contained *feature* left, but needs a real MTC↔PTC message
   responder.
2. **Bucket 3 clusters**, one precise, narrowly-scoped analysis pass at a
   time — accept these are no longer quick wins (cf. the 150605 attempt
   in 1d: matching-core changes regress easily).
3. **1a — strict procedure matching** only if the coupled from-filter /
   match-gated redirect / standalone-check-default work is done together
   (a dedicated slice, not a single fix).
4. Leave **Bucket 2** as-is unless the suite revision is reconciled.

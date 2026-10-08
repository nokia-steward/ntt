# TTCN-3 V5.1.1 migration — notes for ntt

Source: ETSI TR 104 081 V5.1.1 (2025) "Methods for Testing and
Specification (MTS); Guide for the Migration to TTCN-3 V5;
Release #" — `~/Downloads/MTS-104081v5.1.1v001.docx`.

This is a working memo, not a normative reference. It captures
the V4 → V5.1.1 changes that affect what we should accept,
reject, or warn about in `ntt` going forward, and what to expect
when the ETSI conformance suite is re-generated against V5.

Contact at ETSI: **Matthias Simon** (Nokia delegate). Reach out
via Teams for clarifications on individual items.

## Documents

V5 splits the language across seven ES + two TR:

| Doc | Title |
|-----|-------|
| ES 201 873-1 v5.1.1 | TTCN-3 Core Language |
| ES 201 873-7 v5.1.1 | Using ASN.1 with TTCN-3 |
| ES 201 873-9 v5.1.1 | Using XML schema with TTCN-3 |
| ES 201 873-10 v5.1.1 | TTCN-3 Documentation Comment Specification |
| ES 201 873-11 v5.1.1 | Using JSON with TTCN-3 |
| ES 201 873-12 v5.1.1 | **NEW**: TTCN-3 Extensions (folds in 202 784 advanced parameterisation, 202 785 behaviour types, 203 022 advanced matching, 203 790 OO features, plus procedure-based comm + automatic type + shift/rotate moved out of core) |
| ES 201 873-13 v5.1.1 | TRI + TCI merged (replaces 873-5, 873-6, 202 789) |
| TR 104 873 v5.1.1 | TTCN-3 Semantics Description |
| TR 104 081 v5.1.1 | The migration guide itself |

**Dropped entirely**:
- ES 201 873-3 (GFT — graphical presentation)
- ES 201 873-4 (operational semantics — folded informally into TR 104 873)
- ES 201 873-8 (IDL to TTCN-3 mapping)
- ES 202 781 (configuration + deployment support)
- ES 202 786 (continuous signals)

## Core-language changes that affect ntt's semantic checks

### Moved out of core (now extension-only)

If a fixture uses any of the following and a future test run is
gated against "core V5", we should be ready to either:
(a) flag the fixture as `extension-only`, or
(b) keep treating it as supported but tagged `non-core`.

- **Procedure-based communication** — `signature`, `port ... procedure { ... }`,
  `call`, `getcall`, `reply`, `raise`, `catch`, `getreply`,
  `check(getcall|getreply|catch)`. (Migration guide §6.1.2.)
- **Fuzzy + lazy templates** entirely removed. The proposed
  replacement is "dynamic templates" — see below.
- **`trigger` operation** deleted. (§9.5 in the guide.)
- **Shift and rotate operators** (`<<`, `>>`, `<@`, `@>`) moved
  to extensions. (§6.6.)
- **Automatic type** (`var v := 42`) mostly moved to extensions;
  only the restricted form — RHS is a reference or a literal —
  stays in core, plus the `for` cycle variable. (§6.5.5.)
- **`with { template ... }`** module attribute for switching
  between "static" and "dynamic" template evaluation modes. (§6.13.)

### Added or relaxed in core

- **`message` keyword optional** on port type declarations:
  `type port P { ... }` and `type port P message { ... }` are
  identical now. (§6.5.4.)
- **Lowercase string-literal type markers**: `'01'b`, `'AB'h`,
  `'AB'o` accepted in addition to uppercase. (§6.5.2.)
- **`string` as alias for `universal charstring`**; `charstring`
  becomes a subtype of `string`. Keywords stay for backward
  compatibility. (§6.5.2.)
- **Root scope** introduced for predefined functions + useful
  TTCN-3 types so the import rules are simpler. (§6.4.2.)
- **`import all` only** — selective `import { ... }` forms are
  dropped. Attributes on imports may only be attached when the
  source module has none, and must be identical across all import
  sites in a suite. (§6.7.1.)

### Tightened semantics

- **Static vs dynamic templates** (§6.13):
  - Templates with no parameter list ("static"): evaluated once
    at declaration; never re-evaluated.
  - Templates with a (possibly empty) parameter list
    ("dynamic"): evaluated at every invocation, except when
    invoked from a static template's initializer.
  - Local templates with parameters are **no longer allowed**
    (backward-incompatible).
  - Per-module override: `module M { ... } with { template
    "dynamic" }` makes every template in the module dynamic
    regardless of parameter list.
  - This is the functional replacement for fuzzy templates.

- **Record-of / set-of length counting** (§6.5.3) — the rules
  for partially-initialised lists are tightened:
  - `{ 0, 1, -, - }` has 4 elements, last two uninitialised.
  - `{ 0, -, 1, -, - }` has 5 elements.
  - `{ [0]:=0, [1]:=1, [2]:=- }` has 3 elements.
  - On assignment, LHS length follows RHS exactly (5 → 5, 3 → 3, ...).
  - We should sanity-check our `compositeLiteralLen` /
    `array_size_compat.go` and make sure assignments mirror
    these.

### Other notes worth keeping in mind

- **GFT, operational-semantics, IDL-mapping**: dropped — we
  don't implement them, so nothing to do.
- **Type-system re-org** (§6.5.1): no behavioural change but the
  spec section numbers shift, so when wiring new rules to "ETSI
  ES 201 873-1 §6.x.y" we should aim at the V4.17.1 numbering
  while we still target the V4-based conformance suite, and only
  remap once the V5 suite ships.
- **Open type + Map type** are first-class citizens in V5 §6.5.1.
  Map type is already in the conformance suite and we treat it.
  Open type is largely new — worth scoping.

## Action items (not blocking, file as ideas)

- [ ] Add a `--language v5` flag (or `module` attribute sniff)
      that downgrades procedure-based comm fixtures to
      `non-core` instead of failing them outright.
- [ ] Accept the lowercase `'..'b`/`'..'h`/`'..'o` literal forms
      in the scanner. Today these likely parse as syntax errors;
      the V5 suite will rely on them.
- [ ] Recognise `type port P { ... }` (no `message`) as a
      message port. Should be a one-line scanner / parser tweak.
- [ ] Static vs dynamic template enforcement (esp. forbid
      local templates with parameters when `template "static"`
      is in effect). Probably warrants its own
      `template_evaluation_mode.go` rule once we move on the
      V5 suite.
- [ ] Audit the V5 list of "moved to extensions" against the
      current conformance cohort labels — anything we already
      reject as a sem error but the V5 suite expects to accept
      under an extension flag should grow a `non-core` switch.

When in doubt about a specific item, ask Matthias on Teams.

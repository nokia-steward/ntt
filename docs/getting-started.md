# Getting started with ntt

This walkthrough gets you from "I just installed ntt" to running a
TTCN-3 test and using the LSP features in your editor in under five
minutes. It assumes you already have ntt on your PATH; see the
[install instructions](../README.md#install) if you don't.

## 1. Create a test project

Pick a working directory and drop in a single test file. We'll use
`/tmp/hello-ntt` for this example - swap in any path you like, but make
sure the path you pick has no spaces in it (a Windows-friendly
restriction).

```sh
mkdir -p /tmp/hello-ntt
cd /tmp/hello-ntt
```

Create a `package.yml` so ntt recognises this folder as a project root:

```yaml
# /tmp/hello-ntt/package.yml
name: hello
sources:
  - Hello.ttcn3
```

Create the test file itself:

```ttcn3
// /tmp/hello-ntt/Hello.ttcn3
module Hello {
    type component MTC {}

    function add(integer a, integer b) return integer {
        return a + b;
    }

    testcase TC_Add() runs on MTC {
        if (add(2, 3) == 5) {
            setverdict(pass);
        } else {
            setverdict(fail);
        }
    }

    control {
        execute(TC_Add());
    }
}
```

Every testcase names the component it runs on, so the one-line
`type component MTC {}` declaration above is required even for a test
that uses no ports or timers.

## 2. List discovered tests

```sh
ntt list
```

You should see:

```
Hello.TC_Add
```

If you don't, double-check that you ran `ntt` from inside the project
directory and that `package.yml` is in the same folder as the `.ttcn3`
file.

## 3. Run the test

You have two execution backends to pick from:

### 3a. `ntt exec` (interpreter, zero setup)

```sh
ntt exec
```

`ntt exec` walks the parsed tree directly - no codegen, no C++
toolchain, no `make` step. It is the same engine `ntt conformance`
uses against the ETSI suite, which means anything that runs under
`ntt conformance` also runs under `ntt exec`.

```
suite "ntt": pass (1 cases in 156.155µs)
  pass    Hello.TC_Add
```

Pick a different report format with `--format` (`text`, `json`, `junit`,
`tap`, `html`, `profile`) and write it to a file with `--out`.

This is the recommended way to iterate while you're writing tests:
start-up is fast, you don't need a working build chain, and the
error messages come straight from the interpreter.

### 3b. `ntt run` (Go backend)

```sh
ntt run Hello.ttcn3
```

`ntt run` lowers your test to the SSA-ish IR and emits Go that gets
compiled and executed. The first run costs a Go compile; subsequent
runs are cached. Prefer this when you care about steady-state
throughput rather than start-up latency, or when you want to embed
ntt-compiled tests in a Go binary. Unlike `ntt exec`, it takes the
file (or testcase) to run as an argument:

```
suite "Hello": pass (1 cases, build 273ms, run 2ms)
  pass	Hello.TC_Add
```

> **Note.** The Go backend compiles a module that links the ntt runtime,
> so it must run from inside a Go module — the ntt source checkout, or a
> project of your own with a `go.mod`. From a standalone directory like
> `/tmp/hello-ntt` it reports `cannot locate repository root`; use
> `ntt exec` there, which has no such requirement.

If you change `add(2, 3) == 5` to `add(2, 3) == 6` and re-run, you'll
get a failing verdict and a non-zero exit status - exactly what a CI
pipeline needs.

## 4. Format the code

```sh
ntt format Hello.ttcn3
```

This normalises whitespace and reflows long lines while preserving
behaviour. Use `--diff` to preview the changes without writing them
back to disk.

## 5. Lint the project

```sh
ntt lint
```

ntt's built-in linter catches common mistakes (missing default cases,
unused imports, suspicious type coercions). Configure it via the
`lint:` section of `package.yml`; the defaults are sensible for a quick
start.

## 5a. Static semantic checks

`ntt check` runs the lightweight semantic analyser on every module
in the suite (no execution). On top of the obvious "unresolved
identifier" diagnostics it now enforces a growing list of
TTCN-3-spec rules that Eclipse Titan only detects at C++ compile
time:

- **Template restrictions** (clause 15.8): `omit`, `template(value)`,
  `template(present)` use-sites are checked for kind compatibility.
- **Actual parameter rules** (clause 5.4.2): templates can't slip
  into value-typed `in` formals; non-lvalues, constants and template
  constants are refused for `out` / `inout`; named arguments may not
  precede positional ones; missing-required-actual is flagged.
- **Formal parameter restrictions** (clause 5.4.1.1 / 5.4.1.2):
  testcases and template definitions reject `port` / `timer` /
  `default` parameters.
- **Side-effects in restricted contexts** (clause 16.1.4 / 16.2 /
  20.2): function calls that touch ports or component variables are
  flagged inside template / function-default / pattern contexts.
- **Control-part operations** (clause 26.2): forbidden communication
  / component / port operations inside `control { ... }` blocks.
- **Interleave** (clause 20.4): `repeat` and `return` inside an
  `interleave` body.
- **Binary literal formats** (clause 6.1.1): hex/oct/bit strings
  validated against their declared kind, including the odd-digit
  rule for `octetstring`.
- **Const literal type matching** (clause 6.1.1): the literal on the
  right of a `const` / `modulepar` assignment must match the
  declared basic type.
- **Missing call parentheses** (clause 5.4.2): functions, testcases
  and altsteps cannot be referenced without empty `()`.
- **Array dimensions** (clause 6.2.7): dimensions must evaluate to
  positive integers.

The same rules run inside the LSP, so the editor flags violations on
save. They are also the static gate for `ntt conformance`, which is
why the ETSI pass-rate keeps climbing as more rules land.

## 5b. Track ETSI conformance

```sh
ntt conformance testdata/ttcn3-conformance-tests/ATS/
```

`ntt conformance` walks the ETSI conformance suite, runs each
annotated file through the interpreter, and reports the per-file
verdict-match rate (`pass accept` files are expected to pass at run
time; `pass reject` files are expected to be refused by the parser
or the semantic checker; `inconclusive` files are excluded from the
denominator). A baseline JSON snapshot lets CI gate regressions:

```sh
ntt conformance \
    --baseline testdata/conformance-baseline.json \
    --regress 0.5 \
    testdata/ttcn3-conformance-tests/ATS/
```

The current pass-rate and the per-push history are visualised in
[../docs/conformance/index.html](conformance/index.html).

## 6. Hook up your editor

Both [VS Code](https://marketplace.visualstudio.com/items?itemName=Nokia.ttcn3)
and [vim-lsp-settings](https://github.com/mattn/vim-lsp-settings)
auto-detect ntt as the TTCN-3 language server. Open the project folder
and the editor lights up with:

- semantic highlighting and inlay hints
- diagnostics on save
- jump to definition (including jumps from TTCN-3 into ASN.1 files)
- format-on-save via `ntt format`
- code actions (`source.organizeImports`, lint quick-fixes)

You can also point any LSP-aware editor at the binary by configuring
it to launch:

```sh
ntt langserver
```

## 7. Working with Titan projects

If you already have a `*.tpd` file from
[Eclipse Titan](https://projects.eclipse.org/projects/tools.titan),
ntt can read it directly. Run any ntt command from a directory that
contains a `.tpd` (and no `package.yml`) and ntt will discover the
descriptor automatically. You can also point at it explicitly:

```sh
ntt list path/to/myproject.tpd
```

Referenced sub-projects (`<ReferencedProject>` in the XML) are loaded
transitively, so the same command works for multi-module Titan
projects.


## 8. Where to go next

- Browse the [ARCHITECTURE.md](../ARCHITECTURE.md) doc to understand
  how ntt is laid out internally.
- Run `ntt help` for the full command list - `exec`, `run`, `check`,
  `conformance`, `list`, `tags`, `show`, `report`, `cover`, and
  others.
- Read [ntt-vs-titan.md](ntt-vs-titan.md) for the milestone-by-
  milestone feature matrix and the rationale for the interpret-first
  ntt-Titan strategy.
- Look at `examples/cmake/` for a CMake-driven integration that builds
  test adapters together with the TTCN-3 code.
- File feature requests and bug reports on
  [GitHub issues](https://github.com/nokia/ntt/issues).

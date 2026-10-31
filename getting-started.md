---
nav_order: 3
---

# Getting started
{: .fs-9 }

ntt is a free and open application stack for TTCN-3, which lets you focus on writing tests
{: .fs-6 .fw-300 }

<details open markdown="block">
  <summary>
    Table of contents
  </summary>
  {: .text-delta }
1. TOC
{:toc}
</details>


## Command Line Interface

    ntt <command> [<sources>...] [--] [<args>...]

* `<command>`: The command you want to execute. Sub-commands are possible.
* `<sources>...`: The test suite sources. This might be a list of `.ttcn3` files
  or the test suite root directory. If your test suite requires additional
  adapters, the test suite root directory must contain a manifest file (see below).
* `--`: This marker separates the sources list from the remaining arguments.
* `<args>...`: Remaining arguments.

Example:

    ntt show foo.ttcn3 bar.ttcn3 -- name sources


### Available Commands

| Command          | Details
| ---------------- | -------
| `ntt version`    | Shows version, git revision, and build time when available.
| `ntt list`       | Lists tests, controls, modules, imports, or module parameters.
| `ntt show`       | Shows manifest information and variables.
| `ntt tags`       | Writes a ctags file to stdout.
| `ntt lint`       | Reports suspicious or invalid TTCN-3 code.
| `ntt format`     | Formats files in the canonical TTCN-3 style.
| `ntt compile`    | Emits source metadata as JSON, or pipes it to `ntt-gen-<generator>`.
| `ntt dump`       | Dumps syntax trees (`text`, `ttcn3`, `dot`, or tokens).
| `ntt report`     | Summarizes the latest test run.
| `ntt langserver` | Starts the language server on stdin/stdout.
| `ntt completion` | Prints bash completion code.

The command reference is in [docs/reference](docs/reference).


### Custom Commands

Place an executable named `ntt-jaegerschnitzel` on your `PATH` and ntt exposes
it as a subcommand:

    $ ntt jaegerschnitzel +6000


### Environment variables

`NTT_SOURCE_DIR` selects the test suite root when you do not pass sources:

    $ ntt list                      # Lists tests in the current directory

    $ export NTT_SOURCE_DIR=~/foo
    $ ntt list                      # Lists tests in ~/foo


`NTT_CACHE` is a colon-separated list of directories, used like GNU Make's
`VPATH`, to find files such as `ntt.env`:

    $ echo "FOO=23" > ntt.env
    $ mkdir -p build && cd build
    $ NTT_CACHE=.. ntt show -- FOO
    23

`ntt.env` is a list of `KEY=VALUE` lines.

These environment variables overwrite the matching manifest fields:
`NTT_NAME`, `NTT_SOURCES`, `NTT_IMPORTS`, `NTT_PARAMETERS_FILE`,
`NTT_HOOKS_FILE`, `NTT_LINT_FILE`, and `NTT_TIMEOUT`. `NTT_SOURCES` and
`NTT_IMPORTS` are split on the platform path-list separator (`:` on Linux).

`NTT_DEBUG` raises the log level. `NTT_TRACE` raises it further. `--verbose`
does the same from the command line.


## Test Suite Manifest

To execute a test suite you usually need more than just a bunch of TTCN-3 source
files: You need generators, adapters, codecs, a lot of scripting, compile time
configuration, runtime configuration, post processing tools, caching of
build-artifacts and more. A manifest file provides a stable frame for
tools to work together nicely.

Every ntt test suite should provide a manifest file `package.yml` at
the root of the test suite directory structure. Supported fields:


| Name               | Type              | Details
| ------------------ | ----------------- | --------
| `name`             | string            | Name of the test suite.
| `sources`          | string[]          | TTCN-3 source files containing tests.
| `imports`          | string[]          | Dependencies such as adapters, codecs, or libraries.
| `timeout`          | duration          | Default timeout for tests.
| `parameters_file`  | string            | Path to the parameters file. Default: `${NTT_SOURCE_DIR}/${NTT_NAME}.parameters`.
| `hooks_file`       | string            | Path to the hooks file. Default: `${NTT_SOURCE_DIR}/${NTT_NAME}.hooks`.
| `lint_file`        | string            | Path to the lint configuration. Default: `${NTT_SOURCE_DIR}/ntt-lint.yml`.
| `diagnostics`      | string[]          | Diagnostics flags.
| `variables`        | map[string]string | Variables. `ntt.env` and the process environment overwrite these.
| `before_build`     | string[]          | Shell commands run before a build. A non-zero status cancels the build.
| `after_build`      | string[]          | Shell commands run after a build.
| `before_run`       | string[]          | Shell commands run before the tests.
| `after_run`        | string[]          | Shell commands run after the tests.
| `before_test`      | string[]          | Shell commands run before each test.
| `after_test`       | string[]          | Shell commands run after each test. |


**Environment Variables**

Values in the manifest can refer to variables with `$NAME`. A variable defined
in the process environment wins over the same name in `ntt.env`, which wins
over the `variables` section:

    $ echo '{"variables": {"NTT_NAME": "OrignalName" }, "name": "$NTT_NAME" }' > package.yml

    $ ntt show -- name
    OriginalName

    $ NTT_NAME=NewName ntt show -- name
    NewName


`NTT_SOURCES` and `NTT_IMPORTS` replace the `sources` and `imports` lists. Separate
entries with the platform path-list separator, for example
`NTT_SOURCES=foo.ttcn3:bar.ttcn3` on Linux.



# Reference
{: .no_toc }

* TOC
{:toc}


# Commands

## ntt

`ntt` is the main command line front end for working with TTCN-3.
It provides a uniform user interface, where possible:

    ntt <command> [<sources>...] [--] [<args>...]

* `<command>`: The command you want to execute, sub-commands are possible.
* `<sources>...`: The test suite sources. This might be a list of .ttcn3 files
  or the test suite root directory. If your test suite requires additional
  adapters, the test suite root directory must contain a manifest file.
* `--`: This marker is required to separate the sources list from the
  remaining arguments.
* `<args>...`: Remaining arguments.

Example:

    ntt show foo.ttcn3 bar.ttcn3 -- name sources


**Custom Commands**

You can extend and customize ntt through custom commands. Place any executable
with a name like `ntt-banana` in your `PATH` and ntt will automatically make it
available as a subcommand. You can then call it just like any other ntt
command:

    $ ntt banana +6000


**Environment variables**

You may define environment variable `NTT_SOURCE_DIR` to specify a default test
suite root directory:

    $ ntt list                      # Lists tests in current working directory
    $ export NTT_SOURCE_DIR=~/foo
    $ ntt list                      # Now, ntt lists tests in ~/foo


Environment variable `NTT_CACHE` is a colon-separated list of directories and has
similar purpose and behaviour like GNU Make's VPATH. It is use to find files
like `ntt.env`:

    $ echo "FOO=23" > ntt.env
    $ mkdir -p build && cd build
    $ NTT_CACHE=.. ntt show -- FOO
    23


**Debugging**

For debugging purposes you may increase ntt's log level with `--verbose` or
with the `NTT_DEBUG` environment variable. `NTT_TRACE` raises the log level
further.

For performance analysis you may enable [profiling](https://blog.golang.org/pprof)
using the `--cpuprofile` option.


## ntt compile

`ntt compile` reads the suite sources and imports and writes JSON source
metadata to stdout. `--generator <name>` (short `-G`) runs `ntt-gen-<name>` and
pipes that JSON to the generator's stdin. The default generator name is
`stdout`, which prints the JSON and does not start a generator.


## ntt completion
ntt provide bash completion. To load completion run

       . <(ntt completion)

To configure your bash shell to load completions for each session add to your bashrc

        # ~/.bashrc or ~/.profile
        . <(ntt completion)

Note, if bash-completion is not installed on Linux, please install the
'bash-completion' package via your distribution's package manager.

Note, the generated completion file is not compatible with zsh. Please open an
issue (or even pull request) should require support for zsh.


## ntt dump

`ntt dump` prints syntax trees for the suite sources and imports.

| Flag | Output |
| ---- | ------ |
| (none) | indented text tree |
| `--ttcn3` | canonical TTCN-3 formatting |
| `--dot` | Graphviz |
| `--plain --only-tokens` | one token per line |

`--with-trivia`, `--with-position`, and `--with-value` add trivia, offsets, and
token text to the `--plain --only-tokens` dump. `--plain` without
`--only-tokens` is rejected.


## ntt format

`ntt format` rewrites TTCN-3 sources in the canonical style. Files from imports
are left unchanged. With no arguments it scans the current directory for
`package.yml` and TTCN-3 sources.

The formatter has no line-length or indent-style settings. `--tabs-to-spaces N`
converts each tab to N spaces.

| Flag | Effect |
| ---- | ------ |
| `--in-place`, `-i` | write the formatted source back to the file |
| `--diff`, `-d` | print a diff; exit non-zero when a file would change |
| `--list`, `-l` | print paths that would change; exit non-zero when any would |

Exit status is zero when no errors were encountered.


## ntt langserver

Start TTCN-3 [language server](/editors) and wait for input on stdin. This command is
usually used by IDE or editors only.

## ntt lint

The lint command examines TTCN-3 source files and reports suspicious code.
It may find problems not caught by the compiler, but also constructs
considered "bad style".

Lint's exit code is non-zero for erroneous invocation of the tool or if a
problem was reported.


**Formatting Checks**

* `max_lines`: Number of lines a behaviour body must not exceed.
* `aligned_braces`: Braces must be in the same column or same line.
* `require_case_else`: Every select-statement must have one case-else.


**Cyclomatic Complexity Checks**

* `complexity.max`: Cyclomatic complexity muss not exceed.
* `complexity.ignore_guards`: Ignore complexity of alt- and interleave guards


**Naming Convention Checks**

* `naming.modules`: Checks for module identifiers.
* `naming.tests`: Checks for test-case identifier.
* `naming.functions`: Checks for function identifiers.
* `naming.altsteps`: Checks for altstep identifiers.
* `naming.parameters`: Checks for parameter identifiers.
* `naming.component_vars`: Checks for component variable identifiers.
* `naming.component_var_templates`: Checks for variable templates that are component variables.
* `naming.var_templates`: Checks for variable template identifiers.
* `naming.port_types`: Checks for port type identifiers.
* `naming.ports`: Checks for port instance identifiers.
* `naming.global_consts`: Checks for global constant identifiers.
* `naming.component_consts`: Checks for component scoped constant identifiers.
* `naming.templates`: Checks for constant template identifiers.
* `naming.locals`: Checks for local variable identifiers.
* `naming.record`: Checks for record identifiers.
* `naming.record_fields`: Checks for identifiers of record fields.
* `naming.record_of`: Checks for record-of identifiers.
* `naming.set`: Checks for set identifiers.
* `naming.set_fields`: Checks for identifiers of set fields.
* `naming.set_of`: Checks for set-of identifiers.
* `naming.union`: Checks for union identifiers.
* `naming.union_fields`: Checks for identifiers of union fields.
* `naming.enum`: Checks for enum identifiers.
* `naming.enum_label`: Checks for labels of enumerated types.

* `tags.modules`: Checks for module tags.
* `tags.tests`: Checks for test-case tags.


**White-Listing**

* `ignore.modules`: Ignore modules
* `ignore.files`: Ignore files


**Refactoring**

When TTCN-3 code is refactored incrementally, it happens that references to
legacy code are faster added than one can remove them. This check helps with a
warning, as soon as the usage of a symbol exceed a defined limit (`usage.limit`).


**Unused Symbols**

* `unused.modules`: Checks for unused modules


**Example**

```yaml
# Assure braces are aligned (either in same line or in same column).
aligned_braces: true

# Every select-construct require an else-case.
require_case_else: true
max_lines: 40

# Symbol 'foo' must not be used more than 12 times.
usage:
  "foo":
    limit: 12
    text: Use "bar" instead.

# Every module specified in imports-section of the test suite manifest must be
# imported at least once.
unused:
  modules: true

# Cyclomatic complexity (aka McCabe index) must not exceed 15
complexity:
  max: 15
  ignore_guards: true # ignore guard expressions from alt- or interleave statements.

# Documentation tags
tags:
  tests:
    "@author": "testcases must have a @author tag"

naming:
  tests:
    # An exlamation mark inverts the match.
    "!.{130,}": "testcase identifiers must not be longer than 130 characters"

  functions:
    "^[a-z]"      : "function identifiers must begin with a lower case letter"
    "!^(f|func)_" : "function identifiers must not begin with f_ or func_"

  global_consts:
    "^[A-Z0-9_]+$": "global constants must be UPPER_CASE"

ignore:
  modules:
    # Ignore generated modules
    - "^Protobuf_.+$"

  files:
    # Ignore all files from generated folders
    - "generated/"
```

## ntt list
List various types of objects. Subcommands are `tests` (the default), `modules`,
`imports`, `controls`, and `modulepars`.

List control parts, modules, imports or tests. The list command without any explicit
sub-commands will output tests.

List will ignore imported directories when printing tests. If you need to list all
tests from a testsuite you'll have to pass .ttcn3 files as arguments.
Example:

    ntt list $(ntt show -- sources) $(find $(ntt show -- imports) -name \*.ttcn3)


**Filtering**

You can use regular expressions to filter objects. If you pass multiple regular
expressions, all of them must match (AND). Example:

	$ cat example.ttcn3
	testcase foo() ...
	testcase bar() ...
	testcase foobar() ...
	...

	$ ntt list --regex=foo --regex=bar
	example.foobar

	$ ntt list --regex='foo|bar'
	example.foo
	example.bar
	example.foobar


Similarly, you can also specify regular expressions for documentation tags.
Example:

	$ cat example.ttcn3
	// @one
	// @two some-value
	testcase foo() ...

	// @two: some-other-value
	testcase bar() ...
	...

	$ ntt list --tags-regex=@one --tags-regex=@two
	example.foo

	$ ntt list --tags-regex='@two: some'
	example.foo
	example.bar


**Baskets**

Baskets are filters defined by environment variables of the form:

        NTT_LIST_BASKETS_<name> = <filters>

For example, to define a basket "stable" which excludes all objects with @wip
or @flaky tags:

	export NTT_LIST_BASKETS_stable="-X @wip|@flaky"

Baskets become active when they are listed in colon separated environment
variable `NTT_LIST_BASKETS`. If you specify multiple baskets, at least of them
must match (OR).

Rule of thumb: all baskets are ORed, all explicit filter options are ANDed.
Example:

	$ export NTT_LIST_BASKETS_stable="--tags-exclude @wip|@flaky"
	$ export NTT_LIST_BASKETS_ipv6="--tags-regex @ipv6"
	$ NTT_LIST_BASKETS=stable:ipv6 ntt list -R @flaky


Above example will output all tests with a @flaky tag and either @wip or @ipv6 tag.


If a basket is not defined by an environment variable, it's equivalent to a
"--tags-regex" filter. For example, to lists all tests, which have either a
@flaky or a @wip tag:

	# Note, flaky and wip baskets are not specified explicitly.
	$ NTT_LIST_BASKETS=flaky:wip ntt list

	# This does the same:
	$ ntt list --tags-regex="@wip|@flaky"

## ntt mcov

`ntt mcov` is the `ntt-mcov` helper. It is a separate program, not part of the
`ntt` binary. When `ntt-mcov` is on `PATH`, `ntt mcov` runs it.

Extensive usage of wild cards `*` and `?` in templates causes gaps in message
content validation. `ntt mcov` helps detecting those gaps and thus improve test
quality, by printing the message coverage.

It reads a k3 runtime log file from stdin and prints how often each field of a
message was matched against. A field with zero count indicates a message was
never properly validated. Example:
```ttcn3
type record MessageA {
    integer a,
    integer b,
    integer c
}

type record MessageB {
    integer a,
    integer b
}

type record MessageC {
    integer a,
    float f
}

...
p.receive(MessageA:{1,?,2});
p.receive(MessageA:{1,?,?});
p.receive(MessageC:{?,0.1});
...
```
```
$ ntt mcov <example.log
Module1.MessageA.a	2
Module1.MessageA.b	0
Module1.MessageA.c	1
# COV	Module1.MessageA	2/3

Module1.MessageB.a	0
Module1.MessageB.b	0
# COV	Module1.MessageB	0/2

Module2.MessageC.a	0
Module2.MessageC.f	1
# COV	Module2.MessageC	1/2

# TOTAL	3/7 42.86%
```

## ntt report
The report command shows a summary of the latest test run. The summary includes
information such as a list of tests which did not pass, average run times, CPU
load, etc.
Command line options `--json` and `--junit` show similar output, but with JSON
or JUNIT formatting.


**Templating**

ntt uses the Go templates format which you can use to specify custom output templates.
Example:

{% raw %}
	ntt report --template "{{.Name}} took {{.Tests.Duration}}"
{% endraw %}

Available Objects:

 * `.Report` is a collection of test runs
 * `.Report.Cores`: number of CPU cores
 * `.Report.Environ`: list of environment variable
 * `.Report.Getenv`: value of an environment variable
 * `.Report.LineCount`: number of TTCN-3 source code lines
 * `.Report.MaxJobs`: maximum number of parallel test jobs
 * `.Report.MaxLoad`: maximum allowed CPU load
 * `.Report.Modules`: a list of collection sorted by module
 * `.Report.Name`: name of the collection
 * `.Report.Runs`: list of test runs
 * `.Report.Tests`: list of tests (with final verdict)

 * `.RunSlice` is a list of test runs
 * `.RunSlice.Load`: Returnurfsystemload slice for every run
 * `.RunSlice.Average`: Average duration of runs (median)
 * `.RunSlice.Deviation`: Standard deviation
 * `.RunSlice.Duration`: Timespan of first and last test run
 * `.RunSlice.Failed`: A slice of failed test runs (inconc, none, error, fail, ...)
 * `.RunSlice.First`: First test run
 * `.RunSlice.Last`: Last test run
 * `.RunSlice.Longest`: Longest test run
 * `.RunSlice.NotPassed`: A slice of tests without 'pass' verdict
 * `.RunSlice.Result`: Final result (PASSED, FAILED, UNSTABLE, NOEXEC)
 * `.RunSlice.Shortest`: Shortest test run
 * `.RunSlice.Total`: Sum of all test run durations
 * `.RunSlice.Unstable`: List of unstable test runs

 * `.Run` is a individual test run
 * `.Run.ID`: test run ID (e.g. test.Stable_A-2)
 * `.Run.Name`: full qualified test name (test.Stable_A)
 * `.Run.Instance`: test instance (e.g. 2)
 * `.Run.Module`: module name (test)
 * `.Run.Testcase`: testcase name (e.g. Stable_A)
 * `.Run.Verdict`: the test verdict (pass, fail, none, ...)
 * `.Run.Begin`: when the test was started (time.Time Go object)
 * `.Run.End`: when the test ended (time.Time Go object)
 * `.Run.Duration`: a time.Duration Go object
 * `.Run.Load`: the system load when the test was started
 * `.Run.MaxMem`: the maximum memory used when the test ended
 * `.Run.Reason`: optional reason for verdicts
 * `.Run.ReasonFiles`: content of \*.reason files
 * `.Run.RunnerID`: the ID of the runner exeuting the run
 * `.Run.WorkingDir`: working Directory of the test

 * `.File` is a (reason) file
 * `.File.Name`: path to file
 * `.File.Content`: content of file


Additional filters:

 * `green`: output ANSI sequences for color green
 * `red`: output ANSI sequences for color red
 * `orange`: output ANSI sequences for color orange
 * `bold`: output ANSI sequences for bold text
 * `off`: output ANSI sequences to reset attributes
 * `colorize`: colorize output
 * `join`: join input with a separator
 * `json`: encode input using JSON format
 * `min`: returns the minimum of a float slice
 * `max`: returns the maximum of a float slice
 * `median`: returns the median of a float slice



**Examples**

Summary template:
```
{% raw %}
{{bold}}==================================  Summary  =================================={{off}}
{{range .Tests.NotPassed}}{{ printf "%-10s %s" .Verdict .Name  | colorize }}
{{else}}{{if eq (len .Tests) 0}}{{orange}}{{bold}}WARNING: No matching test cases found!{{off}}
{{else}}{{green}}all tests have passed{{off}}
{{end}}{{end}}
{{len .Tests}} test cases took {{bold}}{{.Tests.Duration}}{{off}} to execute (total runs: {{len .Runs}}
{{- with .Tests.Failed}}, {{red}}not passed: {{len .}}{{off}}{{end}}
{{- with .Tests.Unstable}}, {{orange}}unstable: {{len .}}{{off}}{{end}})
{{bold}}==============================================================================={{off}}

{{ printf "%s (±%s)" .Tests.Average .Tests.Deviation | printf "Average  : %-30s CPU cores      : " }}{{printf "%d" .Cores}}
{{ printf "Shortest : %-30s Parallel tests : %d" .Tests.Shortest.Duration .MaxJobs }}
{{ printf "Longest  : %-30s Load limit     : %d" .Tests.Longest.Duration .MaxLoad}}
{{ printf "Total    : %-30s Load average   : %.2f" .Tests.Total (median .Tests.Load)}}

{{bold}}==============================================================================={{off}}
{{bold}}Final Result: {{.Tests.Result | colorize}}{{off}}
{{bold}}==============================================================================={{off}}
{% endraw %}
```


JUnit template:
```xml
{% raw %}
<?xml version="1.0" encoding="UTF-8"?>
<testsuites>{{range .Modules}}

<testsuite name="{{.Name}}" tests="{{len .Tests}}" failures="{{len .Tests.Failed}}" errors="" time="{{.Tests.Total.Seconds}}">
{{range .Tests}}<testcase name="{{.Testcase}}" time="{{.Duration.Seconds}}">
  {{if and (ne .Verdict "unstable") (ne .Verdict "pass")}}<failure>Verdict: {{.Verdict}} {{with .Reason}}({{. | html }}){{end}}
{{range .ReasonFiles}}{{.Name}}: {{.Content}}{{end}}
  </failure>
{{end}}</testcase>

{{end}}</testsuite>
{{end}}</testsuites>
{% endraw %}
```

JSON template:
```json
{% raw %}
{
  "name"          : "{{.Name}}",
  "timestamp"     : {{.Runs.First.Begin.Unix}},
  "cores"         : {{.Cores}},
  "parallel_jobs" : {{.MaxJobs}},
  "max_load"      : {{.MaxLoad}},
  "suite": {
    "linecount": {{.LineCount}}
  },
  "load": {
    "min" : {{min .Tests.Load}},
    "max" : {{max .Tests.Load}},
    "avg" : {{median .Tests.Load}}
  },
  "tests": {
    "result"   : "{{ .Tests.Result }}",
    "tests"    : {{len .Tests }},
    "failed"   : {{len .Tests.Failed}},
    "unstable" : {{.Tests.Unstable | json}},
    "duration" : {
      "real"  : {{.Tests.Duration.Milliseconds}},
      "total" : {{.Tests.Total.Milliseconds}},
      "min"   : {{.Tests.Shortest.Duration.Milliseconds}},
      "max"   : {{.Tests.Longest.Duration.Milliseconds}},
      "avg"   : {{.Tests.Average.Milliseconds}},
      "dev"   : {{.Tests.Deviation.Milliseconds}}
    }
  },
  "env": {{ .Environ | json }}
}
{% endraw %}
```

## ntt show
Show test suite information like name, sources, environment variables, ...
The show command provide additional output formats:

**JSON**

Command line argument `--json` enables output in JSON format, for convenient
processing by Python, JavaScript and other languages. Example:
```json
$ ntt show --json
{
  "args": [],
  "name": "example",
  "source_dir": "~/src/suite",
  "parameters_file": "example.parameters",
  "hooks_file": "example.hooks",
  "lint_file": "ntt-lint.yml",
  "env": [
    "NTT_LIST_BASKETS_stable=-X @wip|@flaky"
  ],
  "sources": [
    "test.ttcn3",
    "components.ttcn3"
  ],
  "imports": [
    "lib"
  ],
  "files": [
    "test.ttcn3",
    "components.ttcn3"
  ]
}
```

**Shell**

Similarly command line argument `--sh` outputs test suite data for convenient
shell consumption. Example:

```bash
#!/bin/bin/bash
# This is a custom command: ntt-banana
if ! . <(ntt show "$@" --sh); then
   echo >&2 "$0: error: $K3_ERROR"
   exit 1
fi

# Continue with loaded data
echo "Test suite Name: $K3_NAME"
echo "Sources: ${K3_SOURCES[*]}"
```

Note, the shell output mostly uses the `K3` prefix for variables. This may change in the future. Example output:
```bash
# This is a generated output of ntt show. Args: []

# k3-hook calls the K3 test hook (if defined) with action passed by $1.
function k3-hook()
{
    if [ -n "$K3_HOOKS_FILE" ]; then
        K3_SOURCES="${K3_SOURCES[*]}" \
        K3_IMPORTS="${K3_IMPORTS[*]}" \
        K3_TTCN3_FILES="${K3_TTCN3_FILES[*]}" \
            "$K3_HOOKS_FILE" "$@" 1>&2
    fi
}

export K3_NAME='example'
export K3_HOOKS_FILE='/home/user/src/suite/example.hooks'
export K3_SOURCE_DIR='/home/user/src/suite'

export 'NTT_LIST_BASKETS_stable=-X @wip|@flaky'

K3_SOURCES=(
	test.ttcn3
	components.ttcn3
)

K3_IMPORTS=(
	lib
)

K3_TTCN3_FILES=(
	test.ttcn3
	components.ttcn3
)

K3_BUILTINS=(
)
```


## ntt tags
The tags command generates an index (or "tag") file for TTCN-3 language objects found in file(s).

This tag file allows these items to be quickly and easily located by a text
editor or other utility. A "tag" signifies a language object for which an index
entry is available (or, alternatively, the index entry created for that
object).

The tags command will also generate tags for fields, members, ... .


## ntt version
Display ntt version if available.

# API

The Go packages are:

* [ttcn3](https://pkg.go.dev/github.com/nokia/ntt/ttcn3): parse trees and semantic lookup
* [ttcn3/syntax](https://pkg.go.dev/github.com/nokia/ntt/ttcn3/syntax): scanner and parser
* [project](https://pkg.go.dev/github.com/nokia/ntt/project): `package.yml` and suite configuration
* [runtime](https://pkg.go.dev/github.com/nokia/ntt/runtime): value representation used by the interpreter
* [interpreter](https://pkg.go.dev/github.com/nokia/ntt/interpreter): evaluates TTCN-3 syntax trees


# Manifest file package.yml

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
| `after_test`       | string[]          | Shell commands run after each test.


**Environment Variables**

Values in the manifest can refer to variables with `$NAME`. A variable defined
in the process environment wins over the same name in `ntt.env`, which wins
over the `variables` section. `ntt.env` is a list of `KEY=VALUE` lines:

    $ echo '{"variables": {"NTT_NAME": "OrignalName" }, "name": "$NTT_NAME" }' > package.yml

    $ ntt show -- name
    OriginalName

    $ NTT_NAME=NewName ntt show -- name
    NewName


`NTT_NAME`, `NTT_SOURCES`, `NTT_IMPORTS`, `NTT_PARAMETERS_FILE`,
`NTT_HOOKS_FILE`, `NTT_LINT_FILE`, and `NTT_TIMEOUT` overwrite the matching
manifest fields. `NTT_SOURCES` and `NTT_IMPORTS` are split on the platform
path-list separator (`:` on Linux).

# CMake

ntt provides a CMake module with functions to help use the NTT/K3 Test
infrastructure.  It searches for program `ntt` and stores its location in
variable `NTT_EXECUTABLE` and creates imported target `ntt::ntt`.

It further provides function `add_ttcn3_suite` for generating a test suite
manifest and function `protobuf_generate_ttcn3` for calling a protoc generator
plugin.


**Conventience Commands**
The CMake module uses the TGT parameter from `add_ttcn3_suite` to create
additional commands to make your CMake live easier:
* `$TGT.lint`: run [linter](#ntt-lint) against test suite.
* `$TGT.tags`: generate [cTags](#ntt-tags) file
* `$TGT.tests`: generate [testcase list](#ntt-list).
* `$TGT.deps`: generate [module dependency graph](#ntt-list).


## add_ttnc3_suite

Add a TTCN-3 test suite manifest for use with NTT and CTest:

```cmake
  add_ttcn3_suite(TGT
        SOURCES src1...
        [DEPENDS ...]
        [NAME name]
        [TIMEOUT secs]
        [HOOKS_FILE executable]
        [PARAMETERS_FILE file]
        [WORKING_DIRECTORY dir]
        [TARGETS target1...]
        [DIAGNOSTICS flags]
  )
```

`add_ttcn3_suite` creates a target TGT and test suite manifest
`package.yml` as `BYPRODUCT`.

The options are:

`SOURCES src1...`  
The list of `.ttcn3` source files. Files specified in `SOURCES` usually are testcases.

`DEPENDS target1...|directory...`  
Specifies additional TTCN-3 packages required by the test suite.
If the argument specifies a target it will be replaced by the location of
the target directory (`$<TARGET_FILE_DIR>`). Additionally a target-level
dependency will be added so that the depending target will be built before
this test suite.

`NAME`  
Specfies the name of the test suite. If this option is not provided, ntt
will assign one.

`TIMEOUT seconds`  
Specifies a timeout after which a test case will be aborted.

`HOOKS_FILE executable`  
Specifies an executable to be used as test hook.
If `HOOKS_FILE` specifies an executable target (created by `add_executable`) it
will automatically be replaced by the location of the executable created at
build time. Additionally a target-level dependency will be added so that
the executable target will be built before this hook is used.

`PARAMETERS_FILE file`  
Specifies a YAML file with test configuration.

`WORKING_DIRECTORY dir`  
Specifies the directory in which to run the tests. If this option is not
provided, the current binary directory is used.

`TARGETS target1...`  
Add additional target-level dependencies. This is used to assure SUTs or third-party code generators are built before a test is executed.

`DIAGNOSTICS flags`  
Writes a `diagnostics` list into `package.yml`. Spaces in the argument separate flags.



## protobuf_generate_ttcn3

Call protoc generator plugin for TTCN-3 output.

```cmake
  protobuf_generate_ttcn3(TGT protofile1...)
```

`protobuf_generate_ttcn3` creates a target TGT for generating TTCN-3 files from
proto-files.  You may use use global variable `Protobuf_IMPORT_DIRS` to add
custom include directories.  

## Example

```cmake
find_package(Protobuf REQUIRED)
find_package(NTT REQUIRED)
protobuf_generate_ttcn3(proto_itf
  foo.proto
  bar.proto
)

add_ttcn3_suite(suite1
    SOURCES   tests.ttcn3
    HOOKS_FILE hook.py
    DEPENDS
        proto_itf # see https://github.com/nokia/ntt/issues/195
    TARGETS
        mySUT
)
```

# VSCode Extension

![vscode TTCN-3 extension preview](/ntt/static/vscode-ttcn3-extension-preview.png)

The [VSCode Extension for TTCN-3](https://marketplace.visualstudio.com/items?itemName=Nokia.ttcn3)
is the part which directly faces the user, while most of heavy lifting is done by ntt
and the [language server protocol](https://microsoft.github.io/language-server-protocol/)
in the background.  

## Settings

Access VSCode settings by pressing `Ctrl` and `,` and then search for `ttcn3`:

![settings cast](/ntt/static/cast-enable-ntt.gif)

Contributed settings:

| Name                                 | Description                                                                 | Default
| ------------------------------------ | --------------------------------------------------------------------------- | -------
| `ttcn3.server.command`               | Command to start the language server. Empty uses the bundled server.       | empty
| `ttcn3.server.automaticUpdate`       | Install and update the language server. Ignored when `command` is set.     | `true`
| `ttcn3.server.usePrereleases`        | Use pre-release language server builds.                                     | `false`
| `ttcn3.server.updateServerURL`       | Base URL for the latest-release query (`$URL/releases/latest`).            | `https://ttcn3.dev/api/v1/ntt`
| `ttcn3.server.ignoreCertificateErrors` | Skip certificate checks while downloading the language server.            | `false`
| `ttcn3.server.toolsPath`             | Extra directories searched for TTCN-3 tools, in the listed order.          | `[]`

The language server also reads these settings. When a client does not send
one, ntt treats it as `false`:

| Name | Description |
| ---- | ----------- |
| `ttcn3.experimental.format.enabled` | Enable document formatting. |
| `ttcn3.experimental.semanticTokens.enabled` | Enable semantic tokens. |
| `ttcn3.experimental.inlayHint.enabled` | Enable inlay hints. |
| `ttcn3.experimental.diagnostics.enabled` | Publish parse errors as diagnostics. |


**Commands**

Access VSCode command by pressing `Ctrl`, `Shift`, `P` and then type `ttcn3`:

![commands cast](/ntt/static/cast-show-status.gif)

Contributed commands:

| Name                              | Description
| --------------------------------- | ----------------------------
| `ttcn3.languageServer.restart`    | Restart language server
| `ttcn3.languageServer.status`     | Show language server status
| `ttcn3.languageServer.debug.toggle` | Toggle language server debug mode


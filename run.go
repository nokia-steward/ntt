package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/nokia/ntt/backend/cpp"
	"github.com/nokia/ntt/backend/golang"
	"github.com/nokia/ntt/ir"
	"github.com/nokia/ntt/ir/lower"
	"github.com/nokia/ntt/ttcn3"
	"github.com/spf13/cobra"
)

var (
	runTarget  string
	runKeep    bool
	runJSON    bool
	runWorkdir string

	runOutput    string
	runConsole   string
	runLive      bool
	runTimeout   time.Duration
	runLogFilter logFilter
)

// RunCommand builds and runs a test suite, printing its structured line log;
// with --target it instead compiles a TTCN-3 file with a backend and runs it.
var RunCommand = &cobra.Command{
	Use:     "run [ <path>... ] [ -- <test id>... ]",
	Aliases: []string{"run2"},
	Short:   "Build and run test suite",
	Long: `Build and run a test suite.

The ntt run command first builds the test suite from the files or
directories passed as first argument list, and then runs it. If no ids are
specified, ntt run executes every control part. When the suite declares
none, it runs every testcase from the sources. --regex, --exclude,
--tags-regex, --tags-exclude, and NTT_LIST_BASKETS filter these default
jobs: a filtered run executes the matching testcases and does not run
control parts.

Running control functions is supported. For example:

        ntt run -- test.A test.control test.B

Test baskets are also supported (see "ntt help list"). Below example will
run all tests with @stable-tag:

	NTT_LIST_BASKETS=stable ntt run

The run prints the structured line log of its events, one per line:
timestamp|event|identity=file:line|fields. Logging filters select which
events are printed on the console. The file given to --output keeps every
event. Filters apply in the order given, on top of all events being
enabled:

        ntt run --activate=fnen|fnlv --deactivate=Suite:paon|rvon

--activate and --deactivate take an event-id regular expression, optionally
limited to one module as Module:pattern. --activate-all is the default when
no logging filter is given. A module id that is not in the suite, or an
event pattern that matches no event, is an error.

--format selects how those events are printed on the console. auto (the
default) colors log events when standard output is a terminal. color always
colors them. plain prints the raw log text. Error events are bold white on
red. A tcfi, cofi, or setv is yellow for inconc, green for pass, and red
for fail or error. plod, perf, vrsn, vach, and bctr are faint grey. auto
stays uncolored when NO_COLOR is set, when NTT_COLORS=never, or when
TERM=dumb. The --output file stores raw events.

        ntt run --format=plain

A testcase may name the verdict it is expected to finish with:

        // @verdict: error
        testcase TC1() {}

When the final verdict differs, ntt run writes a VMIS error and that
testcase fails the run. A verdict that matches the tag keeps the run
successful. Otherwise a testcase that ends with fail or error fails the
run.

A module may instead carry an ETSI/PIXIT verdict tag. "pass accept" accepts
every testcase verdict except error; "pass reject" expects the module to
fail to build. Add "ttcn3verdict:<verdict>" to require that verdict of
every testcase in the module:

        // @verdict: pass accept, ttcn3verdict:pass
        module Suite {}

The suite runs on ntt's engine, on its virtual clock unless --live is
given.

With --target go|cpp, ntt run instead lowers one TTCN-3 file through the
IR (ir/lower), emits source for that backend, builds a binary against the
runtime, executes it, and prints one line per testcase plus a final suite
verdict. Build artefacts go into a temp directory unless --keep is passed.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if runTarget != "" {
			if len(args) != 1 {
				return fmt.Errorf("run: --target takes one file")
			}
			return doRun(cmd, args)
		}
		return runSuite(cmd, args)
	},
}

func init() {
	RootCommand.AddCommand(RunCommand)
	flags := RunCommand.Flags()
	flags.StringVarP(&runOutput, "output", "o", "", "Write logging output to given `file`")
	flags.StringVar(&runConsole, "format", "auto", "Console output `format`: auto, plain, or color")
	flags.AddFlagSet(BasketFlags())
	flags.AddFlagSet(logFilterFlags(&runLogFilter))
	flags.BoolVar(&runLive, "live", false, "run on a real clock (real timers, real concurrency) instead of the virtual one")
	flags.DurationVar(&runTimeout, "timeout", 0, "per-testcase wall-clock limit (0 = the 60s safety default)")
	flags.StringVar(&runTarget, "target", "", "compile one file with a backend, 'go' or 'cpp', and run it")
	flags.BoolVar(&runKeep, "keep", false, "with --target, keep the temporary build directory")
	flags.BoolVar(&runJSON, "json", false, "with --target, print the full result as JSON instead of text")
	flags.StringVar(&runWorkdir, "workdir", "", "with --target, use this directory for build artefacts (implies --keep)")
}

// runResult is what the harness emits (and what `--json` renders).
type runResult struct {
	Suite   string          `json:"suite"`
	Target  string          `json:"target"`
	Source  string          `json:"source"`
	Build   time.Duration   `json:"build_ns"`
	Run     time.Duration   `json:"run_ns"`
	Cases   []runCaseResult `json:"cases"`
	Verdict string          `json:"verdict"`
}

type runCaseResult struct {
	Name    string   `json:"name"`
	Verdict string   `json:"verdict"`
	Reason  string   `json:"reason,omitempty"`
	Logs    []string `json:"logs,omitempty"`
}

func doRun(cmd *cobra.Command, args []string) error {
	path := args[0]
	tree := ttcn3.ParseFile(path)
	if tree == nil {
		return fmt.Errorf("run: parse failed for %s", path)
	}
	if tree.Err != nil {
		return fmt.Errorf("run: %s: %w", path, tree.Err)
	}
	m, _ := lower.Module(tree)
	if m == nil {
		return fmt.Errorf("run: no module found in %s", path)
	}

	dir := runWorkdir
	if dir == "" {
		var err error
		dir, err = os.MkdirTemp("", "ntt-run-")
		if err != nil {
			return err
		}
	} else {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		runKeep = true
	}
	if !runKeep {
		defer os.RemoveAll(dir)
	}

	var (
		result runResult
		err    error
	)
	result.Source = path
	result.Suite = m.Name
	result.Target = strings.ToLower(runTarget)

	switch result.Target {
	case "go", "golang":
		err = runGoTarget(dir, m, &result)
	case "cpp", "c++":
		err = runCppTarget(dir, m, &result)
	default:
		return fmt.Errorf("run: unknown target %q (want go|cpp)", runTarget)
	}
	if err != nil {
		return err
	}

	result.Verdict = aggregate(result.Cases)
	if runJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}
	renderText(os.Stdout, result)
	return nil
}

// ---- Go backend pipeline -------------------------------------------

func runGoTarget(dir string, m interface{}, out *runResult) error {
	root, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("run: cannot locate repository root: %w", err)
	}
	mod, ok := m.(*ir.Module)
	if !ok {
		return fmt.Errorf("run: internal: bad module type")
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), goModFile(root), 0o644); err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := golang.Generate(&buf, mod); err != nil {
		return fmt.Errorf("run: codegen: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "tests"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "tests", "tests.go"), buf.Bytes(), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(goMainSource()), 0o644); err != nil {
		return err
	}

	t := time.Now()
	bin := filepath.Join(dir, "suite")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	if msg, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("run: go build failed: %w\n%s", err, msg)
	}
	out.Build = time.Since(t)

	t = time.Now()
	res, err := exec.Command(bin).Output()
	if err != nil {
		return fmt.Errorf("run: binary failed: %w\n%s", err, res)
	}
	out.Run = time.Since(t)
	return parseHarnessOutput(res, out)
}

// ---- C++ backend pipeline ------------------------------------------

func runCppTarget(dir string, m interface{}, out *runResult) error {
	root, err := findRepoRoot()
	if err != nil {
		return fmt.Errorf("run: cannot locate repository root: %w", err)
	}
	mod, ok := m.(*ir.Module)
	if !ok {
		return fmt.Errorf("run: internal: bad module type")
	}
	include := filepath.Join(root, "runtime/cpp/include")

	var buf bytes.Buffer
	if err := cpp.Generate(&buf, mod); err != nil {
		return fmt.Errorf("run: codegen: %w", err)
	}
	src := filepath.Join(dir, "main.cpp")
	full := buf.String() + "\n" + cppMainSource()
	if err := os.WriteFile(src, []byte(full), 0o644); err != nil {
		return err
	}

	bin := filepath.Join(dir, "suite")
	compiler := os.Getenv("CXX")
	if compiler == "" {
		compiler = "g++"
	}
	t := time.Now()
	cmd := exec.Command(compiler, "-std=c++17", "-O2", "-I", include, "-o", bin, src)
	if msg, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("run: %s failed: %w\n%s", compiler, err, msg)
	}
	out.Build = time.Since(t)

	t = time.Now()
	res, err := exec.Command(bin).Output()
	if err != nil {
		return fmt.Errorf("run: binary failed: %w\n%s", err, res)
	}
	out.Run = time.Since(t)
	return parseHarnessOutput(res, out)
}

// ---- harness output ------------------------------------------------

// parseHarnessOutput consumes the JSON-lines a generated binary
// emits and stuffs them into out.Cases.
func parseHarnessOutput(b []byte, out *runResult) error {
	for _, line := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		if line == "" {
			continue
		}
		var c runCaseResult
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			return fmt.Errorf("run: malformed harness line %q: %w", line, err)
		}
		out.Cases = append(out.Cases, c)
	}
	return nil
}

func aggregate(cases []runCaseResult) string {
	rank := map[string]int{
		"none":   0,
		"pass":   1,
		"inconc": 2,
		"fail":   3,
		"error":  4,
	}
	high := 0
	highName := "none"
	for _, c := range cases {
		if r, ok := rank[strings.ToLower(c.Verdict)]; ok && r > high {
			high = r
			highName = strings.ToLower(c.Verdict)
		}
	}
	return highName
}

func renderText(w *os.File, r runResult) {
	fmt.Fprintf(w, "suite %q: %s (%d cases, build %s, run %s)\n",
		r.Suite, r.Verdict, len(r.Cases), r.Build.Round(time.Millisecond), r.Run.Round(time.Millisecond))
	for _, c := range r.Cases {
		if c.Reason != "" {
			fmt.Fprintf(w, "  %s\t%s\t%s\n", c.Verdict, c.Name, c.Reason)
		} else {
			fmt.Fprintf(w, "  %s\t%s\n", c.Verdict, c.Name)
		}
		for _, l := range c.Logs {
			fmt.Fprintf(w, "    log: %s\n", l)
		}
	}
}

// ---- main source templates -----------------------------------------

func goMainSource() string {
	return `package main

import (
    "context"
    "encoding/json"
    "os"
    "strings"

    tests "ntt-run-suite/tests"
)

func main() {
    enc := json.NewEncoder(os.Stdout)
    for _, c := range tests.RunSuite(context.Background()) {
        _ = enc.Encode(map[string]interface{}{
            "name":    c.Name,
            "verdict": strings.ToLower(c.Verdict.String()),
            "reason":  c.Reason,
            "logs":    c.Logs,
        })
    }
}
`
}

func cppMainSource() string {
	return `
#include <iostream>
#include <sstream>

namespace {
// jsonEscape escapes the few characters JSON forbids in a string
// literal. The harness's input is testcase reasons and log lines, so
// quote / backslash / control chars are the realistic concerns.
std::string jsonEscape(const std::string& in) {
    std::ostringstream out;
    for (char c : in) {
        switch (c) {
        case '"':  out << "\\\""; break;
        case '\\': out << "\\\\"; break;
        case '\n': out << "\\n"; break;
        case '\r': out << "\\r"; break;
        case '\t': out << "\\t"; break;
        default:
            if (static_cast<unsigned char>(c) < 0x20) {
                char buf[8];
                std::snprintf(buf, sizeof(buf), "\\u%04x", c);
                out << buf;
            } else {
                out << c;
            }
        }
    }
    return out.str();
}
}

int main() {
    auto cases = ntt_tests::run_suite();
    for (const auto& c : cases) {
        std::cout << "{\"name\":\"" << jsonEscape(c.name) << "\",";
        std::cout << "\"verdict\":\"" << ntt::to_string(c.verdict) << "\",";
        std::cout << "\"reason\":\"" << jsonEscape(c.reason) << "\",";
        std::cout << "\"logs\":[";
        for (size_t i = 0; i < c.logs.size(); ++i) {
            if (i) std::cout << ",";
            std::cout << "\"" << jsonEscape(c.logs[i]) << "\"";
        }
        std::cout << "]}\n";
    }
    return 0;
}
`
}

// ---- helpers -------------------------------------------------------

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

func goModFile(workspace string) []byte {
	return []byte(fmt.Sprintf(`module ntt-run-suite
go 1.22
require github.com/nokia/ntt v0.0.0
replace github.com/nokia/ntt => %s
`, workspace))
}

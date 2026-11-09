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
	"github.com/nokia/ntt/commands/run"
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

	runLive    bool
	runTimeout time.Duration
)

// ntt run is commands/run's command. On this branch its default backend is
// eval, ntt's engine; --backend=t3xf runs the T3XF virtual machine. The
// engine's own flags, and --target, which compiles one file with the go or
// cpp backend instead, are added here.
func init() {
	run.RegisterBackend("eval", evalBackend)
	run.SetDefaultBackend("eval")
	c := run.Command
	c.Aliases = append(c.Aliases, "run2")
	c.Long = strings.TrimRight(c.Long, "\n") + `

With the eval backend (the default) the suite runs on ntt's engine, on its
virtual clock unless --live is given; --timeout bounds each testcase's wall
clock. --backend=t3xf compiles the suite to T3XF and runs it on the virtual
machine instead. NTT_LIST_BASKETS and NTT_COLORS may stand for the
basket and color variables above, and take precedence.

With --target go|cpp, ntt run instead lowers one TTCN-3 file through the
IR (ir/lower), emits source for that backend, builds a binary against the
runtime, executes it, and prints one line per testcase plus a final suite
verdict. Build artefacts go into a temp directory unless --keep is passed.`
	flags := c.Flags()
	flags.BoolVar(&runLive, "live", false, "with --backend=eval, run on a real clock (real timers, real concurrency) instead of the virtual one")
	flags.DurationVar(&runTimeout, "timeout", 0, "with --backend=eval, per-testcase wall-clock limit (0 = the 60s safety default)")
	flags.StringVar(&runTarget, "target", "", "compile one file with a backend, 'go' or 'cpp', and run it")
	flags.BoolVar(&runKeep, "keep", false, "with --target, keep the temporary build directory")
	flags.BoolVar(&runJSON, "json", false, "with --target, print the full result as JSON instead of text")
	flags.StringVar(&runWorkdir, "workdir", "", "with --target, use this directory for build artefacts (implies --keep)")
	suite := c.RunE
	c.RunE = func(cmd *cobra.Command, args []string) error {
		if runTarget != "" {
			if len(args) != 1 {
				return fmt.Errorf("run: --target takes one file")
			}
			return doRun(cmd, args)
		}
		if b := cmd.Flags().Lookup("backend"); b != nil && b.Value.String() != "eval" && (runLive || runTimeout != 0) {
			return fmt.Errorf("run: --live and --timeout are flags of the eval backend")
		}
		return suite(cmd, args)
	}
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

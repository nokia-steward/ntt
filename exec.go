package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/runtime/cfg"
	"github.com/nokia/ntt/runtime/exec"
	rreport "github.com/nokia/ntt/runtime/report"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
	"github.com/spf13/cobra"
)

var (
	execCfgPath  string
	execFormat   string
	execOutDir   string
	execPatterns []string

	// ExecCommand is the single-process executor entry point. It walks
	// a project, finds testcases (either explicitly via --pattern or
	// from a .cfg [EXECUTE] section), runs each one through a Driver
	// and writes reports in the formats requested by --format.
	ExecCommand = &cobra.Command{
		Use:   "exec [path...]",
		Short: "Run TTCN-3 testcases (single-process)",
		Long: `exec is the in-process test executor. It loads the project, parses
the .cfg file (if any), and runs the requested testcases through the
default Driver. Output is written as a stream to stdout in the format
chosen with --format (one of: text, json, junit, tap, html).

Testcases can be selected three ways, in priority order:
  --pattern        glob matched against the project's testcase list
  --cfg            .cfg file's [EXECUTE] section
  positional args  treated as a fallback set of file/dir paths to scan

If none of those produce a list, exec runs every discovered testcase.

NOTE: M4 ships a static-analysis driver that catches testcases via
their syntax declaration and reports them as Pass when the analyzer is
happy or Error when it isn't. The real interpreter rewire that lets
exec actually execute testcase bodies is the next iteration; this CLI
is the harness that will then need zero changes.`,
		RunE: runExec,
	}
)

func init() {
	RootCommand.AddCommand(ExecCommand)
	ExecCommand.Flags().StringVar(&execCfgPath, "cfg", "", "TTCN-3 module configuration file")
	ExecCommand.Flags().StringVar(&execFormat, "format", "text", "report format: text|json|junit|tap|html")
	ExecCommand.Flags().StringVar(&execOutDir, "out", "", "directory to write the report file (default: stdout)")
	ExecCommand.Flags().StringSliceVar(&execPatterns, "pattern", nil, "testcase patterns to run (glob)")
}

func runExec(cmd *cobra.Command, args []string) error {
	// Build the driver: discover testcases in args (or the working
	// directory), run the semantic analyzer per case as a stand-in for
	// real execution. This is the M4 baseline driver.
	if len(args) == 0 {
		args = []string{"."}
	}
	files := collectTTCN3Files(args)
	driver := newStaticDriver(files)

	var cfgFile *cfg.File
	if execCfgPath != "" {
		f, diags, err := cfg.Load(execCfgPath)
		if err != nil {
			return fmt.Errorf("load cfg: %w", err)
		}
		for _, d := range diags {
			fmt.Fprintf(os.Stderr, "cfg:%d: %s\n", d.Line, d.Message)
		}
		cfgFile = f
	}

	selectors := make([]exec.Selector, 0, len(execPatterns))
	for _, p := range execPatterns {
		selectors = append(selectors, exec.Selector{Name: p, Pattern: true})
	}

	suite, err := exec.Run(context.Background(), exec.Options{
		SuiteName: "ntt",
		Driver:    driver,
		Selectors: selectors,
		Config:    cfgFile,
	})
	if err != nil {
		return err
	}

	out := io.Writer(os.Stdout)
	if execOutDir != "" {
		if err := os.MkdirAll(execOutDir, 0o755); err != nil {
			return err
		}
		path := filepath.Join(execOutDir, "report."+execFormat)
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		defer f.Close()
		out = f
	}

	return writeReport(out, execFormat, suite)
}

func writeReport(w io.Writer, format string, suite *rreport.Suite) error {
	switch strings.ToLower(format) {
	case "junit", "xml":
		return rreport.RenderJUnit(w, suite)
	case "tap":
		return rreport.RenderTAP(w, suite)
	case "json":
		return rreport.RenderJSON(w, suite)
	case "html":
		return rreport.RenderHTML(w, suite)
	case "text", "":
		fmt.Fprintf(w, "suite %q: %s (%d cases in %s)\n", suite.Name, suite.Verdict(), len(suite.Cases), suite.Duration())
		for _, c := range suite.Cases {
			fmt.Fprintf(w, "  %-7s %s.%s\t%s\n", c.Verdict, c.Module, c.Name, c.Reason)
		}
		return nil
	}
	return fmt.Errorf("unknown format %q", format)
}

// staticDriver discovers testcases by walking the syntax trees of
// each file, then dispatches Run through the tree-walking
// interpreter. The verdict and reason come straight from the
// testcase's setverdict calls (or `pass` by default per TTCN-3
// clause 22.4.1).
//
// This used to be a parse-only stub during the M4 milestone. The M2
// follow-on rewire (interpreter.RunTestcase) replaced the stub with
// real execution; the executor's flag set and report formats are
// unchanged so nothing downstream had to move.
type staticDriver struct {
	cases    []string
	owner    map[string]string      // testcase name -> file path
	trees    map[string]*ttcn3.Tree // file path -> parsed tree, kept so Run can reuse them
	modParam map[string]string      // last cfg's [MODULE_PARAMETERS], threaded into RunTestcaseWith
}

// SetModuleParameters records the [MODULE_PARAMETERS] map produced
// by the .cfg loader so each testcase Run can re-apply it before
// the body runs. We snapshot the caller's map so a later mutation
// doesn't bleed into the running suite.
func (d *staticDriver) SetModuleParameters(params map[string]string) error {
	if len(params) == 0 {
		d.modParam = nil
		return nil
	}
	cp := make(map[string]string, len(params))
	for k, v := range params {
		cp[k] = v
	}
	d.modParam = cp
	return nil
}

func newStaticDriver(files []string) *staticDriver {
	d := &staticDriver{
		owner: map[string]string{},
		trees: map[string]*ttcn3.Tree{},
	}
	seen := map[string]bool{}
	for _, p := range files {
		tree := ttcn3.ParseFile(p)
		if tree == nil || tree.Root == nil {
			continue
		}
		d.trees[p] = tree
		for _, modNode := range tree.Modules() {
			mod, ok := modNode.Node.(*syntax.Module)
			if !ok {
				continue
			}
			modName := syntax.Name(mod.Name)
			mod.Inspect(func(n syntax.Node) bool {
				fn, ok := n.(*syntax.FuncDecl)
				if !ok {
					return true
				}
				if !fn.IsTest() {
					return true
				}
				qn := modName + "." + syntax.Name(fn.Name)
				if seen[qn] {
					return true
				}
				seen[qn] = true
				d.cases = append(d.cases, qn)
				d.owner[qn] = p
				return true
			})
		}
	}
	sort.Strings(d.cases)
	return d
}

func (d *staticDriver) Run(ctx context.Context, name string) (rreport.Verdict, string, error) {
	_ = ctx
	path := d.owner[name]
	if path == "" {
		return rreport.Error, "testcase not found", nil
	}
	tree := d.trees[path]
	if tree == nil {
		tree = ttcn3.ParseFile(path)
	}
	if tree == nil || tree.Err != nil {
		reason := "parse error"
		if tree != nil && tree.Err != nil {
			reason = tree.Err.Error()
		}
		return rreport.Error, reason, nil
	}
	// Hand the interpreter every parsed tree in the suite, with the
	// owning tree first so the testcase's own module wins any
	// duplicate-name resolution. The cross-module symbol table
	// inside RunTestcase walks the slice in order; sibling modules
	// must be visible there for `import from X all` and `import
	// from X { ... }` to resolve at exec time.
	trees := make([]*ttcn3.Tree, 0, len(d.trees))
	trees = append(trees, tree)
	for p, t := range d.trees {
		if p == path || t == nil {
			continue
		}
		trees = append(trees, t)
	}
	opts := interpreter.TestcaseOptions{
		ModuleParameters: d.modParam,
		ModuleParamWarning: func(msg string) {
			fmt.Fprintf(os.Stderr, "module parameter: %s\n", msg)
		},
	}
	v, reason, err := interpreter.RunTestcaseWith(trees, name, opts)
	if err != nil {
		return rreport.Error, err.Error(), nil
	}
	return mapVerdict(v), reason, nil
}

// mapVerdict translates the interpreter's runtime.Verdict (a string
// enum) into the report layer's Verdict (an int enum used for fast
// max-merge across testcases at suite level).
func mapVerdict(v runtime.Verdict) rreport.Verdict {
	switch v {
	case runtime.PassVerdict:
		return rreport.Pass
	case runtime.InconcVerdict:
		return rreport.Inconc
	case runtime.FailVerdict:
		return rreport.Fail
	case runtime.ErrorVerdict:
		return rreport.Error
	}
	return rreport.None
}

func (d *staticDriver) List() []string {
	out := make([]string, len(d.cases))
	copy(out, d.cases)
	return out
}

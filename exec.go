package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/runtime/cfg"
	"github.com/nokia/ntt/runtime/exec"
	"github.com/nokia/ntt/runtime/port/tcpport"
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
	execTimeout  time.Duration
	execLive     bool
	execProfile  bool

	// deterministicSafetyTimeout bounds a testcase when the user gave no
	// --timeout, so a body the scheduler does not yet fully model can't
	// wedge the suite.
	deterministicSafetyTimeout = 60 * time.Second

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

Execution model (default): a deterministic discrete-event scheduler (one
component runs at a time; virtual time advances only at quiescence) plus a
virtual clock. Concurrent components interleave deterministically and timers
fire virtually, so verdicts are reproducible and free of real-clock races. A
60s per-testcase safety timeout applies when --timeout is unset.

--live: the same strict engine on a REAL clock with real concurrency, for
driving a live system under test (real timers pace real I/O). Verdicts are
functional, not reproducible-by-construction; use --timeout to bound a run.
The virtual-clock default is for reproducible conformance; --live is for
testing (and later profiling) an external SUT over mapped/networked ports.`,
		RunE: runExec,
	}
)

func init() {
	RootCommand.AddCommand(ExecCommand)
	ExecCommand.Flags().StringVar(&execCfgPath, "cfg", "", "TTCN-3 module configuration file")
	ExecCommand.Flags().StringVar(&execFormat, "format", "text", "report format: text|json|junit|tap|html|profile")
	ExecCommand.Flags().StringVar(&execOutDir, "out", "", "directory to write the report file (default: stdout)")
	ExecCommand.Flags().StringSliceVar(&execPatterns, "pattern", nil, "testcase patterns to run (glob)")
	ExecCommand.Flags().DurationVar(&execTimeout, "timeout", 0,
		"per-testcase wall-clock limit (0 = none). A 60s safety default applies "+
			"when unset.")
	ExecCommand.Flags().BoolVar(&execLive, "live", false,
		"run the strict engine on a REAL clock (real timers, real concurrency) "+
			"for driving a live system under test, instead of the default "+
			"deterministic virtual clock. Bound runs with --timeout.")
	ExecCommand.Flags().BoolVar(&execProfile, "profile", false,
		"capture per-port performance metrics (send/receive counts, throughput, "+
			"send->receive latency percentiles) and report them. Implies --live "+
			"(latency is only meaningful on the real clock). Pair with "+
			"--format=profile for a metrics table, or --format=json for machine "+
			"output.")
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
	driver.timeout = execTimeout
	// --profile implies --live: real-clock latency is meaningless on the
	// virtual clock, where timers fire instantly.
	driver.live = execLive || execProfile
	driver.profiling = execProfile

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
		// Wire built-in test ports declared in [TESTPORT_PARAMETERS]. An
		// external transport drives real I/O, which needs the real clock —
		// switch to live so timers pace the network instead of firing
		// instantly on the virtual clock.
		if n := registerConfiguredTestPorts(cfgFile); n > 0 {
			driver.live = true
		}
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

// registerConfiguredTestPorts wires the built-in TCP test port for every
// port declared with `transport := "tcp"` in the .cfg's
// [TESTPORT_PARAMETERS], so a plain `ntt exec` drives a live SUT with no
// user Go code. Returns how many ports were registered.
//
// Keys follow the Titan grammar `<component>.<port>.<param> := "value"`
// (component is usually `*`, meaning any). Recognised params:
//
//	transport     "tcp" selects the built-in TCP port
//	address       "host:port" (overrides host/port)
//	host, port    combined into host:port when address is absent
//	dial_timeout  a Go duration, e.g. "5s" (default 10s)
//
// Ports are keyed by instance name (the goport resolver matches it), so a
// single address per port name is supported; a per-component address is a
// follow-on.
func registerConfiguredTestPorts(f *cfg.File) int {
	byPort := map[string]map[string]string{}
	var order []string
	for _, p := range f.TestPortParameters() {
		m := byPort[p.Port]
		if m == nil {
			m = map[string]string{}
			byPort[p.Port] = m
			order = append(order, p.Port)
		}
		// A specific component wins over a `*` wildcard for the same param,
		// regardless of declaration order.
		if _, ok := m[p.Param]; !ok || p.Component != "*" {
			m[p.Param] = p.Value
		}
	}
	n := 0
	for _, port := range order {
		params := byPort[port]
		if !strings.EqualFold(params["transport"], "tcp") {
			continue
		}
		addr := params["address"]
		if addr == "" && params["host"] != "" && params["port"] != "" {
			addr = net.JoinHostPort(params["host"], params["port"])
		}
		if addr == "" {
			fmt.Fprintf(os.Stderr, "testport %q: transport=tcp but no address (need address, or host+port)\n", port)
			continue
		}
		var opts []tcpport.Option
		if d := params["dial_timeout"]; d != "" {
			if dt, err := time.ParseDuration(d); err == nil {
				opts = append(opts, tcpport.WithDialTimeout(dt))
			} else {
				fmt.Fprintf(os.Stderr, "testport %q: bad dial_timeout %q: %v\n", port, d, err)
			}
		}
		tcpport.Register(port, addr, opts...)
		n++
	}
	return n
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
	case "profile":
		return rreport.RenderProfile(w, suite)
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

	timeout time.Duration // --timeout: per-testcase wall-clock bound (0 = none)
	live    bool          // --live: real clock + real concurrency (drive a live SUT) instead of the virtual clock

	profiling   bool             // --profile: capture per-port performance metrics
	lastMetrics *rreport.Metrics // metrics from the most recent Run, for LastMetrics
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
	// Default: the deterministic discrete-event scheduler (single-runner
	// token, virtual time advancing only at quiescence) plus the virtual
	// clock, so concurrent components interleave deterministically and
	// verdicts are reproducible. Under --live (d.live) the same strict engine
	// runs on the REAL clock with real concurrency — timers pace real I/O
	// against a live SUT — so leave both off. A per-testcase deadline still
	// bounds the run either way.
	if !d.live {
		opts.DeterministicClock = true
		opts.DeterministicScheduler = true
	}
	timeout := d.timeout
	if timeout <= 0 {
		timeout = deterministicSafetyTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	opts.Context = runCtx

	// Profiling: capture the raw per-port stats at run end and aggregate
	// them (with the wall-clock duration for throughput) into report
	// metrics that LastMetrics hands to the executor.
	d.lastMetrics = nil
	var rawStats map[string]runtime.PortStat
	if d.profiling {
		opts.Profiling = true
		opts.OnProfile = func(s map[string]runtime.PortStat) { rawStats = s }
	}
	start := time.Now()
	v, reason, err := interpreter.RunTestcaseWith(trees, name, opts)
	if d.profiling {
		d.lastMetrics = buildMetrics(rawStats, time.Since(start))
	}
	if err != nil {
		return rreport.Error, err.Error(), nil
	}
	return mapVerdict(v), reason, nil
}

// LastMetrics returns the performance profile of the most recent Run (nil
// when profiling is off or the run captured nothing). Implements
// exec.MetricsProvider; the executor calls it right after each Run.
func (d *staticDriver) LastMetrics() *rreport.Metrics { return d.lastMetrics }

// buildMetrics aggregates the interpreter's raw per-port capture into a
// report profile: latency percentiles per port, and throughput as
// receives over the testcase's wall-clock duration. Ports are ordered by
// key so the report is stable. Returns nil when nothing was captured.
func buildMetrics(raw map[string]runtime.PortStat, dur time.Duration) *rreport.Metrics {
	if len(raw) == 0 {
		return nil
	}
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	m := &rreport.Metrics{}
	for _, k := range keys {
		st := raw[k]
		var thr float64
		if dur > 0 {
			thr = float64(st.Receives) / dur.Seconds()
		}
		m.Ports = append(m.Ports, rreport.PortMetric{
			Port:       displayPortKey(k),
			Sends:      st.Sends,
			Receives:   st.Receives,
			Throughput: thr,
			Latency:    rreport.LatencyStatsFromSamples(st.Latencies),
		})
	}
	return m
}

// displayPortKey strips the internal per-component qualifier
// ("\x00c<id>/name", from runtime.PortKey) so a profile shows the plain
// port instance name.
func displayPortKey(k string) string {
	if len(k) > 0 && k[0] == 0 {
		if i := strings.IndexByte(k, '/'); i >= 0 {
			return k[i+1:]
		}
	}
	return k
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

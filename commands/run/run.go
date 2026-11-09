package run

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nokia/ntt/internal/backend"
	"github.com/nokia/ntt/internal/backend/vm"
	"github.com/nokia/ntt/internal/basket"
	"github.com/nokia/ntt/internal/cache"
	"github.com/nokia/ntt/internal/cli"
	"github.com/nokia/ntt/internal/framework"
	"github.com/nokia/ntt/internal/logfilter"

	"github.com/nokia/ntt/project"
	"github.com/spf13/cobra"
)

// outputPath is the file named by -o/--output. Empty means the log stays on
// the console only.
var outputPath string

// formatName is the console output format named by --format.
var formatName string

// backendName is the runtime named by --backend.
var backendName string

func init() {
	Command.Flags().StringVarP(&outputPath, "output", "o", "", "Write logging output to given `file`")
	Command.Flags().StringVar(&formatName, "format", "auto", "Console output `format`: auto, plain, or color")
	Command.Flags().StringVar(&backendName, "backend", "vm", "Runtime `backend` that executes the tests: vm only (k3r is not linked in this build)")
	_ = Command.Flags().MarkHidden("backend")
	Command.Flags().AddFlagSet(basket.Flags())
	Command.Flags().AddFlagSet(logfilter.Flags())
}

var (
	Command = &cobra.Command{
		Use:   "run [ <path>... ] [ -- <test id>... ]",
		Short: "Build and run test suite",
		Long: `Build and run a test suite.

The ntt run command first builds a test executable using the files or
directories passed as first argument list.
The test executable is then run. If no ids are specified, ntt run executes
every control part. When the suite declares none, it runs every testcase
from the sources. --regex, --exclude, --tags-regex, --tags-exclude, and
K3_LIST_BASKETS filter these default jobs: a filtered run executes the
matching testcases and does not run control parts.

Running control functions is supported. For example:

        ntt run -- test.A test.control test.B


Test baskets are also supported (see "ntt help list"). Bellow example will run
all tests with @stable-tag:

	K3_LIST_BASKETS=stable ntt run

Logging filters select which events are printed on the console. The file
given to --output keeps every event. Filters apply in the order given, on
top of all events being enabled:

        ntt run --activate=fnen|fnlv --deactivate=Suite:paon|rvon

--activate and --deactivate take an event-id regular expression, optionally
limited to one module as Module:pattern. --activate-all is the default when
no logging filter is given. A module id that is not in the suite, or an
event pattern that matches no event, is an error.

--format selects how those events are printed on the console. auto (the
default) colors log events when standard output is a terminal. color always
colors them. plain prints the raw k3log text. Error events are bold white
on red. A tcfi, cofi, or setv is yellow for inconc, green for pass, and red
for fail or error. plod, perf, vrsn, vach, and bctr are faint grey. auto
stays uncolored when NO_COLOR is set, when K3_COLORS=never, or when
TERM=dumb. The --output file stores raw events.

        ntt run --format=plain

A testcase may name the verdict it is expected to finish with:

        // @verdict: error
        testcase TC1() {}

When the final verdict differs, ntt run writes a k3log error and that
testcase fails the run. A verdict that matches the tag keeps the run
successful.

A module may instead carry an ETSI/PIXIT verdict tag. "pass accept" accepts
every testcase verdict except error; "pass reject" expects the module to fail
to build. Add "ttcn3verdict:<verdict>" to require that verdict of every
testcase in the module:

        // @verdict: pass accept, ttcn3verdict:pass
        module Suite {}

A testcase with a verdict tag, a ttcn3verdict, or a @k3:reason tag must run.
When it never finishes, for example because no control part executes it,
ntt run writes a VMIS error with an empty actual verdict, an RMIS error with
an empty actual reason for a @k3:reason tag, and the testcase fails the run:

        ...|VMIS->Verdict mismatch|k3=a.ttcn3:4|Suite.TC1|error|

Once is enough. A filtered run only requires the testcases it started.

A line may name the text it logs. Every ulog event from that line must have
exactly this text, as k3r logs it, and the line must log at least once:

        log(x); // @output: 1

A mismatch, or a line that logged nothing, writes an OMIS error and fails
the run. A filtered run only requires output from the testcases it started.
k3r logs ulog events of such a suite even when K3RFLAGS deactivates them.

A line may name the runtime error it causes, optionally with its k3log error
category:

        type A B(true, false) // @error: CONV

k3r must report an error event from that line, and every error event from
it must have the category. When k3r stops with exit status 2, for example
because it rejects the module while loading it, and every error event it
reported was expected, the run passes. A category mismatch, or a line that
reported no error, writes an EMIS error and fails the run. @error does not
change how a testcase verdict is judged: a testcase that ends with error
still needs "@k3:verdict error". A filtered run only requires errors from
the testcases it started. k3r logs error events of such a suite even when
K3RFLAGS deactivates them.

`,

		RunE: runTests,
	}
)

// Run runs the given jobs in parallel.
func runTests(cmd *cobra.Command, args []string) error {
	console, err := newFormatter(formatName, cmd.OutOrStdout())
	if err != nil {
		return err
	}

	conf, _, err := cli.Parse(cmd.ArgsLenAtDash(), args...)
	if err != nil {
		return fmt.Errorf("failed to open project: %w", err)
	}

	p, err := scan(conf)
	if err != nil {
		return fmt.Errorf("finding tests: %w", err)
	}

	b, err := basket.NewWithFlags("run", cmd.Flags())
	if err != nil {
		return err
	}
	if err := b.LoadFromEnvOrConfig(conf, "K3_LIST_BASKETS"); err != nil {
		return err
	}

	logs, err := logfilter.NewWithFlags(cmd.Flags(), p.Modules)
	if err != nil {
		return err
	}

	// Assure that the project binaries are up-to-date, before we execute the tests.
	tasks, err := framework.BuildTasks(conf)
	if err != nil {
		return fmt.Errorf("building test suite failed: %w", err)
	}
	for _, t := range tasks {
		if err := t.Run(); err != nil {
			if p.ExpectsRejection() {
				return nil
			}
			return fmt.Errorf("+ %s\n%w", t.String(), err)
		}
	}
	if p.ExpectsRejection() && len(p.Tests) == 0 {
		return fmt.Errorf("expected module rejection, but the build succeeded")
	}

	return runJobs(cmd, conf, p, b, logs, console)
}

// runJobs executes the selected jobs. Every k3r event is written to the output
// file when -o is set, as raw k3log text. The console receives the events the
// logging filter allows, rendered with the selected format. An empty selection
// returns without starting the runtime.
func runJobs(cmd *cobra.Command, conf *project.Config, p plan, b basket.Basket, logs logfilter.Filter, console formatter) error {
	logFile, err := openLogFile(outputPath)
	if err != nil {
		return err
	}
	closed := false
	closeFile := func() error {
		if logFile == nil || closed {
			return nil
		}
		closed = true
		if err := logFile.Close(); err != nil {
			return fmt.Errorf("closing log file: %w", err)
		}
		return nil
	}
	defer func() { _ = closeFile() }()

	list := p.selected(b)
	if len(list) == 0 {
		return closeFile()
	}

	runtime, err := newRuntime(cmd, conf, p)
	if err != nil {
		return err
	}
	// A k3r that rejects the module exits while the requests are written.
	// Its events and exit status tell why.
	for _, job := range list {
		if err := runtime.Start(job.Name); errors.Is(err, backend.ErrClosed) {
			break
		} else if err != nil {
			_ = runtime.Close()
			return err
		}
	}
	if err := runtime.CloseRequests(); err != nil && !errors.Is(err, backend.ErrClosed) {
		_ = runtime.Close()
		return err
	}
	track := tracker{jobs: list}
	check := newVerdictCheck(p, !basketActive(b), list)
	outputs := newOutputCheck(p.Outputs, !basketActive(b), list)
	errs := newErrorCheck(p.Errors, !basketActive(b), list)
	report := func(lines []string) error {
		for _, line := range lines {
			fmt.Fprintln(cmd.OutOrStdout(), formatLine(console, line))
			if err := writeLog(logFile, line); err != nil {
				return err
			}
		}
		return nil
	}
	for event := range runtime.Events() {
		if logs.Allows(event.ID(), p.eventModule(event)) {
			fmt.Fprintln(cmd.OutOrStdout(), console.Format(event))
		}
		if err := writeLog(logFile, event.String()); err != nil {
			_ = runtime.Close()
			return err
		}
		track.observe(event)
		lines := append(check.observe(event), outputs.observe(event)...)
		lines = append(lines, errs.observe(event)...)
		if err := report(lines); err != nil {
			_ = runtime.Close()
			return err
		}
	}
	exitErr := runtime.Close()
	if err := report(errs.finish(time.Now().Format(stampLayout))); err != nil {
		return err
	}
	if exitErr != nil && !errs.accepts(exitErr) {
		return exitErr
	}
	// After an accepted exit, k3r did not get to start the remaining testcases.
	if exitErr == nil {
		if err := report(check.finish(time.Now().Format(stampLayout))); err != nil {
			return err
		}
	}
	if err := report(outputs.finish(time.Now().Format(stampLayout))); err != nil {
		return err
	}
	if err := closeFile(); err != nil {
		return err
	}
	if len(check.failed) > 0 || len(outputs.failed) > 0 || len(errs.failed) > 0 {
		return &verdictFailure{tests: check.failed, outputs: outputs.failed, errors: errs.failed}
	}
	return nil
}

// newRuntime starts the backend named by --backend on the suite's T3XF.
func newRuntime(cmd *cobra.Command, conf *project.Config, p plan) (backend.Runtime, error) {
	t3xf := cache.Lookup(fmt.Sprintf("%s.t3xf", conf.Name))
	switch backendName {
	case "vm":
		return vm.NewRuntime(t3xf, vm.WithContext(cmd.Context()), vm.WithModuleParameters(moduleParameters(conf)))
	case "k3r":
		return nil, fmt.Errorf(`runtime backend "k3r" is not available in this build; use --backend=vm (default) or run with an external k3r`)
	default:
		return nil, fmt.Errorf("unknown backend %q, want vm", backendName)
	}
}

// stampLayout is the time layout of a k3log timestamp.
const stampLayout = "20060102T150405.000000"

// moduleParameters returns the suite's module parameters from the
// parameters file. YAML block scalars end with a newline, which is not part
// of the value. Empty values keep the declared default.
func moduleParameters(conf *project.Config) backend.ModuleParFunc {
	m := make(map[string]string)
	if conf != nil {
		for k, v := range conf.Parameters.Parameters {
			if v = strings.TrimRight(v, "\n"); v != "" {
				m[k] = v
			}
		}
	}
	return backend.ParameterMap(m)
}

// openLogFile creates or truncates the log file named by -o. An empty path
// leaves logging on the console only.
func openLogFile(path string) (*os.File, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening log file: %w", err)
	}
	return f, nil
}

// writeLog stores one log line. It does nothing when no log file was requested.
func writeLog(f *os.File, line string) error {
	if f == nil {
		return nil
	}
	if _, err := fmt.Fprintln(f, line); err != nil {
		return fmt.Errorf("writing log file: %w", err)
	}
	return nil
}

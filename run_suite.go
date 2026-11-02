package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nokia/ntt/internal/fs"
	"github.com/nokia/ntt/project"
	rreport "github.com/nokia/ntt/runtime/report"
	"github.com/nokia/ntt/runtime/tl"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/semantic"
	"github.com/nokia/ntt/ttcn3/syntax"
	"github.com/spf13/cobra"
)

// runSuite runs a test suite as `ntt run [ <path>... ] [ -- <test id>... ]`
// does: the test ids given, or every control part, or every testcase when the
// suite declares none; a basket filter runs the testcases it matches. Every
// event of the structured line log goes to --output, those the logging filter
// allows to the console. A testcase whose verdict is fail or error, or other
// than its @verdict tag expects, fails the run.
func runSuite(cmd *cobra.Command, args []string) error {
	console, err := newConsoleFormat(runConsole, cmd.OutOrStdout())
	if err != nil {
		return err
	}
	if Project == nil {
		return fmt.Errorf("failed to open project")
	}
	_, ids := splitArgs(args, cmd.ArgsLenAtDash())
	p, err := scanRunPlan(Project)
	if err != nil {
		return fmt.Errorf("finding tests: %w", err)
	}
	b, err := NewBasketWithFlags("run", cmd.Flags())
	if err != nil {
		return err
	}
	if err := b.LoadFromEnvOrConfig(Project, "NTT_LIST_BASKETS"); err != nil {
		return err
	}
	if err := runLogFilter.check(p.modules); err != nil {
		return err
	}

	files, err := runFiles(Project)
	if err != nil {
		return err
	}
	if err := buildSuite(files, p); err != nil {
		if p.expectsRejection() {
			return nil
		}
		return fmt.Errorf("building test suite failed: %w", err)
	}
	if p.expectsRejection() && len(p.tests) == 0 {
		return fmt.Errorf("expected module rejection, but the build succeeded")
	}

	jobs := p.selected(b)
	if len(ids) > 0 {
		jobs = p.named(ids)
	}
	return runJobs(cmd, files, p, jobs, console)
}

// runFiles is the suite's sources and the sources it imports.
func runFiles(conf *project.Config) ([]string, error) {
	srcs, err := fs.TTCN3Files(conf.Sources...)
	if err != nil {
		return nil, err
	}
	imports, err := fs.TTCN3Files(conf.Imports...)
	if err != nil {
		return nil, err
	}
	return append(srcs, imports...), nil
}

// buildSuite is what building the suite takes: every file parses, and, when
// a module expects to be rejected, the suite is checked as ntt check does.
func buildSuite(files []string, p runPlan) error {
	var errs []error
	var db *ttcn3.DB
	if p.rejectModule {
		db = &ttcn3.DB{}
		db.Index(files...)
	}
	for _, f := range files {
		tree := ttcn3.ParseFile(f)
		if tree == nil {
			errs = append(errs, fmt.Errorf("%s: cannot be read", f))
			continue
		}
		if tree.Err != nil {
			errs = append(errs, tree.Err)
			continue
		}
		if !p.rejectModule {
			continue
		}
		for _, d := range semantic.NewAnalyzer(db).Analyze(tree) {
			if d.Severity == semantic.SeverityError {
				errs = append(errs, fmt.Errorf("%s: %s", f, d.Message))
			}
		}
	}
	return errors.Join(errs...)
}

// runJobs runs jobs one after another on ntt's engine, writing its line log
// through a runLog.
func runJobs(cmd *cobra.Command, files []string, p runPlan, jobs []runJob, console consoleFormat) (err error) {
	var file *os.File
	if runOutput != "" {
		file, err = os.OpenFile(runOutput, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
		if err != nil {
			return fmt.Errorf("opening log file: %w", err)
		}
		defer func() {
			if cerr := file.Close(); cerr != nil && err == nil {
				err = fmt.Errorf("closing log file: %w", cerr)
			}
		}()
	}
	if len(jobs) == 0 {
		return nil
	}
	out := &runLog{console: cmd.OutOrStdout(), format: console, filter: &runLogFilter, plan: p}
	if file != nil {
		out.file = file
	}
	lines := tl.NewLinesWriter(out)

	driver := newStaticDriver(files)
	driver.modParam = moduleParameters(Project)
	driver.live = runLive
	driver.timeout = runTimeout
	// Testcases run one after another on the virtual clock each start from
	// the wall clock; keep the log's timestamps from going backwards.
	driver.testLogger = &tl.Monotonic{Next: lines}
	for _, job := range jobs {
		if !driver.hasJob(job.name) {
			_ = lines.Close()
			return fmt.Errorf("%s: no such testcase or control part", job.name)
		}
	}
	// What fails a job and its log does not show: a control part's own
	// error, a testcase that ends in error before its end is logged.
	var unlogged []string
	for _, job := range jobs {
		v, reason, err := driver.Run(context.Background(), job.name)
		if err != nil {
			_ = lines.Close()
			return err
		}
		if v != rreport.Error {
			continue
		}
		control := strings.HasSuffix(job.name, ".control")
		if control && reason != "" || !control && !out.finished[job.name] {
			if reason == "" {
				reason = "error"
			}
			unlogged = append(unlogged, fmt.Sprintf("%s: %s", job.name, reason))
		}
	}
	if err := lines.Close(); err != nil {
		return fmt.Errorf("writing log: %w", err)
	}
	out.flush()
	if out.err != nil {
		return out.err
	}
	for _, msg := range unlogged {
		fmt.Fprintln(cmd.ErrOrStderr(), msg)
	}
	if len(out.failed) > 0 || len(unlogged) > 0 {
		// The log has said which verdicts failed: exit 1 with no message.
		return errSilentFailure
	}
	return nil
}

// errSilentFailure fails a command whose output has already said why.
var errSilentFailure = errors.New("")

// moduleParameters are the module parameters of the project's parameters
// file; a YAML block scalar's final newline is not part of the value, and an
// empty value keeps the declared default.
func moduleParameters(conf *project.Config) map[string]string {
	if conf == nil {
		return nil
	}
	m := map[string]string{}
	for k, v := range conf.Parameters.Parameters {
		if v = strings.TrimRight(v, "\n"); v != "" {
			m[k] = v
		}
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

// runLog takes the lines of the structured line log: every line to the
// file, those the filter allows to the console, and after a testcase's tcfi
// the error of a verdict its @verdict tag did not expect.
type runLog struct {
	file    io.Writer
	console io.Writer
	format  consoleFormat
	filter  *logFilter
	plan    runPlan

	buf    []byte
	err    error
	failed []string
	// finished are the testcases whose end (tcfi) was logged.
	finished map[string]bool
}

func (l *runLog) Write(b []byte) (int, error) {
	l.buf = append(l.buf, b...)
	for {
		i := strings.IndexByte(string(l.buf), '\n')
		if i < 0 {
			break
		}
		line := string(l.buf[:i])
		l.buf = l.buf[i+1:]
		l.line(line)
	}
	return len(b), l.err
}

// flush takes a last line without its newline.
func (l *runLog) flush() {
	if len(l.buf) > 0 {
		line := string(l.buf)
		l.buf = nil
		l.line(line)
	}
}

func (l *runLog) line(line string) {
	l.emit(line, l.filter.allows(lineEventID(line), l.plan.eventModule(line)))
	if mismatch := l.observe(line); mismatch != "" {
		l.emit(mismatch, true)
	}
}

func (l *runLog) emit(line string, toConsole bool) {
	if l.file != nil && l.err == nil {
		if _, err := fmt.Fprintln(l.file, line); err != nil {
			l.err = fmt.Errorf("writing log file: %w", err)
		}
	}
	if toConsole {
		fmt.Fprintln(l.console, l.format.format(line))
	}
}

// observe judges the verdict of a tcfi line against what the testcase
// expects, and returns the error line of a mismatch:
//
//	timestamp|VMIS->Verdict mismatch|runtime[=file:line]|testcase|expected|actual
//
// runtime is the runtime's identity, tl.RuntimeID.
func (l *runLog) observe(line string) string {
	if lineEventID(line) != "tcfi" {
		return ""
	}
	fields := strings.Split(line, "|")
	if len(fields) < 5 {
		return ""
	}
	name, _, _ := strings.Cut(fields[3], "(")
	verdict := fields[4]
	if l.finished == nil {
		l.finished = map[string]bool{}
	}
	l.finished[name] = true
	mismatch := func(tc runJob, expected string) string {
		l.failed = append(l.failed, name)
		id := tl.RuntimeID()
		if tc.loc != "" {
			id += "=" + tc.loc
		}
		return fmt.Sprintf("%s|VMIS->Verdict mismatch|%s|%s|%s|%s", fields[0], id, name, expected, verdict)
	}
	if tc, ok := l.plan.test(name); ok {
		switch tc.expect.kind {
		case expectExact:
			if verdict == tc.expect.verdict {
				return ""
			}
			return mismatch(tc, tc.expect.verdict)
		case expectNotError:
			if verdict != "error" {
				return ""
			}
		case expectReject:
			if verdict == "error" {
				return ""
			}
			return mismatch(tc, "error")
		}
	}
	if verdict == "fail" || verdict == "error" {
		l.failed = append(l.failed, name)
	}
	return ""
}

// hasJob reports whether the suite has the testcase or the control part
// ("Module.control") name.
func (d *staticDriver) hasJob(name string) bool {
	if _, ok := d.owner[name]; ok {
		return true
	}
	mod, ok := strings.CutSuffix(name, ".control")
	if !ok {
		return false
	}
	for _, p := range d.files {
		tree := d.trees[p]
		if tree == nil {
			continue
		}
		for _, m := range tree.Modules() {
			node, ok := m.Node.(*syntax.Module)
			if !ok || syntax.Name(node.Name) != mod {
				continue
			}
			for _, def := range node.Defs {
				if _, ok := def.Def.(*syntax.ControlPart); ok {
					return true
				}
			}
		}
	}
	return false
}

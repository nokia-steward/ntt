// Package exec is the single-process test executor: it loads a project,
// resolves the testcases requested by `.cfg` files or command-line
// flags, runs each one through the runtime, aggregates verdicts and
// writes reports.
//
// The executor is intentionally codec/interpreter-agnostic: a Driver
// implements the actual "given a testcase name, run it and produce a
// Verdict" contract. The default driver uses the tree-walking
// interpreter in `interpreter`; future drivers (Go-backend, C++ codegen)
// plug in by satisfying the same interface.
package exec

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/nokia/ntt/runtime/cfg"
	"github.com/nokia/ntt/runtime/report"
)

// Selector picks a testcase from the project, either by fully-qualified
// name ("Module.tc_name") or by glob pattern ("Module.*"). Selectors
// from a .cfg [EXECUTE] section come in as exact names.
type Selector struct {
	Name    string
	Pattern bool
}

// Driver is the runtime-specific surface the executor talks to. The
// production interpreter implements it; tests use a tiny in-memory
// driver that doesn't require parsing TTCN-3.
type Driver interface {
	// Run executes the testcase identified by name and returns its
	// verdict + any failure reason. The driver is responsible for any
	// runtime setup (component creation, port wiring, codec dispatch).
	Run(ctx context.Context, name string) (report.Verdict, string, error)

	// List enumerates every testcase the driver knows about. The
	// executor uses this to expand patterns and to support running all
	// testcases when no .cfg is provided.
	List() []string
}

// ModuleParamSetter is the optional surface a Driver can implement
// to receive the [MODULE_PARAMETERS] map parsed out of a .cfg
// file. The executor pushes the map once, before any testcase
// runs; drivers that don't support overrides simply don't
// implement the interface (the executor checks via type assertion).
//
// SetModuleParameters is called at most once per exec.Run; a
// non-nil error aborts the run before any testcase fires.
type ModuleParamSetter interface {
	SetModuleParameters(params map[string]string) error
}

// Options configures one executor invocation.
type Options struct {
	SuiteName string
	Driver    Driver
	Selectors []Selector
	Config    *cfg.File
}

// Run schedules the requested testcases on the driver, collects their
// verdicts, and returns a Suite ready to be passed to report.Render*.
//
// The execution model is intentionally simple: testcases run
// sequentially in the calling goroutine. M5 introduces parallel
// execution across PTCs and remote hosts.
func Run(ctx context.Context, opts Options) (*report.Suite, error) {
	if opts.Driver == nil {
		return nil, fmt.Errorf("exec: Driver is required")
	}
	// Push [MODULE_PARAMETERS] into the driver *before* we touch
	// the testcase list. A driver that doesn't support overrides
	// (e.g. the in-memory mctr test driver) just skips this step.
	// Errors from the setter abort the suite - a bad override is a
	// configuration bug, not a runtime fault.
	if opts.Config != nil {
		if setter, ok := opts.Driver.(ModuleParamSetter); ok {
			if params := opts.Config.ModuleParameters(); len(params) > 0 {
				if err := setter.SetModuleParameters(params); err != nil {
					return nil, fmt.Errorf("apply module parameters: %w", err)
				}
			}
		}
	}
	cases := expandSelectors(opts.Selectors, opts.Driver)
	if len(cases) == 0 && opts.Config != nil {
		for _, name := range opts.Config.ExecuteList() {
			cases = append(cases, name)
		}
	}
	if len(cases) == 0 {
		cases = opts.Driver.List()
	}

	suite := &report.Suite{
		Name:  opts.SuiteName,
		Start: time.Now(),
	}
	for _, name := range cases {
		caseStart := time.Now()
		verdict, reason, err := opts.Driver.Run(ctx, name)
		if err != nil && verdict == report.None {
			verdict = report.Error
			if reason == "" {
				reason = err.Error()
			}
		}
		suite.Cases = append(suite.Cases, report.Case{
			Module:   moduleOf(name),
			Name:     localName(name),
			Verdict:  verdict,
			Reason:   reason,
			Duration: time.Since(caseStart),
		})
	}
	suite.End = time.Now()
	report.SortCases(suite.Cases)
	return suite, nil
}

// expandSelectors maps the user's selector list to a deduplicated list
// of testcase names known to the driver. Patterns expand against the
// driver's List().
func expandSelectors(sels []Selector, d Driver) []string {
	if len(sels) == 0 {
		return nil
	}
	known := d.List()
	out := make([]string, 0, len(sels))
	seen := map[string]bool{}
	for _, s := range sels {
		if !s.Pattern {
			if !seen[s.Name] {
				seen[s.Name] = true
				out = append(out, s.Name)
			}
			continue
		}
		for _, n := range known {
			if matchPattern(s.Name, n) && !seen[n] {
				seen[n] = true
				out = append(out, n)
			}
		}
	}
	sort.Strings(out)
	return out
}

// matchPattern implements a small glob with `*` matching any run of
// non-dot characters and `**` matching across dots. It's not full
// glob but covers the testcase-name patterns users actually write.
func matchPattern(pat, name string) bool {
	// Trivial cases first.
	if pat == "*" || pat == "**" {
		return true
	}
	pi, ni := 0, 0
	for pi < len(pat) && ni < len(name) {
		switch pat[pi] {
		case '*':
			if pi+1 < len(pat) && pat[pi+1] == '*' {
				return matchSuffix(pat[pi+2:], name[ni:])
			}
			// Single-star matches anything except a dot until the
			// next pattern char (or end).
			next := byte(0)
			if pi+1 < len(pat) {
				next = pat[pi+1]
			}
			for ni < len(name) {
				if name[ni] == '.' {
					break
				}
				if next != 0 && name[ni] == next {
					break
				}
				ni++
			}
			pi++
		default:
			if pat[pi] != name[ni] {
				return false
			}
			pi++
			ni++
		}
	}
	for pi < len(pat) && pat[pi] == '*' {
		pi++
	}
	return pi == len(pat) && ni == len(name)
}

func matchSuffix(suffix, rest string) bool {
	if suffix == "" {
		return true
	}
	for i := 0; i <= len(rest); i++ {
		if rest[i:] == suffix {
			return true
		}
	}
	return false
}

func moduleOf(qualified string) string {
	for i, c := range qualified {
		if c == '.' {
			return qualified[:i]
		}
	}
	return ""
}

func localName(qualified string) string {
	for i := len(qualified) - 1; i >= 0; i-- {
		if qualified[i] == '.' {
			return qualified[i+1:]
		}
	}
	return qualified
}

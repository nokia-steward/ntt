// Package lint implements a reusable, AST-based TTCN-3 linter that produces
// structured Problem records suitable for both CLI reporting and LSP
// diagnostics.
//
// Unlike the legacy lint command in the top-level ntt package (which is
// tightly coupled to global state, regexp config and stdout printing), this
// package exposes:
//
//   - A Problem type that carries a code, severity, message, source span and
//     an optional Autofix suggestion.
//   - A Rule interface so individual checks can be plugged in and tested in
//     isolation.
//   - A Linter that runs a configurable set of rules across a parsed Tree.
//
// The Rule lifecycle (Register / Check / Exit) and the
// Reporter-based problem-emission API are inspired by the
// classic visitor / context split familiar from many static
// analyzers (e.g. go vet, ESLint).
//
// The package intentionally ships with only a small, opinionated set of
// rules. Additional rules can be added by satisfying the Rule interface and
// passing them to NewLinter.
package lint

import (
	"sort"
	"sync"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// Severity classifies how serious a Problem is. The values intentionally
// match the LSP DiagnosticSeverity ordering (1=Error … 4=Hint) so callers can
// convert with a trivial cast.
type Severity int

const (
	SeverityError Severity = 1
	SeverityWarn  Severity = 2
	SeverityInfo  Severity = 3
	SeverityHint  Severity = 4
)

// Autofix describes a single, safe textual replacement that resolves a
// Problem. The byte range is half-open and refers to the parsed source.
type Autofix struct {
	// Title is a short, human-readable label shown in the editor's code
	// action menu, e.g. "Remove unused import".
	Title string

	// Begin and End are byte offsets into the original source. They form a
	// half-open range [Begin, End).
	Begin int
	End   int

	// Replacement is the text that should replace the range. It may be
	// empty to indicate a deletion.
	Replacement string
}

// Problem is the canonical lint finding. It is independent from any
// particular output format.
type Problem struct {
	// Code is a stable, short identifier for the rule that produced this
	// problem (e.g. "unused-import"). Editors use this to group and
	// suppress diagnostics.
	Code string

	// Severity is the importance of the finding.
	Severity Severity

	// Message is the human-readable description of the problem.
	Message string

	// Node is the AST node the problem refers to. It is used to compute
	// the source span; callers can use the node directly to produce richer
	// reports.
	Node syntax.Node

	// Span is the source range of the problem. It is precomputed from
	// Node so that callers don't need to chase it through SpanOf.
	Span syntax.Span

	// Fix is an optional suggested edit. When present, code-action capable
	// clients can apply it directly.
	Fix *Autofix
}

// Rule is a single check that inspects a parsed Tree and reports any
// problems via the provided Reporter.
//
// Rules must be safe to invoke concurrently for different trees but may
// retain per-invocation state internally as long as they don't share it
// between trees.
type Rule interface {
	// Code returns the stable identifier of the rule.
	Code() string

	// Check runs the rule against tree and emits any findings through
	// report. Implementations should be tolerant of partial / erroneous
	// trees - the LSP runs them on every keystroke.
	Check(tree *ttcn3.Tree, report Reporter)
}

// Reporter is the callback used by rules to emit problems.
type Reporter func(Problem)

// Linter applies a set of rules to one or more trees.
type Linter struct {
	rules []Rule
}

// NewLinter returns a Linter pre-configured with the provided rules.
// Passing zero rules returns a no-op linter, which is occasionally useful
// in tests.
func NewLinter(rules ...Rule) *Linter {
	out := &Linter{rules: make([]Rule, 0, len(rules))}
	out.rules = append(out.rules, rules...)
	return out
}

// DefaultLinter returns a Linter with the built-in rule set enabled. This
// is what the LSP uses by default.
func DefaultLinter() *Linter {
	return NewLinter(DefaultRules()...)
}

// DefaultRules returns the built-in rule set. Callers that want to compose
// a custom Linter can append to or filter this slice.
func DefaultRules() []Rule {
	return []Rule{
		&UnusedImportRule{},
		&EmptyBlockRule{},
		&AlignedBracesRule{},
		&MissingCaseElseRule{},
	}
}

// Rules returns the configured rule set.
func (l *Linter) Rules() []Rule { return l.rules }

// problemBufPool recycles the per-invocation slice of problems. The LSP
// runs Lint on every keystroke, and most calls produce a handful of
// problems at most, so reusing the underlying array is a measurable
// allocation win at zero correctness cost.
var problemBufPool = sync.Pool{
	New: func() interface{} {
		b := make([]Problem, 0, 16)
		return &b
	},
}

// Lint runs all configured rules against tree and returns the problems
// sorted by source position. Lint is safe to call concurrently across
// different trees.
func (l *Linter) Lint(tree *ttcn3.Tree) []Problem {
	if tree == nil || tree.Root == nil {
		return nil
	}

	bufPtr := problemBufPool.Get().(*[]Problem)
	buf := (*bufPtr)[:0]
	defer func() {
		// Hand the buffer back to the pool when we're done. We don't
		// shrink it: typical lint runs settle around the same size,
		// and a larger backing array reduces grow-and-copy churn on
		// the next call.
		*bufPtr = buf[:0]
		problemBufPool.Put(bufPtr)
	}()

	var mu sync.Mutex
	report := func(p Problem) {
		if p.Node != nil && !p.Span.Begin.IsValid() {
			p.Span = syntax.SpanOf(p.Node)
		}
		mu.Lock()
		buf = append(buf, p)
		mu.Unlock()
	}

	for _, rule := range l.rules {
		rule.Check(tree, report)
	}

	// Return a fresh copy so the caller can outlive the pooled buffer.
	out := make([]Problem, len(buf))
	copy(out, buf)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].Span.Begin, out[j].Span.Begin
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Column < b.Column
	})
	return out
}

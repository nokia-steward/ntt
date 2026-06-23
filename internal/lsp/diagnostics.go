package lsp

import (
	"context"
	"errors"

	"github.com/hashicorp/go-multierror"
	"github.com/nokia/ntt/internal/fs"
	"github.com/nokia/ntt/internal/lsp/protocol"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/lint"
	"github.com/nokia/ntt/ttcn3/semantic"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// linter is the singleton instance used to produce lint diagnostics. It is
// stateless and safe to share across requests, so we pre-allocate it once.
var linter = lint.DefaultLinter()

// Diagnose runs various checks over a ttcn3 test suite.
//
// From LSP spec:
//
//	Diagnostics are "owned" by the server so it is the server's
//	responsibility to clear them if necessary.
//
//	If a language has a project system (for example C#) diagnostics are not
//	cleared when a file closes.  When a project is opened all diagnostics
//	for all files are recomputed (or read from a cache).
//
//	When a file changes it is the server’s responsibility to re-compute
//	diagnostics and push them to the client. If the computed set is empty it
//	has to push the empty array to clear former diagnostics. Newly pushed
//	diagnostics always replace previously pushed diagnostics. There is no
//	merging that happens on the client side.
func (s *Server) Diagnose(uris ...protocol.DocumentURI) {
	s.diagsMu.Lock()
	defer s.diagsMu.Unlock()

	s.diags = make(map[string][]protocol.Diagnostic)
	defer s.syncDiagnostics()

	for _, uri := range uris {
		// Publish an empty list first so that previously-reported
		// diagnostics for this file are cleared. The deferred
		// syncDiagnostics call will then push the real findings.
		s.client.PublishDiagnostics(context.TODO(), &protocol.PublishDiagnosticsParams{
			Diagnostics: make([]protocol.Diagnostic, 0),
			URI:         uri,
		})
		tree := ttcn3.ParseFile(string(uri))
		if err := tree.Err; err != nil {
			s.reportError(err)
		}
		// Only run the linter on trees that parsed cleanly. Running it
		// on a broken tree produces too much noise to be useful while
		// the user is mid-edit.
		if tree.Err == nil {
			s.reportLintProblems(uri, linter.Lint(tree))
			s.reportSemanticDiagnostics(uri, semantic.NewAnalyzer(&s.db).Analyze(tree))
		}
	}
}

// reportSemanticDiagnostics converts semantic-analyzer findings into LSP
// diagnostics and records them against the given URI. Like the lint
// problems they are flushed in bulk by the deferred syncDiagnostics call
// in Diagnose.
func (s *Server) reportSemanticDiagnostics(uri protocol.DocumentURI, diags []semantic.Diagnostic) {
	if len(diags) == 0 {
		return
	}
	key := string(uri)
	for _, d := range diags {
		s.diags[key] = append(s.diags[key], protocol.Diagnostic{
			Severity: protocol.DiagnosticSeverity(d.Severity),
			Source:   "ntt-semantic",
			Code:     d.Code,
			Message:  d.Message,
			Range:    setProtocolRange(d.Span.Begin, d.Span.End),
		})
	}
}

// reportLintProblems converts the structured lint findings into LSP
// diagnostics and records them against the given URI for syncDiagnostics to
// publish in bulk.
func (s *Server) reportLintProblems(uri protocol.DocumentURI, problems []lint.Problem) {
	if len(problems) == 0 {
		return
	}
	lintCfg := s.LintConfig()
	key := string(uri)
	for _, p := range problems {
		if lintCfg.IsRuleDisabled(p.Code) {
			continue
		}
		diag := protocol.Diagnostic{
			Severity: protocol.DiagnosticSeverity(p.Severity),
			Source:   "ntt-lint",
			Code:     p.Code,
			Message:  p.Message,
			Range:    setProtocolRange(p.Span.Begin, p.Span.End),
		}
		// Attach the autofix payload so the codeAction handler can
		// produce a quick fix without re-running the linter.
		if p.Fix != nil {
			diag.Data = map[string]interface{}{
				"autofix": map[string]interface{}{
					"title":       p.Fix.Title,
					"begin":       p.Fix.Begin,
					"end":         p.Fix.End,
					"replacement": p.Fix.Replacement,
				},
			}
		}
		s.diags[key] = append(s.diags[key], diag)
	}
}

func (s *Server) reportError(err error) {
	var (
		serr syntax.Error
		merr *multierror.Error
	)

	switch {

	// Unpack multierrors
	case errors.As(err, &merr):
		for _, e := range merr.Errors {
			s.reportError(e)
		}

	// Errors with a location will become diagnostics
	case errors.As(err, &serr):
		span := syntax.SpanOf(serr.Node)
		uri := string(fs.Open(span.Filename).URI())
		diag := protocol.Diagnostic{
			Severity: protocol.SeverityError,
			Source:   string(fs.URI(span.Filename)),
			Range:    setProtocolRange(span.Begin, span.End),
			Message:  serr.Msg,
		}
		s.diags[uri] = append(s.diags[uri], diag)

	// Unknown errors and errors without location will become error notification.
	default:
		s.Fatal(context.TODO(), err.Error())

	}
}

func (s *Server) syncDiagnostics() {
	if s.serverConfig.DiagnosticsEnabled {
		for k, v := range s.diags {
			s.client.PublishDiagnostics(context.TODO(), &protocol.PublishDiagnosticsParams{
				Diagnostics: v,
				URI:         protocol.DocumentURI(k),
			})
		}
	}
}

package lsp

import (
	"github.com/nokia/ntt/project"
)

// FmtConfig returns the manifest-level [tools.fmt] section for the
// first suite known to the server, or a zero FmtOptions when no
// manifest defines one. It is the canonical lookup the LSP
// formatter handler consults before falling back to defaults.
//
// We pick the first suite deliberately: in a multi-root workspace each
// suite typically inherits the same fmt settings, and asking the user
// which suite to use would be more disruptive than helpful. A future
// iteration may select per-file.
func (s *Server) FmtConfig() project.FmtOptions {
	suites := s.snapshotSuites()
	if len(suites) == 0 || suites[0] == nil || suites[0].Config == nil {
		return project.FmtOptions{}
	}
	return suites[0].Config.Tools.Fmt.Options()
}

// LintConfig returns the manifest-level [tools.lint] section for the
// first suite, or a zero value when no manifest defines one. The LSP
// linter consults this to drop diagnostics from disabled rules before
// publishing them.
func (s *Server) LintConfig() project.LintTool {
	suites := s.snapshotSuites()
	if len(suites) == 0 || suites[0] == nil || suites[0].Config == nil {
		return project.LintTool{}
	}
	return suites[0].Config.Tools.Lint
}

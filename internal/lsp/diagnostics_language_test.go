package lsp

import (
	"fmt"
	"testing"

	"github.com/nokia/ntt/internal/fs"
	"github.com/nokia/ntt/internal/lsp/protocol"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/semantic"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// A module the server analyzes is in the edition of the suite its file is
// in, whatever the language of the directory the server was started in;
// of a suite inside another, the inner one's.
func TestDiagnoseReadsTheSuiteLanguage(t *testing.T) {
	defer func(l string) { syntax.DefaultLanguage = l }(syntax.DefaultLanguage)

	src := `module %s { type enumerated E { A, B(1) } control { var E e := A(0) } }`
	files := map[string]string{
		"TestDiagnoseLanguage_v5.ttcn3":    fmt.Sprintf(src, "M5"),
		"TestDiagnoseLanguage_v4.ttcn3":    fmt.Sprintf(src, "M4"),
		"TestDiagnoseLanguage_inner.ttcn3": fmt.Sprintf(src, "MI"),
	}
	v5 := newTestSuite(t, map[string]string{"TestDiagnoseLanguage_v5.ttcn3": files["TestDiagnoseLanguage_v5.ttcn3"]})
	v5.Config.Root, v5.Config.Language = "/v5", syntax.V5Language
	v4 := newTestSuite(t, map[string]string{
		"TestDiagnoseLanguage_v4.ttcn3":    files["TestDiagnoseLanguage_v4.ttcn3"],
		"TestDiagnoseLanguage_inner.ttcn3": files["TestDiagnoseLanguage_inner.ttcn3"],
	})
	v4.Config.Root = "/v4"
	inner := newTestSuite(t, map[string]string{"TestDiagnoseLanguage_inner.ttcn3": files["TestDiagnoseLanguage_inner.ttcn3"]})
	inner.Config.Root, inner.Config.Language = "/v4/inner", syntax.V5Language
	s := &Server{Suites: Suites{roots: map[string]*Suite{"/v5": v5, "/v4": v4, "/v4/inner": inner}}}

	want := map[string]syntax.Edition{
		"TestDiagnoseLanguage_v5.ttcn3":    syntax.V5,
		"TestDiagnoseLanguage_v4.ttcn3":    syntax.V4,
		"TestDiagnoseLanguage_inner.ttcn3": syntax.V5,
	}
	for _, global := range []string{"", syntax.V5Language} {
		syntax.DefaultLanguage = global
		for file, edition := range want {
			uri := protocol.DocumentURI(fs.URI(file))
			tree := ttcn3.ParseFile(string(uri))
			if tree.Err != nil {
				t.Fatalf("%s: %v", file, tree.Err)
			}
			s.setDefaultLanguage(uri, tree)
			if got := tree.Nodes[0].(*syntax.Module).Edition(); got != edition {
				t.Errorf("%s, started in %q: edition %d, want %d", file, global, got, edition)
			}
			// V4 has no integer of a label's own to name; V5 does.
			ordinal := false
			for _, d := range semantic.NewAnalyzer(&ttcn3.DB{}).Analyze(tree) {
				ordinal = ordinal || d.Code == "enum-ordinal-in-value-ref"
			}
			if ordinal != (edition == syntax.V4) {
				t.Errorf("%s, started in %q: enum-ordinal-in-value-ref reported %v in edition %d", file, global, ordinal, edition)
			}
		}
	}
}

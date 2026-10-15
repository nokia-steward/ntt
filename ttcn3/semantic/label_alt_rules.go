// label_alt_rules.go enforces ETSI ES 201 873-1 clause 19.7:
// "Labels are not allowed to label the alternatives within an
//  \`alt\` statement, nor an \`interleave\` statement."
//
// In the parser an \`alt { ... }\` block's \`Body.Stmts\` is a
// mix of \`CommClause\` (\`[]\` alternatives) and any other
// statements written between alternatives. A bare \`label L\`
// (a \`BranchStmt\` with \`Tok.Kind() == LABEL\`) sitting at
// that level is the exact pattern this rule rejects, e.g.
//
//	alt {
//	    [] p.receive { ... }
//	    label L_wrong;          // ← error
//	    [] p.receive { ... }
//	}
//
// Labels still legal inside the body of a comm clause are
// untouched.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkLabelAltRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		if fd, ok := d.Def.(*syntax.FuncDecl); ok && fd != nil &&
			fd.KindTok != nil && fd.KindTok.Kind() == syntax.ALTSTEP &&
			fd.Body != nil {
			diags = append(diags, labelAtAltLevelDiags(fd.Body, "altstep")...)
		}
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch s := n.(type) {
		case *syntax.AltStmt:
			if s == nil || s.Body == nil {
				return true
			}
			diags = append(diags, labelAtAltLevelDiags(s.Body, "alt")...)
		}
		return true
	})
	return diags
}

func labelAtAltLevelDiags(body *syntax.BlockStmt, kind string) []Diagnostic {
	var diags []Diagnostic
	for _, st := range body.Stmts {
		bs, ok := st.(*syntax.BranchStmt)
		if !ok || bs == nil || bs.Tok == nil {
			continue
		}
		if bs.Tok.Kind() != syntax.LABEL {
			continue
		}
		name := ""
		if bs.Label != nil {
			name = bs.Label.String()
		}
		diags = append(diags, Diagnostic{
			Code:     "label-at-alt-alternative-level",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"label %q sits at the alternative level of an %s statement; labels are not allowed there (ETSI 19.7)",
				name, kind),
			Node: bs,
			Span: syntax.SpanOf(bs),
		})
	}
	return diags
}

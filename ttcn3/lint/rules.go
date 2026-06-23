package lint

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// UnusedImportRule reports `import from M all;` and explicit `type/const/...`
// imports whose imported names are never referenced inside the importing
// module. The rule produces an Autofix that removes the offending import
// declaration entirely (the legacy CLI lint only warned and required manual
// cleanup).
type UnusedImportRule struct{}

func (r *UnusedImportRule) Code() string { return "unused-import" }

func (r *UnusedImportRule) Check(tree *ttcn3.Tree, report Reporter) {
	for _, mod := range tree.Modules() {
		m, ok := mod.Node.(*syntax.Module)
		if !ok {
			continue
		}

		// Collect every identifier referenced inside this module so we
		// can check imports against it. We intentionally inspect Tok
		// strings rather than resolved symbols: that gives us a
		// conservative over-approximation - if a name appears anywhere
		// in source we treat the import as used. This avoids false
		// positives until full symbol resolution is in place.
		used := make(map[string]bool)
		m.Inspect(func(n syntax.Node) bool {
			switch n := n.(type) {
			case *syntax.ImportDecl:
				// Don't count uses inside the import itself.
				return false
			case *syntax.Ident:
				if n.Tok != nil {
					used[n.Tok.String()] = true
				}
				if n.Tok2 != nil {
					used[n.Tok2.String()] = true
				}
			}
			return true
		})

		// Walk top-level import declarations and check each.
		m.Inspect(func(n syntax.Node) bool {
			imp, ok := n.(*syntax.ImportDecl)
			if !ok {
				return true
			}
			r.checkImport(imp, used, report)
			return false
		})
	}
}

func (r *UnusedImportRule) checkImport(imp *syntax.ImportDecl, used map[string]bool, report Reporter) {
	if imp.Module == nil || imp.Module.Tok == nil {
		return
	}
	modName := imp.Module.Tok.String()

	// "import from M all;" - we have no specific names to check, so
	// fall back to "module name itself appears as a qualifier somewhere".
	if len(imp.List) == 0 {
		if used[modName] {
			return
		}
		report(Problem{
			Code:     r.Code(),
			Severity: SeverityWarn,
			Message:  fmt.Sprintf("import of %q appears to be unused", modName),
			Node:     imp,
			Fix:      removeNodeFix(imp, fmt.Sprintf("Remove unused import of %q", modName)),
		})
		return
	}

	// Explicit kind imports: "import from M { type A, B; const C; }". We
	// keep the import if any of its imported names (or the module name)
	// appears elsewhere.
	if used[modName] {
		return
	}
	for _, kind := range imp.List {
		if kind == nil {
			continue
		}
		for _, id := range kind.List {
			if isAnyReferenced(id, used) {
				return
			}
		}
	}
	report(Problem{
		Code:     r.Code(),
		Severity: SeverityWarn,
		Message:  fmt.Sprintf("import of %q appears to be unused", modName),
		Node:     imp,
		Fix:      removeNodeFix(imp, fmt.Sprintf("Remove unused import of %q", modName)),
	})
}

// EmptyBlockRule flags function, altstep, testcase and control bodies that
// contain no statements. An empty body is almost always either an oversight
// or stale code that should be removed.
type EmptyBlockRule struct{}

func (r *EmptyBlockRule) Code() string { return "empty-block" }

func (r *EmptyBlockRule) Check(tree *ttcn3.Tree, report Reporter) {
	tree.Inspect(func(n syntax.Node) bool {
		switch d := n.(type) {
		case *syntax.FuncDecl:
			if isEmptyBlock(d.Body) {
				kind := "function"
				if d.KindTok != nil {
					kind = d.KindTok.String()
				}
				report(Problem{
					Code:     r.Code(),
					Severity: SeverityInfo,
					Message:  fmt.Sprintf("empty %s body", kind),
					Node:     d.Body,
				})
			}
		case *syntax.ControlPart:
			if isEmptyBlock(d.Body) {
				report(Problem{
					Code:     r.Code(),
					Severity: SeverityInfo,
					Message:  "empty control part body",
					Node:     d.Body,
				})
			}
		}
		return true
	})
}

// AlignedBracesRule reports `{` and `}` that are neither on the same line
// nor in the same column. Mirroring this rule from the legacy CLI lint into
// the LSP layer makes the check visible as you type.
type AlignedBracesRule struct{}

func (r *AlignedBracesRule) Code() string { return "aligned-braces" }

func (r *AlignedBracesRule) Check(tree *ttcn3.Tree, report Reporter) {
	tree.Inspect(func(n syntax.Node) bool {
		var lb, rb syntax.Token
		switch d := n.(type) {
		case *syntax.Module:
			lb, rb = d.LBrace, d.RBrace
		case *syntax.BlockStmt:
			lb, rb = d.LBrace, d.RBrace
		case *syntax.CompositeLiteral:
			lb, rb = d.LBrace, d.RBrace
		case *syntax.StructSpec:
			lb, rb = d.LBrace, d.RBrace
		case *syntax.EnumSpec:
			lb, rb = d.LBrace, d.RBrace
		case *syntax.GroupDecl:
			lb, rb = d.LBrace, d.RBrace
		case *syntax.StructTypeDecl:
			lb, rb = d.LBrace, d.RBrace
		}
		if lb == nil || rb == nil {
			return true
		}
		l := syntax.Begin(lb)
		rr := syntax.Begin(rb)
		if l.Line == rr.Line || l.Column == rr.Column {
			return true
		}
		report(Problem{
			Code:     r.Code(),
			Severity: SeverityInfo,
			Message:  "braces are not aligned (must share a line or column)",
			Node:     rb,
		})
		return true
	})
}

// MissingCaseElseRule warns about select-statements that have no `case else`
// branch. This is the LSP-side complement of the legacy `require_case_else`
// configuration option.
type MissingCaseElseRule struct{}

func (r *MissingCaseElseRule) Code() string { return "missing-case-else" }

func (r *MissingCaseElseRule) Check(tree *ttcn3.Tree, report Reporter) {
	tree.Inspect(func(n syntax.Node) bool {
		sel, ok := n.(*syntax.SelectStmt)
		if !ok {
			return true
		}
		for _, cc := range sel.Body {
			if cc.Case == nil { // case else
				return true
			}
		}
		report(Problem{
			Code:     r.Code(),
			Severity: SeverityWarn,
			Message:  "select-statement has no case else branch",
			Node:     sel,
		})
		return true
	})
}

func isEmptyBlock(b *syntax.BlockStmt) bool {
	return b != nil && len(b.Stmts) == 0
}

func isAnyReferenced(e syntax.Expr, used map[string]bool) bool {
	if e == nil {
		return false
	}
	// Some leaf node types (notably Ident) implement Inspect as a no-op,
	// so we have to short-circuit and check them directly. For
	// composite expressions we still want to walk so that things like
	// `from M except { type X }` are handled correctly.
	if id, ok := e.(*syntax.Ident); ok {
		return identIsUsed(id, used)
	}
	found := false
	e.Inspect(func(n syntax.Node) bool {
		if id, ok := n.(*syntax.Ident); ok {
			if identIsUsed(id, used) {
				found = true
			}
		}
		return !found
	})
	return found
}

func identIsUsed(id *syntax.Ident, used map[string]bool) bool {
	if id == nil {
		return false
	}
	if id.Tok != nil && used[id.Tok.String()] {
		return true
	}
	if id.Tok2 != nil && used[id.Tok2.String()] {
		return true
	}
	return false
}

func removeNodeFix(n syntax.Node, title string) *Autofix {
	if n == nil {
		return nil
	}
	span := syntax.SpanOf(n)
	if !span.Begin.IsValid() {
		return nil
	}
	first := n.FirstTok()
	last := n.LastTok()
	if first == nil || last == nil {
		return nil
	}
	return &Autofix{
		Title:       title,
		Begin:       first.Pos(),
		End:         last.End(),
		Replacement: "",
	}
}

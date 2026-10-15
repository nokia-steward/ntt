// var_scope_rules.go enforces a narrow form of ETSI ES 201
// 873-1 clause 5.2 scope visibility:
//
//	A local variable declared inside a block (the body of a
//	loop, conditional, or any nested compound statement) is
//	visible only within that block and any nested blocks.
//	A reference to that name after the declaring block has
//	closed shall be rejected.
//
// We restrict the check to function / testcase / altstep
// bodies, where the scope chain is purely lexical. For each
// such body we:
//
//  1. Walk the AST and record every local `var` / `var
//     template` / `const` / `timer` declaration along with
//     the BlockStmt that introduced it.
//  2. Build, for every node in the body, the chain of
//     enclosing BlockStmts ("scope chain").
//  3. For every Ident reference whose name was declared
//     locally exactly once, emit a diagnostic when the
//     reference's scope chain does not contain the
//     declaring BlockStmt.
//
// The "declared exactly once" guard keeps the rule narrow:
// shadowing across sibling blocks is left to a future
// type-aware pass.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkVarScopeRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Body == nil {
			continue
		}
		paramNames := collectFuncParamNames(fn)
		diags = append(diags, walkFuncForScope(fn.Body, paramNames)...)
	}
	return diags
}

type localDecl struct {
	block *syntax.BlockStmt
	node  syntax.Node
}

func walkFuncForScope(body *syntax.BlockStmt, paramNames map[string]bool) []Diagnostic {
	if body == nil {
		return nil
	}
	// 1. Collect local declarations: map name -> list of
	//    declaring blocks. We also note the syntactic node
	//    so we can point diagnostics at a sensible span.
	decls := map[string][]localDecl{}
	parent := map[*syntax.BlockStmt]*syntax.BlockStmt{}
	parent[body] = nil
	var walk func(b *syntax.BlockStmt)
	walk = func(b *syntax.BlockStmt) {
		if b == nil {
			return
		}
		for _, st := range b.Stmts {
			collectScopeStmt(st, b, decls, parent, &walk)
		}
	}
	walk(body)

	// 2. Build a name -> single declaration mapping for
	//    names declared exactly once (avoids shadowing
	//    false positives) and not also a parameter name.
	singleDecl := map[string]localDecl{}
	for name, ds := range decls {
		if paramNames[name] {
			continue
		}
		if len(ds) != 1 {
			continue
		}
		singleDecl[name] = ds[0]
	}
	if len(singleDecl) == 0 {
		return nil
	}

	// 3. Walk again, this time tracking the scope chain at
	//    each statement / expression, and emit a diagnostic
	//    on out-of-scope identifier reads.
	var diags []Diagnostic
	scopeChain := func(b *syntax.BlockStmt) map[*syntax.BlockStmt]bool {
		out := map[*syntax.BlockStmt]bool{}
		for cur := b; cur != nil; cur = parent[cur] {
			out[cur] = true
		}
		return out
	}
	var walkRef func(b *syntax.BlockStmt, chain map[*syntax.BlockStmt]bool)
	walkRef = func(b *syntax.BlockStmt, chain map[*syntax.BlockStmt]bool) {
		if b == nil {
			return
		}
		seen := map[string]bool{}
		for _, st := range b.Stmts {
			collectScopeRefs(st, b, chain, singleDecl, &diags, walkRef, seen)
		}
	}
	walkRef(body, scopeChain(body))
	return diags
}

func collectScopeStmt(
	st syntax.Stmt,
	cur *syntax.BlockStmt,
	decls map[string][]localDecl,
	parent map[*syntax.BlockStmt]*syntax.BlockStmt,
	walk *func(*syntax.BlockStmt),
) {
	switch s := st.(type) {
	case *syntax.DeclStmt:
		if s == nil {
			return
		}
		vd, ok := s.Decl.(*syntax.ValueDecl)
		if !ok || vd == nil {
			return
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil {
				continue
			}
			name := dec.Name.String()
			decls[name] = append(decls[name], localDecl{
				block: cur,
				node:  dec,
			})
		}
	case *syntax.BlockStmt:
		if s != nil {
			parent[s] = cur
			(*walk)(s)
		}
	case *syntax.IfStmt:
		if s == nil {
			return
		}
		if s.Then != nil {
			parent[s.Then] = cur
			(*walk)(s.Then)
		}
		if e, ok := s.Else.(*syntax.BlockStmt); ok && e != nil {
			parent[e] = cur
			(*walk)(e)
		} else if ei, ok := s.Else.(*syntax.IfStmt); ok && ei != nil {
			collectScopeStmt(ei, cur, decls, parent, walk)
		}
	case *syntax.WhileStmt:
		if s == nil || s.Body == nil {
			return
		}
		parent[s.Body] = cur
		(*walk)(s.Body)
	case *syntax.DoWhileStmt:
		if s == nil || s.Body == nil {
			return
		}
		parent[s.Body] = cur
		(*walk)(s.Body)
	case *syntax.ForStmt:
		if s == nil || s.Body == nil {
			return
		}
		// `for` init declaration introduces a name visible
		// inside the body but not outside; record the body
		// as the declaring block to keep the rule sound.
		parent[s.Body] = cur
		if s.Init != nil {
			if ds, ok := s.Init.(*syntax.DeclStmt); ok && ds != nil {
				if vd, ok := ds.Decl.(*syntax.ValueDecl); ok && vd != nil {
					for _, dec := range vd.Decls {
						if dec == nil || dec.Name == nil {
							continue
						}
						name := dec.Name.String()
						decls[name] = append(decls[name], localDecl{
							block: s.Body,
							node:  dec,
						})
					}
				}
			}
		}
		(*walk)(s.Body)
	case *syntax.ForRangeStmt:
		if s == nil || s.Body == nil {
			return
		}
		parent[s.Body] = cur
		if s.Init != nil {
			if ds, ok := s.Init.(*syntax.DeclStmt); ok && ds != nil {
				if vd, ok := ds.Decl.(*syntax.ValueDecl); ok && vd != nil {
					for _, dec := range vd.Decls {
						if dec == nil || dec.Name == nil {
							continue
						}
						name := dec.Name.String()
						decls[name] = append(decls[name], localDecl{
							block: s.Body,
							node:  dec,
						})
					}
				}
			}
		}
		(*walk)(s.Body)
	case *syntax.AltStmt:
		if s == nil || s.Body == nil {
			return
		}
		parent[s.Body] = cur
		(*walk)(s.Body)
	case *syntax.SelectStmt:
		if s == nil {
			return
		}
		for _, cc := range s.Body {
			if cc == nil {
				continue
			}
			parent[cc.Body] = cur
			(*walk)(cc.Body)
		}
	case *syntax.CommClause:
		if s != nil && s.Body != nil {
			parent[s.Body] = cur
			(*walk)(s.Body)
		}
	}
}

func collectScopeRefs(
	st syntax.Stmt,
	cur *syntax.BlockStmt,
	chain map[*syntax.BlockStmt]bool,
	singleDecl map[string]localDecl,
	diags *[]Diagnostic,
	walkRef func(*syntax.BlockStmt, map[*syntax.BlockStmt]bool),
	seen map[string]bool,
) {
	if st == nil {
		return
	}
	// Recurse into nested compound statements (which open
	// child scopes) first so that the expression inspector
	// below does NOT walk into them and re-check references
	// at the wrong scope chain.
	switch s := st.(type) {
	case *syntax.BlockStmt:
		descendBlock(s, cur, chain, walkRef)
		return
	case *syntax.IfStmt:
		if s == nil {
			return
		}
		inspectExprForScope(s.Cond, cur, chain, singleDecl, diags, seen)
		descendBlock(s.Then, cur, chain, walkRef)
		// Else may be an IfStmt (else-if) or a BlockStmt.
		switch e := s.Else.(type) {
		case *syntax.BlockStmt:
			descendBlock(e, cur, chain, walkRef)
		case *syntax.IfStmt:
			collectScopeRefs(e, cur, chain, singleDecl, diags, walkRef, seen)
		}
		return
	case *syntax.WhileStmt:
		if s == nil {
			return
		}
		inspectExprForScope(s.Cond, cur, chain, singleDecl, diags, seen)
		descendBlock(s.Body, cur, chain, walkRef)
		return
	case *syntax.DoWhileStmt:
		if s == nil {
			return
		}
		inspectExprForScope(s.Cond, cur, chain, singleDecl, diags, seen)
		descendBlock(s.Body, cur, chain, walkRef)
		return
	case *syntax.ForStmt:
		if s == nil {
			return
		}
		// The for-loop variable is registered against
		// s.Body. Cond / Post run in the same scope as the
		// body, so build the body's child chain first and
		// inspect Cond / Post under it.
		body := s.Body
		var bodyChain map[*syntax.BlockStmt]bool
		if body != nil {
			bodyChain = map[*syntax.BlockStmt]bool{body: true}
			for k, v := range chain {
				bodyChain[k] = v
			}
		} else {
			bodyChain = chain
		}
		inspectExprForScope(s.Cond, cur, bodyChain, singleDecl, diags, seen)
		// Post is a Stmt - re-use the regular walker.
		if s.Post != nil {
			collectScopeRefs(s.Post, body, bodyChain, singleDecl, diags, walkRef, seen)
		}
		descendBlock(body, cur, chain, walkRef)
		return
	case *syntax.ForRangeStmt:
		if s == nil {
			return
		}
		body := s.Body
		var bodyChain map[*syntax.BlockStmt]bool
		if body != nil {
			bodyChain = map[*syntax.BlockStmt]bool{body: true}
			for k, v := range chain {
				bodyChain[k] = v
			}
		} else {
			bodyChain = chain
		}
		inspectExprForScope(s.Range, cur, bodyChain, singleDecl, diags, seen)
		descendBlock(body, cur, chain, walkRef)
		return
	case *syntax.AltStmt:
		if s != nil {
			descendBlock(s.Body, cur, chain, walkRef)
		}
		return
	case *syntax.SelectStmt:
		if s != nil {
			if s.Tag != nil {
				inspectExprForScope(s.Tag, cur, chain, singleDecl, diags, seen)
			}
			for _, cc := range s.Body {
				if cc != nil {
					descendBlock(cc.Body, cur, chain, walkRef)
				}
			}
		}
		return
	case *syntax.CommClause:
		if s != nil {
			if s.X != nil {
				inspectExprForScope(s.X, cur, chain, singleDecl, diags, seen)
			}
			descendBlock(s.Body, cur, chain, walkRef)
		}
		return
	}
	// Leaf statement (ExprStmt, DeclStmt, AssignStmt, etc.).
	// Inspect its sub-expressions.
	syntax.Inspect(st, func(n syntax.Node) bool {
		if id, ok := n.(*syntax.Ident); ok && id != nil {
			name := id.String()
			if d, ok := singleDecl[name]; ok {
				if !chain[d.block] && d.block != cur && !sameKey(d, id, seen) {
					*diags = append(*diags, Diagnostic{
						Code:     "var-out-of-scope",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"%q is referenced outside the block in which it was declared (ETSI 5.2)",
							name),
						Node: id,
						Span: syntax.SpanOf(id),
					})
				}
			}
		}
		return true
	})
}

func inspectExprForScope(
	expr syntax.Node,
	cur *syntax.BlockStmt,
	chain map[*syntax.BlockStmt]bool,
	singleDecl map[string]localDecl,
	diags *[]Diagnostic,
	seen map[string]bool,
) {
	if expr == nil {
		return
	}
	syntax.Inspect(expr, func(n syntax.Node) bool {
		if id, ok := n.(*syntax.Ident); ok && id != nil {
			name := id.String()
			if d, ok := singleDecl[name]; ok {
				if !chain[d.block] && d.block != cur && !sameKey(d, id, seen) {
					*diags = append(*diags, Diagnostic{
						Code:     "var-out-of-scope",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"%q is referenced outside the block in which it was declared (ETSI 5.2)",
							name),
						Node: id,
						Span: syntax.SpanOf(id),
					})
				}
			}
		}
		return true
	})
}

func descendBlock(
	blk syntax.Node,
	cur *syntax.BlockStmt,
	parentChain map[*syntax.BlockStmt]bool,
	walkRef func(*syntax.BlockStmt, map[*syntax.BlockStmt]bool),
) {
	b, ok := blk.(*syntax.BlockStmt)
	if !ok || b == nil {
		return
	}
	child := map[*syntax.BlockStmt]bool{b: true}
	for k, v := range parentChain {
		child[k] = v
	}
	walkRef(b, child)
}

// sameKey deduplicates: an Ident that IS the declaration node
// itself should not be flagged when we accidentally inspect it
// during reference-walking.
func sameKey(d localDecl, id *syntax.Ident, seen map[string]bool) bool {
	// We don't have a deterministic identity for syntax
	// nodes; use the position span as a poor man's key.
	pos := syntax.SpanOf(id)
	key := fmt.Sprintf("%d:%d", pos.Begin, pos.End)
	if seen[key] {
		return true
	}
	if dec, ok := d.node.(*syntax.Declarator); ok && dec != nil && dec.Name == id {
		seen[key] = true
		return true
	}
	return false
}

// collectFuncParamNames returns the set of formal parameter
// names declared on fn. Parameters are visible throughout the
// body and must be excluded from out-of-scope checks.
func collectFuncParamNames(fn *syntax.FuncDecl) map[string]bool {
	out := map[string]bool{}
	if fn == nil || fn.Params == nil {
		return out
	}
	for _, p := range fn.Params.List {
		if p == nil || p.Name == nil {
			continue
		}
		out[p.Name.String()] = true
	}
	return out
}

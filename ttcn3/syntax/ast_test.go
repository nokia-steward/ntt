package syntax

import (
	"fmt"
	"strings"
	"testing"
)

// sexpr returns a compact S-expression of the syntax tree. Nodes are printed
// as "(Type children...)", tokens, identifiers and literals as their text.
func sexpr(n Node) string {
	if IsNil(n) {
		return ""
	}
	if tok, ok := n.(Token); ok {
		return tok.String()
	}
	var parts []string
	for _, c := range n.Children() {
		if s := sexpr(c); s != "" {
			parts = append(parts, s)
		}
	}
	switch n := n.(type) {
	case *Ident:
		return n.String()
	case *ValueLiteral:
		return n.Tok.String()
	}
	name := strings.TrimPrefix(fmt.Sprintf("%T", n), "*syntax.")
	return "(" + name + " " + strings.Join(parts, " ") + ")"
}

func TestAST(t *testing.T) {
	tests := []struct {
		input string
		parse func(p *parser) Node
		want  string
	}{
		// Field selection applies to the decoded value: "(v => T).f"
		{
			input: `v.p => T.f`,
			parse: func(p *parser) Node { return p.parseExpr() },
			want:  `(SelectorExpr (BinaryExpr (SelectorExpr v . p) => T) . f)`,
		},
		{
			input: `v => (T, "UTF-8")`,
			parse: func(p *parser) Node { return p.parseExpr() },
			want:  `(BinaryExpr v => (ParenExpr ( T "UTF-8" )))`,
		},

		// "not" extends to relational operators, but not to "and".
		{
			input: `not a == b and c`,
			parse: func(p *parser) Node { return p.parseExpr() },
			want:  `(BinaryExpr (UnaryExpr not (BinaryExpr a == b)) and c)`,
		},
		{
			input: `not4b a & b or4b c`,
			parse: func(p *parser) Node { return p.parseExpr() },
			want:  `(BinaryExpr (UnaryExpr not4b (BinaryExpr a & b)) or4b c)`,
		},
		{
			input: `-a * b`,
			parse: func(p *parser) Node { return p.parseExpr() },
			want:  `(BinaryExpr (UnaryExpr - a) * b)`,
		},

		{
			input: `f({1}...)`,
			parse: func(p *parser) Node { return p.parseExpr() },
			want:  `(CallExpr f (ParenExpr ( (PostExpr (CompositeLiteral { 1 }) ...) )))`,
		},

		// Redirects belong to the communication operation, not to the address.
		{
			input: `p.receive(t) from a -> value v`,
			parse: func(p *parser) Node { return p.parseExpr() },
			want:  `(RedirectExpr (BinaryExpr (CallExpr (SelectorExpr p . receive) (ParenExpr ( t ))) from a) -> value v)`,
		},
		{
			input: `p.receive from address:(20..40) -> sender s`,
			parse: func(p *parser) Node { return p.parseExpr() },
			want:  `(RedirectExpr (BinaryExpr (SelectorExpr p . receive) from (BinaryExpr address : (ParenExpr ( (BinaryExpr 20 .. 40) )))) -> sender s)`,
		},
		{
			input: `p.check(from self -> sender s)`,
			parse: func(p *parser) Node { return p.parseExpr() },
			want:  `(CallExpr (SelectorExpr p . check) (ParenExpr ( (RedirectExpr (BinaryExpr from self) -> sender s) )))`,
		},

		// Attribute value retrieval
		{
			input: `T.variant("Codec")`,
			parse: func(p *parser) Node { return p.parseExpr() },
			want:  `(CallExpr (SelectorExpr T . variant) (ParenExpr ( "Codec" )))`,
		},

		// Call statements
		{
			input: `p.call(s) to a { [] p.getreply {} }`,
			parse: func(p *parser) Node { return p.parseStmt() },
			want:  `(CallStmt (ExprStmt (BinaryExpr (CallExpr (SelectorExpr p . call) (ParenExpr ( s ))) to a)) (BlockStmt { (CommClause [ ] (ExprStmt (SelectorExpr p . getreply)) (BlockStmt { })) }))`,
		},
		{
			input: `c.call(f()) -> value v verdict w catch(stop) {} catch(timeout) {}`,
			parse: func(p *parser) Node { return p.parseStmt() },
			want:  `(CallStmt (ExprStmt (RedirectExpr (CallExpr (SelectorExpr c . call) (ParenExpr ( (CallExpr f (ParenExpr ( ))) ))) -> value v verdict w)) (CatchClause catch (ParenExpr ( stop )) (BlockStmt { })) (CatchClause catch (ParenExpr ( timeout )) (BlockStmt { })))`,
		},
		{
			input: `@nodefault p.receive`,
			parse: func(p *parser) Node { return p.parseStmt() },
			want:  `(ExprStmt @nodefault (SelectorExpr p . receive))`,
		},
		{
			input: `select union (var U u := f(); u) {}`,
			parse: func(p *parser) Node { return p.parseStmt() },
			want:  `(SelectStmt select union ( (DeclStmt (ValueDecl var U (Declarator u := (CallExpr f (ParenExpr ( )))))) ; u ) { })`,
		},

		// Declarations
		{
			input: `(template @lazy @deterministic record of integer p... := {})`,
			parse: func(p *parser) Node { return p.parseFormalPars() },
			want:  `(FormalPars ( (FormalPar (RestrictionSpec template) @lazy @deterministic (ListSpec record of (RefSpec integer)) p ... := (CompositeLiteral { })) ))`,
		},
		{
			input: `type record R { integer, B b optional }`,
			parse: func(p *parser) Node { return p.parseTypeDecl() },
			want:  `(StructTypeDecl type record R { (Field (RefSpec integer)) (Field (RefSpec B) b optional) })`,
		},
		{
			input: `import from M -> A all`,
			parse: func(p *parser) Node { return p.parseImport() },
			want:  `(ImportDecl import from M -> A (DefKindExpr all))`,
		},
		{
			input: `modulepar p := 1`,
			parse: func(p *parser) Node { return p.parseModuleDef() },
			want:  `(ModuleDef (ValueDecl modulepar (Declarator p := 1)))`,
		},
	}

	for _, tt := range tests {
		p := NewParser([]byte(tt.input))
		n := tt.parse(p)
		if err := p.Err(); err != nil {
			t.Errorf("Parse(%q): %v", tt.input, err)
			continue
		}
		if got := sexpr(n); got != tt.want {
			t.Errorf("Parse(%q):\n\tgot:  %s\n\twant: %s", tt.input, got, tt.want)
		}
		if n.Pos() > n.End() {
			t.Errorf("Parse(%q): invalid span [%d, %d)", tt.input, n.Pos(), n.End())
		}
	}
}

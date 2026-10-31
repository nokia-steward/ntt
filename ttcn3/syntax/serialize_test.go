package syntax_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nokia/ntt/ttcn3/syntax"
	"github.com/stretchr/testify/assert"
)

func TestSerialize(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{input: "", want: []string{
			"Open Root",
			"Close Root",
		}},
		{input: "// c\n", want: []string{
			"Open Root",
			`Token COMMENT "// c"`,
			"Close Root",
		}},
		{input: "f(x, /* c */ y);", want: []string{
			"Open Root",
			"Open ExprStmt",
			"Open CallExpr",
			"Open Ident",
			`Token IDENT "f"`,
			"Close Ident",
			"Open ParenExpr",
			`Token ( "("`,
			"Open Ident",
			`Token IDENT "x"`,
			"Close Ident",
			`Token , ","`,
			`Token COMMENT "/* c */"`,
			"Open Ident",
			`Token IDENT "y"`,
			"Close Ident",
			`Token ) ")"`,
			"Close ParenExpr",
			"Close CallExpr",
			"Close ExprStmt",
			`Token ; ";"`,
			"Close Root",
		}},
		{input: "module M { // c\n const integer x := 1; }", want: []string{
			"Open Root",
			"Open Module",
			`Token module "module"`,
			"Open Ident",
			`Token IDENT "M"`,
			"Close Ident",
			`Token { "{"`,
			`Token COMMENT "// c"`,
			"Open ModuleDef",
			"Open ValueDecl",
			`Token const "const"`,
			"Open Ident",
			`Token IDENT "integer"`,
			"Close Ident",
			"Open Declarator",
			"Open Ident",
			`Token IDENT "x"`,
			"Close Ident",
			`Token := ":="`,
			"Open ValueLiteral",
			`Token INT "1"`,
			"Close ValueLiteral",
			"Close Declarator",
			"Close ValueDecl",
			"Close ModuleDef",
			`Token ; ";"`,
			`Token } "}"`,
			"Close Module",
			"Close Root",
		}},
		{input: "x := (0) ifpresent", want: []string{
			"Open Root",
			"Open ExprStmt",
			"Open BinaryExpr",
			"Open Ident",
			`Token IDENT "x"`,
			"Close Ident",
			`Token := ":="`,
			"Open UnaryExpr",
			"Open ParenExpr",
			`Token ( "("`,
			"Open ValueLiteral",
			`Token INT "0"`,
			"Close ValueLiteral",
			`Token ) ")"`,
			"Close ParenExpr",
			`Token ifpresent "ifpresent"`,
			"Close UnaryExpr",
			"Close BinaryExpr",
			"Close ExprStmt",
			"Close Root",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			root := syntax.Parse([]byte(tt.input))
			if err := root.Err(); err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, e := range syntax.Serialize(root) {
				got = append(got, e.String())
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSerializeSubtree(t *testing.T) {
	root := syntax.Parse([]byte("// a\nf(x, y) // b\n"))
	var got []string
	for _, e := range syntax.Serialize(root.Nodes[0]) {
		if tok := e.Token(); tok != nil {
			got = append(got, tok.String())
		}
	}
	assert.Equal(t, []string{"f", "(", "x", ",", "y", ")"}, got)

	root = syntax.Parse([]byte("x := (0, /* c */ 1) ifpresent"))
	var unary syntax.Node
	root.Inspect(func(n syntax.Node) bool {
		if _, ok := n.(*syntax.UnaryExpr); ok {
			unary = n
		}
		return unary == nil
	})
	got = nil
	for _, e := range syntax.Serialize(unary) {
		if tok := e.Token(); tok != nil {
			got = append(got, tok.String())
		}
	}
	assert.Equal(t, []string{"(", "0", ",", "/* c */", "1", ")", "ifpresent"}, got)
}

func TestSerializeEmptyNode(t *testing.T) {
	root := syntax.Parse([]byte("/* c */ a + b"))
	root.Nodes[0].(*syntax.ExprStmt).Expr.(*syntax.BinaryExpr).X = &syntax.Ident{}
	var got []string
	for _, e := range syntax.Serialize(root) {
		got = append(got, e.String())
	}
	assert.Equal(t, []string{
		"Open Root",
		`Token COMMENT "/* c */"`,
		`Token IDENT "a"`,
		"Open ExprStmt",
		"Open BinaryExpr",
		"Open Ident",
		"Close Ident",
		`Token + "+"`,
		"Open Ident",
		`Token IDENT "b"`,
		"Close Ident",
		"Close BinaryExpr",
		"Close ExprStmt",
		"Close Root",
	}, got)
}

func TestSerializeNesting(t *testing.T) {
	for _, input := range []string{
		"x := ",
		"module M { const integer x := ",
		"module M { all /*c*/ timer.stop; }",
	} {
		root := syntax.Parse([]byte(input))
		assertNesting(t, root, syntax.Serialize(root))
	}
}

// assertNesting verifies that every open-event is nested inside the
// open-event of its parent node.
func assertNesting(t *testing.T, root syntax.Node, events []syntax.Event) {
	t.Helper()
	parent := make(map[syntax.Node]syntax.Node)
	var visit func(n syntax.Node)
	visit = func(n syntax.Node) {
		for _, c := range n.Children() {
			if _, ok := c.(syntax.Token); !ok && !syntax.IsNil(c) {
				parent[c] = n
				visit(c)
			}
		}
	}
	visit(root)

	var stack []syntax.Node
	for _, e := range events {
		switch e.Kind {
		case syntax.OpenEvent:
			if p, ok := parent[e.Node]; ok && (len(stack) == 0 || stack[len(stack)-1] != p) {
				t.Errorf("%s at %d is not nested in its parent", e, e.Node.Pos())
			}
			stack = append(stack, e.Node)
		case syntax.CloseEvent:
			stack = stack[:len(stack)-1]
		}
	}
}

func TestSerializeEmptyEvent(t *testing.T) {
	assert.Equal(t, "Token", syntax.Event{}.String())
}

// TestSerializeLossless verifies that serializing the conformance tests
// yields every token exactly once, in source order, and balanced open- and
// close-events, which enclose all tokens of their node. Files with syntax
// errors are included to exercise error recovery.
func TestSerializeLossless(t *testing.T) {
	dir := os.Getenv("NTT_CONFORMANCE_DIR")
	if dir == "" {
		dir = "../../testdata/ttcn3-conformance-tests/ATS/core_language"
	}
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("conformance tests not found: %s", err)
	}

	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !strings.HasSuffix(path, ".ttcn") && !strings.HasSuffix(path, ".ttcn3") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(dir, path)
		t.Run(rel, func(t *testing.T) {
			root := syntax.Parse(b, syntax.WithFilename(path))

			var want []int
			for tok := root.FirstTok(); tok != nil; tok = tok.NextTok() {
				if tok.Kind() != syntax.EOF {
					want = append(want, tok.Pos())
				}
			}

			// Error nodes share tokens with their neighbours.
			shared := make(map[int]bool)
			root.Inspect(func(n syntax.Node) bool {
				if n, ok := n.(*syntax.ErrorNode); ok {
					for _, c := range n.Children() {
						shared[c.Pos()] = true
					}
				}
				return true
			})

			events := syntax.Serialize(root)
			var got []int
			var stack []int
			tokenAt := make(map[int]int) // token position -> event index
			for i, e := range events {
				switch e.Kind {
				case syntax.TokenEvent:
					got = append(got, e.Token().Pos())
					tokenAt[e.Token().Pos()] = i
				case syntax.OpenEvent:
					stack = append(stack, i)
				case syntax.CloseEvent:
					if len(stack) == 0 || events[stack[len(stack)-1]].Node != e.Node {
						t.Fatalf("unbalanced close event %s", e)
					}
					open := stack[len(stack)-1]
					stack = stack[:len(stack)-1]

					for _, c := range e.Node.Children() {
						tok, ok := c.(syntax.Token)
						if !ok || syntax.IsNil(tok) || tok.Kind() == syntax.EOF || shared[tok.Pos()] {
							continue
						}
						if j := tokenAt[tok.Pos()]; j < open || i < j {
							t.Errorf("token %q at %d is outside of %s", tok, tok.Pos(), e)
						}
					}
				}
			}
			assert.Empty(t, stack)
			assert.Equal(t, want, got)
			assertNesting(t, root, events)
		})
		return nil
	})
}

package lsp_test

import (
	"fmt"
	"testing"

	"github.com/nokia/ntt/internal/fs"
	"github.com/nokia/ntt/internal/lsp"
	"github.com/nokia/ntt/internal/lsp/protocol"
	"github.com/nokia/ntt/ttcn3"
	"github.com/stretchr/testify/assert"
)

type Hint struct {
	Line  uint32
	Char  uint32
	Label string
}

func TestInlayHintForFunction(t *testing.T) {
	actual := testInlayHint(t, nil, `
module Test {
    function func(integer a := 0, integer b := 0, integer c := 0) {}
    function test() {
        func(1, 2, 3)
        func(1,
             2,
             3)
        func(a := 1, b := 2, c := 3)
        func(1 + 2)
        func(1, 2, c := 3)
    }
}`)

	assert.Equal(t, []Hint{
		// All parameters in the same line.
		{Line: 4, Char: 13, Label: "a :="},
		{Line: 4, Char: 16, Label: "b :="},
		{Line: 4, Char: 19, Label: "c :="},
		// Parameters spanning multiple lines.
		{Line: 5, Char: 13, Label: "a :="},
		{Line: 6, Char: 13, Label: "b :="},
		{Line: 7, Char: 13, Label: "c :="},
		// Binary expression parameter.
		{Line: 8, Char: 13, Label: "a :="},
		// Mixed assignment / value list notation.
		{Line: 9, Char: 13, Label: "a :="},
		{Line: 9, Char: 14, Label: "b :="},
	}[0], actual[0])
}

func TestInlayHintForTemplate(t *testing.T) {
	actual := testInlayHint(t, nil, `
module Test {
    template integer templ(integer x, integer y) := (x .. y)
    function test() {
        var template integer t := templ(2, 3)
    }
}`)

	assert.Equal(t, []Hint{
		{Line: 4, Char: 40, Label: "x :="},
		{Line: 6, Char: 43, Label: "y :="},
	}[0], actual[0])
}

func TestInlayHintNestedCalls(t *testing.T) {
	actual := testInlayHint(t, nil, `
module Test {
    function foo(integer a) return integer { return 1; }
    function bar(integer b) return integer { return 1; }
    function baz(integer c) return integer { return 1; }
    function test() {
        foo(bar(baz(1)))
    }
}`)

	assert.Equal(t, []Hint{
		{Line: 6, Char: 12, Label: "a :="},
		{Line: 6, Char: 16, Label: "b :="},
		{Line: 6, Char: 20, Label: "c :="},
	}[0], actual[0])
}

func TestInlayHintVariadic(t *testing.T) {
	actual := testInlayHint(t, nil, `
module Test {
    function f(integer a, integer b...) {}
    function test() {
        f(1, 2, 3)
    }
}`)

	assert.Equal(t, []Hint{
		{Line: 4, Char: 10, Label: "a :="},
	}, actual)
}

func testInlayHint(t *testing.T, rng *protocol.Range, text string) []Hint {
	t.Helper()

	file := fmt.Sprintf("%s.ttcn3", t.Name())
	fs.SetContent(file, []byte(text))
	tree := ttcn3.ParseFile(file)
	if tree.Err != nil {
		t.Fatal(tree.Err)
	}

	// Build index to for tree.Lookup to resolve imported symbols.
	db := &ttcn3.DB{}
	db.Index(file)

	begin := tree.Pos()
	end := tree.End()
	if rng != nil {
		begin = tree.PosFor(int(rng.Start.Line), int(rng.Start.Character))
		end = tree.PosFor(int(rng.End.Line), int(rng.End.Character))
	}

	var hints []Hint
	for _, h := range lsp.ProcessInlayHint(tree, db, begin, end) {
		hints = append(hints, Hint{
			Line:  h.Position.Line,
			Char:  h.Position.Character,
			Label: h.Label[0].Value,
		})
	}
	return hints
}

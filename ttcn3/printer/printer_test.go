package printer_test

import (
	"bytes"
	"testing"

	"github.com/nokia/ntt/ttcn3/printer"
	"github.com/nokia/ntt/ttcn3/syntax"
	"github.com/stretchr/testify/assert"
)

// TestPrint verifies that the printed output contains the same tokens as the
// input.
func TestPrint(t *testing.T) {
	tests := []string{
		`function f(in template @lazy @deterministic integer x[2]... := {}) {}`,
		`function f(record of integer x) return record of record {}[3] {}`,
		`var @fuzzy @deterministic integer x := f(r...);`,
		`template @fuzzy @deterministic integer x := ?;`,
		`template(omit) integer x := omit;`,
		`type record R { integer, M.T optional }`,
		`modulepar m := 1;`,
	}
	for _, input := range tests {
		root := syntax.Parse([]byte(input))
		if err := root.Err(); err != nil {
			t.Fatalf("%q: %v", input, err)
		}
		var buf bytes.Buffer
		if err := printer.Print(&buf, root.Nodes[0]); err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, tokens(root), tokens(syntax.Parse(buf.Bytes())), input)
	}
}

func tokens(root *syntax.Root) []string {
	var s []string
	for tok := root.FirstTok(); tok != nil; tok = tok.NextTok() {
		if k := tok.Kind(); k != syntax.EOF && k != syntax.SEMICOLON {
			s = append(s, tok.String())
		}
	}
	return s
}

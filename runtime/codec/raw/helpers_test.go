package raw_test

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/attr"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// parseAttrs is a shared helper that extracts the first WithSpec in src
// and returns the AttributeSet it parses to. Codec tests use it to keep
// fixtures readable as TTCN-3 source.
func parseAttrs(t *testing.T, src string) *attr.AttributeSet {
	t.Helper()
	tree := ttcn3.Parse(src)
	if tree.Err != nil {
		t.Fatalf("parse: %v", tree.Err)
	}
	var first *syntax.WithSpec
	tree.Root.Inspect(func(n syntax.Node) bool {
		if w, ok := n.(*syntax.WithSpec); ok && first == nil {
			first = w
			return false
		}
		return true
	})
	if first == nil {
		t.Fatalf("no WithSpec in source")
	}
	set, _ := attr.Parse(first)
	return set
}

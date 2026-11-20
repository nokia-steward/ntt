package interpreter

import (
	"testing"

	"github.com/nokia/ntt/runtime"
)

// TestWalkComponentArray covers the row-major leaf walk used by the
// `any/all from <compArray>.<op>` queries: it must yield the correct
// multi-dimensional index path for each leaf and short-circuit when
// the callback returns false.
func TestWalkComponentArray(t *testing.T) {
	leaf := func(id int64) *runtime.ComponentRef {
		r := &runtime.ComponentRef{ID: id}
		r.SetAlive(true)
		return r
	}
	row := func(refs ...*runtime.ComponentRef) *runtime.List {
		l := &runtime.List{ListType: runtime.RECORD_OF}
		for _, r := range refs {
			l.Elements = append(l.Elements, r)
		}
		return l
	}

	// 2-D: [[a,b],[c]] -> visits (0,0)(0,1)(1,0).
	grid := &runtime.List{ListType: runtime.RECORD_OF, Elements: []runtime.Object{
		row(leaf(1), leaf(2)),
		row(leaf(3)),
	}}
	var paths [][]int
	walkComponentArray(grid, nil, func(idx []int, ref *runtime.ComponentRef) bool {
		paths = append(paths, append([]int(nil), idx...))
		return true
	})
	want := [][]int{{0, 0}, {0, 1}, {1, 0}}
	if len(paths) != len(want) {
		t.Fatalf("visited %v, want %v", paths, want)
	}
	for i := range want {
		if len(paths[i]) != len(want[i]) || paths[i][0] != want[i][0] || paths[i][1] != want[i][1] {
			t.Fatalf("path[%d] = %v, want %v", i, paths[i], want[i])
		}
	}

	// Short-circuit: stop after the first leaf.
	count := 0
	walkComponentArray(grid, nil, func(idx []int, ref *runtime.ComponentRef) bool {
		count++
		return false
	})
	if count != 1 {
		t.Fatalf("short-circuit visited %d leaves, want 1", count)
	}
}

// TestParsePortIndices covers the @index recovery helper used by
// `any from p.<procOp> -> @index value v`: it must pull the integer
// subscripts out of a port-instance name relative to the array base,
// for both 1-D and multi-dimensional ports, and reject non-matching
// or unsubscripted names.
func TestParsePortIndices(t *testing.T) {
	cases := []struct {
		name string
		base string
		want []int
	}{
		{"p[1]", "p", []int{1}},
		{"p[0]", "p", []int{0}},
		{"p[1][2]", "p", []int{1, 2}},
		{"p[3][0][7]", "p", []int{3, 0, 7}},
		{"p", "p", nil},     // bare array name, no subscript
		{"q[1]", "p", nil},  // different base
		{"pp[1]", "p", nil}, // base must be followed by '['
		{"p[x]", "p", nil},  // non-integer subscript
	}
	for _, c := range cases {
		got := parsePortIndices(c.name, c.base)
		if len(got) != len(c.want) {
			t.Errorf("parsePortIndices(%q,%q) = %v, want %v", c.name, c.base, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("parsePortIndices(%q,%q) = %v, want %v", c.name, c.base, got, c.want)
				break
			}
		}
	}
}

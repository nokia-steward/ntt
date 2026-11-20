package format

import (
	"bytes"
	"strings"
	"testing"
)

func TestWrappingFormatter_WrapsLongFunctionSignature(t *testing.T) {
	const src = `module M { function f(integer a, integer b, integer c, integer d, integer e, integer f) { return; } }`
	var buf bytes.Buffer
	opts := DefaultOptions()
	opts.PrintWidth = 60
	if err := NewWrappingFormatter(opts).Fprint(&buf, src); err != nil {
		t.Fatalf("fprint failed: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "\n") {
		t.Fatalf("expected wrapped output, got: %q", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if visualLen(line, opts.TabWidth) > opts.PrintWidth*2 {
			t.Errorf("line still too long after wrap: %q", line)
		}
	}
}

func TestWrappingFormatter_KeepsShortLinesAsIs(t *testing.T) {
	const src = `module M { function f() { return; } }`
	var buf bytes.Buffer
	opts := DefaultOptions()
	opts.PrintWidth = 80
	if err := NewWrappingFormatter(opts).Fprint(&buf, src); err != nil {
		t.Fatalf("fprint failed: %v", err)
	}
	out := buf.String()
	if strings.Count(out, "function f()") != 1 {
		t.Fatalf("expected the function signature to stay on one line, got:\n%s", out)
	}
}

func TestOuterBracketPair(t *testing.T) {
	cases := []struct {
		in           string
		wantO, wantC int
	}{
		{"f(a, b, c)", 1, 9},
		{"x = {1, 2, 3}", 4, 12},
		{"plain", -1, -1},
		{"f(\"a)b\", c)", 1, 10},
	}
	for _, c := range cases {
		o, cl := outerBracketPair(c.in)
		if o != c.wantO || cl != c.wantC {
			t.Errorf("outerBracketPair(%q) = (%d,%d), want (%d,%d)", c.in, o, cl, c.wantO, c.wantC)
		}
	}
}

func TestSplitTopLevel(t *testing.T) {
	got := splitTopLevel("a, b(c, d), {e, f}", ',')
	want := []string{"a", " b(c, d)", " {e, f}"}
	if len(got) != len(want) {
		t.Fatalf("split = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("split[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func visualLen(line string, tab int) int {
	n := 0
	if tab <= 0 {
		tab = 8
	}
	for _, r := range line {
		if r == '\t' {
			n += tab - (n % tab)
			continue
		}
		n++
	}
	return n
}

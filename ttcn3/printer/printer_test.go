package printer_test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nokia/ntt/ttcn3/printer"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// TestRoundTrip verifies that printing a syntax tree yields source code
// which parses to the same sequence of tokens.
func TestRoundTrip(t *testing.T) {
	tests := []string{
		`module M {
			import from A -> B { type T };
			modulepar p := 1;
			modulepar record of integer q := {};
			type record R { integer, B b optional };
			type union U { integer, charstring };
			signature S(record of integer p) return set of integer exception (record {});
			type function F() runs on C system C return integer;
			type altstep interleave A() runs on C;
			private function @deterministic f(template @lazy @deterministic record of integer p... := {}) return integer {
				var @fuzzy @deterministic integer v := not a == b;
				var template @lazy @deterministic integer t := ?;
				var integer i := v.p => T.f;
				var integer x := f({1}...) + ???;
				x := T.variant("C") & T.encode & t.present & present(t);
				if (x) {} else {}
				while (x) {}
				do {} while (x);
				select union (var U u := f(); u) { case (i) {} case else {} }
				@nodefault p.receive(t) from a -> value v;
				alt @nodefault { [] p.receive(t ifpresent) from a -> value v {} }
				p.call(s) to all component { [] p.getreply {} }
				c.call(f()) -> value v verdict w catch (stop) {} catch (timeout) {}
				var C c := C.create alive;
				???;
			}
		}
		with { variant {"Codec1", "Codec2"}."Rule1" }`,
		`module M {
			type class C extends B runs on X {
				create(integer i) {}
				var integer i;
			}
			template @fuzzy @deterministic integer t(integer i) modifies b := @dynamic { return 1 };
		}`,
	}

	for _, input := range tests {
		testRoundTrip(t, input, input)
	}
}

// TestRoundTripConformance verifies the printer with all valid conformance
// tests.
func TestRoundTripConformance(t *testing.T) {
	dir := "../../testdata/ttcn3-conformance-tests/ATS/core_language"
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("conformance tests not found: %v", err)
	}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".ttcn") {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(src, []byte("pass accept")) && syntax.Parse(src).Err() == nil {
			testRoundTrip(t, path, string(src))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func testRoundTrip(t *testing.T, name string, input string) {
	t.Helper()
	root := syntax.Parse([]byte(input))
	if err := root.Err(); err != nil {
		t.Errorf("Parse(%q): %v", name, err)
		return
	}
	var buf bytes.Buffer
	for _, n := range root.Nodes {
		if err := printer.Print(&buf, n); err != nil {
			t.Fatal(err)
		}
	}
	output := buf.String()
	root2 := syntax.Parse([]byte(output))
	if err := root2.Err(); err != nil {
		t.Errorf("Parse(Print(%q)) failed:\n%s\n%v", name, output, err)
		return
	}
	if want, got := tokens(root), tokens(root2); want != got {
		t.Errorf("Print(%q) changed tokens:\n\twant: %s\n\tgot:  %s", name, want, got)
	}
}

// tokens returns the tokens of the syntax tree without separators.
func tokens(n syntax.Node) string {
	var buf bytes.Buffer
	for tok := n.FirstTok(); tok != nil && tok.Kind() != syntax.EOF; tok = tok.NextTok() {
		switch tok.Kind() {
		case syntax.COMMA, syntax.SEMICOLON, syntax.COMMENT, syntax.PREPROC:
			continue
		}
		buf.WriteString(tok.String())
		buf.WriteString(" ")
	}
	return buf.String()
}

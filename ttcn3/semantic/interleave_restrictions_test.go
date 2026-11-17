package semantic

import (
	"strings"
	"testing"
)

// interleaveModule is a module whose testcase runs an interleave of two
// branches: the first receives 1 and then does body, the second receives 2.
func interleaveModule(body string) string {
	return `module M {
		type port P message { inout integer }
		type component C { port P p, q }
		testcase tc() runs on C {
			var integer n := 0;
			interleave {
				[] p.receive(integer:1) {
					` + body + `
				}
				[] p.receive(integer:2) { }
			}
		}
	}`
}

// A loop is allowed in an interleave within a statement block that has
// no reception statement, even in a branch that receives after it, and an
// alt or an interleave may be nested in an interleave (ETSI ES 201 873-1
// 20.4 d; 20.4 forbids neither nesting).
func TestInterleaveAllowed(t *testing.T) {
	for name, body := range map[string]string{
		"a for loop before a receive":      `for (var integer i := 0; i < 3; i := i + 1) { n := n + 1 } q.receive(integer:3);`,
		"a while loop after a receive":     `q.receive(integer:3); while (n < 3) { n := n + 1 }`,
		"a do-while loop before a receive": `do { n := n + 1 } while (n < 3); q.receive(integer:3);`,
		"a nested interleave":              `interleave { [] q.receive(integer:3) { } [] q.receive(integer:4) { } } n := 1;`,
		"a nested alt":                     `alt { [] q.receive(integer:3) { } [] q.receive(integer:4) { } }`,
	} {
		for _, c := range analyzeCodes(t, interleaveModule(body)) {
			if strings.HasPrefix(c, "interleave-") {
				t.Errorf("%s: unexpected %s", name, c)
			}
		}
	}
}

// A loop whose block contains a reception statement is not (20.4 d).
func TestInterleaveLoopWithReception(t *testing.T) {
	for name, body := range map[string]string{
		"a for loop":      `for (var integer i := 0; i < 3; i := i + 1) { q.receive(integer:3) }`,
		"a while loop":    `while (n < 3) { q.receive; n := n + 1 }`,
		"a do-while loop": `do { q.receive; n := n + 1 } while (n < 3);`,
	} {
		if codes := analyzeCodes(t, interleaveModule(body)); !hasCode(codes, "interleave-forbidden-control") {
			t.Errorf("%s: want interleave-forbidden-control, got %v", name, codes)
		}
	}
}

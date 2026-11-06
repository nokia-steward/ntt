package interpreter_test

import (
	"testing"

	"github.com/nokia/ntt/runtime"
)

// An interleave altstep takes each of its alternatives once, in the
// order their events come (ETSI 16.2.0): called, taken as an alternative
// of an alt, or activated as a default.
func TestInterleaveAltstep(t *testing.T) {
	for name, use := range map[string]string{
		"called":         `a_all();`,
		"an alternative": `alt { [] a_all() [] g.timeout { setverdict(fail, "timed out") } }`,
		"a default": `var default d := activate(a_all());
			alt { [] p2.receive(integer:4) { setverdict(fail, "4 arrived") } [] g.timeout { setverdict(fail, "timed out") } }
			deactivate(d);`,
	} {
		v, reason := runSched(t, "M.tc", `module M {
			type port P message { inout integer }
			type component C { port P p1, p2; var charstring order := ""; timer g := 2.0 }
			altstep interleave a_all() runs on C {
				var charstring sep := ",";
				[] p2.receive(integer:1) { order := order & "1" & sep }
				[] p2.receive(integer:2) { order := order & "2" & sep }
				[] p2.receive(integer:3) { order := order & "3" & sep }
			}
			testcase tc() runs on C {
				connect(self:p1, self:p2);
				g.start;
				p1.send(3); p1.send(2); p1.send(1);
				`+use+`
				if (order == "3,2,1,") { setverdict(pass) } else { setverdict(fail, order) }
			}
		}`)
		if v != runtime.PassVerdict {
			t.Errorf("%s: verdict = %s (%s), want pass", name, v, reason)
		}
	}
}

// break in a branch of an interleave altstep leaves it, the alternatives
// not yet taken with it.
func TestInterleaveAltstepBreak(t *testing.T) {
	v, reason := runSched(t, "M.tc", `module M {
		type port P message { inout integer }
		type component C { port P p1, p2; var integer n := 0 }
		altstep interleave a_three() runs on C {
			[] p2.receive(integer:1) { n := n + 1; break }
			[] p2.receive(integer:2) { n := n + 10 }
			[] p2.receive(integer:3) { n := n + 100 }
		}
		testcase tc() runs on C {
			connect(self:p1, self:p2);
			p1.send(2); p1.send(1); p1.send(3);
			a_three();
			timer t := 0.5; t.start;
			alt {
				[] p2.receive(integer:3) { if (n == 11) { setverdict(pass) } else { setverdict(fail, n) } }
				[] t.timeout { setverdict(fail, "3 was taken: ", n) }
			}
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

// After a reception in a branch's block, the rest of the block is
// interleaved with the other alternatives (20.4): here the block waits
// for 3 while 2, behind it in the queue, is taken by the other branch.
func TestInterleaveAltstepReceptionInABranch(t *testing.T) {
	v, reason := runSched(t, "M.tc", `module M {
		type port P message { inout integer }
		type component C { port P p1, p2; var charstring order := "" }
		altstep interleave a() runs on C {
			[] p2.receive(integer:1) { order := order & "1"; p2.receive(integer:3); order := order & "3" }
			[] p2.receive(integer:2) { order := order & "2" }
		}
		testcase tc() runs on C {
			connect(self:p1, self:p2);
			p1.send(1); p1.send(2); p1.send(3);
			a();
			if (order == "123") { setverdict(pass) } else { setverdict(fail, order) }
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

// A break in a branch of an interleave altstep taken as an alternative
// leaves the alt around it too, the alternative's own block not run
// (19.12), in the branch it was entered by and in one taken later.
func TestInterleaveAltstepBreakAsAnAlternative(t *testing.T) {
	for _, first := range []string{"1", "2"} {
		v, reason := runSched(t, "M.tc", `module M {
			type port P message { inout integer }
			type component C { port P p1, p2; var charstring order := "" }
			altstep interleave a() runs on C {
				[] p2.receive(integer:1) { order := order & "1"; break }
				[] p2.receive(integer:2) { order := order & "2" }
			}
			testcase tc() runs on C {
				connect(self:p1, self:p2);
				p1.send(`+first+`); p1.send(3 - `+first+`);
				timer g := 1.0; g.start;
				alt {
					[] a() { order := order & "B" }
					[] g.timeout { order := order & "T" }
				}
				if (match(order, pattern "*B*")) { setverdict(fail, order) } else { setverdict(pass) }
			}
		}`)
		if v != runtime.PassVerdict {
			t.Errorf("%s first: verdict = %s (%s), want pass", first, v, reason)
		}
	}
}

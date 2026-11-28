package interpreter_test

import (
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// TestTimersEndWithTheirScope: a timer declared in a function, an altstep
// or a block ends with it (ETSI ES 201 873-1 12): `any timer` no longer
// names it, so a timeout of the caller's own timer is the one taken. A
// timer the caller passes or one aliasing it is the caller's, and goes on;
// one passed to an activated default is the default's (20.5.2) until the
// default is deactivated, or the behaviour that activated it ends.
func TestTimersEndWithTheirScope(t *testing.T) {
	decls := `type component C { timer tc_c := 3.0 }
		function f() { timer tf := 0.2; tf.start }
		function fp(timer p) { p.start(0.2) }
		function fa(timer p) { timer al := p; }
		function farr() { timer ta[2] := {0.2, 0.3}; ta[0].start; ta[1].start }
		function fl() { timer g := 0.001; g.start }
		altstep as1() runs on C { timer la := 0.1; [] tc_c.timeout { la.start } }
		altstep da(timer p) runs on C { [] p.timeout { setverdict(pass) } }`
	takesOwn := `alt { [] any timer.timeout { if (t1.running) { setverdict(fail, "took an ended timer") } else { setverdict(pass) } } }`
	for name, body := range map[string]string{
		"function":     `timer t1 := 0.5; t1.start; f(); ` + takesOwn,
		"block":        `timer t1 := 0.5; t1.start; if (true) { timer b := 0.2; b.start } ` + takesOwn,
		"timer array":  `timer t1 := 0.5; t1.start; farr(); ` + takesOwn,
		"altstep":      `timer t1 := 0.5; t1.start; tc_c.start(0.1); alt { [] as1() {} } ` + takesOwn,
		"called often": `for (var integer i := 0; i < 2000; i := i + 1) { fl() } timer t1 := 0.1; t1.start; ` + takesOwn,
		"parameter": `timer t1 := 0.5; t1.start; timer p; fp(p);
			alt { [] p.timeout { setverdict(pass) } [] t1.timeout { setverdict(fail, "the passed timer ended") } }`,
		"passed to a default": `if (true) { timer tl := 0.1; tl.start; activate(da(tl)) }
			timer w := 0.5; w.start;
			alt { [] w.timeout { setverdict(fail, "the default's timer ended") } }`,
		"after its default": `var default d;
			if (true) { timer tl := 0.2; tl.start; d := activate(da(tl)) }
			deactivate(d);
			timer t1 := 0.5; t1.start; ` + takesOwn,
		"alias": `timer t1 := 0.3; t1.start; fa(t1);
			if (not t1.running) { setverdict(fail, "the aliased timer ended") }
			t1.timeout; setverdict(pass)`,
	} {
		src := "module M {\n" + decls + "\ntestcase tc() runs on C {\n" + body + "\n}\n}"
		for _, k := range clocks {
			v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
			if err != nil || v != runtime.PassVerdict {
				t.Errorf("%s, %s clock: %s (%s) %v", name, k.name, v, reason, err)
			}
		}
	}
}

// TestGetverdictIsTheComponentsOwn: getverdict gives the local verdict of
// the component running it (ETSI ES 201 873-1 24.3) — in the MTC its own,
// not the testcase's, which a PTC's verdict moves; in a PTC its own too.
// The PTCs end inconc; a getverdict giving another's verdict sets fail.
func TestGetverdictIsTheComponentsOwn(t *testing.T) {
	src := `module M {
		type component C {}
		function pf() runs on C { setverdict(inconc) }
		function pg() runs on C {
			if (getverdict != none) { setverdict(fail, "the PTC's getverdict gave another's") }
			setverdict(inconc);
			if (getverdict != inconc) { setverdict(fail, "the PTC's getverdict gave another's") }
		}
		testcase tc() runs on C {
			setverdict(pass);
			var C p := C.create; p.start(pf()); p.done;
			var C q := C.create; q.start(pg()); q.done;
			if (getverdict != pass) { setverdict(fail, "the MTC's getverdict gave another's") }
		}
	}`
	for _, k := range clocks {
		v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
		if err != nil || v != runtime.InconcVerdict {
			t.Errorf("%s clock: %s (%s) %v; want inconc", k.name, v, reason, err)
		}
	}
}

// TestDeactivateOfNoActiveDefaultFails: deactivating a default reference
// that is not active on the component — deactivated already, never
// initialised, or another component's — is an error; null has no effect
// (ETSI ES 201 873-1 20.5.3).
func TestDeactivateOfNoActiveDefaultFails(t *testing.T) {
	decls := `type component C { var default cd }
		altstep a() { [] any timer.timeout {} }
		function pd(default d) runs on C { deactivate(d) }`
	for name, c := range map[string]struct {
		body string
		want runtime.Verdict
	}{
		"twice":         {`var default d := activate(a()); deactivate(d); deactivate(d); setverdict(pass)`, runtime.ErrorVerdict},
		"uninitialised": {`var default d; deactivate(d); setverdict(pass)`, runtime.ErrorVerdict},
		"null":          {`var default d := null; deactivate(d); setverdict(pass)`, runtime.PassVerdict},
		"once":          {`var default d := activate(a()); deactivate(d); setverdict(pass)`, runtime.PassVerdict},
		"after all":     {`var default d := activate(a()); deactivate; deactivate(d); setverdict(pass)`, runtime.ErrorVerdict},
		"another's": {`setverdict(pass); var default d := activate(a());
			var C p := C.create; p.start(pd(d)); p.done`, runtime.ErrorVerdict},
	} {
		src := "module M {\n" + decls + "\ntestcase tc() runs on C {\n" + c.body + "\n}\n}"
		for _, k := range clocks {
			v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
			if err != nil || v != c.want {
				t.Errorf("%s, %s clock: %s (%s) %v; want %s", name, k.name, v, reason, err, c.want)
			}
		}
	}
	// In a control part too.
	for name, c := range map[string]struct {
		body string
		want runtime.Verdict
	}{
		"twice": {`var default d := activate(a()); deactivate(d); deactivate(d); execute(tc())`, runtime.ErrorVerdict},
		"once":  {`var default d := activate(a()); deactivate(d); execute(tc())`, runtime.PassVerdict},
	} {
		src := "module M {\n" + decls + "\ntestcase tc() runs on C { setverdict(pass) }\ncontrol {\n" + c.body + "\n}\n}"
		if v, reason := runControl(t, "M", src); v != c.want {
			t.Errorf("control, %s: %s (%s); want %s", name, v, reason, c.want)
		}
	}
}

// TestTimersHeldByADefaultEndWithTheBehaviour: an alive component's
// behaviour that passed a local timer to a default leaves none behind it:
// the behaviour started next does not see it.
func TestTimersHeldByADefaultEndWithTheBehaviour(t *testing.T) {
	src := `module M {
		type component C {}
		altstep da(timer p) runs on C { [] p.timeout {} }
		function f1() runs on C { timer tl := 5.0; tl.start; activate(da(tl)) }
		function f2() runs on C {
			if (any timer.running) { setverdict(fail, "the previous behaviour's timer runs") } else { setverdict(pass) }
		}
		testcase tc() runs on C {
			var C p := C.create alive;
			p.start(f1()); p.done;
			p.start(f2()); p.done;
		}
	}`
	assertSameVerdictBothClocks(t, src, runtime.PassVerdict)
}

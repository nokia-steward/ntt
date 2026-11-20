package interpreter_test

import (
	"testing"
	"time"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// defaultsModule declares three logging defaults on one timer, and f_wait,
// whose alt only the defaults can end: v_log records which one did.
const defaultsModule = `
	type component C { timer T; var charstring v_log := ""; var default vc_d }
	function f_get() runs on C return default { return vc_d }
	altstep a(charstring s) runs on C { [] T.timeout { v_log := v_log & s } }
	altstep b() runs on C { [] any timer.timeout {} }
	function f_wait() runs on C {
		timer g := 0.5;
		T.start(0.1); g.start;
		alt { [] g.timeout { v_log := v_log & "g" } }
		g.stop;
	}
	function want(charstring w) runs on C {
		if (v_log == w) { setverdict(pass) } else { setverdict(fail, "log " & v_log & ", want " & w) }
	}
	function f_suspend(default d) runs on C { deactivate(d, true) }
`

// deactivate(d, true) suspends a default: it keeps its place in the list
// of defaults but is not invoked, until activate(d) re-activates it there
// and returns d (ETSI ES 201 873-1 20.5.0, 20.5.2, 20.5.3).
func TestDefaultSuspension(t *testing.T) {
	for name, body := range map[string]string{
		"suspended is skipped": `var default d := activate(a("1"));
			deactivate(d, true); f_wait(); want("g")`,
		"re-activated runs": `var default d := activate(a("1"));
			deactivate(d, true); f_wait(); activate(d); f_wait(); want("g1")`,
		"keeps its place": `var default d1 := activate(a("1")), d2 := activate(a("2")), d3 := activate(a("3"));
			deactivate(d2, true); f_wait(); activate(d2); f_wait();
			deactivate(d3); f_wait(); deactivate(d2); f_wait(); want("3321")`,
		"the next one is invoked": `var default d1 := activate(a("1")), d2 := activate(a("2"));
			deactivate(d2, true); f_wait(); want("1")`,
		"activate returns the reference": `var default d := activate(a("1"));
			deactivate(d, true);
			var default e := activate(d);
			if (e != d) { setverdict(fail, "another reference") } else { setverdict(pass) }`,
		"false removes": `var default d := activate(a("1"));
			deactivate(d, false); f_wait(); want("g")`,
		"a suspended one is removed": `var default d := activate(a("1"));
			deactivate(d, true); deactivate(d); f_wait(); want("g")`,
		"deactivate removes the suspended ones": `var default d := activate(a("1"));
			deactivate(d, true); deactivate; f_wait(); want("g")`,
		"suspending twice": `var default d := activate(a("1"));
			deactivate(d, true); deactivate(d, true); activate(d); f_wait(); want("1")`,
		"null with suspend": `deactivate(null, true); setverdict(pass)`,
		"re-activated through a function": `vc_d := activate(a("1"));
			deactivate(vc_d, true); var default e := activate(f_get());
			f_wait(); want("1");
			if (e != vc_d) { setverdict(fail, "another reference") }`,
	} {
		t.Run(name, func(t *testing.T) {
			assertSameVerdictBothClocks(t, "module M {"+defaultsModule+
				"testcase tc() runs on C {\n"+body+"\n}\n}", runtime.PassVerdict)
		})
	}
}

// Re-activating a default that is not suspended, and a suspend expression
// that is no boolean value, are errors (ETSI ES 201 873-1 20.5.2 e, 20.5.3
// b, c), as is suspending another component's default (20.5.3).
func TestDefaultSuspensionErrors(t *testing.T) {
	for name, body := range map[string]string{
		"activate of an active default": `var default d := activate(b());
			setverdict(pass); activate(d)`,
		"activate of a removed default": `var default d := activate(b());
			deactivate(d); setverdict(pass); activate(d)`,
		"activate of an uninitialised reference": `var default d;
			setverdict(pass); activate(d)`,
		"uninitialised suspend expression": `var boolean s; var default d := activate(b());
			setverdict(pass); deactivate(d, s)`,
		"another component's default": `var default d := activate(b());
			setverdict(pass); var C p := C.create; p.start(f_suspend(d)); p.done`,
	} {
		src := "module M {" + defaultsModule + "testcase tc() runs on C {\n" + body + "\n}\n}"
		for _, k := range clocks {
			v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
			if err != nil || v != runtime.ErrorVerdict {
				t.Errorf("%s, %s clock: %s (%s) %v; want error", name, k.name, v, reason, err)
			}
		}
	}
}

// A control part suspends and re-activates its own defaults too.
func TestDefaultSuspensionInControl(t *testing.T) {
	decls := `type component C {}
		testcase tcPass() runs on C { setverdict(pass) }
		testcase tcFail() runs on C { setverdict(fail) }
		altstep a(timer tg) { [] tg.timeout {} }`
	for name, c := range map[string]struct {
		body string
		want runtime.Verdict
	}{
		"suspended, then re-activated": {`timer tg := 0.1;
			var default d := activate(a(tg));
			deactivate(d, true);
			tg.start;
			timer t := 0.3;
			t.start;
			t.timeout;
			if (t.running) { execute(tcFail()) }
			activate(d);
			tg.start;
			t.start;
			t.timeout;
			if (t.running) { execute(tcPass()) } else { execute(tcFail()) }`, runtime.PassVerdict},
		"activate of an active default": {`timer tg := 0.1;
			var default d := activate(a(tg));
			activate(d);
			execute(tcPass())`, runtime.ErrorVerdict},
	} {
		src := "module M {\n" + decls + "\ncontrol {\n" + c.body + "\n}\n}"
		for _, k := range clocks {
			opts := k.opts
			opts.TestcaseTimeout = 10 * time.Second
			v, reason, err := interpreter.RunControlWith([]*ttcn3.Tree{parse(t, src)}, "M", opts)
			if v != c.want {
				t.Errorf("%s, %s clock: %s (%s) %v; want %s", name, k.name, v, reason, err, c.want)
			}
		}
	}
}

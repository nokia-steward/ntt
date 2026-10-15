package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestComponentCallUncaughtStopRejected(t *testing.T) {
	tree := parse(t, `module M {
		type component C {}
		function f() runs on C { stop; }
		testcase tc() runs on C system C {
			var C c := C.create;
			c.call(f());
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "component-call-uncaught-stop") {
		t.Fatalf("expected component-call-uncaught-stop, got %v", codes(diags))
	}
}

func TestComponentCallCaughtStopAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type component C {}
		function f() runs on C { stop; }
		testcase tc() runs on C system C {
			var C c := C.create;
			c.call(f()) catch(stop) {}
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "component-call-uncaught-stop") {
		t.Fatalf("unexpected component-call-uncaught-stop, got %v", codes(diags))
	}
}

func TestComponentCallUncaughtTimeoutRejected(t *testing.T) {
	tree := parse(t, `module M {
		type component C {}
		function f() runs on C {
			timer t := 3.0;
			t.start;
			t.timeout;
		}
		testcase tc() runs on C system C {
			var C c := C.create;
			c.call(f(), 1.0);
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if !containsCode(diags, "component-call-uncaught-timeout") {
		t.Fatalf("expected component-call-uncaught-timeout, got %v", codes(diags))
	}
}

func TestComponentCallCaughtTimeoutAccepted(t *testing.T) {
	tree := parse(t, `module M {
		type component C {}
		function f() runs on C {
			timer t := 3.0;
			t.start;
			t.timeout;
		}
		testcase tc() runs on C system C {
			var C c := C.create;
			c.call(f(), 1.0) catch(timeout) {}
		}
	}`)
	diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
	if containsCode(diags, "component-call-uncaught-timeout") {
		t.Fatalf("unexpected component-call-uncaught-timeout, got %v", codes(diags))
	}
}

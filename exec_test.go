package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	rreport "github.com/nokia/ntt/runtime/report"
)

// writeTC writes a TTCN-3 source file to a temp dir and returns its path.
func writeTC(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "m.ttcn")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestExecDeterministic covers the `ntt exec --deterministic` wiring: the
// staticDriver runs the testcase on the strict discrete-event scheduler +
// virtual clock, so a 30s timer fires instantly (virtual time) and the
// verdict is produced without any real-clock wait.
func TestExecDeterministic(t *testing.T) {
	path := writeTC(t, `module m {
		type component C {}
		testcase tc() runs on C system C {
			timer t := 30.0;
			t.start;
			t.timeout;
			setverdict(pass);
		}
	}`)
	d := newStaticDriver([]string{path})

	start := time.Now()
	v, reason, err := d.Run(context.Background(), "m.tc")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v != rreport.Pass {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("30s timer took %v; the deterministic virtual clock should fire it instantly", elapsed)
	}
}

// TestExecDeterministicForkedPTC covers concurrent execution under the
// scheduler: an alive PTC forks, runs, and the MTC's blocking comp.done
// parks so the PTC is scheduled — a case the retired approximate engine
// modelled only by skipping the body.
func TestExecDeterministicForkedPTC(t *testing.T) {
	path := writeTC(t, `module m {
		type component C {}
		function f() runs on C { setverdict(pass); }
		testcase tc() runs on C system C {
			var C p := C.create alive;
			p.start(f());
			p.done;
			setverdict(pass);
		}
	}`)
	d := newStaticDriver([]string{path})

	v, reason, err := d.Run(context.Background(), "m.tc")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if v != rreport.Pass {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}
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

// TestExecLiveUsesRealClock covers the `ntt exec --live` wiring: with
// live=true the staticDriver leaves both deterministic knobs off, so the
// same strict engine runs on the REAL clock — a 0.4s timer takes real wall
// time to fire, whereas the virtual-clock default fires it instantly. This
// is the mode that paces real I/O against a live SUT.
func TestExecLiveUsesRealClock(t *testing.T) {
	src := `module m {
		type component C {}
		testcase tc() runs on C system C {
			timer t := 0.4;
			t.start;
			t.timeout;
			setverdict(pass);
		}
	}`
	path := writeTC(t, src)

	// Virtual-clock default: fires instantly.
	dv := newStaticDriver([]string{path})
	startV := time.Now()
	if v, reason, err := dv.Run(context.Background(), "m.tc"); err != nil || v != rreport.Pass {
		t.Fatalf("virtual Run: verdict=%s reason=%q err=%v, want pass", v, reason, err)
	}
	if elapsed := time.Since(startV); elapsed > 200*time.Millisecond {
		t.Fatalf("virtual clock took %v for a 0.4s timer; should fire instantly", elapsed)
	}

	// --live: real clock paces the 0.4s timer for real.
	dl := newStaticDriver([]string{path})
	dl.live = true
	startL := time.Now()
	if v, reason, err := dl.Run(context.Background(), "m.tc"); err != nil || v != rreport.Pass {
		t.Fatalf("live Run: verdict=%s reason=%q err=%v, want pass", v, reason, err)
	}
	if elapsed := time.Since(startL); elapsed < 350*time.Millisecond {
		t.Fatalf("live clock took only %v for a 0.4s timer; the real clock should pace it", elapsed)
	}
}
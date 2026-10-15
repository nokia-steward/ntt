//go:build linux || darwin

package interpreter_test

import (
	"context"
	"syscall"
	"testing"
	"time"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/ttcn3"
)

// TestLiveClock_KilledOnExitedAliveComponentDoesNotSpin covers `.killed` on
// an `alive` component whose behaviour has ended: it is done but not killed,
// so the statement blocks (ETSI 21.3.8) until something kills it. The wait
// looped on the component's exit channel, already closed, and burned a core
// for as long as it blocked. Here it blocks until the run's deadline, and
// must spend that time waiting rather than computing.
func TestLiveClock_KilledOnExitedAliveComponentDoesNotSpin(t *testing.T) {
	cpu := func() float64 {
		var ru syscall.Rusage
		if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
			t.Fatalf("getrusage: %v", err)
		}
		return float64(ru.Utime.Sec) + float64(ru.Utime.Usec)/1e6
	}
	before := cpu()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_, _, _ = interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, `module M {
		type port P message { inout charstring }
		type component W { port P p }
		function once() runs on W { p.send("x"); }
		testcase tc() runs on W system W {
			var W b := W.create alive;
			connect(b:p, self:p);
			b.start(once());
			b.killed;
		}
	}`)}, "M.tc", interpreter.TestcaseOptions{Context: ctx})
	after := cpu()
	if used := after - before; used > 0.25 {
		t.Fatalf("blocking on .killed for 0.5s used %.2fs of CPU; it should wait, not spin", used)
	}
}

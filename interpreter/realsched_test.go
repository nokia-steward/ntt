package interpreter_test

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/runtime/port"
	"github.com/nokia/ntt/runtime/port/api"
	"github.com/nokia/ntt/runtime/port/goport"
	"github.com/nokia/ntt/ttcn3"
)

// sendCounter tallies sends per port-instance name so a test can prove a
// worker actually executed its send/receive loop.
type sendCounter struct {
	mu sync.Mutex
	m  map[string]int
}

func newSendCounter() *sendCounter { return &sendCounter{m: map[string]int{}} }

func (c *sendCounter) inc(inst string) {
	c.mu.Lock()
	c.m[inst]++
	c.mu.Unlock()
}

func (c *sendCounter) get(inst string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.m[inst]
}

func (c *sendCounter) instances() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, len(c.m))
	for k := range c.m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// asyncEchoPort is a pure-Go test port that counts sends per instance
// and, like a real network peer, replies ASYNCHRONOUSLY from its own
// goroutine (per the api.TestPort contract). The async reply forces the
// worker's alt to actually park and be woken by Inject, exercising the
// real-scheduler combined port+timer park.
type asyncEchoPort struct {
	api.Base
	inst string
	c    *sendCounter
}

func (e *asyncEchoPort) Send(_ context.Context, env *port.Envelope) error {
	e.c.inc(e.inst)
	if obj, ok := env.Payload.(runtime.Object); ok {
		go goport.Inject(e.inst, obj)
	}
	return nil
}

// TestRealScheduler_SingleWorkerLoopRuns is the Phase-A proof: with
// RealScheduler on, an `alive` PTC running
// `while(true){ send; alt{receive|t_guard.timeout}; pace }` actually runs
// on a real goroutine, issues many requests, and each receive is woken by
// the port's async Inject (not the guard timer). `all component.stop`
// then terminates the loop promptly. Today (default model) this worker is
// skipped and issues zero requests — see the default-off test below.
func TestRealScheduler_SingleWorkerLoopRuns(t *testing.T) {
	c := newSendCounter()
	goport.Register("P", func(inst string) api.TestPort { return &asyncEchoPort{inst: inst, c: c} })
	t.Cleanup(goport.Reset)

	src := `module M {
		type port P message { inout integer }
		type component C { port P p }
		function f_worker() runs on C {
			timer t_pace := 0.001;
			timer t_guard := 5.0;
			map(self:p, system:p);
			while (true) {
				p.send(1);
				t_guard.start;
				alt {
					[] p.receive(integer:?) { }
					[] t_guard.timeout { setverdict(inconc); stop; }
				}
				t_pace.start; t_pace.timeout;
			}
		}
		testcase tc() runs on C system C {
			var C w := C.create alive;
			w.start(f_worker());
			timer t_run := 0.1;
			t_run.start; t_run.timeout;
			all component.stop;
		}
	}`

	start := time.Now()
	_, _, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc",
		interpreter.TestcaseOptions{RealScheduler: true})
	if err != nil {
		t.Fatalf("RunTestcaseWith: %v", err)
	}
	elapsed := time.Since(start)

	// Phase B qualifies the worker's port to a per-PTC instance key, so
	// sum across whatever instance the single worker owns.
	total := 0
	for _, in := range c.instances() {
		total += c.get(in)
	}
	if total < 3 {
		t.Fatalf("worker sent %d requests, want >= 3 (loop not running or receives not waking)", total)
	}
	// The run is a ~0.1s pace window plus teardown; if the while(true)
	// worker were not terminated by `all component.stop`, WaitPTCs would
	// force-stop it only after its 5s cap.
	if elapsed > 3*time.Second {
		t.Fatalf("run took %v, want ~0.1s+teardown (worker not stopped)", elapsed)
	}
}

// TestRealScheduler_FourWorkersOwnReplies is the full-acceptance proof:
// 4 `alive` PTCs, all with `port P p`, each run the load loop. Each
// worker sends its OWN id and asserts every reply equals that id — so a
// cross-routed reply (the shared-queue bug) would flip the verdict to
// fail. The Go port sees FOUR DISTINCT instances (per-PTC port identity)
// each with sends > 0. Today, without routing, the 4 would collapse to a
// single instance/queue.
func TestRealScheduler_FourWorkersOwnReplies(t *testing.T) {
	c := newSendCounter()
	goport.Register("P", func(inst string) api.TestPort { return &asyncEchoPort{inst: inst, c: c} })
	t.Cleanup(goport.Reset)

	src := `module M {
		type port P message { inout integer }
		type component C { port P p }
		function f_worker(integer id) runs on C {
			var integer got;
			var integer n := 0;
			timer t_pace := 0.001;
			timer t_guard := 5.0;
			map(self:p, system:p);
			while (true) {
				p.send(id);
				t_guard.start;
				alt {
					[] p.receive(integer:?) -> value got {
						if (got != id) { setverdict(fail, "foreign reply"); stop; }
						n := n + 1;
						if (n == 3) { setverdict(pass); }
					}
					[] t_guard.timeout { setverdict(inconc); stop; }
				}
				t_pace.start; t_pace.timeout;
			}
		}
		testcase tc() runs on C system C {
			var C w[4];
			for (var integer i := 0; i < 4; i := i + 1) {
				w[i] := C.create alive;
				w[i].start(f_worker(i));
			}
			timer t_run := 0.15;
			t_run.start; t_run.timeout;
			all component.stop;
		}
	}`

	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc",
		interpreter.TestcaseOptions{RealScheduler: true})
	if err != nil {
		t.Fatalf("RunTestcaseWith: %v", err)
	}
	if v == runtime.FailVerdict {
		t.Fatalf("verdict = fail (%s): a worker received a foreign reply (routing collapsed)", reason)
	}

	insts := c.instances()
	if len(insts) != 4 {
		t.Fatalf("port instances = %d %v, want 4 distinct per-PTC instances", len(insts), insts)
	}
	for _, in := range insts {
		if c.get(in) < 1 {
			t.Fatalf("instance %q sent %d, want >= 1 (worker did not run)", in, c.get(in))
		}
	}
}

// TestRealScheduler_CrossTestcaseInstanceIsolation guards the goport
// teardown hook: component IDs restart each testcase, so a worker in run
// 2 gets the SAME qualified port key as run 1. Without clearing the
// per-instance cache between runs, run 2 would reuse run 1's stale
// TestPort (bound to run 1's counter). We swap the counter between runs
// via a holder the factory reads at port-creation time; if isolation
// works, run 2 creates a fresh port that increments the run-2 counter.
func TestRealScheduler_CrossTestcaseInstanceIsolation(t *testing.T) {
	type holder struct{ c *sendCounter }
	h := &holder{c: newSendCounter()}
	goport.Register("P", func(inst string) api.TestPort { return &asyncEchoPort{inst: inst, c: h.c} })
	t.Cleanup(goport.Reset)

	src := `module M {
		type port P message { inout integer }
		type component C { port P p }
		function f_worker() runs on C {
			timer t_pace := 0.001;
			timer t_guard := 5.0;
			map(self:p, system:p);
			while (true) {
				p.send(1);
				t_guard.start;
				alt {
					[] p.receive(integer:?) { }
					[] t_guard.timeout { stop; }
				}
				t_pace.start; t_pace.timeout;
			}
		}
		testcase tc() runs on C system C {
			var C w := C.create alive;
			w.start(f_worker());
			timer t_run := 0.08;
			t_run.start; t_run.timeout;
			all component.stop;
		}
	}`
	tree := parse(t, src)

	run := func() {
		_, _, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{tree}, "M.tc",
			interpreter.TestcaseOptions{RealScheduler: true})
		if err != nil {
			t.Fatalf("RunTestcaseWith: %v", err)
		}
	}

	run() // run 1 -> h.c (counter 1)
	run1Total := 0
	for _, in := range h.c.instances() {
		run1Total += h.c.get(in)
	}
	if run1Total < 1 {
		t.Fatalf("run 1 sent %d, want >= 1", run1Total)
	}

	h.c = newSendCounter() // swap counter for run 2
	run()                  // run 2 must create fresh ports -> counter 2
	run2Total := 0
	for _, in := range h.c.instances() {
		run2Total += h.c.get(in)
	}
	if run2Total < 1 {
		t.Fatalf("run 2 sent %d on a fresh counter, want >= 1 (stale TestPort reused across testcases)", run2Total)
	}
}

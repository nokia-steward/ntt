package goport_test

import (
	"context"
	"sync"
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/runtime/port"
	"github.com/nokia/ntt/runtime/port/api"
	"github.com/nokia/ntt/runtime/port/goport"
	"github.com/nokia/ntt/ttcn3"
)

func parse(t *testing.T, src string) *ttcn3.Tree {
	t.Helper()
	tree := ttcn3.Parse(src)
	if tree == nil || tree.Err != nil {
		t.Fatalf("parse error: %v\n%s", tree, src)
	}
	return tree
}

func run(t *testing.T, tc, src string) (runtime.Verdict, string) {
	t.Helper()
	v, reason, err := interpreter.RunTestcase([]*ttcn3.Tree{parse(t, src)}, tc)
	if err != nil {
		t.Fatalf("RunTestcase(%s): %v", tc, err)
	}
	return v, reason
}

// echoPort is a pure-Go test port that bounces every sent value straight
// back to the testcase, as if a peer had replied with the same value.
type echoPort struct {
	api.Base
	inst string
}

func (e *echoPort) Send(_ context.Context, env *port.Envelope) error {
	if obj, ok := env.Payload.(runtime.Object); ok {
		goport.Inject(e.inst, obj)
	}
	return nil
}

// TestGoPort_SendReceiveEcho is the headline proof: a pure-Go port,
// registered with no cgo, carries `p.send` into Go and a `p.receive`
// back out via Inject.
func TestGoPort_SendReceiveEcho(t *testing.T) {
	goport.Register("P", func(inst string) api.TestPort { return &echoPort{inst: inst} })
	t.Cleanup(goport.Reset)

	v, reason := run(t, "M.tc", `module M {
		type port P message { inout integer }
		type component C { port P p }
		testcase tc() runs on C system C {
			map(self:p, system:p);
			p.send(5);
			alt {
				[] p.receive(5) { setverdict(pass); }
				[] p.receive { setverdict(fail, "wrong value"); }
			}
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

// lifecyclePort records every lifecycle / send call the runtime drives,
// proving the Titan-style hooks (OnMap/OnUnmap/OnStart/OnStop/Send) are
// all wired.
type lifecyclePort struct {
	api.Base
	mu                          sync.Mutex
	maps, unmaps, starts, stops int
	sent                        []runtime.Object
}

func (p *lifecyclePort) OnMap(context.Context) error   { p.bump(&p.maps); return nil }
func (p *lifecyclePort) OnUnmap(context.Context) error { p.bump(&p.unmaps); return nil }
func (p *lifecyclePort) OnStart(context.Context) error { p.bump(&p.starts); return nil }
func (p *lifecyclePort) OnStop(context.Context) error  { p.bump(&p.stops); return nil }
func (p *lifecyclePort) Send(_ context.Context, env *port.Envelope) error {
	p.mu.Lock()
	if obj, ok := env.Payload.(runtime.Object); ok {
		p.sent = append(p.sent, obj)
	}
	p.mu.Unlock()
	return nil
}
func (p *lifecyclePort) bump(n *int) { p.mu.Lock(); *n++; p.mu.Unlock() }

func TestGoPort_FullLifecycle(t *testing.T) {
	lp := &lifecyclePort{}
	goport.Register("P", func(string) api.TestPort { return lp })
	t.Cleanup(goport.Reset)

	v, reason := run(t, "M.tc", `module M {
		type port P message { inout integer }
		type component C { port P p }
		testcase tc() runs on C system C {
			map(self:p, system:p);
			p.start;
			p.send(1);
			p.send(2);
			p.stop;
			unmap(self:p, system:p);
			setverdict(pass);
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
	lp.mu.Lock()
	defer lp.mu.Unlock()
	if lp.maps != 1 || lp.unmaps != 1 || lp.starts != 1 || lp.stops != 1 || len(lp.sent) != 2 {
		t.Fatalf("lifecycle: map=%d unmap=%d start=%d stop=%d sends=%d, want 1/1/1/1/2",
			lp.maps, lp.unmaps, lp.starts, lp.stops, len(lp.sent))
	}
}

// asyncPort delivers a value from its own I/O goroutine after OnMap, so
// the testcase's blocking receive must park and wake on the injected
// message - exercising the concurrent receive path end to end in Go.
type asyncPort struct {
	api.Base
	inst string
}

func (a *asyncPort) OnMap(context.Context) error {
	go goport.Inject(a.inst, runtime.NewInt(42))
	return nil
}

func TestGoPort_AsyncReceive(t *testing.T) {
	goport.Register("P", func(inst string) api.TestPort { return &asyncPort{inst: inst} })
	t.Cleanup(goport.Reset)

	v, reason := run(t, "M.tc", `module M {
		type port P message { inout integer }
		type component C { port P p }
		testcase tc() runs on C system C {
			timer t_guard := 5.0;
			map(self:p, system:p);
			t_guard.start;
			alt {
				[] p.receive(42) { setverdict(pass); }
				[] t_guard.timeout { setverdict(fail, "no async delivery"); }
			}
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

// callPort answers a procedure call synchronously by writing the reply
// value to the envelope's Reply channel.
type callPort struct {
	api.Base
}

func (callPort) Call(_ context.Context, env *port.Envelope) error {
	if env.Reply != nil {
		env.Reply <- runtime.NewInt(99)
	}
	return nil
}

// TestGoPort_ProcedureCall proves the procedure path (p.call -> driver,
// reply -> getreply) is wired for a pure-Go port.
func TestGoPort_ProcedureCall(t *testing.T) {
	goport.Register("P", func(string) api.TestPort { return callPort{} })
	t.Cleanup(goport.Reset)

	v, reason := run(t, "M.tc", `module M {
		signature S() return integer;
		type port P procedure { inout S }
		type component C { port P p }
		testcase tc() runs on C system C {
			var integer v_ret := 0;
			map(self:p, system:p);
			p.call(S:{}) {
				[] p.getreply(S:?) -> value v_ret { }
			}
			if (v_ret == 99) { setverdict(pass); }
			else { setverdict(fail, "wrong reply", v_ret); }
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

// replyPort answers a procedure call by delivering the reply
// asynchronously via InjectReply (rather than the synchronous env.Reply
// channel).
type replyPort struct {
	api.Base
	inst string
}

func (p *replyPort) Call(_ context.Context, _ *port.Envelope) error {
	goport.InjectReply(p.inst, nil, runtime.NewInt(42))
	return nil
}

// TestGoPort_ProcedureReplyAsync proves InjectReply delivers a reply's
// return value to getreply (the path used when a port replies out of
// band rather than on the env.Reply channel).
func TestGoPort_ProcedureReplyAsync(t *testing.T) {
	goport.Register("P", func(inst string) api.TestPort { return &replyPort{inst: inst} })
	t.Cleanup(goport.Reset)

	v, reason := run(t, "M.tc", `module M {
		signature S() return integer;
		type port P procedure { inout S }
		type component C { port P p }
		testcase tc() runs on C system C {
			var integer v_ret := 0;
			map(self:p, system:p);
			p.call(S:{}) {
				[] p.getreply(S:?) -> value v_ret { }
			}
			if (v_ret == 42) { setverdict(pass); }
			else { setverdict(fail, "ret", v_ret); }
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

// excPort answers a procedure call by raising an exception via
// InjectException.
type excPort struct {
	api.Base
	inst string
}

func (p *excPort) Call(_ context.Context, _ *port.Envelope) error {
	goport.InjectException(p.inst, runtime.NewInt(99))
	return nil
}

// TestGoPort_ProcedureException proves InjectException reaches a catch.
func TestGoPort_ProcedureException(t *testing.T) {
	goport.Register("P", func(inst string) api.TestPort { return &excPort{inst: inst} })
	t.Cleanup(goport.Reset)

	v, reason := run(t, "M.tc", `module M {
		signature S() exception(integer);
		type port P procedure { inout S }
		type component C { port P p }
		testcase tc() runs on C system C {
			var integer v_exc := 0;
			map(self:p, system:p);
			p.call(S:{}) {
				[] p.catch(S, integer:?) -> value v_exc { }
			}
			if (v_exc == 99) { setverdict(pass); }
			else { setverdict(fail, "wrong exception", v_exc); }
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

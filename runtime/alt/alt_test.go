package alt_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/nokia/ntt/runtime/alt"
	"github.com/nokia/ntt/runtime/component"
	"github.com/nokia/ntt/runtime/port"
	"github.com/nokia/ntt/runtime/timer"
)

func TestAlt_PortReceive(t *testing.T) {
	a := port.New("A", port.Message, 0)
	b := port.New("B", port.Message, 0)
	port.Connect(a, b)

	var got interface{}
	done := make(chan struct{})

	go func() {
		v := alt.Run([]alt.Branch{{
			Port: b,
			Body: func(x interface{}) {
				if env, ok := x.(*port.Envelope); ok {
					got = env.Payload
				}
			},
		}}, nil)
		if v.BranchIndex != 0 {
			t.Errorf("BranchIndex = %d, want 0", v.BranchIndex)
		}
		close(done)
	}()

	time.Sleep(5 * time.Millisecond)
	a.Send("hi")

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("alt did not return")
	}
	if got != "hi" {
		t.Errorf("payload = %v, want hi", got)
	}
}

func TestAlt_TimerWins(t *testing.T) {
	tm := timer.New("T")
	tm.Start(20 * time.Millisecond)

	idleP := port.New("P", port.Message, 0)
	done := make(chan struct{})

	go func() {
		v := alt.Run([]alt.Branch{
			{Port: idleP},
			{Timer: tm},
		}, nil)
		if v.BranchIndex != 1 {
			t.Errorf("BranchIndex = %d, want 1 (timer)", v.BranchIndex)
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("alt timeout branch did not fire")
	}
}

func TestAlt_ComponentDone(t *testing.T) {
	r := component.NewRegistry()
	c := r.New("c", component.PTC)
	c.Start(func(*component.Component) error {
		time.Sleep(10 * time.Millisecond)
		return nil
	})

	v := alt.Run([]alt.Branch{{
		Component:      c,
		ComponentEvent: alt.OnDone,
	}}, nil)
	if v.BranchIndex != 0 {
		t.Errorf("BranchIndex = %d, want 0", v.BranchIndex)
	}
}

func TestAlt_PortFilter(t *testing.T) {
	// Match the TTCN-3 semantics of `receive(template)`: the filter
	// is checked against the HEAD of the queue. A matching head fires
	// the branch and consumes it; a non-matching head leaves the alt
	// blocked (no skip-ahead).
	a := port.New("A", port.Message, 0)
	b := port.New("B", port.Message, 0)
	port.Connect(a, b)
	a.Send("take")

	var fired int32
	v := alt.Run([]alt.Branch{{
		Port: b,
		PortFilter: func(e *port.Envelope) bool {
			return e.Payload == "take"
		},
		Body: func(interface{}) { atomic.StoreInt32(&fired, 1) },
	}}, nil)
	if v.BranchIndex != 0 {
		t.Errorf("BranchIndex = %d, want 0", v.BranchIndex)
	}
	if atomic.LoadInt32(&fired) != 1 {
		t.Error("filter branch did not fire")
	}
}

func TestAlt_PortFilter_NonMatchingHeadBlocks(t *testing.T) {
	// A non-matching head plus a wildcard fallback branch should still
	// resolve: scanning order means the wildcard catches the message.
	a := port.New("A", port.Message, 0)
	b := port.New("B", port.Message, 0)
	port.Connect(a, b)
	a.Send("skip")

	var caught interface{}
	v := alt.Run([]alt.Branch{
		{
			Port:       b,
			PortFilter: func(e *port.Envelope) bool { return e.Payload == "take" },
		},
		{
			Port: b,
			Body: func(x interface{}) { caught = x.(*port.Envelope).Payload },
		},
	}, nil)
	if v.BranchIndex != 1 {
		t.Errorf("BranchIndex = %d, want 1 (wildcard)", v.BranchIndex)
	}
	if caught != "skip" {
		t.Errorf("caught = %v, want skip", caught)
	}
}

func TestAlt_Stop(t *testing.T) {
	p := port.New("P", port.Message, 0)
	stop := make(chan struct{})
	go func() {
		time.Sleep(20 * time.Millisecond)
		close(stop)
	}()
	v := alt.Run([]alt.Branch{{Port: p}}, stop)
	if v.BranchIndex != -1 {
		t.Errorf("BranchIndex = %d, want -1 (stopped)", v.BranchIndex)
	}
}

func TestAlt_GuardSkips(t *testing.T) {
	a := port.New("A", port.Message, 0)
	b := port.New("B", port.Message, 0)
	port.Connect(a, b)
	a.Send("data")

	var fired int
	v := alt.Run([]alt.Branch{
		{Port: b, Guard: func() bool { return false }, Body: func(interface{}) { fired = 1 }},
		{Port: b, Guard: func() bool { return true }, Body: func(interface{}) { fired = 2 }},
	}, nil)
	if v.BranchIndex != 1 {
		t.Errorf("BranchIndex = %d, want 1", v.BranchIndex)
	}
	if fired != 2 {
		t.Errorf("fired = %d, want 2", fired)
	}
}

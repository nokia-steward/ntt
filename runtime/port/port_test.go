package port_test

import (
	"testing"
	"time"

	"github.com/nokia/ntt/runtime/port"
)

func TestSendReceive(t *testing.T) {
	a := port.New("A", port.Message, 0)
	b := port.New("B", port.Message, 0)
	port.Connect(a, b)
	a.Send("hello")
	msg := b.Receive()
	if msg.Payload != "hello" {
		t.Fatalf("payload = %v, want hello", msg.Payload)
	}
	if msg.From != a {
		t.Fatalf("From = %v, want %v", msg.From, a)
	}
}

func TestTry_Empty(t *testing.T) {
	p := port.New("P", port.Message, 0)
	if _, ok := p.Try(); ok {
		t.Fatal("Try on empty should return false")
	}
}

func TestSnapshot_CopiesWithoutDraining(t *testing.T) {
	a := port.New("A", port.Message, 0)
	b := port.New("B", port.Message, 0)
	port.Connect(a, b)
	a.Send(1)
	a.Send(2)
	a.Send(3)
	msgs := b.Snapshot()
	if len(msgs) != 3 {
		t.Fatalf("got %d msgs, want 3", len(msgs))
	}
	if b.Len() != 3 {
		t.Fatalf("Snapshot should NOT consume entries, Len = %d", b.Len())
	}
}

func TestPeekAndDropHead(t *testing.T) {
	a := port.New("A", port.Message, 0)
	b := port.New("B", port.Message, 0)
	port.Connect(a, b)
	a.Send("first")
	a.Send("second")

	env, ok := b.Peek()
	if !ok || env.Payload != "first" {
		t.Fatalf("Peek = (%v, %v)", env, ok)
	}
	if env, ok = b.Peek(); !ok || env.Payload != "first" {
		t.Fatalf("Peek should be idempotent")
	}
	b.DropHead()
	env, ok = b.Peek()
	if !ok || env.Payload != "second" {
		t.Fatalf("after DropHead, Peek = (%v, %v)", env, ok)
	}
}

func TestProcedureCall(t *testing.T) {
	caller := port.New("C", port.Procedure, 0)
	server := port.New("S", port.Procedure, 0)
	port.Connect(caller, server)

	done := caller.Call("ping")

	go func() {
		msg := server.Receive()
		if msg.Reply == nil {
			t.Errorf("Reply channel should be non-nil for procedure call")
			return
		}
		msg.Reply <- "pong"
	}()

	select {
	case reply := <-done:
		if reply != "pong" {
			t.Fatalf("reply = %v, want pong", reply)
		}
	case <-time.After(time.Second):
		t.Fatal("procedure reply timed out")
	}
}

func TestConnectIdempotent(t *testing.T) {
	a := port.New("A", port.Message, 0)
	b := port.New("B", port.Message, 0)
	port.Connect(a, b)
	port.Connect(a, b)
	if len(a.Peers()) != 1 {
		t.Fatalf("Peers = %v, want 1", a.Peers())
	}
	port.Disconnect(a, b)
	if len(a.Peers()) != 0 {
		t.Fatalf("Peers after disconnect = %v, want 0", a.Peers())
	}
}

func TestClear(t *testing.T) {
	a := port.New("A", port.Message, 0)
	b := port.New("B", port.Message, 0)
	port.Connect(a, b)
	a.Send(1)
	a.Send(2)
	b.Clear()
	if _, ok := b.Try(); ok {
		t.Fatal("Clear should have emptied the queue")
	}
}

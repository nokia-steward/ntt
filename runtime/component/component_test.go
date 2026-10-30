package component_test

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nokia/ntt/runtime/component"
)

func TestRegistry_NewAndRoles(t *testing.T) {
	r := component.NewRegistry()
	mtc := r.New("mtc", component.MTC)
	sys := r.New("system", component.System)
	a := r.New("A", component.PTC)
	b := r.New("B", component.PTC)

	if got := r.MTC(); got != mtc {
		t.Errorf("MTC = %v, want %v", got, mtc)
	}
	if got := r.System(); got != sys {
		t.Errorf("System = %v, want %v", got, sys)
	}
	if got := len(r.PTCs()); got != 2 {
		t.Errorf("PTCs = %d, want 2", got)
	}
	_ = a
	_ = b
}

func TestComponent_StartDone(t *testing.T) {
	r := component.NewRegistry()
	c := r.New("c", component.PTC)
	var fired int32
	c.Start(func(*component.Component) error {
		atomic.StoreInt32(&fired, 1)
		return nil
	})
	select {
	case <-c.Done():
	case <-time.After(time.Second):
		t.Fatal("Done not closed")
	}
	if atomic.LoadInt32(&fired) != 1 {
		t.Error("behaviour did not run")
	}
	if c.State() != component.Done {
		t.Errorf("State = %s, want done", c.State())
	}
	if c.Alive() {
		t.Error("Alive should be false after Done")
	}
}

func TestComponent_Kill(t *testing.T) {
	r := component.NewRegistry()
	c := r.New("c", component.PTC)
	c.Start(func(c *component.Component) error {
		select {
		case <-c.Killed():
			return component.ErrKilled
		case <-time.After(time.Second):
			return errors.New("not killed in time")
		}
	})
	time.Sleep(10 * time.Millisecond)
	c.Kill()
	select {
	case <-c.Done():
	case <-time.After(time.Second):
		t.Fatal("Done not closed after Kill")
	}
	if c.State() != component.Killed {
		t.Errorf("State = %s, want killed", c.State())
	}
	if !errors.Is(c.Err(), component.ErrKilled) {
		t.Errorf("Err = %v, want ErrKilled", c.Err())
	}
}

func TestComponent_PanicBecomesError(t *testing.T) {
	r := component.NewRegistry()
	c := r.New("c", component.PTC)
	c.Start(func(*component.Component) error {
		panic("boom")
	})
	<-c.Done()
	if c.Err() == nil {
		t.Fatal("expected non-nil Err after panic")
	}
}

package arena

import (
	"sync"
	"testing"
)

func TestArena_BytesGrows(t *testing.T) {
	a := New()
	for i := 0; i < 1000; i++ {
		b := a.Bytes(64)
		if len(b) != 64 {
			t.Fatalf("Bytes returned %d, want 64", len(b))
		}
	}
}

func TestArena_String(t *testing.T) {
	a := New()
	s := a.String("hello world")
	if s != "hello world" {
		t.Fatalf("String returned %q", s)
	}
}

func TestArena_AllocZero(t *testing.T) {
	type S struct {
		X, Y int
	}
	a := New()
	s := Alloc[S](a)
	if s.X != 0 || s.Y != 0 {
		t.Fatalf("Alloc did not zero memory: %+v", *s)
	}
	s.X = 7
	s.Y = 11
	if s.X != 7 || s.Y != 11 {
		t.Fatalf("expected to be able to write: %+v", *s)
	}
}

func TestArena_ResetReuses(t *testing.T) {
	a := New()
	a.Bytes(100)
	a.Bytes(200)
	a.Reset()
	if a.used != 0 || len(a.cur) != 0 {
		t.Fatalf("Reset did not clear state")
	}
}

func TestPool_GetReturnsEmpty(t *testing.T) {
	a := Get()
	a.Bytes(50)
	Put(a)

	a2 := Get()
	defer Put(a2)
	if a2.used != 0 {
		t.Fatalf("Get returned an arena with used=%d, want 0", a2.used)
	}
}

func TestPool_ConcurrentUse(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a := Get()
			for j := 0; j < 100; j++ {
				_ = a.Bytes(32)
			}
			Put(a)
		}()
	}
	wg.Wait()
}

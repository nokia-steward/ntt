package runtime

import "testing"

func TestAsciiSingleRuneCachePoolsCharstrings(t *testing.T) {
	// Single-ASCII-character charstrings now come from a shared
	// pool so the `body[i] == "X"` hot path stops mallocing one
	// *String per iteration.
	a := NewCharstring("{")
	b := NewCharstring("{")
	if a != b {
		t.Fatalf("NewCharstring(\"{\") returned different pointers (%p, %p); cache miss", a, b)
	}
	if !a.Interned {
		t.Fatalf("cached *String not marked Interned")
	}
	// Multi-rune strings stay un-pooled (the cache is by design
	// limited to single-rune ASCII; longer literals don't benefit
	// because the comparison cost dominates allocation).
	c := NewCharstring("ab")
	d := NewCharstring("ab")
	if c == d {
		t.Fatalf("multi-rune NewCharstring returned the same pointer (%p)", c)
	}
}

func TestCloneIfInternedCopiesOnWrite(t *testing.T) {
	cached := NewCharstring("X")
	clone, swapped := cached.CloneIfInterned()
	if !swapped || clone == cached {
		t.Fatalf("CloneIfInterned on interned string: swapped=%v ptr-same=%v", swapped, clone == cached)
	}
	clone.Value[0] = 'Y'
	if cached.Value[0] != 'X' {
		t.Fatalf("clone mutation leaked back into cache: cached=%q", string(cached.Value))
	}

	// A non-interned *String reports no swap.
	plain := &String{Value: []rune("abc"), ascii: true}
	got, swapped := plain.CloneIfInterned()
	if swapped || got != plain {
		t.Fatalf("CloneIfInterned on plain string: swapped=%v ptr-same=%v", swapped, got == plain)
	}
}

func TestNewCharstringFromRunesUsesCache(t *testing.T) {
	cached := NewCharstring("Y")
	fromRunes := NewCharstringFromRunes([]rune{'Y'})
	if cached != fromRunes {
		t.Fatalf("NewCharstringFromRunes did not hit cache: cached=%p fromRunes=%p", cached, fromRunes)
	}
	multi := NewCharstringFromRunes([]rune{'A', 'B'})
	if multi.Interned {
		t.Fatalf("multi-rune from-runes string marked Interned")
	}
}

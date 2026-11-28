package syntax_test

import (
	"sync"
	"testing"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// TestRoot_PositionIsSafeForConcurrentUse resolves positions in one tree
// from several goroutines at once, as test components do when each logs
// where its operations are. Every answer must match a sequential pass.
// The line cache behind Position used to be four fields written one after
// another, which a concurrent reader could see half-updated.
func TestRoot_PositionIsSafeForConcurrentUse(t *testing.T) {
	src := benchmarkSource(500)
	root := syntax.Parse(src)
	want := make([]syntax.Position, len(src))
	for off := range src {
		want[off] = root.Position(off)
	}
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			x := g + 1
			for i := 0; i < 20000; i++ {
				x = x*1103515245 + 12345
				off := (x & 0x7fffffff) % len(src)
				if got := root.Position(off); got != want[off] {
					t.Errorf("Position(%d) = %v, want %v", off, got, want[off])
					return
				}
			}
		}(g)
	}
	wg.Wait()
}

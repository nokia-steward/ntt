package interpreter

import (
	"testing"
)

// BenchmarkGoroutineIDFast measures the asm fast path. On a machine
// where init() has flipped useFastGoid==true (linux/amd64, default
// Go layout), this is the cost paid on every eval() entry/exit.
func BenchmarkGoroutineIDFast(b *testing.B) {
	if !useFastGoid {
		b.Skip("fast path not enabled on this build")
	}
	var sink uint64
	for i := 0; i < b.N; i++ {
		sink = fastGoroutineID()
	}
	_ = sink
}

// BenchmarkGoroutineIDSlow measures the stack-parsing fallback the
// fast path is meant to replace. The ratio is the per-call speedup
// the interpreter sees once we leave the fallback behind.
func BenchmarkGoroutineIDSlow(b *testing.B) {
	var sink uint64
	for i := 0; i < b.N; i++ {
		sink = slowGoroutineID()
	}
	_ = sink
}

// BenchmarkDepthEnterLeave measures the steady-state cost of an
// enter()+leave() pair (the cost paid per eval() node) on whichever
// goroutineID path init() picked. Drops if either the asm read or
// the sync.Map atomic regresses.
func BenchmarkDepthEnterLeave(b *testing.B) {
	var d depthStore
	for i := 0; i < b.N; i++ {
		d.enter()
		d.leave()
	}
}

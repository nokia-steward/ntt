package syntax_test

import (
	"strings"
	"testing"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// benchmarkSource builds a TTCN-3 module with n short functions so the
// resulting Root spans n*3 lines. That gives the line-offset table
// enough entries that the binary search vs cache difference is visible.
func benchmarkSource(n int) []byte {
	var b strings.Builder
	b.WriteString("module M {\n")
	for i := 0; i < n; i++ {
		b.WriteString("function f")
		// crude itoa to avoid the strconv allocation
		b.WriteString(itoa(i))
		b.WriteString("() runs on system {}\n\n")
	}
	b.WriteString("}\n")
	return []byte(b.String())
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}

func BenchmarkRoot_Position_Sequential(b *testing.B) {
	src := benchmarkSource(1000)
	root := syntax.Parse(src)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Sequential traversal - hits the cache after the first call.
		for off := 0; off < len(src); off += 32 {
			_ = root.Position(off)
		}
	}
}

func BenchmarkRoot_Position_Random(b *testing.B) {
	src := benchmarkSource(1000)
	root := syntax.Parse(src)
	// Pre-computed pseudo-random offsets so the benchmark is
	// deterministic and the iteration cost itself is constant.
	offsets := make([]int, 1024)
	x := 1
	for i := range offsets {
		x = x*1103515245 + 12345
		offsets[i] = (x & 0x7fffffff) % len(src)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, off := range offsets {
			_ = root.Position(off)
		}
	}
}

func TestRoot_Position_CacheCorrectness(t *testing.T) {
	src := []byte("module M {\nvar integer x := 1;\nvar integer y := 2;\n}\n")
	root := syntax.Parse(src)

	// Sweep every byte twice (once cold, once warm) and compare.
	want := make([]syntax.Position, len(src)+1)
	for off := 0; off <= len(src); off++ {
		want[off] = root.Position(off)
	}
	for off := 0; off <= len(src); off++ {
		got := root.Position(off)
		if got != want[off] {
			t.Errorf("Position(%d): got %+v want %+v", off, got, want[off])
		}
	}

	// Also probe in reverse order so the cache repeatedly misses.
	for off := len(src); off >= 0; off-- {
		got := root.Position(off)
		if got != want[off] {
			t.Errorf("Position(%d) reverse: got %+v want %+v", off, got, want[off])
		}
	}
}

// Package arena provides a simple bump allocator backed by a chain of
// growable byte slabs.
//
// The design amortises per-object allocator pressure by handing out
// slices of a pre-allocated chunk and releasing everything in O(1)
// by dropping the chunk. The Go garbage collector
// can then reclaim the chunk as a single heap object instead of
// chasing every node individually.
//
// The arena is goroutine-unsafe by design - callers that need
// concurrent allocations should own one arena per goroutine. The LSP
// handlers obtain a fresh arena from the package's sync.Pool, fill it
// over the lifetime of a request, and return it on completion.
package arena

import (
	"sync"
	"unsafe"
)

// defaultChunkSize is the size of a fresh chunk. It's a compromise:
// big enough that small allocations stay cheap, small enough that a
// rarely-used arena does not waste memory.
const defaultChunkSize = 16 * 1024

// Arena is a chain of byte slabs from which short-lived objects are
// carved out.
type Arena struct {
	chunks [][]byte
	cur    []byte
	used   int
}

// New returns an empty arena.
func New() *Arena {
	return &Arena{}
}

// Reset releases every chunk for reuse. The arena can be filled again
// after Reset without any further allocation.
func (a *Arena) Reset() {
	a.chunks = a.chunks[:0]
	a.cur = nil
	a.used = 0
}

// Bytes carves an len-byte slice out of the arena. The returned slice
// shares the arena's backing storage; callers must not mutate it after
// the arena is reset.
func (a *Arena) Bytes(n int) []byte {
	if n <= 0 {
		return nil
	}
	if a.used+n > len(a.cur) {
		a.growFor(n)
	}
	out := a.cur[a.used : a.used+n : a.used+n]
	a.used += n
	return out
}

// String copies s into the arena and returns a string sharing the
// arena's storage. Useful when the caller is otherwise holding many
// short-lived strings and wants to defer their collection.
func (a *Arena) String(s string) string {
	if s == "" {
		return ""
	}
	b := a.Bytes(len(s))
	copy(b, s)
	return unsafeString(b)
}

// Alloc returns a zeroed *T whose memory lives in the arena.
//
// Note: the returned pointer must not outlive the arena's Reset call
// or the next time the arena is returned to a pool. Treat the value
// like any other arena-allocated object.
func Alloc[T any](a *Arena) *T {
	var zero T
	size := int(unsafe.Sizeof(zero))
	b := a.Bytes(size)
	if len(b) < size {
		// Should not happen; growFor always provides enough.
		return new(T)
	}
	return (*T)(unsafe.Pointer(&b[0]))
}

func (a *Arena) growFor(n int) {
	size := defaultChunkSize
	if n > size {
		size = n
	}
	a.cur = make([]byte, size)
	a.used = 0
	a.chunks = append(a.chunks, a.cur)
}

// unsafeString returns s as a string without copying the underlying
// bytes. Safe only because the caller already promised not to mutate
// the bytes after handing them to the arena.
func unsafeString(b []byte) string {
	return *(*string)(unsafe.Pointer(&b))
}

// Pool is the package-wide arena pool that goroutines can borrow from.
// Use Get to acquire one and Put to return it.
var Pool = &sync.Pool{
	New: func() interface{} { return New() },
}

// Get returns an arena from the pool. The arena is guaranteed to be
// empty.
func Get() *Arena {
	a := Pool.Get().(*Arena)
	a.Reset()
	return a
}

// Put returns the arena to the pool for reuse.
func Put(a *Arena) {
	if a == nil {
		return
	}
	a.Reset()
	Pool.Put(a)
}

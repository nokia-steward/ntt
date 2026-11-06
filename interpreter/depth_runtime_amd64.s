// Assembly helper to read the current goroutine's `g` pointer from
// the runtime's TLS slot. See depth_runtime.go for the why.
//
// amd64: the Go runtime keeps g in its thread-local slot, which the
// assembler's TLS pseudo-register addresses on every operating system
// (FS on Linux, GS on macOS and Windows), as the compiler's own
// goroutine-aware preludes do. We copy that slot into the return value
// and return -- a few instructions, ~1-2 ns vs runtime.Stack's tens of
// microseconds. init() checks the result against runtime.Stack before
// using it.

//go:build amd64

#include "textflag.h"

// func getg() unsafe.Pointer
TEXT ·getg(SB), NOSPLIT, $0-8
	MOVQ (TLS), AX
	MOVQ AX, ret+0(FP)
	RET

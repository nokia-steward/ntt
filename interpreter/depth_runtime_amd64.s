// Assembly helper to read the current goroutine's `g` pointer from
// the runtime's TLS slot. See depth_runtime.go for the why.
//
// linux/amd64 specific: the Go runtime stores g in the TLS slot at
// FS:[0], identical to what the compiler emits when generating
// goroutine-aware preludes. We copy that slot into the return value
// and return -- five instructions, ~1-2 ns vs runtime.Stack's tens
// of microseconds.

//go:build amd64 && linux

#include "textflag.h"

// func getg() unsafe.Pointer
TEXT ·getg(SB), NOSPLIT, $0-8
	MOVQ (TLS), AX
	MOVQ AX, ret+0(FP)
	RET

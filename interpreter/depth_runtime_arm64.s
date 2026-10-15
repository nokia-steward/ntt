// Assembly helper to read the current goroutine's `g` pointer. See
// depth_runtime.go for the why.
//
// arm64: Go keeps g in a register of its own (R28), on every operating
// system. init() checks the result against runtime.Stack before using
// it.

//go:build arm64

#include "textflag.h"

// func getg() unsafe.Pointer
TEXT ·getg(SB), NOSPLIT, $0-8
	MOVD	g, R0
	MOVD	R0, ret+0(FP)
	RET

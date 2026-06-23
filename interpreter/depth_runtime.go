// Package interpreter: fast goroutine-id extraction.
//
// The tree-walking interpreter calls eval() recursively for every
// AST node. The depth tracker in depth.go needs a per-goroutine
// counter so concurrent testcases (one goroutine per PTC) don't
// interfere; that means every eval() needs the current goroutine's
// id.
//
// The portable form (runtime.Stack(buf, false)) costs tens of
// microseconds because it freezes the runtime and serializes a stack
// frame for parsing. For a 250-byte body the f_extractNodeObject
// helper triggers ~10 evals per outer-loop iteration -> several
// hundred thousand stack walks for a single testcase, which is the
// minutes-long hang we were seeing.
//
// This file replaces that with a direct read of g.goid via a tiny
// piece of Go assembly that grabs the current g pointer out of the
// runtime's TLS slot (see depth_runtime_amd64.s for the linux/amd64
// build; other architectures fall back to runtime.Stack via the
// build-constrained shim below).
//
// goidOffset is calibrated at init() against runtime.Stack so we
// fail loudly if a future Go release shuffles the g struct instead
// of silently returning the wrong id.
package interpreter

import (
	"fmt"
	"os"
	"runtime"
	"unsafe"

	nttruntime "github.com/nokia/ntt/runtime"
)

// getg is implemented in depth_runtime_amd64.s for linux/amd64. On
// other targets the build tag in that file is unset, and we fall
// back to the slow path defined in depth_runtime_fallback.go.

// goidOffset is the byte offset of the goid field within runtime.g.
// Stable across Go 1.5..1.26 on amd64 (and arm64) but calibrated at
// init() to survive a future Go layout change.
var goidOffset uintptr = 152

// useFastGoid is flipped to true once init() has verified the
// runtime hack works on this platform. When false, goroutineID()
// falls back to the slow stack-parsing form.
var useFastGoid bool

func fastGoroutineID() uint64 {
	if !useFastGoid {
		return slowGoroutineID()
	}
	g := getg()
	if g == nil {
		return 0
	}
	return *(*uint64)(unsafe.Pointer(uintptr(g) + goidOffset))
}

// slowGoroutineID is the original stack-parsing form, kept as the
// fallback for non-amd64 builds and as the calibration reference.
func slowGoroutineID() uint64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	const prefix = "goroutine "
	if n < len(prefix) || string(buf[:len(prefix)]) != prefix {
		return 0
	}
	var id uint64
	for i := len(prefix); i < n; i++ {
		c := buf[i]
		if c < '0' || c > '9' {
			break
		}
		id = id*10 + uint64(c-'0')
	}
	return id
}

// runtimeStack is retained for backwards compatibility with any
// caller that still references it.
func runtimeStack(buf []byte) int { return runtime.Stack(buf, false) }

// NTT_DEPTH_SLOW=1 forces the slow stack-parsing form even when the
// assembly fast path is available. Useful for measuring the speedup
// and as an escape hatch if a future Go release breaks the layout
// hack between releases of ntt.
func init() {
	// Publish the fast goid to the runtime package so the per-PTC
	// compStack lookup in TestcaseExec can use it without pulling
	// in the depth_runtime assembly.
	nttruntime.GoroutineIDFn = fastGoroutineID
	if os.Getenv("NTT_DEPTH_SLOW") != "" {
		if os.Getenv("NTT_DEPTH_DEBUG") != "" {
			fmt.Fprintln(os.Stderr, "[ntt-depth] NTT_DEPTH_SLOW=1: forcing slow path")
		}
		return
	}
	g := getg()
	if g == nil {
		fmt.Fprintln(os.Stderr, "[ntt-depth] getg() returned nil; staying on slow goid path")
		return
	}
	expected := slowGoroutineID()
	if expected == 0 {
		fmt.Fprintln(os.Stderr, "[ntt-depth] slowGoroutineID() returned 0; staying on slow goid path")
		return
	}
	if got := *(*uint64)(unsafe.Pointer(uintptr(g) + goidOffset)); got == expected {
		useFastGoid = true
		if os.Getenv("NTT_DEPTH_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "[ntt-depth] fast goid offset=%d (default)\n", goidOffset)
		}
		return
	}
	for off := uintptr(64); off < 512; off += 8 {
		if got := *(*uint64)(unsafe.Pointer(uintptr(g) + off)); got == expected {
			goidOffset = off
			useFastGoid = true
			if os.Getenv("NTT_DEPTH_DEBUG") != "" {
				fmt.Fprintf(os.Stderr, "[ntt-depth] fast goid offset=%d (calibrated)\n", goidOffset)
			}
			return
		}
	}
	fmt.Fprintln(os.Stderr, "[ntt-depth] goid calibration failed; staying on slow goid path")
}

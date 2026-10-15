// Package cabi documents the C ABI for ntt test ports. The hand-
// written header (ntt_port.h) and the optional Titan compatibility
// shim (ntt_titan_compat.h) live here; the cgo implementation of the
// symbols those headers declare lives in the `cgo` subpackage so the
// rest of the runtime stays cgo-free.
//
// To compile a test port against the ntt header from C:
//
//	#include <ntt_port.h>
//	#include <stdio.h>
//
//	static int my_send(void *u, NTT_Buffer p) {
//	    printf("got %zu bytes\n", p.len);
//	    return 0;
//	}
//
//	static NTT_TestPort port = {
//	    .name = "demo",
//	    .send = my_send,
//	};
//
//	int main(void) { return ntt_port_register(&port); }
//
// Existing Eclipse Titan test ports can reuse the Titan-named
// `Handler_Add_Fd_Read` / `TTCN_Buffer` API by also including
// `ntt_titan_compat.h`; the shim is a header-only mapping onto the
// ntt primitives and adds no runtime dependency.
//
// The Go runtime side fulfils these symbols via the
// `runtime/port/api/cabi/cgo` package. Import it (typically with a
// blank identifier) from the binary's main package so the cgo
// initialiser registers itself:
//
//	import _ "github.com/nokia/ntt/runtime/port/api/cabi/cgo"
package cabi

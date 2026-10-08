//go:build cabicgo

// Build-tag-gated import of the cabi/cgo test-port bridge. Building
// ntt with `-tags cabicgo` (or via the dedicated `make ntt-cabicgo`
// target) wires the bridge into the runtime so the interpreter
// automatically routes port operations on instances registered via
// ntt_port_register() through the C side. Users who don't need C/C++
// test ports get a cgo-free build by default; turning the tag on
// pulls in the cgo toolchain.
package main

import _ "github.com/nokia/ntt/runtime/port/api/cabi/cgo"

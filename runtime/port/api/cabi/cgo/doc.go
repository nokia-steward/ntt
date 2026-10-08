// Package cgo implements the cgo bridge for the C ABI described in
// `runtime/port/api/cabi/ntt_port.h`. Linking this package into an
// ntt binary makes the following C symbols available to test ports
// compiled from C / C++:
//
//   ntt_port_register             - hand a NTT_TestPort* to the runtime
//   ntt_port_unregister           - remove it
//   ntt_event_loop_add_fd_read    - watch an fd for POLLIN
//   ntt_event_loop_add_fd_write   - watch an fd for POLLOUT
//   ntt_event_loop_remove_fd      - stop watching an fd
//
// The package is build-tagged behind cgo so that downstream projects
// that don't need C++ test ports keep building without a C toolchain.
// Import it with a blank identifier from the binary's main package
// to pull in the bridge:
//
//	import _ "github.com/nokia/ntt/runtime/port/api/cabi/cgo"
//
// or build with the `cabicgo` tag to have the runtime register the
// bridge automatically. The Registry() function returns the set of
// currently registered test ports as Go-side TestPort wrappers; the
// runtime calls it during port instantiation to attach the C-side
// implementation to the matching runtime/port.Port.
package cgo

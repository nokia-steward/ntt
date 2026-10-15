//go:build cgo && (linux || darwin || freebsd)

// This file is part of the cgo package's production build because
// Go's cgo translator refuses to compile `import "C"` in _test.go
// files. The fixture functions are isolated to a separate package
// boundary via the FixtureXxx prefix so production code never
// accidentally pulls them in.

package cgo

/*
#include <stddef.h>
#include <string.h>
#include <unistd.h>
#include "ntt_port.h"

static int  fx_on_map_calls;
static int  fx_on_unmap_calls;
static int  fx_on_start_calls;
static int  fx_on_stop_calls;
static int  fx_send_calls;
static char fx_last_payload[256];
static size_t fx_last_payload_len;

static int  fx_on_map(void *u)   { (void)u; fx_on_map_calls++;   return 0; }
static int  fx_on_unmap(void *u) { (void)u; fx_on_unmap_calls++; return 0; }
static int  fx_on_start(void *u) { (void)u; fx_on_start_calls++; return 0; }
static int  fx_on_stop(void *u)  { (void)u; fx_on_stop_calls++;  return 0; }

static int  fx_send(void *u, NTT_Buffer p) {
    (void)u;
    fx_send_calls++;
    size_t n = p.len < sizeof(fx_last_payload) ? p.len : sizeof(fx_last_payload);
    memcpy(fx_last_payload, p.data, n);
    fx_last_payload_len = n;
    return 0;
}

static NTT_TestPort fx_port = {
    "fixture", NULL,
    fx_on_map, fx_on_unmap, fx_on_start, fx_on_stop,
    fx_send, NULL,
    { NULL, NULL }
};

static int fx_register(void)   { return ntt_port_register(&fx_port); }
static void fx_unregister(void) { ntt_port_unregister((char *)"fixture"); }
static int fx_get_send_calls(void) { return fx_send_calls; }
static int fx_get_map_calls(void)  { return fx_on_map_calls; }
static size_t fx_copy_payload(char *dst, size_t cap) {
    size_t n = fx_last_payload_len < cap ? fx_last_payload_len : cap;
    memcpy(dst, fx_last_payload, n);
    return n;
}

static int fx_fd_callback_fd;
static int fx_fd_callback_calls;
static void fx_fd_callback(int fd, void *user) {
    (void)user;
    fx_fd_callback_fd = fd;
    fx_fd_callback_calls++;
    char buf[8];
    while (read(fd, buf, sizeof(buf)) > 0) {}
}
static int fx_get_fd_callback_calls(void) { return fx_fd_callback_calls; }
static int fx_get_fd_callback_fd(void) { return fx_fd_callback_fd; }
static int fx_register_fd_read(int fd) {
    return ntt_event_loop_add_fd_read(fd, fx_fd_callback, NULL);
}
static void fx_remove_fd(int fd) { ntt_event_loop_remove_fd(fd); }

static int fx_pipe(int *fds) { return pipe(fds); }
static int fx_write_byte(int fd) { return (int)write(fd, "x", 1); }
static int fx_close(int fd) { return close(fd); }

// fx_inject_via_runtime is the C-side caller that mimics what a real
// test port does inside its on_send hook: pick the inject pointer off
// the NTT_Runtime sub-struct the bridge stashed during OnMap, then
// call it. Returns the inject() return code (0 success, !=0 failure)
// or -2 when the runtime callback hasn't been wired up yet (sign of
// a broken / forgotten OnMap path).
static int fx_inject_via_runtime(const uint8_t *data, size_t len) {
    if (fx_port.runtime.inject == NULL) return -2;
    NTT_Buffer buf = { data, len };
    return fx_port.runtime.inject(fx_port.runtime.handle, buf);
}
*/
import "C"

import "unsafe"

// Fixture-only helpers re-exported to the test file in plain Go so
// the test code itself doesn't need to import "C". Kept lowercase
// where possible so they don't leak into the package's public API.

// FixtureRegister registers the fx_port and returns the rc.
func FixtureRegister() int { return int(C.fx_register()) }

// FixtureUnregister removes the fx_port. Safe to call repeatedly.
func FixtureUnregister() { C.fx_unregister() }

// FixtureMapCalls returns the C-side on_map counter.
func FixtureMapCalls() int { return int(C.fx_get_map_calls()) }

// FixtureSendCalls returns the C-side send counter.
func FixtureSendCalls() int { return int(C.fx_get_send_calls()) }

// FixturePayload returns whatever bytes the fixture port's send hook
// received on its most recent invocation.
func FixturePayload() []byte {
	var buf [256]C.char
	n := C.fx_copy_payload(&buf[0], C.size_t(len(buf)))
	out := make([]byte, int(n))
	for i := 0; i < int(n); i++ {
		out[i] = byte(buf[i])
	}
	return out
}

// FixturePipe creates a pipe via pipe(2) and returns the (read, write) fds.
func FixturePipe() (int, int, int) {
	var fds [2]C.int
	rc := C.fx_pipe(&fds[0])
	return int(fds[0]), int(fds[1]), int(rc)
}

// FixtureClose closes one of the pipe fds.
func FixtureClose(fd int) { C.fx_close(C.int(fd)) }

// FixtureWriteByte writes one byte into the supplied fd.
func FixtureWriteByte(fd int) int { return int(C.fx_write_byte(C.int(fd))) }

// FixtureRegisterFdRead arms the supplied fd for POLLIN with the
// fixture's read-callback. Returns the rc from
// ntt_event_loop_add_fd_read.
func FixtureRegisterFdRead(fd int) int { return int(C.fx_register_fd_read(C.int(fd))) }

// FixtureRemoveFd unsubscribes the fd from the loop.
func FixtureRemoveFd(fd int) { C.fx_remove_fd(C.int(fd)) }

// FixtureFdCallbackCalls returns the running count of fd-ready
// callback invocations.
func FixtureFdCallbackCalls() int { return int(C.fx_get_fd_callback_calls()) }

// FixtureFdCallbackFd returns the fd argument captured by the most
// recent callback.
func FixtureFdCallbackFd() int { return int(C.fx_get_fd_callback_fd()) }

// FixtureInjectViaRuntime calls the inject callback the bridge
// stashed on the fixture port's NTT_Runtime substruct (via
// CDriver.OnMap -> ntt_setup_runtime). Returns the inject() rc
// (0 on success). Returns -2 if the runtime callback was never set,
// which is the signal callers want when checking "did OnMap wire
// up the inject path the test port relies on?".
func FixtureInjectViaRuntime(data []byte) int {
	var cdata *C.uint8_t
	clen := C.size_t(len(data))
	if len(data) > 0 {
		cdata = (*C.uint8_t)(unsafe.Pointer(&data[0]))
	}
	return int(C.fx_inject_via_runtime(cdata, clen))
}

//go:build cgo && (linux || darwin || freebsd)

package cgo

/*
#include <stddef.h>
#include "ntt_port.h"

// Trampoline that lets Go call a C function pointer indirectly. cgo
// won't let us invoke an NTT_FdReadyCallback directly from Go code,
// so we keep the indirect dispatch on the C side.
static void ntt_invoke_fd_callback(NTT_FdReadyCallback cb, int fd, void *user) {
    if (cb) { cb(fd, user); }
}
*/
import "C"

import (
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// fdEntry is one watched file descriptor.
type fdEntry struct {
	fd     int
	events int16 // unix.POLLIN / unix.POLLOUT (additive)
	cb     C.NTT_FdReadyCallback
	user   unsafe.Pointer
}

var (
	loopOnce  sync.Once
	loopMu    sync.Mutex
	loopFds   map[int]*fdEntry
	loopReady chan struct{} // notifies the loop that the fd set changed
)

// startLoop spins up the poll(2) goroutine. The loop sleeps in
// syscall.Poll with a short timeout so it stays responsive to
// add/remove from other goroutines via the loopReady wakeup.
func startLoop() {
	loopOnce.Do(func() {
		loopFds = map[int]*fdEntry{}
		loopReady = make(chan struct{}, 1)
		go pollLoop()
	})
}

func wakeLoop() {
	select {
	case loopReady <- struct{}{}:
	default:
	}
}

func pollLoop() {
	for {
		loopMu.Lock()
		fds := make([]unix.PollFd, 0, len(loopFds)+1)
		owners := make([]*fdEntry, 0, len(loopFds))
		for _, e := range loopFds {
			fds = append(fds, unix.PollFd{Fd: int32(e.fd), Events: e.events})
			owners = append(owners, e)
		}
		loopMu.Unlock()

		if len(fds) == 0 {
			// No fds to watch. Block on the wake channel
			// instead of spinning poll(2) on an empty set.
			select {
			case <-loopReady:
			case <-time.After(250 * time.Millisecond):
			}
			continue
		}

		_, err := unix.Poll(fds, 100)
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			// On any other poll error: drain the wake
			// channel and try again. We don't have a
			// better place to log this from inside the
			// cabi cgo bridge.
			continue
		}
		for i, pfd := range fds {
			if pfd.Revents == 0 {
				continue
			}
			e := owners[i]
			if e == nil || e.cb == nil {
				continue
			}
			// Dispatch on the loop goroutine: the C ABI
			// contract is that callbacks may not block.
			C.ntt_invoke_fd_callback(e.cb, C.int(e.fd), e.user)
		}
		// Drain the wake channel after a poll iteration so we
		// don't immediately re-enter when nothing changed.
		select {
		case <-loopReady:
		default:
		}
	}
}

//export ntt_event_loop_add_fd_read
func ntt_event_loop_add_fd_read(fd C.int, cb C.NTT_FdReadyCallback, user unsafe.Pointer) C.int {
	return addFd(int(fd), unix.POLLIN, cb, user)
}

//export ntt_event_loop_add_fd_write
func ntt_event_loop_add_fd_write(fd C.int, cb C.NTT_FdReadyCallback, user unsafe.Pointer) C.int {
	return addFd(int(fd), unix.POLLOUT, cb, user)
}

func addFd(fd int, events int16, cb C.NTT_FdReadyCallback, user unsafe.Pointer) C.int {
	if fd < 0 {
		return -1
	}
	startLoop()
	loopMu.Lock()
	defer loopMu.Unlock()
	if e, ok := loopFds[fd]; ok {
		// Same fd registered twice: union the event masks and
		// take the new callback. Matches Titan's behaviour of
		// allowing a port to upgrade from read-only to read+write
		// without remove+add.
		e.events |= events
		e.cb = cb
		e.user = user
	} else {
		loopFds[fd] = &fdEntry{fd: fd, events: events, cb: cb, user: user}
	}
	wakeLoop()
	return 0
}

//export ntt_event_loop_remove_fd
func ntt_event_loop_remove_fd(fd C.int) {
	loopMu.Lock()
	delete(loopFds, int(fd))
	loopMu.Unlock()
	wakeLoop()
}

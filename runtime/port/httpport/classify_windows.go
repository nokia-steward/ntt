//go:build windows

package httpport

import (
	"errors"
	"syscall"
)

// Winsock error numbers. Go's syscall package exposes WSAECONNRESET on
// Windows but not the others, and the POSIX-named constants it does define
// (syscall.ECONNREFUSED and friends) carry different values from the ones
// the network stack actually returns — so matching against them silently
// never fires. The numbers below are the documented, stable Winsock codes.
//
// They are matched numerically rather than by message text because Windows
// error strings come from FormatMessage and are localized: "connection
// refused" never appears, and on a non-English host neither would any other
// phrase we could look for.
const (
	wsaeNetUnreach  = syscall.Errno(10051)
	wsaeTimedOut    = syscall.Errno(10060)
	wsaeConnRefused = syscall.Errno(10061)
	wsaeHostUnreach = syscall.Errno(10065)
)

// platformReason classifies the Windows-specific socket errors. It returns
// ok=false when the error is not one of them, leaving the caller's
// portable checks to run.
func platformReason(err error) (string, bool) {
	switch {
	case errors.Is(err, wsaeConnRefused):
		return reasonRefused, true
	case errors.Is(err, syscall.WSAECONNRESET):
		return reasonReset, true
	case errors.Is(err, wsaeHostUnreach), errors.Is(err, wsaeNetUnreach):
		return reasonUnreachable, true
	case errors.Is(err, wsaeTimedOut):
		return reasonTimeout, true
	}
	return "", false
}

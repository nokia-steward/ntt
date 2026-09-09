package httpport

// Unit tests for the transport-error classifier. These are in the internal
// test package deliberately: classify is a pure function, so every branch —
// including ones that need network conditions a test machine cannot be
// relied on to produce (EHOSTUNREACH on a network with a catch-all route,
// for instance) — can be exercised hermetically. The end-to-end tests in
// httpport_test.go prove the wiring from a real failure through to a TTCN-3
// template; these prove the table.

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"syscall"
	"testing"

	"github.com/nokia/ntt/runtime"
)

// wrapped mimics how the http client presents an error: *url.Error around
// *net.OpError around the underlying cause.
func wrapped(cause error) error {
	return &url.Error{
		Op:  "Get",
		URL: "http://example.invalid/x",
		Err: &net.OpError{Op: "dial", Net: "tcp", Err: cause},
	}
}

func TestClassify(t *testing.T) {
	for _, k := range []struct {
		name string
		err  error
		want string
	}{
		{"refused", wrapped(syscall.ECONNREFUSED), reasonRefused},
		{"host unreachable", wrapped(syscall.EHOSTUNREACH), reasonUnreachable},
		{"net unreachable", wrapped(syscall.ENETUNREACH), reasonUnreachable},
		{"reset", wrapped(syscall.ECONNRESET), reasonReset},
		{"eof", wrapped(io.EOF), reasonReset},
		{"unexpected eof", wrapped(io.ErrUnexpectedEOF), reasonReset},
		{"deadline", wrapped(context.DeadlineExceeded), reasonTimeout},
		{"dns", wrapped(&net.DNSError{Err: "no such host", Name: "x.invalid"}), reasonDNS},
		{"unknown authority", wrapped(x509.UnknownAuthorityError{}), reasonTLS},
		{"hostname mismatch", wrapped(x509.HostnameError{Host: "wrong"}), reasonTLS},
		{"cert invalid", wrapped(x509.CertificateInvalidError{Reason: x509.Expired}), reasonTLS},
		{"unclassified", wrapped(errors.New("something else entirely")), reasonOther},

		// Message fallbacks, for platforms whose errors do not unwrap to the
		// syscall constants. The Windows phrasings are covered here because
		// the Windows-only numeric path (classify_windows.go) cannot be
		// compiled on the machine most of this is developed on.
		{"windows refused text", errors.New(
			"No connection could be made because the target machine actively refused it."), reasonRefused},
		{"windows reset text", errors.New(
			"An existing connection was forcibly closed by the remote host."), reasonReset},
		{"windows host unreachable text", errors.New(
			"A socket operation was attempted to an unreachable host."), reasonUnreachable},
		{"posix refused text", errors.New("dial tcp 1.2.3.4:80: connect: connection refused"), reasonRefused},
		{"posix no route text", errors.New("dial tcp 1.2.3.4:80: connect: no route to host"), reasonUnreachable},
		{"timeout text", errors.New("context deadline exceeded (Client.Timeout exceeded)"), reasonTimeout},
	} {
		t.Run(k.name, func(t *testing.T) {
			if got := classify(k.err); got != k.want {
				t.Fatalf("classify(%v) = %q, want %q", k.err, got, k.want)
			}
		})
	}
}

// TestClassifyPrefersSpecificCause guards the ordering the classifier
// depends on: a certificate rejection and a refused connection are both
// "the request failed", but they send an engineer to different places, so
// the specific cause must win over a generic timeout wrapper.
func TestClassifyPrefersSpecificCause(t *testing.T) {
	// A DNS failure that also reports itself as a timeout must classify as
	// dns: the name is the problem, the wait is the symptom.
	dns := wrapped(&net.DNSError{Err: "i/o timeout", Name: "x.invalid", IsTimeout: true})
	if got := classify(dns); got != reasonDNS {
		t.Fatalf("classify(dns timeout) = %q, want %q", got, reasonDNS)
	}
}

// TestTransportErrorReasonValues pins the wire contract: these integers are
// declared by every suite that matches on TransportError, so a renumbering
// here silently breaks all of them.
func TestTransportErrorReasonValues(t *testing.T) {
	for i, key := range []string{
		reasonRefused, reasonUnreachable, reasonTimeout,
		reasonDNS, reasonTLS, reasonOther, reasonReset,
	} {
		ev, err := runtimeEnumValue(key)
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		if ev.IntValue() != i {
			t.Fatalf("%s has value %d, want %d — renumbering breaks every declared TransportErrorReason",
				key, ev.IntValue(), i)
		}
	}
}

func runtimeEnumValue(key string) (*runtime.EnumValue, error) {
	ev, err := runtime.NewEnumValueByKey(transportErrorReasons, key)
	if err != nil {
		return nil, fmt.Errorf("enum %q: %w", key, err)
	}
	return ev, nil
}

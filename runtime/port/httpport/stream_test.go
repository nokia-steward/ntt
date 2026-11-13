package httpport_test

import (
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/runtime/port/httpport"
)

const streamDecls = `
	type record HttpHeader { charstring name, charstring val }
	type record of HttpHeader HttpHeaders;
	type record HttpRequest  { charstring method, charstring path, charstring body, HttpHeaders headers optional }
	type record HttpResponse { integer status, charstring body }
	type record HttpLine { charstring line }
	type record SseEvent { charstring event, charstring data, charstring id }
	type record HttpStreamEnd { charstring detail }
	type enumerated TransportErrorReason {
		refused(0), unreachable(1), timeout(2), dns(3), tls(4), other(5), reset(6), oversize(7)
	}
	type record TransportError { TransportErrorReason reason, charstring detail }
	type port P message { out HttpRequest; in HttpResponse, HttpLine, SseEvent, HttpStreamEnd, TransportError }
	type component C { port P p }
`

// sseHandler answers /events with two server-sent events, flushed apart
// and the second after longer than the port's timeout, then ends the
// stream; /forbidden with a 403 and a body.
func sseHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/forbidden" {
		http.Error(w, "no token", http.StatusForbidden)
		return
	}
	if r.Header.Get("Authorization") != "Bearer t" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	fl := w.(http.Flusher)
	fmt.Fprint(w, ": a comment\nevent: link\ndata: up\nid: 1\n\n")
	fl.Flush()
	time.Sleep(700 * time.Millisecond)
	fmt.Fprint(w, "data: line one\ndata: line two\n\n")
	fl.Flush()
}

const streamSuite = `module M {` + streamDecls + `
	testcase tc() runs on C system C {
		timer g := 5.0;
		map(self:p, system:p);
		g.start;
		p.send(HttpRequest:{ method := "GET", path := "/events", body := "",
			headers := { { name := "Authorization", val := "Bearer t" } } });
		alt { [] p.receive(HttpResponse:{ status := 200, body := "" }) {} [] p.receive { setverdict(fail, "no stream start"); stop } }
		alt { [] p.receive(SseEvent:{ event := "link", data := "up", id := "1" }) {} [] p.receive { setverdict(fail, "first event"); stop } }
		alt {
			[] p.receive(SseEvent:{ event := "message", data := "line one\nline two", id := "1" }) {}
			[] p.receive { setverdict(fail, "second event"); stop }
			[] g.timeout { setverdict(fail, "the stream stalled"); stop }
		}
		alt { [] p.receive(HttpStreamEnd:?) {} [] p.receive { setverdict(fail, "no end of stream"); stop } }
		p.send(HttpRequest:{ method := "GET", path := "/forbidden", body := "", headers := omit });
		alt {
			[] p.receive(HttpResponse:{ status := 403, body := pattern "no token*" }) { setverdict(pass) }
			[] p.receive { setverdict(fail, "an error answer not delivered whole") }
		}
	}
}`

// TestHTTPPort_StreamSSE: with stream := "sse", a response is delivered
// as it arrives — its status, each event, then the end of the stream —
// the timeout bounding only the wait for the headers; an error answer is
// delivered whole.
func TestHTTPPort_StreamSSE(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(sseHandler))
	defer srv.Close()
	httpport.Register("p", srv.URL, httpport.WithStream(httpport.StreamSSE), httpport.WithTimeout(300*time.Millisecond))
	t.Cleanup(httpport.Reset)
	if v, reason := run(t, "M.tc", streamSuite); v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

// TestHTTPPort_StreamSSEOverHTTPS: the same over TLS.
func TestHTTPPort_StreamSSEOverHTTPS(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(sseHandler))
	defer srv.Close()
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	httpport.Register("p", srv.URL, httpport.WithStream(httpport.StreamSSE), httpport.WithTLS(httpport.TLS{CACert: ca}))
	t.Cleanup(httpport.Reset)
	if v, reason := run(t, "M.tc", streamSuite); v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

// TestHTTPPort_StreamLines: with stream := "lines", each line of the body
// is delivered on its own.
func TestHTTPPort_StreamLines(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "first\r\nsecond\n")
	}))
	defer srv.Close()
	httpport.Register("p", srv.URL, httpport.WithStream(httpport.StreamLines))
	t.Cleanup(httpport.Reset)
	v, reason := run(t, "M.tc", `module M {`+streamDecls+`
		testcase tc() runs on C system C {
			timer g := 5.0;
			map(self:p, system:p);
			g.start;
			p.send(HttpRequest:{ method := "GET", path := "/", body := "", headers := omit });
			alt { [] p.receive(HttpResponse:{ status := 200, body := "" }) {} [] p.receive { setverdict(fail, "start"); stop } }
			alt { [] p.receive(HttpLine:{ line := "first" }) {} [] p.receive { setverdict(fail, "first"); stop } }
			alt { [] p.receive(HttpLine:{ line := "second" }) {} [] p.receive { setverdict(fail, "second"); stop } }
			alt { [] p.receive(HttpStreamEnd:?) { setverdict(pass) } [] p.receive { setverdict(fail, "end") } [] g.timeout { setverdict(fail, "timeout") } }
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

// TestHTTPPort_UnmapEndsAStream: a stream the server never ends is ended
// by unmapping the port, and the testcase finishes.
func TestHTTPPort_UnmapEndsAStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: hello\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done() // until the client goes
	}))
	defer srv.Close()
	httpport.Register("p", srv.URL, httpport.WithStream(httpport.StreamSSE))
	t.Cleanup(httpport.Reset)
	start := time.Now()
	v, reason := run(t, "M.tc", `module M {`+streamDecls+`
		testcase tc() runs on C system C {
			timer g := 5.0;
			map(self:p, system:p);
			g.start;
			p.send(HttpRequest:{ method := "GET", path := "/", body := "", headers := omit });
			alt { [] p.receive(HttpResponse:?) {} [] g.timeout { setverdict(fail, "start"); stop } }
			alt { [] p.receive(SseEvent:{ event := ?, data := "hello", id := ? }) {} [] g.timeout { setverdict(fail, "event"); stop } }
			unmap(self:p, system:p);
			setverdict(pass);
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("took %v: unmapping did not end the stream", d)
	}
}

// eventsSuite receives the stream start, then expects the events want in
// order, then the end of the stream.
func eventsSuite(want ...string) string {
	body := ""
	for _, w := range want {
		body += `alt { [] p.receive(SseEvent:` + w + `) {} [] p.receive -> value v { setverdict(fail, "unexpected ", v); stop } [] g.timeout { setverdict(fail, "timeout"); stop } }
`
	}
	return `module M {` + streamDecls + `
		testcase tc() runs on C system C {
			timer g := 5.0;
			var anytype v;
			map(self:p, system:p);
			g.start;
			p.send(HttpRequest:{ method := "GET", path := "/", body := "", headers := omit });
			alt { [] p.receive(HttpResponse:{ status := 200, body := "" }) {} [] g.timeout { setverdict(fail, "start"); stop } }
			` + body + `
			alt { [] p.receive(HttpStreamEnd:?) { setverdict(pass) } [] p.receive -> value v { setverdict(fail, "want the end, got ", v) } }
		}
	}`
}

func sseServer(t *testing.T, raw string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, raw)
	}))
}

// TestHTTPPort_SSEParsing: the event stream format's corners (WHATWG HTML,
// server-sent events): a byte order mark, CR-only line ends, an event
// type reset by a block without data, the last event id carried over.
func TestHTTPPort_SSEParsing(t *testing.T) {
	srv := sseServer(t, "\ufeffevent: ping\r\rid: 7\rdata: a\r\rdata: b\r\n\r\n")
	defer srv.Close()
	httpport.Register("p", srv.URL, httpport.WithStream(httpport.StreamSSE))
	t.Cleanup(httpport.Reset)
	v, reason := run(t, "M.tc", eventsSuite(
		`{ event := "message", data := "a", id := "7" }`,
		`{ event := "message", data := "b", id := "7" }`))
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

// TestHTTPPort_SSEEventEndingInCR: an event whose blank line ends in a CR
// is delivered at once, not when the stream's next byte comes.
func TestHTTPPort_SSEEventEndingInCR(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: now\r\r")
		w.(http.Flusher).Flush()
		<-r.Context().Done() // nothing more, until the client goes
	}))
	defer srv.Close()
	httpport.Register("p", srv.URL, httpport.WithStream(httpport.StreamSSE))
	t.Cleanup(httpport.Reset)
	v, reason := run(t, "M.tc", `module M {`+streamDecls+`
		testcase tc() runs on C system C {
			timer g := 5.0;
			map(self:p, system:p);
			g.start;
			p.send(HttpRequest:{ method := "GET", path := "/", body := "", headers := omit });
			alt { [] p.receive(HttpResponse:?) {} [] g.timeout { setverdict(fail, "start"); stop } }
			alt { [] p.receive(SseEvent:{ event := ?, data := "now", id := ? }) { setverdict(pass) } [] g.timeout { setverdict(fail, "the event was held back") } }
			unmap(self:p, system:p);
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

// TestHTTPPort_RESTOnAnSSEPort: an answer that is no event stream, on an
// SSE port, arrives whole.
func TestHTTPPort_RESTOnAnSSEPort(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()
	httpport.Register("p", srv.URL, httpport.WithStream(httpport.StreamSSE))
	t.Cleanup(httpport.Reset)
	v, reason := run(t, "M.tc", `module M {`+streamDecls+`
		testcase tc() runs on C system C {
			timer g := 5.0;
			map(self:p, system:p);
			g.start;
			p.send(HttpRequest:{ method := "GET", path := "/", body := "", headers := omit });
			alt { [] p.receive(HttpResponse:{ status := 200, body := "{""ok"":true}" }) { setverdict(pass) } [] p.receive { setverdict(fail) } [] g.timeout { setverdict(fail, "timeout") } }
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

// TestHTTPPort_UnreceivedStreamIsStopped: a stream whose values the suite
// does not receive is stopped, with an oversize error, before it fills
// the memory.
func TestHTTPPort_UnreceivedStreamIsStopped(t *testing.T) {
	defer httpport.SetMaxPendingStream(50)()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for i := 0; i < 1000; i++ {
			fmt.Fprintf(w, "data: %d\n\n", i)
		}
	}))
	defer srv.Close()
	httpport.Register("p", srv.URL, httpport.WithStream(httpport.StreamSSE))
	t.Cleanup(httpport.Reset)
	v, reason := run(t, "M.tc", `module M {`+streamDecls+`
		testcase tc() runs on C system C {
			timer w := 1.0;
			map(self:p, system:p);
			p.send(HttpRequest:{ method := "GET", path := "/", body := "", headers := omit });
			w.start; w.timeout;
			var integer n := 0;
			timer g := 1.0; g.start;
			alt {
				[] p.receive(TransportError:{ reason := oversize, detail := ? }) { if (n <= 51) { setverdict(pass) } else { setverdict(fail, n) } }
				[] p.receive(HttpStreamEnd:?) { setverdict(fail, "the stream was not stopped") }
				[] p.receive { n := n + 1; repeat }
				[] g.timeout { setverdict(fail, "no oversize error after ", n) }
			}
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

// TestHTTPPort_StreamPortBounds: on a stream port a second map ends the
// first mapping's stream, an error answer's body is read within the
// timeout, and a Host header is the request's host.
func TestHTTPPort_StreamPortBounds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/forever":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: x\n\n")
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		case "/slow-error":
			w.WriteHeader(500)
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
			case <-time.After(5 * time.Second):
			}
		case "/host":
			fmt.Fprint(w, r.Host)
		}
	}))
	defer srv.Close()
	httpport.Register("p", srv.URL, httpport.WithStream(httpport.StreamSSE), httpport.WithTimeout(300*time.Millisecond))
	t.Cleanup(httpport.Reset)
	start := time.Now()
	v, reason := run(t, "M.tc", `module M {`+streamDecls+`
		testcase tc() runs on C system C {
			timer g := 4.0;
			map(self:p, system:p);
			g.start;
			p.send(HttpRequest:{ method := "GET", path := "/forever", body := "", headers := omit });
			alt { [] p.receive(SseEvent:?) {} [] p.receive(HttpResponse:?) { repeat } [] g.timeout { setverdict(fail, "no event"); stop } }
			map(self:p, system:p);
			p.send(HttpRequest:{ method := "GET", path := "/slow-error", body := "", headers := omit });
			alt {
				[] p.receive(TransportError:{ reason := timeout, detail := ? }) {}
				[] p.receive -> value v { setverdict(fail, "want a timeout, got ", v); stop }
				[] g.timeout { setverdict(fail, "the error body was not bounded"); stop }
			}
			p.send(HttpRequest:{ method := "GET", path := "/host", body := "", headers := { { "Host", "example.test" } } });
			alt {
				[] p.receive(HttpResponse:{ status := 200, body := "example.test" }) {}
				[] p.receive -> value v { setverdict(fail, "host: ", v); stop }
			}
			unmap(self:p, system:p);
			setverdict(pass);
		}
		var anytype v;
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("took %v: a stream outlived its mapping", d)
	}
}

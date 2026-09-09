package httpport_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/runtime/port/httpport"
	"github.com/nokia/ntt/ttcn3"
)

func parse(t *testing.T, src string) *ttcn3.Tree {
	t.Helper()
	tree := ttcn3.Parse(src)
	if tree == nil || tree.Err != nil {
		t.Fatalf("parse error: %v\n%s", tree, src)
	}
	return tree
}

func run(t *testing.T, tc, src string) (runtime.Verdict, string) {
	t.Helper()
	// Zero-value options => real clock + real concurrency: the live-SUT
	// execution mode a networked port must run under.
	v, reason, err := interpreter.RunTestcase([]*ttcn3.Tree{parse(t, src)}, tc)
	if err != nil {
		t.Fatalf("RunTestcase(%s): %v", tc, err)
	}
	return v, reason
}

// decls is the TTCN-3 preamble every test shares: the request/response
// record pair this port speaks.
const decls = `
	type record HttpRequest  { charstring method, charstring path, charstring body }
	type record HttpResponse { integer status, charstring body }
	type enumerated TransportErrorReason {
		refused(0), unreachable(1), timeout(2), dns(3), tls(4), other(5), reset(6)
	}
	type record TransportError { TransportErrorReason reason, charstring detail }
	type port P message { out HttpRequest; in HttpResponse, TransportError }
	type component C { port P p }
`

// TestHTTPPort_JSONRoundTrip is the headline proof: a GET reaches a real
// HTTP server and its JSON body and status come back to p.receive.
func TestHTTPPort_JSONRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/health" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok"}`)
	}))
	defer srv.Close()

	httpport.Register("p", srv.URL)
	t.Cleanup(httpport.Reset)

	v, reason := run(t, "M.tc", `module M {`+decls+`
		testcase tc() runs on C system C {
			timer g := 5.0;
			map(self:p, system:p);
			g.start;
			p.send(HttpRequest:{ method := "GET", path := "/api/v1/health", body := "" });
			alt {
				[] p.receive(HttpResponse:{ status := 200, body := "{\"status\":\"ok\"}" }) { setverdict(pass); }
				[] p.receive { setverdict(fail, "unexpected response"); }
				[] g.timeout { setverdict(fail, "no response"); }
			}
			unmap(self:p, system:p);
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

// TestHTTPPort_PostBodyReachesServer proves the method, path and request
// body are all carried, not just the path.
func TestHTTPPort_PostBodyReachesServer(t *testing.T) {
	var gotMethod, gotPath, gotBody, gotCT atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotMethod.Store(r.Method)
		gotPath.Store(r.URL.Path)
		gotBody.Store(string(b))
		gotCT.Store(r.Header.Get("Content-Type"))
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprint(w, `{"jobId":"j-1"}`)
	}))
	defer srv.Close()

	httpport.Register("p", srv.URL)
	t.Cleanup(httpport.Reset)

	v, reason := run(t, "M.tc", `module M {`+decls+`
		testcase tc() runs on C system C {
			timer g := 5.0;
			map(self:p, system:p);
			g.start;
			p.send(HttpRequest:{ method := "POST", path := "/api/v1/upgrade", body := "{\"component\":\"nic\"}" });
			alt {
				[] p.receive(HttpResponse:{ status := 202, body := ? }) { setverdict(pass); }
				[] p.receive { setverdict(fail, "wrong status"); }
				[] g.timeout { setverdict(fail, "no response"); }
			}
			unmap(self:p, system:p);
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
	if got := gotMethod.Load(); got != "POST" {
		t.Errorf("method = %v, want POST", got)
	}
	if got := gotPath.Load(); got != "/api/v1/upgrade" {
		t.Errorf("path = %v, want /api/v1/upgrade", got)
	}
	if got := gotBody.Load(); got != `{"component":"nic"}` {
		t.Errorf("body = %v, want the JSON payload", got)
	}
	if got := gotCT.Load(); got != "application/json" {
		t.Errorf("content-type = %v, want application/json (the default)", got)
	}
}

// TestHTTPPort_TransportFailureIsVisible covers the honesty contract: when
// the endpoint is down the testcase must SEE it, as a TransportError whose
// reason it can branch on, rather than just time out with no explanation.
func TestHTTPPort_TransportFailureIsVisible(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // nothing is listening there now

	httpport.Register("p", url)
	t.Cleanup(httpport.Reset)

	v, reason := run(t, "M.tc", `module M {`+decls+`
		testcase tc() runs on C system C {
			timer g := 5.0;
			map(self:p, system:p);
			g.start;
			p.send(HttpRequest:{ method := "GET", path := "/api/v1/health", body := "" });
			alt {
				[] p.receive(TransportError:{ reason := refused, detail := ? }) { setverdict(pass); }
				[] p.receive(TransportError:{ reason := ?, detail := ? }) { setverdict(fail, "wrong reason"); }
				[] p.receive { setverdict(fail, "expected a TransportError"); }
				[] g.timeout { setverdict(fail, "failure was silent: nothing at all"); }
			}
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (a dead endpoint must surface as TransportError refused)", v, reason)
	}
}

// TestHTTPPort_ErrorStatusIsDelivered proves a 4xx/5xx is a normal response
// the testcase can assert on, not an error.
func TestHTTPPort_ErrorStatusIsDelivered(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	httpport.Register("p", srv.URL)
	t.Cleanup(httpport.Reset)

	v, reason := run(t, "M.tc", `module M {`+decls+`
		testcase tc() runs on C system C {
			timer g := 5.0;
			map(self:p, system:p);
			g.start;
			p.send(HttpRequest:{ method := "GET", path := "/api/v1/inventory", body := "" });
			alt {
				[] p.receive(HttpResponse:{ status := 503, body := ? }) { setverdict(pass); }
				[] p.receive { setverdict(fail, "wrong status"); }
				[] g.timeout { setverdict(fail, "no response"); }
			}
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

// TestHTTPPort_DefaultsMethodToGet proves `method` is optional.
func TestHTTPPort_DefaultsMethodToGet(t *testing.T) {
	var got atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Store(r.Method)
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	httpport.Register("p", srv.URL)
	t.Cleanup(httpport.Reset)

	v, reason := run(t, "M.tc", `module M {
		type record HttpRequest  { charstring path }
		type record HttpResponse { integer status, charstring body }
		type port P message { out HttpRequest; in HttpResponse }
		type component C { port P p }
		testcase tc() runs on C system C {
			timer g := 5.0;
			map(self:p, system:p);
			g.start;
			p.send(HttpRequest:{ path := "/whatever" });
			alt {
				[] p.receive(HttpResponse:{ status := 200, body := "ok" }) { setverdict(pass); }
				[] g.timeout { setverdict(fail, "no response"); }
			}
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
	if m := got.Load(); m != "GET" {
		t.Fatalf("method = %v, want GET by default", m)
	}
}

// TestHTTPPort_PerComponentBaseURLs proves one port name can address a
// different service instance per component — the shape a per-node
// DaemonSet has, where each node is its own endpoint.
func TestHTTPPort_PerComponentBaseURLs(t *testing.T) {
	newNode := func(tag string) (*httptest.Server, *int64) {
		var hits int64
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt64(&hits, 1)
			b, _ := json.Marshal(map[string]string{"node": tag})
			w.Write(b)
		}))
		return s, &hits
	}
	a, hitsA := newNode("node-a")
	defer a.Close()
	b, hitsB := newNode("node-b")
	defer b.Close()

	httpport.RegisterRules("p", []httpport.Rule{
		{Component: "*", BaseURL: a.URL},
		{Component: "node-b", BaseURL: b.URL},
	})
	t.Cleanup(httpport.Reset)

	// The PTC is named node-b, so its rule must win over the * default.
	v, reason := run(t, "M.tc", `module M {`+decls+`
		function probe(charstring expected) runs on C {
			timer g := 5.0;
			map(self:p, system:p);
			g.start;
			p.send(HttpRequest:{ method := "GET", path := "/who", body := "" });
			alt {
				[] p.receive(HttpResponse:{ status := 200, body := expected }) { setverdict(pass); }
				[] p.receive { setverdict(fail, "reached the wrong node"); }
				[] g.timeout { setverdict(fail, "no response"); }
			}
			unmap(self:p, system:p);
		}
		testcase tc() runs on C system C {
			var C w := C.create("node-b") alive;
			w.start(probe("{\"node\":\"node-b\"}"));
			w.done;
			setverdict(pass);
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
	if atomic.LoadInt64(hitsB) == 0 || atomic.LoadInt64(hitsA) != 0 {
		t.Fatalf("connections: A=%d B=%d, want A=0 B>0 (the node-b rule must win)",
			atomic.LoadInt64(hitsA), atomic.LoadInt64(hitsB))
	}
}

// TestHTTPPort_TransportErrorReasons covers the classification the suite
// branches on: refused (nothing listening — often a restarting pod) must be
// distinguishable from timeout (listening but wedged), because they are
// different findings.
func TestHTTPPort_TransportErrorReasons(t *testing.T) {
	// A listener that accepts and then never answers, so the request times
	// out rather than being refused.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c // hold it open, answer nothing
		}
	}()

	// A closed port, for refused.
	dead, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	deadURL := "http://" + dead.Addr().String()
	dead.Close()

	for _, k := range []struct {
		name, baseURL, wantReason string
		timeout                   time.Duration
	}{
		{"refused", deadURL, "refused", 5 * time.Second},
		{"timeout", "http://" + ln.Addr().String(), "timeout", 150 * time.Millisecond},
		{"dns", "http://no-such-host.invalid", "dns", 5 * time.Second},
	} {
		t.Run(k.name, func(t *testing.T) {
			httpport.Register("p", k.baseURL, httpport.WithTimeout(k.timeout))
			t.Cleanup(httpport.Reset)
			v, reason := run(t, "M.tc", `module M {`+decls+`
				testcase tc() runs on C system C {
					timer g := 10.0;
					map(self:p, system:p);
					g.start;
					p.send(HttpRequest:{ method := "GET", path := "/x", body := "" });
					alt {
						[] p.receive(TransportError:{ reason := `+k.wantReason+`, detail := ? }) { setverdict(pass); }
						[] p.receive(TransportError:{ reason := ?, detail := ? }) { setverdict(fail, "wrong reason"); }
						[] p.receive { setverdict(fail, "expected a TransportError"); }
						[] g.timeout { setverdict(fail, "nothing arrived"); }
					}
				}
			}`)
			if v != runtime.PassVerdict {
				t.Fatalf("%s: verdict = %s (%s), want pass (reason %s)", k.name, v, reason, k.wantReason)
			}
		})
	}
}

// TestHTTPPort_TransportErrorEnumValuesArePinned guards the documented
// contract: the port emits reasons with fixed integer values, and a suite
// declaring them explicitly must match regardless of the order those lines
// appear in. If this breaks, every TransportError template in the field
// silently stops matching.
func TestHTTPPort_TransportErrorEnumValuesArePinned(t *testing.T) {
	dead, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	url := "http://" + dead.Addr().String()
	dead.Close()

	httpport.Register("p", url)
	t.Cleanup(httpport.Reset)

	// Declared in a deliberately shuffled order, with explicit values.
	v, reason := run(t, "M.tc", `module M {
		type record HttpRequest  { charstring method, charstring path, charstring body }
		type record HttpResponse { integer status, charstring body }
		type enumerated TransportErrorReason {
			other(5), timeout(2), refused(0), tls(4), dns(3), unreachable(1)
		}
		type record TransportError { TransportErrorReason reason, charstring detail }
		type port P message { out HttpRequest; in HttpResponse, TransportError }
		type component C { port P p }
		testcase tc() runs on C system C {
			timer g := 5.0;
			map(self:p, system:p);
			g.start;
			p.send(HttpRequest:{ method := "GET", path := "/x", body := "" });
			alt {
				[] p.receive(TransportError:{ reason := refused, detail := ? }) { setverdict(pass); }
				[] p.receive { setverdict(fail, "explicit enum values did not match"); }
				[] g.timeout { setverdict(fail, "nothing arrived"); }
			}
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (explicit enum values must pin the contract)", v, reason)
	}
}

// TestHTTPPort_ResetMidRequest covers the window a service actually spends
// being torn down: the listener ACCEPTS the connection and the process then
// goes away, so the client sees a reset or an orderly EOF rather than a
// refusal. That is retryable like `refused` and must not land in `other` —
// a catch-all that also contains the commonest transient failure is one a
// suite cannot branch on.
func TestHTTPPort_ResetMidRequest(t *testing.T) {
	for _, k := range []struct {
		name  string
		close func(net.Conn)
	}{
		{"rst", func(c net.Conn) {
			if tc, ok := c.(*net.TCPConn); ok {
				tc.SetLinger(0) // force RST rather than FIN
			}
			c.Close()
		}},
		{"eof", func(c net.Conn) { c.Close() }},
	} {
		t.Run(k.name, func(t *testing.T) {
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer ln.Close()
			go func() {
				for {
					c, err := ln.Accept()
					if err != nil {
						return
					}
					// Accept, read a little of the request, then die.
					one := make([]byte, 1)
					_, _ = c.Read(one)
					k.close(c)
				}
			}()

			httpport.Register("p", "http://"+ln.Addr().String())
			t.Cleanup(httpport.Reset)

			v, reason := run(t, "M.tc", `module M {`+decls+`
				testcase tc() runs on C system C {
					timer g := 10.0;
					map(self:p, system:p);
					g.start;
					p.send(HttpRequest:{ method := "GET", path := "/x", body := "" });
					alt {
						[] p.receive(TransportError:{ reason := reset, detail := ? }) { setverdict(pass); }
						[] p.receive(TransportError:{ reason := other, detail := ? }) { setverdict(fail, "mid-restart drop fell into the other catch-all"); }
						[] p.receive(TransportError:{ reason := ?, detail := ? }) { setverdict(fail, "wrong reason"); }
						[] p.receive { setverdict(fail, "expected a TransportError"); }
						[] g.timeout { setverdict(fail, "nothing arrived"); }
					}
				}
			}`)
			if v != runtime.PassVerdict {
				t.Fatalf("%s: verdict = %s (%s), want pass (reason reset)", k.name, v, reason)
			}
		})
	}
}

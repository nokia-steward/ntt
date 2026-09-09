package httpport_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

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
	type port P message { out HttpRequest; in HttpResponse }
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
// the endpoint is down the testcase must SEE it (status 0 plus the reason)
// rather than just time out with no explanation.
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
				[] p.receive(HttpResponse:{ status := 0, body := ? }) { setverdict(pass); }
				[] p.receive { setverdict(fail, "expected a status-0 transport error"); }
				[] g.timeout { setverdict(fail, "failure was silent: no response at all"); }
			}
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (a dead endpoint must surface as status 0)", v, reason)
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

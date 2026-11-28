package httpport_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/runtime/port/httpport"
)

const headerDecls = `
	type record HttpHeader { charstring name, charstring val }
	type record of HttpHeader HttpHeaders;
	type record HttpRequest  { charstring method, charstring path, charstring body, HttpHeaders headers optional }
	type record HttpResponse { integer status, charstring body }
	type record HttpResponseH { integer status, charstring body, HttpHeaders headers }
	type port P message { out HttpRequest; in HttpResponse }
	type port PH message { out HttpRequest; in HttpResponseH }
	type component C { port P p; port PH ph }
`

// TestHTTPPort_RequestHeaders: headers given in the request reach the
// server — an Authorization header, a Content-Type overriding the
// default — and a request without the field still works.
func TestHTTPPort_RequestHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth":
			if r.Header.Get("Authorization") != "Bearer t0k" || r.Header.Values("X-Multi")[1] != "two" ||
				r.Header.Get("Content-Type") != "text/plain" {
				http.Error(w, "headers: "+r.Header.Get("Authorization")+" "+r.Header.Get("Content-Type"), 400)
				return
			}
			w.Write([]byte("ok"))
		case "/plain":
			w.Write([]byte("plain"))
		}
	}))
	defer srv.Close()
	httpport.Register("p", srv.URL)
	t.Cleanup(httpport.Reset)

	v, reason := run(t, "M.tc", `module M {`+headerDecls+`
		testcase tc() runs on C system C {
			timer g := 5.0;
			map(self:p, system:p);
			g.start;
			var HttpResponse r;
			p.send(HttpRequest:{ method := "POST", path := "/auth", body := "x",
				headers := { { name := "Authorization", val := "Bearer t0k" },
				             { "X-Multi", "one" }, { "X-Multi", "two" },
				             { name := "Content-Type", val := "text/plain" } } });
			alt {
				[] p.receive(HttpResponse:{ status := 200, body := "ok" }) {}
				[] p.receive(HttpResponse:?) -> value r { setverdict(fail, r); stop }
				[] g.timeout { setverdict(fail, "no response"); stop }
			}
			p.send(HttpRequest:{ method := "GET", path := "/plain", body := "", headers := omit });
			alt {
				[] p.receive(HttpResponse:{ status := 200, body := "plain" }) { setverdict(pass) }
				[] g.timeout { setverdict(fail, "no response without headers") }
			}
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

// TestHTTPPort_ResponseHeaders: asked for, the response carries its
// headers; not asked for, the response record is the two fields it was.
func TestHTTPPort_ResponseHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "42")
		w.Write([]byte("hi"))
	}))
	defer srv.Close()
	httpport.Register("ph", srv.URL, httpport.WithResponseHeaders())
	httpport.Register("p", srv.URL)
	t.Cleanup(httpport.Reset)

	v, reason := run(t, "M.tc", `module M {`+headerDecls+`
		testcase tc() runs on C system C {
			timer g := 5.0;
			map(self:ph, system:ph);
			map(self:p, system:p);
			g.start;
			ph.send(HttpRequest:{ method := "GET", path := "/", body := "", headers := omit });
			var HttpResponseH rh;
			alt {
				[] ph.receive(HttpResponseH:{ status := 200, body := "hi", headers := ? }) -> value rh {}
				[] g.timeout { setverdict(fail, "no response with headers"); stop }
			}
			var boolean found := false;
			for (var integer i := 0; i < lengthof(rh.headers); i := i + 1) {
				if (rh.headers[i].name == "X-Request-Id" and rh.headers[i].val == "42") { found := true }
			}
			if (not found) { setverdict(fail, "X-Request-Id not among ", rh.headers); stop }
			p.send(HttpRequest:{ method := "GET", path := "/", body := "", headers := omit });
			alt {
				[] p.receive(HttpResponse:{ status := 200, body := "hi" }) { setverdict(pass) }
				[] g.timeout { setverdict(fail, "the plain response changed shape") }
			}
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

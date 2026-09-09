// Package httpport is a built-in HTTP test port: it binds a TTCN-3 message
// port to a live HTTP endpoint with no user C code and no cgo, so a
// testcase can drive a REST service — the common shape for a microservice
// under test — and `ntt exec --profile` can report its per-request latency.
//
//	httpport.Register("MyApi_PT", "http://127.0.0.1:8080")
//	// ... run a testcase whose ports are of type MyApi_PT.
//
// The TTCN-3 side models one request/response exchange as a record pair:
//
//	type record HttpRequest  { charstring method, charstring path, charstring body }
//	type record HttpResponse { integer status, charstring body }
//	type port ApiPort message { out HttpRequest; in HttpResponse }
//
// `map(self:p, system:sp)` binds the base URL, `p.send(req)` issues the
// request, and the response is delivered to `p.receive` as an HttpResponse.
// Bodies stay charstring, so a JSON API composes with whatever types the
// suite already generates from its schema — this port does not need to know
// them.
//
// Request record fields (all charstring; only `path` is required):
//
//	method       "GET" (default), "POST", …
//	path         appended to the base URL, e.g. "/api/v1/health"
//	body         request body; omitted or empty sends no body
//	contentType  defaults to application/json when a body is present
//
// Response record fields, always exactly these two, so a template can match
// on them without knowing about optional fields:
//
//	status       HTTP status code; 0 when the request never completed
//	body         response body, or the transport error when status is 0
//
// A transport failure (connection refused, DNS, timeout) therefore surfaces
// in-script as `status == 0` with the reason in `body`, rather than as a
// silent absence of reply.
//
// It is a thin layer over runtime/port/goport, which installs the global
// port-driver provider and pushes responses back into the running testcase
// via Inject; httpport.Reset restores the previous provider.
//
// Requests are issued on their own goroutine, so the runtime goroutine
// never blocks on I/O. Responses are injected in completion order: with one
// request in flight at a time — the usual request/response pattern — that
// is also request order.
//
// Not modelled: TLS/mTLS (plain http:// only), request or response headers
// beyond content type, and binary (non-UTF-8) bodies.
package httpport

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/runtime/port"
	"github.com/nokia/ntt/runtime/port/api"
	"github.com/nokia/ntt/runtime/port/goport"
)

// defaultTimeout bounds a single request so a hung SUT fails the testcase
// through its own guard timer instead of wedging the run.
const defaultTimeout = 30 * time.Second

// maxBodyLen caps a response body so a runaway endpoint can't make the
// read loop allocate unbounded memory.
const maxBodyLen = 64 << 20 // 64 MiB

// Rule binds one component selector to a base URL. At map time the port
// picks the rule that best matches the mapping component, so different
// components can drive different instances of the same service — one per
// node in a cluster, for example.
type Rule struct {
	// Component selects which components this rule applies to: "*" (any),
	// "mtc" (the main test component), a component name (from
	// MyComp.create("name")), or a component type name.
	Component string
	BaseURL   string        // e.g. "http://10.0.0.7:8080"
	Timeout   time.Duration // per-request timeout (defaultTimeout if <= 0)
}

type config struct {
	timeout time.Duration
}

// Option configures a registered HTTP test port.
type Option func(*config)

// WithTimeout overrides the per-request timeout (default 30s).
func WithTimeout(d time.Duration) Option {
	return func(c *config) { c.timeout = d }
}

// Register binds a TTCN-3 message port name to an HTTP test port that
// issues requests against baseURL, for any component. Call once per port
// name at startup; the first Register installs the global port-driver
// provider. Defer Reset in tests to restore the previous provider.
func Register(portName, baseURL string, opts ...Option) {
	cfg := config{timeout: defaultTimeout}
	for _, o := range opts {
		o(&cfg)
	}
	RegisterRules(portName, []Rule{{Component: "*", BaseURL: baseURL, Timeout: cfg.timeout}})
}

// RegisterRules binds a port name to component-selective base URLs. The
// rule whose Component best matches the mapping component wins, by
// precedence: its name, then "mtc" for the MTC, then its component type,
// then "*".
func RegisterRules(portName string, rules []Rule) {
	norm := make([]Rule, len(rules))
	for i, r := range rules {
		if r.Timeout <= 0 {
			r.Timeout = defaultTimeout
		}
		r.BaseURL = strings.TrimRight(r.BaseURL, "/")
		norm[i] = r
	}
	goport.Register(portName, func(inst string) api.TestPort {
		return &httpPort{Base: api.Base{PortName: "http:" + inst}, inst: inst, rules: norm}
	})
}

// Reset clears every registration and restores the previous port-driver
// provider. Intended for tests.
func Reset() { goport.Reset() }

// httpPort is a single HTTP-backed test-port instance. The runtime calls
// OnMap/Send from the port's owning goroutine (one at a time); each request
// runs on its own goroutine and hands the response back through
// goport.Inject, the sanctioned cross-goroutine path.
type httpPort struct {
	api.Base
	inst  string
	rules []Rule

	mu      sync.Mutex
	baseURL string
	client  *http.Client
	mapped  bool
	wg      sync.WaitGroup // in-flight requests, joined on unmap
}

// resolveRule picks the rule matching the component doing the map, in
// precedence order: the component's name, "mtc" (for the MTC), its
// component type, then "*". OnMap runs on the mapping component's
// goroutine, so CurrentComponent identifies it.
func (p *httpPort) resolveRule() (Rule, bool) {
	var name, typeName string
	isMTC := true // a nil CurrentComponent is the MTC's main body
	if exec := runtime.CurrentExec(); exec != nil {
		if c := exec.CurrentComponent(); c != nil {
			name, typeName = c.Name, c.TypeName
			isMTC = c.ID == exec.MTCID()
		}
	}
	find := func(sel string) (Rule, bool) {
		if sel == "" {
			return Rule{}, false
		}
		for _, r := range p.rules {
			if r.Component == sel {
				return r, true
			}
		}
		return Rule{}, false
	}
	if r, ok := find(name); ok {
		return r, true
	}
	if isMTC {
		if r, ok := find("mtc"); ok {
			return r, true
		}
	}
	if r, ok := find(typeName); ok {
		return r, true
	}
	return find("*")
}

// OnMap binds the base URL for the mapping component. Unlike a
// connection-oriented port there is nothing to dial: HTTP connects per
// request, and probing here would need a health path this port cannot
// know. A misconfigured or unreachable endpoint therefore surfaces on the
// first send, as a status 0 response carrying the transport error.
func (p *httpPort) OnMap(context.Context) error {
	rule, ok := p.resolveRule()
	if !ok {
		return fmt.Errorf("httpport %s: no base-URL rule matches the mapping component", p.inst)
	}
	if rule.BaseURL == "" {
		return fmt.Errorf("httpport %s: empty base URL", p.inst)
	}
	p.mu.Lock()
	p.baseURL = rule.BaseURL
	p.client = &http.Client{Timeout: rule.Timeout}
	p.mapped = true
	p.mu.Unlock()
	return nil
}

// OnUnmap waits for in-flight requests so no response is injected into a
// torn-down testcase, then unbinds.
func (p *httpPort) OnUnmap(context.Context) error { return p.shutdown() }

// OnStop mirrors OnUnmap for `p.stop`.
func (p *httpPort) OnStop(context.Context) error { return p.shutdown() }

func (p *httpPort) shutdown() error {
	p.mu.Lock()
	if !p.mapped {
		p.mu.Unlock()
		return nil
	}
	p.mapped = false
	client := p.client
	p.mu.Unlock()

	p.wg.Wait() // in-flight requests finish (each bounded by the timeout)
	if client != nil {
		client.CloseIdleConnections()
	}
	return nil
}

// Send issues one request described by the payload record and injects the
// response. It returns once the request has been handed to its goroutine,
// so the runtime goroutine never blocks on I/O.
func (p *httpPort) Send(_ context.Context, env *port.Envelope) error {
	req, err := decodeRequest(env.Payload)
	if err != nil {
		return err
	}
	p.mu.Lock()
	base, client, mapped := p.baseURL, p.client, p.mapped
	p.mu.Unlock()
	if !mapped {
		return fmt.Errorf("httpport %s: send before map", p.inst)
	}

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		status, body := p.do(client, base, req)
		goport.Inject(p.inst, newResponse(status, body))
	}()
	return nil
}

// do performs the request and returns (status, body). A transport failure
// yields (0, reason) so the testcase can see why nothing arrived.
func (p *httpPort) do(client *http.Client, base string, r request) (int, string) {
	var bodyReader io.Reader
	if r.body != "" {
		bodyReader = bytes.NewReader([]byte(r.body))
	}
	url := base + r.path
	req, err := http.NewRequest(r.method, url, bodyReader)
	if err != nil {
		return 0, fmt.Sprintf("build request %s %s: %v", r.method, url, err)
	}
	if r.body != "" {
		ct := r.contentType
		if ct == "" {
			ct = "application/json"
		}
		req.Header.Set("Content-Type", ct)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Sprintf("%s %s: %v", r.method, url, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyLen))
	if err != nil {
		return resp.StatusCode, fmt.Sprintf("read body: %v", err)
	}
	return resp.StatusCode, string(b)
}

// request is the decoded form of an outgoing HttpRequest record.
type request struct {
	method, path, body, contentType string
}

// decodeRequest reads the TTCN-3 request record. Only `path` is required;
// `method` defaults to GET.
func decodeRequest(payload interface{}) (request, error) {
	obj, ok := payload.(runtime.Object)
	if !ok {
		return request{}, fmt.Errorf("httpport: non-runtime payload %T", payload)
	}
	rec, ok := obj.(*runtime.Record)
	if !ok {
		return request{}, fmt.Errorf("httpport: payload is %s, want a record with method/path/body fields", obj.Type())
	}
	r := request{
		method:      str(rec, "method"),
		path:        str(rec, "path"),
		body:        str(rec, "body"),
		contentType: str(rec, "contentType"),
	}
	if r.method == "" {
		r.method = http.MethodGet
	}
	r.method = strings.ToUpper(r.method)
	if r.path == "" {
		return request{}, fmt.Errorf("httpport: request record has no `path` field")
	}
	if !strings.HasPrefix(r.path, "/") {
		r.path = "/" + r.path
	}
	return r, nil
}

// str reads a charstring field, returning "" when absent or not a string.
func str(rec *runtime.Record, name string) string {
	v, ok := rec.Fields[name]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(*runtime.String); ok {
		return string(s.Value)
	}
	return ""
}

// newResponse builds the TTCN-3 HttpResponse record. It carries exactly
// `status` and `body`, so a receive template can match on them without
// having to account for fields it does not declare.
func newResponse(status int, body string) *runtime.Record {
	rec := runtime.NewRecord()
	rec.Fields["status"] = runtime.NewInt(status)
	rec.Fields["body"] = runtime.NewCharstring(body)
	return rec
}

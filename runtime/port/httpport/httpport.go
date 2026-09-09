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
//	status       HTTP status code
//	body         response body
//
// A request that never completed is a different KIND of event from a server
// answering — the SUT was not there, rather than saying no — so it arrives
// as a second inbound type rather than as a sentinel status:
//
//	type enumerated TransportErrorReason {
//	    refused(0), unreachable(1), timeout(2), dns(3), tls(4), other(5),
//	    reset(6)
//	}
//	type record TransportError { TransportErrorReason reason, charstring detail }
//	type port ApiPort message { out HttpRequest; in HttpResponse, TransportError }
//
// `reason` is machine-matchable, which is the point: `refused` (nothing
// listening) and `reset` (accepted, then dropped mid-request — a service
// being torn down) are both retryable, while `timeout` (listening but
// wedged) is a finding. `detail` carries the underlying message for
// logging; it is diagnostic only, so do not match on it.
//
// Declare the enumeration with the explicit values shown above. A TTCN-3
// enumeration's integer values come from declaration order unless written
// down, and matching compares the value as well as the label, so pinning
// them keeps a later reordering of those lines from silently breaking every
// TransportError template.
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
// An `https://` base URL enables TLS. By default the server is verified
// against the system roots; supply a CA bundle, a client certificate for
// mutual TLS, or an SNI/name override through the TLS settings (see Rule
// and WithTLS). Certificate files are read at map time, so a bad path fails
// the map operation and names the file.
//
// Not modelled: request or response headers beyond content type, and binary
// (non-UTF-8) bodies.
package httpport

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"syscall"
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

// warnInsecureOnce keeps the skip-verify warning to one line per run.
var warnInsecureOnce sync.Once

// Transport-failure reasons, in the canonical order that fixes their
// integer values. A suite must declare the matching TTCN-3 enumeration with
// these explicit values (see the package comment): enum matching compares
// the integer as well as the label.
// New reasons are APPENDED, never inserted: the integers are part of the
// contract with every suite that declared the enumeration, so renumbering
// an existing label would silently break their templates.
const (
	reasonRefused     = "refused"     // 0 — nothing listening
	reasonUnreachable = "unreachable" // 1 — no route to host / network down
	reasonTimeout     = "timeout"     // 2 — no answer within the deadline
	reasonDNS         = "dns"         // 3 — name did not resolve
	reasonTLS         = "tls"         // 4 — handshake or verification failed
	reasonOther       = "other"       // 5 — unclassified
	reasonReset       = "reset"       // 6 — accepted, then dropped mid-request
)

// transportErrorReasons numbers the reasons 0..6 in the order above.
var transportErrorReasons = runtime.NewEnumType("TransportErrorReason",
	reasonRefused, reasonUnreachable, reasonTimeout, reasonDNS, reasonTLS,
	reasonOther, reasonReset)

// TLS describes the client-side TLS settings for an `https://` base URL.
// The zero value verifies the server against the system roots, which is
// what a service with a publicly-rooted or cluster-CA certificate needs.
// Paths are read at map time, so a bad path surfaces as a map error on the
// testcase rather than at registration.
type TLS struct {
	// CACert is a PEM bundle used to verify the server. Empty means the
	// system roots.
	CACert string
	// ClientCert and ClientKey are the PEM client certificate and key for
	// mutual TLS. Both must be set, or neither.
	ClientCert string
	ClientKey  string
	// ServerName overrides the name checked against the server's
	// certificate (and sent as SNI). Useful when connecting by IP.
	ServerName string
	// Insecure disables server-certificate verification. TEST ENVIRONMENTS
	// ONLY: it removes the guarantee that you are talking to the intended
	// service, so a suite using it cannot make a security claim.
	Insecure bool
}

// Rule binds one component selector to a base URL. At map time the port
// picks the rule that best matches the mapping component, so different
// components can drive different instances of the same service — one per
// node in a cluster, for example.
type Rule struct {
	// Component selects which components this rule applies to: "*" (any),
	// "mtc" (the main test component), a component name (from
	// MyComp.create("name")), or a component type name.
	Component string
	BaseURL   string        // e.g. "http://10.0.0.7:8080" or "https://…"
	Timeout   time.Duration // per-request timeout (defaultTimeout if <= 0)
	// TLS configures an https base URL. Nil verifies against the system
	// roots; it is ignored for a plain http:// URL.
	TLS *TLS
}

type config struct {
	timeout time.Duration
	tls     *TLS
}

// Option configures a registered HTTP test port.
type Option func(*config)

// WithTimeout overrides the per-request timeout (default 30s).
func WithTimeout(d time.Duration) Option {
	return func(c *config) { c.timeout = d }
}

// WithTLS sets the client TLS settings used for an https base URL.
func WithTLS(t TLS) Option {
	return func(c *config) { c.tls = &t }
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
	RegisterRules(portName, []Rule{{Component: "*", BaseURL: baseURL, Timeout: cfg.timeout, TLS: cfg.tls}})
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
	client := &http.Client{Timeout: rule.Timeout}
	// TLS applies to an https base URL. Building the config here (rather
	// than at registration) means a missing or malformed cert file fails
	// the map operation, so the testcase reports an error verdict naming
	// the file instead of failing later for an unrelated-looking reason.
	if strings.HasPrefix(strings.ToLower(rule.BaseURL), "https://") {
		cfg, err := buildTLSConfig(rule.TLS)
		if err != nil {
			return fmt.Errorf("httpport %s: %w", p.inst, err)
		}
		client.Transport = &http.Transport{TLSClientConfig: cfg}
	}
	p.mu.Lock()
	p.baseURL = rule.BaseURL
	p.client = client
	p.mapped = true
	p.mu.Unlock()
	return nil
}

// buildTLSConfig turns the declarative TLS settings into a tls.Config,
// reading any certificate files from disk. A nil t verifies against the
// system roots.
func buildTLSConfig(t *TLS) (*tls.Config, error) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if t == nil {
		return cfg, nil
	}
	cfg.ServerName = t.ServerName
	cfg.InsecureSkipVerify = t.Insecure
	if t.Insecure {
		// Say so, once, on stderr. Skipping verification is a legitimate
		// convenience against a self-signed test endpoint, but a run that
		// did it cannot support a security claim — and that is easy to
		// forget when the setting lives in a config file nobody re-reads.
		warnInsecureOnce.Do(func() {
			fmt.Fprintln(os.Stderr,
				"httpport: TLS certificate verification is DISABLED (insecure_skip_verify); "+
					"the identity of the server is not checked")
		})
	}
	if t.CACert != "" {
		pem, err := os.ReadFile(t.CACert)
		if err != nil {
			return nil, fmt.Errorf("reading ca_cert: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("ca_cert %s: no certificates found", t.CACert)
		}
		cfg.RootCAs = pool
	}
	switch {
	case t.ClientCert != "" && t.ClientKey != "":
		pair, err := tls.LoadX509KeyPair(t.ClientCert, t.ClientKey)
		if err != nil {
			return nil, fmt.Errorf("loading client certificate: %w", err)
		}
		cfg.Certificates = []tls.Certificate{pair}
	case t.ClientCert != "" || t.ClientKey != "":
		return nil, fmt.Errorf("mutual TLS needs both client_cert and client_key")
	}
	return cfg, nil
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
		status, body, failure := p.do(client, base, req)
		if failure != nil {
			goport.Inject(p.inst, newTransportError(failure))
			return
		}
		goport.Inject(p.inst, newResponse(status, body))
	}()
	return nil
}

// transportFailure is a request that never produced an HTTP response.
type transportFailure struct {
	reason string // one of the reason* constants
	detail string // underlying message, for logging only
}

// do performs the request. It returns either (status, body, nil) for a
// server response — including 4xx and 5xx, which are answers — or a
// non-nil transportFailure when no response was obtained.
func (p *httpPort) do(client *http.Client, base string, r request) (int, string, *transportFailure) {
	var bodyReader io.Reader
	if r.body != "" {
		bodyReader = bytes.NewReader([]byte(r.body))
	}
	url := base + r.path
	req, err := http.NewRequest(r.method, url, bodyReader)
	if err != nil {
		return 0, "", &transportFailure{reasonOther, fmt.Sprintf("build request %s %s: %v", r.method, url, err)}
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
		return 0, "", &transportFailure{classify(err), fmt.Sprintf("%s %s: %v", r.method, url, err)}
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyLen))
	if err != nil {
		// Headers arrived, so the SUT did answer; report the status and
		// surface the truncated read in the body rather than pretending
		// the request never happened.
		return resp.StatusCode, fmt.Sprintf("read body: %v", err), nil
	}
	return resp.StatusCode, string(b), nil
}

// classify maps a transport error onto one of the reason labels. The
// specific causes are checked before the general ones: a certificate
// rejection and a refused connection are both "the request failed", but
// they send an engineer to different places.
func classify(err error) string {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return reasonDNS
	}
	// Certificate / handshake problems: the transport worked, trust did not.
	var (
		unknownAuthority x509.UnknownAuthorityError
		hostnameErr      x509.HostnameError
		certInvalid      x509.CertificateInvalidError
		recordHeaderErr  tls.RecordHeaderError
	)
	if errors.As(err, &unknownAuthority) || errors.As(err, &hostnameErr) ||
		errors.As(err, &certInvalid) || errors.As(err, &recordHeaderErr) {
		return reasonTLS
	}
	if msg := err.Error(); strings.Contains(msg, "tls:") || strings.Contains(msg, "x509:") {
		return reasonTLS
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return reasonTimeout
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return reasonTimeout
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return reasonRefused
	}
	// Accepted, then dropped before answering: the peer WAS there and went
	// away mid-request — a pod being torn down, typically. Retryable like
	// refused, and distinct from it (something was listening) and from
	// timeout (it is gone, not slow). A reset and a graceful EOF differ
	// only in politeness; from a suite's side they are the same event, so
	// they share one reason rather than splitting a distinction no branch
	// would act on.
	if errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return reasonReset
	}
	if errors.Is(err, syscall.EHOSTUNREACH) || errors.Is(err, syscall.ENETUNREACH) {
		return reasonUnreachable
	}
	// Fall back to the message for platforms whose errors don't unwrap to
	// the syscall constants above.
	switch msg := strings.ToLower(err.Error()); {
	case strings.Contains(msg, "connection refused"):
		return reasonRefused
	case strings.Contains(msg, "no route to host"), strings.Contains(msg, "network is unreachable"):
		return reasonUnreachable
	case strings.Contains(msg, "timeout"), strings.Contains(msg, "deadline exceeded"):
		return reasonTimeout
	case strings.Contains(msg, "connection reset"), strings.Contains(msg, "unexpected eof"),
		strings.Contains(msg, "server closed idle connection"):
		return reasonReset
	}
	return reasonOther
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

// newTransportError builds the TTCN-3 TransportError record: a matchable
// `reason` enumeration plus the underlying message in `detail`.
func newTransportError(f *transportFailure) *runtime.Record {
	rec := runtime.NewRecord()
	ev, err := runtime.NewEnumValueByKey(transportErrorReasons, f.reason)
	if err != nil {
		// Unreachable for the constants above; degrade to `other` rather
		// than dropping the message and looking like a lost reply.
		ev, _ = runtime.NewEnumValueByKey(transportErrorReasons, reasonOther)
	}
	rec.Fields["reason"] = ev
	rec.Fields["detail"] = runtime.NewCharstring(f.detail)
	return rec
}

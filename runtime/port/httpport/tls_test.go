package httpport_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/runtime/port/httpport"
)

// writePEM writes b as a PEM file of the given type and returns its path.
func writePEM(t *testing.T, dir, name, typ string, b []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: b}), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// newClientCert mints a self-signed client certificate and returns its
// cert path, key path and the parsed cert (for the server's ClientCAs).
func newClientCert(t *testing.T, dir string) (certPath, keyPath string, cert *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "ntt-test-client"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return writePEM(t, dir, "client.crt", "CERTIFICATE", der),
		writePEM(t, dir, "client.key", "EC PRIVATE KEY", keyDER),
		parsed
}

// caFileFor writes the TLS server's own certificate out as a CA bundle.
func caFileFor(t *testing.T, dir string, srv *httptest.Server) string {
	t.Helper()
	return writePEM(t, dir, "ca.crt", "CERTIFICATE", srv.Certificate().Raw)
}

const tlsProbe = `
	timer g := 5.0;
	map(self:p, system:p);
	g.start;
	p.send(HttpRequest:{ method := "GET", path := "/secure", body := "" });
`

// TestHTTPPort_TLSVerifiesWithCACert is the headline TLS case: an https
// endpoint whose certificate is verified against a supplied CA bundle.
func TestHTTPPort_TLSVerifiesWithCACert(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"secure":true}`)
	}))
	defer srv.Close()
	dir := t.TempDir()

	httpport.Register("p", srv.URL, httpport.WithTLS(httpport.TLS{
		CACert:     caFileFor(t, dir, srv),
		ServerName: "example.com", // httptest's cert is issued for example.com
	}))
	t.Cleanup(httpport.Reset)

	v, reason := run(t, "M.tc", `module M {`+decls+`
		testcase tc() runs on C system C {`+tlsProbe+`
			alt {
				[] p.receive(HttpResponse:{ status := 200, body := "{\"secure\":true}" }) { setverdict(pass); }
				[] p.receive(HttpResponse:{ status := 0, body := ? }) { setverdict(fail, "TLS handshake failed"); }
				[] g.timeout { setverdict(fail, "no response"); }
			}
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

// TestHTTPPort_TLSUntrustedIsRejected proves verification is real: without
// the CA the handshake must fail, and fail VISIBLY (status 0 + reason),
// not silently succeed.
func TestHTTPPort_TLSUntrustedIsRejected(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "should not be reached")
	}))
	defer srv.Close()

	httpport.Register("p", srv.URL) // no CA: system roots don't know this cert
	t.Cleanup(httpport.Reset)

	v, reason := run(t, "M.tc", `module M {`+decls+`
		testcase tc() runs on C system C {`+tlsProbe+`
			alt {
				[] p.receive(HttpResponse:{ status := 0, body := ? }) { setverdict(pass); }
				[] p.receive { setverdict(fail, "an untrusted certificate was accepted"); }
				[] g.timeout { setverdict(fail, "failure was silent"); }
			}
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (an untrusted cert must be refused, visibly)", v, reason)
	}
}

// TestHTTPPort_TLSInsecureSkipVerify covers the documented test-only escape
// hatch for self-signed endpoints.
func TestHTTPPort_TLSInsecureSkipVerify(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	httpport.Register("p", srv.URL, httpport.WithTLS(httpport.TLS{Insecure: true}))
	t.Cleanup(httpport.Reset)

	v, reason := run(t, "M.tc", `module M {`+decls+`
		testcase tc() runs on C system C {`+tlsProbe+`
			alt {
				[] p.receive(HttpResponse:{ status := 200, body := "ok" }) { setverdict(pass); }
				[] p.receive { setverdict(fail, "unexpected response"); }
				[] g.timeout { setverdict(fail, "no response"); }
			}
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

// TestHTTPPort_MutualTLS is the case a Kubernetes API server needs: the
// server demands a client certificate and verifies it.
func TestHTTPPort_MutualTLS(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, clientCA := newClientCert(t, dir)

	pool := x509.NewCertPool()
	pool.AddCert(clientCA)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.TLS.PeerCertificates) == 0 {
			http.Error(w, "no client cert", http.StatusUnauthorized)
			return
		}
		fmt.Fprintf(w, `{"cn":%q}`, r.TLS.PeerCertificates[0].Subject.CommonName)
	}))
	srv.TLS = &tls.Config{ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: pool}
	srv.StartTLS()
	defer srv.Close()

	httpport.Register("p", srv.URL, httpport.WithTLS(httpport.TLS{
		CACert:     caFileFor(t, dir, srv),
		ServerName: "example.com",
		ClientCert: certPath,
		ClientKey:  keyPath,
	}))
	t.Cleanup(httpport.Reset)

	v, reason := run(t, "M.tc", `module M {`+decls+`
		testcase tc() runs on C system C {`+tlsProbe+`
			alt {
				[] p.receive(HttpResponse:{ status := 200, body := "{\"cn\":\"ntt-test-client\"}" }) { setverdict(pass); }
				[] p.receive(HttpResponse:{ status := 0, body := ? }) { setverdict(fail, "mTLS handshake failed"); }
				[] p.receive { setverdict(fail, "unexpected response"); }
				[] g.timeout { setverdict(fail, "no response"); }
			}
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (client certificate must be presented)", v, reason)
	}
}

// TestHTTPPort_TLSBadCertPathFailsAtMap proves a misconfigured cert path is
// reported against the map operation, naming the file — not discovered as a
// mysterious transport error later.
func TestHTTPPort_TLSBadCertPathFailsAtMap(t *testing.T) {
	httpport.Register("p", "https://127.0.0.1:1", httpport.WithTLS(httpport.TLS{
		CACert: filepath.Join(t.TempDir(), "does-not-exist.pem"),
	}))
	t.Cleanup(httpport.Reset)

	v, _ := run(t, "M.tc", `module M {`+decls+`
		testcase tc() runs on C system C {
			map(self:p, system:p);
			setverdict(pass);
		}
	}`)
	if v == runtime.PassVerdict {
		t.Fatal("verdict = pass; a missing ca_cert must fail the map operation")
	}
}

package tcpport_test

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/runtime/port/tcpport"
	"github.com/nokia/ntt/runtime/port/tlsconf"
)

// selfSigned mints a certificate for 127.0.0.1, writes it out as a CA
// bundle, and returns the server's tls.Certificate and the bundle's path.
func selfSigned(t *testing.T) (tls.Certificate, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "ntt-test-server"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, ca
}

// startTLSLineServer serves TLS on 127.0.0.1: for each line it reads it
// answers "echo: <line>", and after the line "bye" it closes the
// connection. It returns the address, the CA bundle that verifies it, and
// a stop function.
func startTLSLineServer(t *testing.T) (addr, ca string, stop func()) {
	t.Helper()
	cert, ca := selfSigned(t)
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer conn.Close()
				r := bufio.NewReader(conn)
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					line = strings.TrimRight(line, "\r\n")
					if _, err := conn.Write([]byte("echo: " + line + "\n")); err != nil {
						return
					}
					if line == "bye" {
						return
					}
				}
			}()
		}
	}()
	return ln.Addr().String(), ca, func() { ln.Close(); wg.Wait() }
}

const tlsSuite = `module M {
	type enumerated DisconnectReason { closed(0), aborted(1) }
	type record Disconnected { DisconnectReason reason, charstring detail }
	type port P message { inout charstring; in Disconnected }
	type component C { port P p }
	testcase tc() runs on C system C {
		timer g := 5.0;
		map(self:p, system:p);
		g.start;
		p.send("hello");
		alt {
			[] p.receive(charstring:"echo: hello") {}
			[] p.receive { setverdict(fail, "unexpected value"); stop }
			[] g.timeout { setverdict(fail, "no answer"); stop }
		}
		p.send("bye");
		alt {
			[] p.receive(charstring:"echo: bye") { repeat }
			[] p.receive(Disconnected:{ reason := closed, detail := ? }) { setverdict(pass) }
			[] p.receive { setverdict(fail, "unexpected value"); stop }
			[] g.timeout { setverdict(fail, "the hang-up was silent") }
		}
	}
}`

// TestTCPPort_TLS: with TLS on, the port is a verified TLS client over
// the same framing, and a hang-up is reported as on plain TCP.
func TestTCPPort_TLS(t *testing.T) {
	addr, ca, stop := startTLSLineServer(t)
	defer stop()
	tcpport.Register("p", addr, tcpport.WithTLS(tlsconf.Config{CACert: ca}), tcpport.WithDisconnectEvent())
	t.Cleanup(tcpport.Reset)
	if v, reason := run(t, "M.tc", tlsSuite); v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

// TestTCPPort_TLSVerifiesTheServer: a server the CA does not vouch for
// fails the map, naming TLS, rather than being talked to.
func TestTCPPort_TLSVerifiesTheServer(t *testing.T) {
	addr, _, stop := startTLSLineServer(t)
	defer stop()
	_, otherCA := selfSigned(t)
	tcpport.Register("p", addr, tcpport.WithTLS(tlsconf.Config{CACert: otherCA}))
	t.Cleanup(tcpport.Reset)
	v, reason := run(t, "M.tc", `module M {
		type port P message { inout charstring }
		type component C { port P p }
		testcase tc() runs on C system C { map(self:p, system:p); setverdict(pass); }
	}`)
	if v != runtime.ErrorVerdict || !strings.Contains(reason, "TLS") {
		t.Fatalf("verdict = %s (%s), want an error naming TLS", v, reason)
	}
}

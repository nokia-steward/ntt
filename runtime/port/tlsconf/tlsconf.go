// Package tlsconf holds the client-side TLS settings the built-in test
// ports share — the HTTP port for an https:// base URL, the TCP port when
// TLS is turned on — so both read the same configuration the same way.
package tlsconf

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"sync"
)

// Config describes the client-side TLS settings of a test port. The zero
// value verifies the server against the system roots, which is what a
// service with a publicly-rooted or cluster-CA certificate needs. Paths are
// read when the port is mapped, so a bad path surfaces as a map error on
// the testcase rather than at registration.
type Config struct {
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

// warnInsecureOnce keeps the skip-verify warning to one line per run.
var warnInsecureOnce sync.Once

// Build turns the declarative settings into a tls.Config, reading any
// certificate files from disk. A nil t verifies against the system roots.
// who names the port in the skip-verify warning.
func Build(t *Config, who string) (*tls.Config, error) {
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
			fmt.Fprintf(os.Stderr,
				"%s: TLS certificate verification is DISABLED (insecure_skip_verify); "+
					"the identity of the server is not checked\n", who)
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

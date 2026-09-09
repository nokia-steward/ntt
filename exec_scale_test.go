package main

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/nokia/ntt/runtime/port/tcpport"
	rreport "github.com/nokia/ntt/runtime/report"
)

// one server per "node": replies its own tag so we can prove each
// component reached ITS endpoint, not a sibling's.
func startNodeServer(t *testing.T, tag string) (addr string, hits *int64, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	hits = new(int64)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			atomic.AddInt64(hits, 1)
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer conn.Close()
				sc := bufio.NewScanner(conn)
				for sc.Scan() {
					if _, err := conn.Write([]byte(tag + "\n")); err != nil {
						return
					}
				}
			}()
		}
	}()
	return ln.Addr().String(), hits, func() { ln.Close(); wg.Wait() }
}

// TestPerComponentAddressesScale proves per-component test-port addresses
// hold at real cluster sizes, not just the two-component case: N components,
// each named and mapped to its OWN endpoint, must each reach exactly that
// endpoint. Asserted server-side (one connection per node) so it does not
// depend on verdict propagation from forked PTCs.
func TestPerComponentAddressesScale(t *testing.T) {
	for _, N := range []int{2, 10, 100} {
		t.Run(fmt.Sprintf("N=%d", N), func(t *testing.T) {
			rules := make([]tcpport.Rule, 0, N)
			hits := make([]*int64, N)
			for i := 0; i < N; i++ {
				tag := fmt.Sprintf("node-%02d", i)
				addr, h, stop := startNodeServer(t, tag)
				defer stop()
				hits[i] = h
				rules = append(rules, tcpport.Rule{Component: tag, Addr: addr})
			}
			tcpport.RegisterRules("p", rules)
			t.Cleanup(tcpport.Reset)

			src := `module m {
				type port P message { inout charstring }
				type component C { port P p }
				function f(charstring expected) runs on C {
					timer g := 10.0;
					map(self:p, system:p);
					g.start;
					p.send("who");
					alt {
						[] p.receive(expected) { setverdict(pass); }
						[] p.receive { setverdict(fail, "reached the WRONG node"); }
						[] g.timeout { setverdict(fail, "no reply"); }
					}
					unmap(self:p, system:p);
				}
				testcase tc() runs on C system C {
`
			for i := 0; i < N; i++ {
				src += fmt.Sprintf("\t\t\t\t\tvar C c%d := C.create(\"node-%02d\") alive; c%d.start(f(\"node-%02d\"));\n", i, i, i, i)
			}
			for i := 0; i < N; i++ {
				src += fmt.Sprintf("\t\t\t\t\tc%d.done;\n", i)
			}
			src += `					setverdict(pass);
				}
			}`
			d := newStaticDriver([]string{writeTC(t, src)})
			d.live = true
			v, reason, err := d.Run(context.Background(), "m.tc")
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if v != rreport.Pass {
				t.Fatalf("verdict=%s reason=%q, want pass", v, reason)
			}
			for i := 0; i < N; i++ {
				if got := atomic.LoadInt64(hits[i]); got != 1 {
					t.Errorf("node-%02d got %d connections, want exactly 1", i, got)
				}
			}
		})
	}
}

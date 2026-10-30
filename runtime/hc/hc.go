// Package hc implements the host controller: it dials the master,
// announces the testcases it can run, and serves Run requests by
// calling the local executor. Hosts are typically one per physical
// machine in a distributed test farm; on a single machine, multiple
// hc instances let teams partition their suite across cores.
package hc

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/nokia/ntt/runtime/exec"
	"github.com/nokia/ntt/runtime/report"
	"github.com/nokia/ntt/runtime/wire"
)

// Host is the runtime state of one host controller process.
type Host struct {
	Name   string
	Driver exec.Driver
}

// New builds a Host wired to the given driver.
func New(name string, d exec.Driver) *Host { return &Host{Name: name, Driver: d} }

// Dial connects to the master at addr, sends Hello, and then serves
// Run requests from the master until the connection closes. Returns
// when the connection closes for any reason.
func (h *Host) Dial(ctx context.Context, addr string) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	enc := wire.NewEncoder(conn)
	dec := wire.NewDecoder(conn)

	if err := enc.EncodeBody(wire.HelloKind, "", wire.Hello{
		Host:    h.Name,
		Cases:   h.Driver.List(),
		Version: "0.1",
	}); err != nil {
		return err
	}
	if _, err := dec.Decode(); err != nil {
		return fmt.Errorf("waiting for Ready: %w", err)
	}

	for {
		msg, err := dec.Decode()
		if err != nil {
			return err
		}
		switch msg.Kind {
		case wire.RunKind:
			var run wire.Run
			if err := msg.UnmarshalBody(&run); err != nil {
				continue
			}
			start := time.Now()
			verdict, reason, err := h.Driver.Run(ctx, run.Case)
			if err != nil && verdict == report.None {
				verdict = report.Error
				if reason == "" {
					reason = err.Error()
				}
			}
			_ = enc.EncodeBody(wire.VerdictKind, msg.ID, wire.Verdict{
				Case:     run.Case,
				Verdict:  verdict.String(),
				Reason:   reason,
				Duration: time.Since(start).Seconds(),
			})
		case wire.StopKind:
			_ = enc.EncodeBody(wire.GoodbyeKind, msg.ID, wire.Goodbye{})
			return nil
		}
	}
}

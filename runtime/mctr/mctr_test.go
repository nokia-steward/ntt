package mctr_test

import (
	"context"
	"testing"
	"time"

	"github.com/nokia/ntt/runtime/exec"
	"github.com/nokia/ntt/runtime/hc"
	"github.com/nokia/ntt/runtime/mctr"
	"github.com/nokia/ntt/runtime/report"
)

type fakeDriver struct {
	cases   []string
	results map[string]report.Verdict
}

func (f fakeDriver) Run(_ context.Context, name string) (report.Verdict, string, error) {
	return f.results[name], "", nil
}
func (f fakeDriver) List() []string { return append([]string{}, f.cases...) }

func TestMasterHostEndToEnd(t *testing.T) {
	m := mctr.NewMaster()
	addr, err := m.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer m.Close()

	host := hc.New("host-1", fakeDriver{
		cases: []string{"M.tc_a", "M.tc_b"},
		results: map[string]report.Verdict{
			"M.tc_a": report.Pass,
			"M.tc_b": report.Fail,
		},
	})

	hostDone := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() { hostDone <- host.Dial(ctx, addr) }()

	if err := m.WaitForHosts(ctx, 1); err != nil {
		t.Fatalf("WaitForHosts: %v", err)
	}

	v := m.Run(ctx, "M.tc_a", time.Second)
	if v != report.Pass {
		t.Errorf("tc_a verdict = %v, want pass", v)
	}
	v = m.Run(ctx, "M.tc_b", time.Second)
	if v != report.Fail {
		t.Errorf("tc_b verdict = %v, want fail", v)
	}
	v = m.Run(ctx, "M.nonexistent", time.Second)
	if v != report.Error {
		t.Errorf("nonexistent verdict = %v, want error", v)
	}
	_ = exec.Selector{} // keep the exec import referenced
}

func TestHostsList(t *testing.T) {
	m := mctr.NewMaster()
	addr, _ := m.Listen("127.0.0.1:0")
	defer m.Close()

	host := hc.New("h", fakeDriver{cases: []string{"X.y"}})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go func() { _ = host.Dial(ctx, addr) }()

	if err := m.WaitForHosts(ctx, 1); err != nil {
		t.Fatalf("WaitForHosts: %v", err)
	}
	if got := len(m.Hosts()); got != 1 {
		t.Errorf("hosts = %d, want 1", got)
	}
}

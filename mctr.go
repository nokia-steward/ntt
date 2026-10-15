package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/nokia/ntt/runtime/exec"
	"github.com/nokia/ntt/runtime/hc"
	"github.com/nokia/ntt/runtime/mctr"
	rreport "github.com/nokia/ntt/runtime/report"
	"github.com/spf13/cobra"
)

var (
	mctrListen  string
	mctrHosts   int
	mctrCases   []string
	mctrTimeout time.Duration

	hcMasterAddr string
	hcName       string

	// MctrCommand is the master controller entry point. It binds an
	// address, waits for the requested number of host controllers,
	// dispatches the configured cases and prints aggregate verdicts.
	MctrCommand = &cobra.Command{
		Use:   "mctr",
		Short: "Master controller for distributed test execution",
		Long: `mctr is the master controller of ntt's distributed executor. It
listens on an address, waits for host controllers (ntt hc) to register,
then dispatches testcases according to --cases. The result is an
aggregate verdict printed to stdout.

For a single-host smoke test, run:

    ntt mctr --listen :7777 --hosts 1 --cases M.tc_a,M.tc_b &
    ntt hc   --master 127.0.0.1:7777 --name h1 testdata/...
`,
		RunE: runMctr,
	}

	// HcCommand is the host controller entry point. It connects to the
	// master and serves whatever testcases the local driver advertises.
	HcCommand = &cobra.Command{
		Use:   "hc [path...]",
		Short: "Host controller for distributed test execution",
		RunE:  runHc,
	}
)

func init() {
	RootCommand.AddCommand(MctrCommand)
	RootCommand.AddCommand(HcCommand)

	MctrCommand.Flags().StringVar(&mctrListen, "listen", ":7777", "address to listen on")
	MctrCommand.Flags().IntVar(&mctrHosts, "hosts", 1, "number of host controllers to wait for")
	MctrCommand.Flags().StringSliceVar(&mctrCases, "cases", nil, "comma-separated list of cases to run")
	MctrCommand.Flags().DurationVar(&mctrTimeout, "timeout", 30*time.Second, "per-case timeout")

	HcCommand.Flags().StringVar(&hcMasterAddr, "master", "127.0.0.1:7777", "master address")
	HcCommand.Flags().StringVar(&hcName, "name", "hc-1", "host name to advertise")
}

func runMctr(cmd *cobra.Command, args []string) error {
	m := mctr.NewMaster()
	addr, err := m.Listen(mctrListen)
	if err != nil {
		return err
	}
	defer m.Close()
	fmt.Fprintf(os.Stderr, "mctr: listening on %s\n", addr)

	ctx, cancel := signalContext()
	defer cancel()

	if err := m.WaitForHosts(ctx, mctrHosts); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "mctr: %d host(s) connected\n", mctrHosts)

	suite := &rreport.Suite{Name: "distributed", Start: time.Now()}
	for _, c := range mctrCases {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		start := time.Now()
		verdict := m.Run(ctx, c, mctrTimeout)
		suite.Cases = append(suite.Cases, rreport.Case{
			Module:   moduleOf(c),
			Name:     localName(c),
			Verdict:  verdict,
			Duration: time.Since(start),
		})
	}
	suite.End = time.Now()

	fmt.Printf("suite verdict: %s\n", suite.Verdict())
	for _, c := range suite.Cases {
		fmt.Printf("  %-7s %s.%s\n", c.Verdict, c.Module, c.Name)
	}
	return nil
}

func runHc(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		args = []string{"."}
	}
	files := collectTTCN3Files(args)
	driver := newStaticDriver(files)

	host := hc.New(hcName, driver)
	ctx, cancel := signalContext()
	defer cancel()

	fmt.Fprintf(os.Stderr, "hc: connecting to %s as %s with %d cases\n",
		hcMasterAddr, hcName, len(driver.List()))
	if err := host.Dial(ctx, hcMasterAddr); err != nil {
		// Treat EOF as a clean disconnect after Stop, not a failure.
		if err.Error() == "EOF" {
			return nil
		}
		return err
	}
	return nil
}

func moduleOf(qn string) string {
	for i, c := range qn {
		if c == '.' {
			return qn[:i]
		}
	}
	return ""
}

func localName(qn string) string {
	for i := len(qn) - 1; i >= 0; i-- {
		if qn[i] == '.' {
			return qn[i+1:]
		}
	}
	return qn
}

func signalContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		cancel()
	}()
	return ctx, cancel
}

// Keep an exec import referenced so future iterations have a clean
// hook for distributing testcases via the executor instead of the
// static driver.
var _ exec.Driver = (*staticDriver)(nil)

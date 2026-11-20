package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	rreport "github.com/nokia/ntt/runtime/report"
	"github.com/nokia/ntt/runtime/tl"
	"github.com/spf13/cobra"
)

var (
	// LogCommand groups the commands that read TCI-TL test logs, the logs
	// `ntt exec --log` writes.
	LogCommand = &cobra.Command{
		Use:   "log",
		Short: "Work with TCI-TL test logs (ntt exec --log)",
	}

	logDiffCommand = &cobra.Command{
		Use:   "diff A B",
		Short: "Compare what each component did in two runs",
		Long: `diff compares two TCI-TL test logs testcase by testcase, and within a
testcase component by component, and reports where each component's actions
first differ. It exits 0 when the runs agree, 1 when they differ, and 2 when a
log cannot be read.

Its first use is checking that a suite behaves the same on both clocks:

	ntt exec --log virtual.jsonl suite/
	ntt exec --live --log live.jsonl suite/
	ntt log diff virtual.jsonl live.jsonl

Components legitimately interleave differently from run to run, and differ in
when a message arrived relative to what a component was doing. So events that
record arrivals rather than actions — a message or call arriving in a queue, a
receive failing to match whatever was at the head of its queue, an alt round
that found nothing — are not compared, nor is the elapsed time a timer read
reports. Every send and receive with its value and template, every verdict,
timer, component and port operation and every alt step is, with where in the
test specification it was performed. Components are matched by the name they
were created with, numbered in creation order when several share one, and
named under their creator when a component other than the MTC created them;
their numeric ids are not compared, and a broadcast's destinations are a set.

Either log format may be given, in any combination.`,
		Args: usageExit2(2),
		RunE: runLogDiff,
	}

	logProfileCommand = &cobra.Command{
		Use:   "profile LOG",
		Short: "Report the per-port performance profile of a logged run",
		Long: `profile computes, from a TCI-TL test log, the per-port profile that
ntt exec --profile measures while a run is live, and reports it the same way:
per testcase, each port's sends and receives, receives per second over the
testcase, and the send->receive latency percentiles.

	ntt exec --live --log run.xml suite/
	ntt log profile run.xml

A port's sends and receives are its logged send and receive operations, and
each send answered by a receive on the same port, before the next send, is
one latency sample. Timestamps are those of the log, in microseconds; a log
of a run on the virtual clock gives virtual time. A port of a component other
than the MTC is shown as component:port. Either log format may be given.`,
		Args: usageExit2(1),
		RunE: runLogProfile,
	}
	logProfileFormat string
)

func init() {
	RootCommand.AddCommand(LogCommand)
	LogCommand.AddCommand(logDiffCommand)
	LogCommand.AddCommand(logProfileCommand)
	logProfileCommand.Flags().StringVar(&logProfileFormat, "format", "profile", "output format: profile (a table) or json")
	logDiffCommand.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
		return nil
	})
}

// usageExit2 requires n arguments and, as diff(1) does, exits 2 on a usage
// error, which 1 — the logs differ — must not be mistaken for.
func usageExit2(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != n {
			fmt.Fprintf(os.Stderr, "usage: ntt %s %s\n", cmd.Parent().Name(), cmd.Use)
			os.Exit(2)
		}
		return nil
	}
}

func runLogProfile(cmd *cobra.Command, args []string) error {
	if logProfileFormat != "profile" && logProfileFormat != "json" {
		return fmt.Errorf("unknown format %q: want profile or json", logProfileFormat)
	}
	l, err := readTestLog(args[0])
	if err != nil {
		return err
	}
	if l.Truncated {
		fmt.Fprintf(os.Stderr, "%s: the log ends mid-event, as a killed run's does; profiling its complete events\n", args[0])
	}
	s := profileSuite(strings.TrimSuffix(filepath.Base(args[0]), filepath.Ext(args[0])), l.Profile())
	if logProfileFormat == "json" {
		return rreport.RenderJSON(os.Stdout, s)
	}
	return rreport.RenderProfile(os.Stdout, s)
}

// profileSuite shapes a log's profile as the report ntt exec --profile
// renders.
func profileSuite(name string, tcs []tl.TestcaseProfile) *rreport.Suite {
	s := &rreport.Suite{Name: name}
	for _, tc := range tcs {
		c := rreport.Case{Name: tc.Name, Module: tc.Module, Duration: tc.Duration, Reason: tc.Reason}
		if c.Name == "" {
			c.Name = tc.Testcase
		}
		// A testcase the log has no end for has no verdict: none.
		c.Verdict, _ = rreport.VerdictFromString(tc.Verdict)
		m := &rreport.Metrics{}
		for _, p := range tc.Ports {
			port := p.Port
			if p.Component != "mtc" {
				port = p.Component + ":" + p.Port
			}
			var thr float64
			if tc.Duration > 0 {
				thr = float64(p.Receives) / tc.Duration.Seconds()
			}
			m.Ports = append(m.Ports, rreport.PortMetric{Port: port, Sends: p.Sends, Receives: p.Receives,
				Throughput: thr, Latency: rreport.LatencyStatsFromSamples(p.Latencies)})
		}
		if len(m.Ports) > 0 {
			c.Metrics = m
		}
		s.Cases = append(s.Cases, c)
	}
	return s
}

// runLogDiff exits the way diff(1) does: 0 when the runs agree, 1 when
// they differ (the differences are on stdout), 2 when a log cannot be read.
func runLogDiff(cmd *cobra.Command, args []string) error {
	logs := make([]*tl.Log, 2)
	for i, path := range args {
		l, err := readTestLog(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		if l.Truncated {
			fmt.Fprintf(os.Stderr, "%s: the log ends mid-event, as a killed run's does; comparing its complete events\n", path)
		}
		logs[i] = l
	}
	if !writeLogDiff(os.Stdout, args[0], args[1], tl.Compare(logs[0], logs[1])) {
		os.Exit(1)
	}
	return nil
}

func readTestLog(path string) (*tl.Log, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	l, err := tl.ReadLog(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return l, nil
}

// writeLogDiff reports each testcase's comparison and whether all agreed.
func writeLogDiff(w io.Writer, nameA, nameB string, results []tl.Result) bool {
	same, differ := 0, 0
	outsideDiffers := false
	for _, r := range results {
		if r.Testcase == tl.Outside {
			// Not a testcase: counted apart.
			if !r.Same() {
				outsideDiffers = true
			}
		}
		switch {
		case r.Missing == "a":
			differ++
			fmt.Fprintf(w, "%s: only in %s\n", r.Testcase, nameB)
		case r.Missing == "b":
			differ++
			fmt.Fprintf(w, "%s: only in %s\n", r.Testcase, nameA)
		case r.Same():
			same++
			fmt.Fprintf(w, "%s: same (%d events)\n", r.Testcase, r.Compared)
		default:
			differ++
			fmt.Fprintf(w, "%s: differs\n", r.Testcase)
			for _, d := range r.Differences {
				fmt.Fprintf(w, "  %s, action %d:\n", d.Component, d.Index+1)
				sa, sb := summaryOrNone(d.A), summaryOrNone(d.B)
				if sa == sb {
					// The difference is in something the summary does
					// not show: give what was compared.
					sa, sb = d.CanonA, d.CanonB
				}
				fmt.Fprintf(w, "    %s: %s\n", nameA, sa)
				fmt.Fprintf(w, "    %s: %s\n", nameB, sb)
			}
		}
	}
	for _, r := range results {
		if r.Testcase == tl.Outside {
			if r.Same() {
				same--
			} else {
				differ--
			}
		}
	}
	fmt.Fprintf(w, "%d testcases: %d same, %d differ\n", same+differ, same, differ)
	if outsideDiffers {
		fmt.Fprintf(w, "and events outside any testcase differ\n")
	}
	return differ == 0 && !outsideDiffers
}

func summaryOrNone(n *tl.Node) string {
	if n == nil {
		return "(nothing: the component did no more)"
	}
	return tl.Summary(n)
}

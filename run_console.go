package main

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
	"github.com/nokia/ntt/internal/env"
	"github.com/spf13/pflag"
)

// lineEventIDs are the event ids of the structured line log, its events in
// lower case and its errors in upper case. A logging filter must match one.
var lineEventIDs = strings.Fields(`
	acfg alen allv alrp alwt asen aslv bctr coal cocr codo cofi cokd coki coru
	cosp cost cpen cplv dbg1 dbg2 dbg3 deco decv dtac dtde dten dtlv dump else
	enco fnen fnlv fxen fxlv getv ilen illv matc mpar mprq paon perf pllg plod
	ptck ptcl ptcn ptdi ptds ptha ptmp ptpu ptqu ptre ptrx ptsd ptsp ptst pttr
	ptun rvon sdbg setv tcen tcfi tclv tcst tmrd tmru tmsp tmst tmto tycr tyde
	tyrs uact ulog vach vrsn wait
	ARRV ARTE ARUN BREF CONE CONV DEAD DIRE DIV0 DKEY DOME DTDE FILE FLOW FOEX
	ICST IGRP INOP LENE LIDE LKUP MANY MAPE NAME NOIM NRTE NSPR NULL OMIT OSUF
	PABV PAOV PARE PATE PLEX PLOD RNGE SIZE SNAC SNAP SRUN SYSE TIME TSTP TYPE
	UBLK UNAS UNDF UNOP UTOB VMIS VRSN WAIT WCPA WPAC`)

// lineEventID is the event id of a log line, its second field up to the
// error code or description it may carry: "tcfi", "VMIS".
func lineEventID(line string) string {
	fields := strings.SplitN(line, "|", 3)
	if len(fields) < 2 || len(fields[1]) < 4 {
		return ""
	}
	return fields[1][:4]
}

// isErrorEvent reports whether id is an error's: in upper case.
func isErrorEvent(id string) bool {
	return id != "" && strings.ToUpper(id) == id && strings.ToLower(id) != id
}

// The logging filter selects the events printed on the console; the
// --output file keeps every event. Its rules apply in the order given, on
// top of every event being shown: --activate and --deactivate take an event
// id regular expression, limited to one module as Module:pattern;
// --activate-all and --deactivate-all set all.
type filterOp int

const (
	filterActivate filterOp = iota
	filterActivateAll
	filterDeactivate
	filterDeactivateAll
)

type filterRule struct {
	op      filterOp
	module  string
	pattern *regexp.Regexp
}

// logFilter is the ordered rules of the logging flags.
type logFilter struct {
	rules []filterRule
}

// logFilterFlags returns the logging flags; their values collect in f.
func logFilterFlags(f *logFilter) *pflag.FlagSet {
	fs := pflag.NewFlagSet("logfilter", pflag.ContinueOnError)
	fs.VarP(&patternFlag{f: f, op: filterActivate}, "activate", "a", "Activate logging matching `REGEXP`")
	all := fs.VarPF(&allFlag{f: f, op: filterActivateAll}, "activate-all", "A", "Activate all logging (default)")
	all.NoOptDefVal = "true"
	fs.VarP(&patternFlag{f: f, op: filterDeactivate}, "deactivate", "d", "Deactivate logging matching `REGEXP`")
	none := fs.VarPF(&allFlag{f: f, op: filterDeactivateAll}, "deactivate-all", "D", "Deactivate all logging")
	none.NoOptDefVal = "true"
	return fs
}

// check rejects a rule of a module the suite does not declare.
func (f *logFilter) check(modules []string) error {
	known := map[string]bool{}
	for _, m := range modules {
		known[m] = true
	}
	for _, r := range f.rules {
		if r.module != "" && !known[r.module] {
			return fmt.Errorf("unknown module %q", r.module)
		}
	}
	return nil
}

// allows reports whether an event of id, of module ("" when unknown), is
// printed. A module's rule overrides the global setting for that module.
func (f *logFilter) allows(id, module string) bool {
	on, moduleSet, moduleOn := true, false, false
	for _, r := range f.rules {
		switch r.op {
		case filterActivateAll:
			on = true
		case filterDeactivateAll:
			on = false
		case filterActivate, filterDeactivate:
			if !r.pattern.MatchString(id) {
				continue
			}
			state := r.op == filterActivate
			if r.module == "" {
				on = state
			} else if r.module == module {
				moduleSet, moduleOn = true, state
			}
		}
	}
	if moduleSet {
		return moduleOn
	}
	return on
}

type patternFlag struct {
	f  *logFilter
	op filterOp
}

func (p *patternFlag) String() string { return "" }
func (p *patternFlag) Type() string   { return "string" }

func (p *patternFlag) Set(s string) error {
	module, pattern, ok := strings.Cut(s, ":")
	if !ok {
		module, pattern = "", s
	}
	if module == "" && ok {
		return fmt.Errorf("missing module id in %q", s)
	}
	if pattern == "" {
		return fmt.Errorf("missing event pattern in %q", s)
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return fmt.Errorf("bad regular expression: %s", pattern)
	}
	matches := false
	for _, id := range lineEventIDs {
		if re.MatchString(id) {
			matches = true
			break
		}
	}
	if !matches {
		return fmt.Errorf("event pattern %q matches no event", pattern)
	}
	p.f.rules = append(p.f.rules, filterRule{op: p.op, module: module, pattern: re})
	return nil
}

type allFlag struct {
	f  *logFilter
	op filterOp
}

func (a *allFlag) String() string   { return "false" }
func (a *allFlag) Type() string     { return "bool" }
func (a *allFlag) IsBoolFlag() bool { return true }

func (a *allFlag) Set(s string) error {
	if s != "true" {
		return fmt.Errorf("invalid value %q", s)
	}
	a.f.rules = append(a.f.rules, filterRule{op: a.op})
	return nil
}

// consoleFormat renders a log line for the console. plain is the line as it
// is; color colors errors bold white on red, a tcfi, cofi or setv by its
// verdict (yellow inconc, green pass, red fail or error) and plod, perf,
// vrsn, vach and bctr faint grey; auto is color on a terminal that allows
// it (not NO_COLOR, NTT_COLORS=never or TERM=dumb), plain otherwise.
type consoleFormat struct {
	color bool
}

func newConsoleFormat(name string, out io.Writer) (consoleFormat, error) {
	switch name {
	case "plain":
		return consoleFormat{}, nil
	case "color":
		return consoleFormat{color: true}, nil
	case "auto", "":
		return consoleFormat{color: colorsWanted(out)}, nil
	}
	return consoleFormat{}, fmt.Errorf("unknown console format %q (auto, plain, or color)", name)
}

func colorsWanted(out io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" || env.Getenv("NTT_COLORS") == "never" {
		return false
	}
	f, ok := out.(*os.File)
	if !ok {
		return false
	}
	fd := f.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

var (
	errorColor  = forcedColor(color.Bold, color.FgWhite, color.BgRed)
	inconcColor = forcedColor(color.FgYellow)
	passColor   = forcedColor(color.FgGreen)
	failColor   = forcedColor(color.FgRed)
	faintColor  = forcedColor(color.Faint, color.FgHiBlack)
)

// forcedColor paints whether or not the process's output is a terminal: the
// console format decides.
func forcedColor(attrs ...color.Attribute) *color.Color {
	c := color.New(attrs...)
	c.EnableColor()
	return c
}

func (f consoleFormat) format(line string) string {
	if !f.color {
		return line
	}
	if c := lineColor(line); c != nil {
		return c.Sprint(line)
	}
	return line
}

func lineColor(line string) *color.Color {
	id := lineEventID(line)
	if isErrorEvent(id) {
		return errorColor
	}
	switch id {
	case "plod", "perf", "vrsn", "vach", "bctr":
		return faintColor
	}
	fields := strings.Split(line, "|")
	field := func(i int) string {
		if i < len(fields) {
			return fields[i]
		}
		return ""
	}
	var verdict string
	switch id {
	case "tcfi", "setv":
		verdict = field(4)
	case "cofi":
		verdict = field(3)
	}
	switch verdict {
	case "inconc":
		return inconcColor
	case "pass":
		return passColor
	case "fail", "error":
		return failColor
	}
	return nil
}

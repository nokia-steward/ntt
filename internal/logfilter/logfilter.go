// Package logfilter defines the console logging filters for k3 run.
//
// Filters are applied in the order they are given, on top of all events being
// enabled. When none of the flags is set, every event is shown, which is what
// --activate-all selects.
//
// --activate and --deactivate take an event-id regular expression. Prefix it
// with a module id to limit the filter to that module:
//
//	--deactivate=UeProcedures:paon|rvon
//	--deactivate=RadParams:f.en|f.lv
//
// A module id that is not in the suite, or an event pattern that matches no
// event id, is an error. The filter decides what is printed on the console.
// It does not drop events from the output file.
package logfilter

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/nokia/ntt/internal/k3log"

	"github.com/spf13/pflag"
)

// Flags returns the logging activation flags.
//
//	-a, --activate=REGEXP   Activate logging matching REGEXP
//	-A, --activate-all      Activate all logging (default)
//	-d, --deactivate=REGEXP Deactivate logging matching REGEXP
//	-D, --deactivate-all    Deactivate all logging
func Flags() *pflag.FlagSet {
	st := &state{}
	fs := pflag.NewFlagSet("logfilter", pflag.ContinueOnError)
	fs.VarP(&patternFlag{state: st, op: activate}, "activate", "a", "Activate logging matching `REGEXP`")
	all := fs.VarPF(&allFlag{state: st, op: activateAll}, "activate-all", "A", "Activate all logging (default)")
	all.NoOptDefVal = "true"
	fs.VarP(&patternFlag{state: st, op: deactivate}, "deactivate", "d", "Deactivate logging matching `REGEXP`")
	none := fs.VarPF(&allFlag{state: st, op: deactivateAll}, "deactivate-all", "D", "Deactivate all logging")
	none.NoOptDefVal = "true"
	return fs
}

// Filter is an ordered list of logging activation rules.
type Filter struct {
	rules []rule
}

// NewWithFlags reads the logging flags from fs. modules are the module ids
// of the suite; a filter that names any other module is rejected. An event
// pattern that matches no event id is rejected while the flag is parsed.
func NewWithFlags(fs *pflag.FlagSet, modules []string) (Filter, error) {
	st := stateFrom(fs)
	if st == nil {
		return Filter{}, fmt.Errorf("logging flags are not registered")
	}
	known := make(map[string]struct{}, len(modules))
	for _, module := range modules {
		known[module] = struct{}{}
	}
	for _, r := range st.rules {
		if r.module == "" {
			continue
		}
		if _, ok := known[r.module]; !ok {
			return Filter{}, fmt.Errorf("unknown module %q", r.module)
		}
	}
	return Filter{rules: append([]rule(nil), st.rules...)}, nil
}

// Allows reports whether an event with the given id is printed on the console.
// module is the module that produced the event, or "" when it is unknown.
// An empty filter allows every event. Otherwise the rules are applied in order
// on top of all events being enabled. A module rule overrides the global
// setting for that module only.
func (f Filter) Allows(id, module string) bool {
	if len(f.rules) == 0 {
		return true
	}
	on := true
	moduleSet := false
	moduleOn := false
	for _, r := range f.rules {
		switch r.op {
		case activateAll:
			on = true
		case deactivateAll:
			on = false
		case activate, deactivate:
			if r.pattern == nil || !r.pattern.MatchString(id) {
				continue
			}
			state := r.op == activate
			if r.module == "" {
				on = state
			} else if r.module == module {
				moduleSet = true
				moduleOn = state
			}
		}
	}
	if moduleSet {
		return moduleOn
	}
	return on
}

// Args returns the filters in the order they were given. It is empty when no
// logging flag was set.
func (f Filter) Args() []string {
	if len(f.rules) == 0 {
		return nil
	}
	args := make([]string, 0, len(f.rules))
	for _, r := range f.rules {
		switch r.op {
		case activateAll:
			args = append(args, "--activate-all")
		case deactivateAll:
			args = append(args, "--deactivate-all")
		case activate:
			args = append(args, "--activate="+r.raw)
		case deactivate:
			args = append(args, "--deactivate="+r.raw)
		}
	}
	return args
}

// Reset drops filters accumulated on fs. cobra keeps flag values between
// parses, and each occurrence of these flags appends a rule.
func Reset(fs *pflag.FlagSet) {
	if st := stateFrom(fs); st != nil {
		st.rules = nil
	}
}

type op int

const (
	activate op = iota
	deactivate
	activateAll
	deactivateAll
)

type rule struct {
	op      op
	module  string
	raw     string
	pattern *regexp.Regexp
}

type state struct {
	rules []rule
}

type patternFlag struct {
	state *state
	op    op
}

func (f *patternFlag) String() string { return "" }
func (f *patternFlag) Type() string   { return "REGEXP" }

func (f *patternFlag) Set(s string) error {
	module, pattern, err := splitPattern(s)
	if err != nil {
		return err
	}
	re, err := checkEvents(pattern)
	if err != nil {
		return err
	}
	f.state.rules = append(f.state.rules, rule{op: f.op, module: module, raw: s, pattern: re})
	return nil
}

type allFlag struct {
	state *state
	op    op
}

func (f *allFlag) String() string   { return "false" }
func (f *allFlag) Type() string     { return "bool" }
func (f *allFlag) IsBoolFlag() bool { return true }

func (f *allFlag) Set(s string) error {
	if s != "true" {
		return fmt.Errorf("invalid value %q", s)
	}
	f.state.rules = append(f.state.rules, rule{op: f.op})
	return nil
}

func stateFrom(fs *pflag.FlagSet) *state {
	if fs == nil {
		return nil
	}
	flag := fs.Lookup("activate")
	if flag == nil {
		return nil
	}
	switch v := flag.Value.(type) {
	case *patternFlag:
		return v.state
	default:
		return nil
	}
}

// splitPattern splits "Module:event-regexp" into the module id and the event
// pattern. A pattern with no colon applies to every module.
func splitPattern(raw string) (module, pattern string, err error) {
	module, pattern, ok := strings.Cut(raw, ":")
	if !ok {
		module, pattern = "", raw
	}
	if module == "" && strings.Contains(raw, ":") {
		return "", "", fmt.Errorf("missing module id in %q", raw)
	}
	if pattern == "" {
		return "", "", fmt.Errorf("missing event pattern in %q", raw)
	}
	return module, pattern, nil
}

// checkEvents compiles pattern and reports whether it matches at least one
// known event id.
func checkEvents(pattern string) (*regexp.Regexp, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("bad regular expression: %s", pattern)
	}
	for id := range k3log.Categories {
		if re.MatchString(id) {
			return re, nil
		}
	}
	return nil, fmt.Errorf("event pattern %q matches no event", pattern)
}

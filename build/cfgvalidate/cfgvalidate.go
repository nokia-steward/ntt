// Package cfgvalidate runs structural checks on a parsed .cfg file
// and surfaces diagnostics for common mistakes that the parser is too
// permissive to flag. Validation is a separate layer so the parser
// stays useful for "best-effort round-trip" workflows.
package cfgvalidate

import (
	"fmt"
	"strings"

	"github.com/nokia/ntt/runtime/cfg"
)

// Issue is one validation finding.
type Issue struct {
	Section string
	Key     string
	Line    int
	Code    string
	Message string
}

// String renders the issue in `file:line:section.key: code: message` form.
func (i Issue) String() string {
	loc := fmt.Sprintf("%d:%s", i.Line, i.Section)
	if i.Key != "" {
		loc += "." + i.Key
	}
	return fmt.Sprintf("%s: %s: %s", loc, i.Code, i.Message)
}

// knownSections lists the section headers the Annex D grammar
// defines. Anything else triggers a warning (vendor extensions are
// allowed; we just want users to notice typos).
var knownSections = map[string]bool{
	"MODULE_PARAMETERS":   true,
	"LOGGING":             true,
	"EXECUTE":             true,
	"TESTPORT_PARAMETERS": true,
	"EXTERNAL_COMMANDS":   true,
	"GROUPS":              true,
	"COMPONENTS":          true,
	"MAIN_CONTROLLER":     true,
	"PROFILER":            true,
	"INCLUDE":             true,
	"DEFINE":              true,
	"ORDERED_INCLUDE":     true,
}

// Validate returns every Issue found in f. The empty slice means the
// configuration looks valid.
func Validate(f *cfg.File) []Issue {
	if f == nil {
		return nil
	}
	var out []Issue
	seenSections := map[string]int{}
	for _, sec := range f.Sections {
		if _, dup := seenSections[sec.Name]; dup {
			out = append(out, Issue{
				Section: sec.Name,
				Code:    "duplicate-section",
				Message: fmt.Sprintf("section %s already appeared earlier", sec.Name),
			})
		}
		seenSections[sec.Name]++
		if !knownSections[sec.Name] {
			out = append(out, Issue{
				Section: sec.Name,
				Code:    "unknown-section",
				Message: fmt.Sprintf("unknown section %s (typo?)", sec.Name),
			})
		}
		out = append(out, validateSection(sec)...)
	}
	return out
}

func validateSection(sec *cfg.Section) []Issue {
	var out []Issue
	keys := map[string]int{}
	for _, st := range sec.Settings {
		if st.Comment {
			continue
		}
		if st.Key != "" {
			if prior, dup := keys[st.Key]; dup {
				out = append(out, Issue{
					Section: sec.Name,
					Key:     st.Key,
					Line:    st.Line,
					Code:    "duplicate-key",
					Message: fmt.Sprintf("key %s already appeared on line %d", st.Key, prior),
				})
			}
			keys[st.Key] = st.Line
		}
		out = append(out, validateSetting(sec.Name, st)...)
	}
	return out
}

func validateSetting(section string, st cfg.Setting) []Issue {
	var out []Issue
	switch section {
	case "EXECUTE":
		text := st.Key
		if text == "" {
			text = strings.TrimSpace(st.Raw)
		}
		if !strings.Contains(text, ".") {
			out = append(out, Issue{
				Section: section,
				Key:     text,
				Line:    st.Line,
				Code:    "execute.missing-qualifier",
				Message: fmt.Sprintf("entry %q is missing the Module.case form", text),
			})
		}
	case "MODULE_PARAMETERS":
		if st.Key == "" {
			out = append(out, Issue{
				Section: section,
				Line:    st.Line,
				Code:    "module-parameters.unkeyed",
				Message: fmt.Sprintf("line %q has no key=value form", st.Raw),
			})
		}
	}
	return out
}

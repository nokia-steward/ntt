// Package cfg implements the TTCN-3 Module Configuration File grammar
// (Annex D of ETSI ES 201 873-1 v4.11.1 and the Titan extensions
// documented in the User Guide). A `.cfg` file groups
// `[SECTION_NAME] key=value` blocks: [MODULE_PARAMETERS], [LOGGING],
// [EXECUTE], [TESTPORT_PARAMETERS], [EXTERNAL_COMMANDS], [GROUPS],
// [COMPONENTS] and [MAIN_CONTROLLER].
//
// The parser is intentionally permissive: it preserves unknown sections
// and unknown lines so a round-trip through Parse + Encode (future)
// loses no information. Diagnostics are returned alongside the parsed
// File so callers can surface line-level errors without aborting on
// the first bad line.
package cfg

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// File is the parsed view of a .cfg file. Sections are stored in
// source order; lookup by name uses Section / Setting helpers.
type File struct {
	Path     string
	Sections []*Section
}

// Section is one `[NAME]` block plus its body lines.
type Section struct {
	Name     string
	Settings []Setting
}

// Setting is a single non-blank, non-comment line inside a section.
// Most TTCN-3 configuration lines are key=value; comments and lines
// without `=` keep the entire content in Raw.
type Setting struct {
	Key     string
	Value   string
	Raw     string
	Comment bool
	Line    int
}

// Diagnostic carries a position-tagged note from the parser.
type Diagnostic struct {
	Message string
	Line    int
}

// Parse parses a .cfg document from r and returns the File plus any
// diagnostics. The parser never returns an error: malformed lines are
// reported via diagnostics so callers get the whole file even when
// individual lines are broken.
func Parse(r io.Reader) (*File, []Diagnostic) {
	f := &File{}
	var diags []Diagnostic
	var current *Section
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	lineno := 0
	for sc.Scan() {
		lineno++
		line := strings.TrimRight(sc.Text(), "\r")
		trim := strings.TrimSpace(line)
		switch {
		case trim == "":
			continue
		case strings.HasPrefix(trim, "#") || strings.HasPrefix(trim, "//"):
			if current == nil {
				continue
			}
			current.Settings = append(current.Settings, Setting{
				Raw:     line,
				Comment: true,
				Line:    lineno,
			})
		case strings.HasPrefix(trim, "[") && strings.HasSuffix(trim, "]"):
			name := strings.TrimSpace(trim[1 : len(trim)-1])
			if name == "" {
				diags = append(diags, Diagnostic{Message: "empty section header", Line: lineno})
				continue
			}
			current = &Section{Name: name}
			f.Sections = append(f.Sections, current)
		default:
			if current == nil {
				diags = append(diags, Diagnostic{
					Message: fmt.Sprintf("line outside any [SECTION]: %q", trim),
					Line:    lineno,
				})
				continue
			}
			setting := Setting{Raw: line, Line: lineno}
			if eq := strings.Index(line, ":="); eq >= 0 {
				setting.Key = strings.TrimSpace(line[:eq])
				setting.Value = strings.TrimSpace(line[eq+2:])
			} else if eq := strings.Index(line, "="); eq >= 0 {
				setting.Key = strings.TrimSpace(line[:eq])
				setting.Value = strings.TrimSpace(line[eq+1:])
			}
			current.Settings = append(current.Settings, setting)
		}
	}
	if err := sc.Err(); err != nil {
		diags = append(diags, Diagnostic{Message: "scan error: " + err.Error()})
	}
	return f, diags
}

// Load reads a .cfg file from disk.
func Load(path string) (*File, []Diagnostic, error) {
	r, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer r.Close()
	f, d := Parse(r)
	f.Path = path
	return f, d, nil
}

// Section returns the first section matching name (case-sensitive), or
// nil if absent.
func (f *File) Section(name string) *Section {
	for _, s := range f.Sections {
		if s.Name == name {
			return s
		}
	}
	return nil
}

// Setting returns the first non-comment setting in section that has the
// given key. The two-return signature lets callers tell "absent" from
// "present-but-empty".
func (s *Section) Setting(key string) (Setting, bool) {
	if s == nil {
		return Setting{}, false
	}
	for _, st := range s.Settings {
		if st.Comment {
			continue
		}
		if st.Key == key {
			return st, true
		}
	}
	return Setting{}, false
}

// AllSettings returns every non-comment setting in declaration order.
func (s *Section) AllSettings() []Setting {
	if s == nil {
		return nil
	}
	out := make([]Setting, 0, len(s.Settings))
	for _, st := range s.Settings {
		if !st.Comment {
			out = append(out, st)
		}
	}
	return out
}

// ExecuteList is the parsed [EXECUTE] section as a list of test
// invocations. The Titan grammar allows `Module.testcase` and
// `Module.control`; we keep the original text so callers can resolve it
// against their own naming policy.
func (f *File) ExecuteList() []string {
	sec := f.Section("EXECUTE")
	if sec == nil {
		return nil
	}
	var out []string
	for _, st := range sec.Settings {
		if st.Comment {
			continue
		}
		if st.Key != "" {
			out = append(out, st.Key)
			continue
		}
		out = append(out, strings.TrimSpace(st.Raw))
	}
	return out
}

// TestPortParam is one parsed [TESTPORT_PARAMETERS] entry. The Titan
// grammar keys these as `<component>.<port>.<param> := "value"`, where
// component may be `*` (any component). Value has any surrounding double
// quotes stripped.
type TestPortParam struct {
	Component string
	Port      string
	Param     string
	Value     string
}

// TestPortParameters returns the [TESTPORT_PARAMETERS] section parsed into
// (component, port, param, value) tuples in declaration order. Entries
// whose key isn't the expected three dot-separated parts are skipped.
func (f *File) TestPortParameters() []TestPortParam {
	sec := f.Section("TESTPORT_PARAMETERS")
	if sec == nil {
		return nil
	}
	var out []TestPortParam
	for _, st := range sec.Settings {
		if st.Comment || st.Key == "" {
			continue
		}
		parts := strings.SplitN(st.Key, ".", 3)
		if len(parts) != 3 {
			continue
		}
		out = append(out, TestPortParam{
			Component: strings.TrimSpace(parts[0]),
			Port:      strings.TrimSpace(parts[1]),
			Param:     strings.TrimSpace(parts[2]),
			Value:     unquote(st.Value),
		})
	}
	return out
}

// unquote strips a single pair of surrounding double quotes from a .cfg
// value, leaving unquoted values untouched.
func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// ModuleParameters returns the [MODULE_PARAMETERS] settings as a map
// from key to value text. Duplicate keys keep the last value, which
// matches Titan's behaviour.
func (f *File) ModuleParameters() map[string]string {
	sec := f.Section("MODULE_PARAMETERS")
	if sec == nil {
		return nil
	}
	out := map[string]string{}
	for _, st := range sec.Settings {
		if st.Comment || st.Key == "" {
			continue
		}
		out[st.Key] = st.Value
	}
	return out
}

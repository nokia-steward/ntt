package syntax

import "strings"

// An Edition is the major release of the TTCN-3 core language a module is
// written in. Where two editions disagree, the analyzer and the interpreter
// follow the module's.
type Edition int

const (
	// V4 is ETSI ES 201 873-1 V4, the edition of a module that says no
	// other.
	V4 Edition = iota

	// V5 is the next major release of the core language, drafted in the
	// working space of the ETSI MTS task forces,
	// https://labs.etsi.org/rep/mts/ttcn3/standard.
	V5
)

// V5Language is the language identifier that writes a module in V5:
// `module M language "TTCN-3:5" { ... }`. The release defines none yet.
// It names the major version, not a year: the identifiers of years are
// V4's, whose releases take a year other than their own (V4.17.1, of 2025,
// is "TTCN-3:2023"), so any year could yet name a V4 one.
const V5Language = "TTCN-3:5"

// DefaultLanguage is the language of a module whose language clause names
// no TTCN-3 edition: a project's (its manifest's `language`, or
// NTT_LANGUAGE). Empty, such a module is V4. It is set before any module is
// read.
var DefaultLanguage string

// Edition returns the edition the module is written in: the one its
// language clause names, else the one DefaultLanguage names, else V4.
func (m *Module) Edition() Edition {
	if m != nil && m.Language != nil {
		for _, t := range m.Language.List {
			if t == nil {
				continue
			}
			if e, ok := languageEdition(strings.Trim(t.String(), `"`)); ok {
				return e
			}
		}
	}
	if e, ok := languageEdition(DefaultLanguage); ok {
		return e
	}
	return V4
}

// languageEdition is the edition a language identifier names, if it names
// one: V5Language V5, any other TTCN-3 one (`"TTCN-3:2018
// Object-Oriented"`) V4.
func languageEdition(lang string) (Edition, bool) {
	lang = strings.TrimSpace(lang)
	switch {
	case lang == V5Language || strings.HasPrefix(lang, V5Language+" "):
		return V5, true
	case strings.HasPrefix(lang, "TTCN-3:"):
		return V4, true
	}
	return V4, false
}

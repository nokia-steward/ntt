// Package asn1 provides a pragmatic, hand-written front-end for ASN.1
// source files referenced from TTCN-3 test suites.
//
// 3GPP TTCN-3 suites commonly import ASN.1 modules for protocol message
// definitions (e.g. RRC, NGAP). Until now ntt had no way to understand
// those files at all - even producing a "module XYZ not found" error
// when the importing TTCN-3 file mentioned them. This package fills
// that gap with header parsing (module identifier, oid, tagging
// defaults, EXPORTS, IMPORTS) plus a coarse pass over the body to
// extract assignment names. That is enough to:
//
//   - Resolve `import from ASN1Module all` style references.
//   - Power "Go to definition" jumps from TTCN-3 into the corresponding
//     ASN.1 assignment.
//   - Surface helpful diagnostics when a referenced assignment doesn't
//     exist in the imported ASN.1 module.
//
// A full ASN.1 type checker (and round-trip transformation into the
// TTCN-3 type system) is tracked separately. This package is the
// minimum that unblocks the LSP for ASN.1-heavy suites.
package asn1

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nokia/ntt/internal/fs"
)

// Module is the in-memory representation of a single ASN.1 module.
type Module struct {
	// Name is the module identifier, e.g. "RRC-PDU-Definitions".
	Name string

	// OID is the optional object identifier following the module
	// name, including the surrounding braces (e.g. "{ itu-t (0) ... }").
	OID string

	// TaggingDefault is one of "EXPLICIT", "IMPLICIT", "AUTOMATIC"
	// or the empty string when unspecified.
	TaggingDefault string

	// Imports is a list of "module -> symbol names" mappings,
	// preserving source order.
	Imports []Import

	// Exports lists explicitly EXPORTed assignments. Empty means
	// "EXPORTS ALL" (or no EXPORTS clause at all - both are treated
	// as exporting everything).
	Exports []string

	// Assignments lists every type/value/object assignment found in
	// the module body, in source order. The Kind field reflects an
	// educated guess based on the assignment's first non-whitespace
	// token after `::=`.
	Assignments []Assignment

	// Filename is the path that produced this module, when known.
	Filename string

	// Diagnostics records issues encountered while parsing.
	Diagnostics []Diagnostic
}

// Import is a single "FROM Module" clause in an IMPORTS block.
type Import struct {
	From    string   // the source module name
	Symbols []string // symbols imported; empty means "IMPORTS ALL"
}

// AssignmentKind classifies an ASN.1 assignment.
type AssignmentKind int

const (
	UnknownKind AssignmentKind = iota
	TypeKind
	ValueKind
	ObjectClassKind
)

// Assignment is a single `Name ::= ...` entry in the module body.
type Assignment struct {
	Name string
	Kind AssignmentKind
}

// Diagnostic is a parse-time issue with a source line for context.
type Diagnostic struct {
	Line    int
	Column  int
	Message string
}

// ParseFile reads and parses the ASN.1 source at path, returning the
// legacy Module envelope. Reads go through the workspace virtual file
// system so editor buffers override on-disk contents.
func ParseFile(path string) (*Module, error) {
	b, err := fs.Open(path).Bytes()
	if err != nil {
		return nil, err
	}
	m := Parse(b)
	m.Filename = path
	return m, nil
}

// ParseFileFull reads path and returns the full AST. Use this when you
// need byte-precise position info for the assignments (e.g. for "go to
// definition" jumps in the LSP). Same fs-aware semantics as ParseFile.
func ParseFileFull(path string) (*astModuleFull, error) {
	b, err := fs.Open(path).Bytes()
	if err != nil {
		return nil, err
	}
	m := ParseModule(b)
	if m != nil {
		m.Filename = path
	}
	return m, nil
}

// AssignmentName returns the name on the LHS of an AST assignment.
// It's a re-export so the ttcn3 package can avoid an import of the
// internal/asn1/ast package directly.
func AssignmentName(a astAssignment) string { return astAssignmentName(a) }

// Parse parses src as an ASN.1 module and returns the legacy Module
// envelope. Under the hood we use the full X.680/X.681/X.682/X.683
// frontend in this package and adapt the result to the legacy shape so
// existing callers (notably ttcn3/db.go) keep working unchanged. The
// adapter will be removed once Phase 10's retire-envelope-shim task
// lands.
func Parse(src []byte) *Module {
	return adaptModule(ParseModule(src))
}

// String renders a Module's exported summary for debugging.
func (m *Module) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "module %s", m.Name)
	if m.TaggingDefault != "" {
		fmt.Fprintf(&b, " %s TAGS", m.TaggingDefault)
	}
	names := make([]string, 0, len(m.Assignments))
	for _, a := range m.Assignments {
		names = append(names, a.Name)
	}
	sort.Strings(names)
	fmt.Fprintf(&b, " (%d defs: %s)", len(names), strings.Join(names, ", "))
	return b.String()
}

// HasAssignment reports whether m exports an assignment of the given
// name. When no explicit EXPORTS clause is present every assignment is
// considered exported.
func (m *Module) HasAssignment(name string) bool {
	if len(m.Exports) > 0 {
		for _, e := range m.Exports {
			if e == name {
				return true
			}
		}
		return false
	}
	for _, a := range m.Assignments {
		if a.Name == name {
			return true
		}
	}
	return false
}

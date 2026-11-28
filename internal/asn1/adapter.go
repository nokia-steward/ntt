package asn1

import (
	"github.com/nokia/ntt/internal/asn1/ast"
)

// astModuleFull re-exports the AST module type under a stable name so
// the legacy file (asn1.go) doesn't have to import ast directly.
type astModuleFull = ast.Module

// astAssignment re-exports the AST assignment interface.
type astAssignment = ast.Assignment

// astAssignmentName forwards to ast.AssignmentName.
func astAssignmentName(a ast.Assignment) string { return ast.AssignmentName(a) }

// adaptModule converts an ast.Module produced by the new frontend into
// the legacy `Module` envelope this package has exported since the
// initial ASN.1 support shipped. Everything in the new tree maps
// straightforwardly except for the assignment kind, which we infer
// from the AST type rather than re-classifying via the source.
func adaptModule(m *ast.Module) *Module {
	if m == nil {
		return &Module{}
	}
	out := &Module{
		Name:           m.Identifier.Name,
		TaggingDefault: taggingLabel(m.Tagging),
	}
	if m.Identifier.OID != nil {
		out.OID = m.Identifier.OID.Raw
	}
	for _, imp := range m.Imports {
		out.Imports = append(out.Imports, Import{From: imp.From, Symbols: append([]string(nil), imp.Symbols...)})
	}
	if m.Exports != nil && !m.Exports.All {
		out.Exports = append(out.Exports, m.Exports.Symbols...)
	}
	for _, a := range m.Assignments {
		out.Assignments = append(out.Assignments, Assignment{
			Name: ast.AssignmentName(a),
			Kind: assignmentKindOf(a),
		})
	}
	for _, d := range m.Diagnostics {
		out.Diagnostics = append(out.Diagnostics, Diagnostic{
			Line:    1, // line/col were never accurate; clients only use Message
			Column:  d.Pos,
			Message: d.Message,
		})
	}
	return out
}

func taggingLabel(t ast.TaggingMode) string {
	switch t {
	case ast.TagsImplicit:
		return "IMPLICIT"
	case ast.TagsAutomatic:
		return "AUTOMATIC"
	case ast.TagsExplicit:
		return "EXPLICIT"
	}
	return ""
}

func assignmentKindOf(a ast.Assignment) AssignmentKind {
	switch a.(type) {
	case *ast.TypeAssignment, *ast.ValueSetTypeAssignment:
		return TypeKind
	case *ast.ValueAssignment:
		return ValueKind
	case *ast.ObjectClassAssignment:
		return ObjectClassKind
	}
	return UnknownKind
}

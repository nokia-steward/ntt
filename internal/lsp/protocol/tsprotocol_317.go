// Additions for LSP 3.17 that the bundled tsprotocol.go (last
// regenerated from vscode-languageserver-node on 2022-01-26) does not
// yet define. We keep them in a separate file so a future, complete
// regeneration of tsprotocol.go can safely overwrite the original
// without losing these stop-gap types.
//
// The types below cover the high-value 3.17 additions:
//
//   - Pull-model diagnostics (textDocument/diagnostic) which let the
//     server compute diagnostics on demand instead of pushing them
//     after every keystroke.
//   - Type hierarchy (textDocument/prepareTypeHierarchy plus
//     typeHierarchy/supertypes and typeHierarchy/subtypes).
//   - Position-encoding negotiation, which is a small struct that
//     attaches to the existing InitializeResult capability map under a
//     new property.
//
// The types are intentionally placed in package protocol so the rest
// of the LSP server can use them without an import alias.

package protocol

// DocumentDiagnosticParams are the parameters of a
// textDocument/diagnostic request.
//
// @since 3.17.0
type DocumentDiagnosticParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	// Identifier allows the server to maintain multiple diagnostic
	// sources per document.
	Identifier string `json:"identifier,omitempty"`
	// PreviousResultId is the result id of the previous response so
	// the server can return DocumentDiagnosticReportKind = unchanged.
	PreviousResultId string `json:"previousResultId,omitempty"`
}

// DocumentDiagnosticReport is the response to a textDocument/diagnostic
// request. Use Kind = "full" with Items populated, or Kind =
// "unchanged" with ResultId echoed back to the client.
//
// @since 3.17.0
type DocumentDiagnosticReport struct {
	Kind     string       `json:"kind"`
	ResultId string       `json:"resultId,omitempty"`
	Items    []Diagnostic `json:"items,omitempty"`
}

// TypeHierarchyItem is the type-hierarchy companion to
// CallHierarchyItem.
//
// @since 3.17.0
type TypeHierarchyItem struct {
	Name           string      `json:"name"`
	Kind           SymbolKind  `json:"kind"`
	Tags           []SymbolTag `json:"tags,omitempty"`
	Detail         string      `json:"detail,omitempty"`
	URI            DocumentURI `json:"uri"`
	Range          Range       `json:"range"`
	SelectionRange Range       `json:"selectionRange"`
	Data           interface{} `json:"data,omitempty"`
}

// TypeHierarchyPrepareParams are the parameters of
// textDocument/prepareTypeHierarchy.
//
// @since 3.17.0
type TypeHierarchyPrepareParams struct {
	TextDocumentPositionParams
	WorkDoneProgressParams
}

// TypeHierarchySupertypesParams are the parameters of
// typeHierarchy/supertypes.
//
// @since 3.17.0
type TypeHierarchySupertypesParams struct {
	Item TypeHierarchyItem `json:"item"`
	WorkDoneProgressParams
	PartialResultParams
}

// TypeHierarchySubtypesParams are the parameters of
// typeHierarchy/subtypes.
//
// @since 3.17.0
type TypeHierarchySubtypesParams struct {
	Item TypeHierarchyItem `json:"item"`
	WorkDoneProgressParams
	PartialResultParams
}

// PositionEncodingKind names a position-encoding the server understands.
//
// @since 3.17.0
type PositionEncodingKind string

const (
	PositionEncodingUTF8  PositionEncodingKind = "utf-8"
	PositionEncodingUTF16 PositionEncodingKind = "utf-16"
	PositionEncodingUTF32 PositionEncodingKind = "utf-32"
)

// LSPProtocolVersion is the version of the LSP spec that the bundled
// types + the additions in this file are intended to cover.
const LSPProtocolVersion = "3.17.0"

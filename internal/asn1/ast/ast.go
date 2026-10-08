// Package ast defines the abstract syntax tree for ASN.1 source files
// covering X.680 (types/values/constraints/tagging), X.681 (information
// object classes), X.682 (general constraints) and X.683
// (parameterisation).
//
// All nodes implement Node and expose byte-precise source positions
// suitable for LSP responses and diagnostics. Position values are byte
// offsets into the same source slice the lexer scanned.
package ast

// Node is the root interface implemented by every AST node.
type Node interface {
	Pos() int // byte offset of the first token
	End() int // byte offset past the last token
}

// Span is a small embeddable helper so concrete node types don't have
// to spell out Pos() / End() repeatedly. It's exported so the parser
// (which lives in a separate package) can construct nodes via plain
// struct literals.
type Span struct {
	P int
	E int
}

// Pos returns the byte offset of the first token.
func (s Span) Pos() int { return s.P }

// End returns the byte offset past the last token.
func (s Span) End() int { return s.E }

// SetRange updates the span. The pointer receiver lets callers update
// a span in-place through method promotion (e.g. `m.SetRange(0, 42)`
// when m embeds Span).
func (s *Span) SetRange(pos, end int) { s.P, s.E = pos, end }

// NewSpan constructs a Span with the given offsets.
func NewSpan(pos, end int) Span { return Span{P: pos, E: end} }

// TypeBase, ValueBase, ConstraintElementBase, and ObjectSetElementBase
// embed Span and contribute the marker methods that distinguish each
// AST family. They're exported so callers in other packages (notably
// the parser) can construct nodes via struct literals.
type TypeBase struct{ Span }

func (TypeBase) typeNode() {}

type ValueBase struct{ Span }

func (ValueBase) valueNode() {}

type ConstraintElementBase struct{ Span }

func (ConstraintElementBase) constraintElementNode() {}

type ObjectSetElementBase struct{ Span }

func (ObjectSetElementBase) objectSetElementNode() {}


// ---------------------------------------------------------------------------
// Top level
// ---------------------------------------------------------------------------

// Module is the top of the tree, one per ASN.1 source file.
type Module struct {
	Span
	Identifier  ModuleIdentifier
	Tagging     TaggingMode    // default tagging mode
	Extensible  bool           // EXTENSIBILITY IMPLIED
	Exports     *Exports       // optional; nil = "EXPORTS ALL"
	Imports     []*Import      // FROM clauses, in source order
	Assignments []Assignment   // type/value/class/object/set assignments
	Diagnostics []Diagnostic   // accumulated during parse + resolve
	Filename    string         // path that produced this module (empty for in-memory)
}

// ModuleIdentifier is "Name { ... }" header.
type ModuleIdentifier struct {
	Span
	Name       string
	OID        *OID // optional
	IRI        string
	DefinitiveName string // raw text for editor display
}

// OID is an ASN.1 object identifier value (a sequence of name/number
// components). The raw source text is preserved in Raw for round-trip.
type OID struct {
	Span
	Components []OIDComponent
	Raw        string
}

// OIDComponent is one element of an OID, e.g. `itu-t(0)` or `0`.
type OIDComponent struct {
	Span
	Name   string // optional textual part
	Number int64  // numeric value if known, otherwise 0
	HasNum bool   // distinguishes "name" from "0"
}

// TaggingMode is one of the three module-level tagging defaults.
type TaggingMode int

const (
	TagsExplicit  TaggingMode = iota // default per X.680
	TagsImplicit
	TagsAutomatic
)

// Exports is the optional EXPORTS clause body.
type Exports struct {
	Span
	All     bool     // "EXPORTS ALL"
	Symbols []string // explicit symbol list when All is false
}

// Import is a single "Symbol[, Symbol]* FROM Module [{OID}]" entry.
type Import struct {
	Span
	Symbols []string
	From    string
	OID     *OID
}

// ---------------------------------------------------------------------------
// Assignments
// ---------------------------------------------------------------------------

// Assignment is the interface implemented by every top-level assignment
// inside a module body.
type Assignment interface {
	Node
	assignmentName() string
}

// AssignmentName returns the identifier on the left-hand side of an
// assignment.
func AssignmentName(a Assignment) string { return a.assignmentName() }

// TypeAssignment: `Name ::= Type`.
type TypeAssignment struct {
	Span
	Name       string
	Params     *ParameterList // optional X.683 parameter list
	Type       Type
}

func (a *TypeAssignment) assignmentName() string { return a.Name }

// ValueAssignment: `name Type ::= Value`.
type ValueAssignment struct {
	Span
	Name  string
	Type  Type
	Value Value
}

func (a *ValueAssignment) assignmentName() string { return a.Name }

// ValueSetTypeAssignment: `Name Type ::= { ElementSet }`.
type ValueSetTypeAssignment struct {
	Span
	Name string
	Type Type
	Set  *ElementSet
}

func (a *ValueSetTypeAssignment) assignmentName() string { return a.Name }

// ObjectClassAssignment: `NAME ::= CLASS { fieldSpecs } [WITH SYNTAX { ... }]`.
type ObjectClassAssignment struct {
	Span
	Name       string
	Params     *ParameterList
	Class      *ObjectClass
}

func (a *ObjectClassAssignment) assignmentName() string { return a.Name }

// ObjectAssignment: `name CLASSREF ::= { objectBody }`.
type ObjectAssignment struct {
	Span
	Name     string
	Params   *ParameterList
	ClassRef *TypeRef
	Object   *Object
}

func (a *ObjectAssignment) assignmentName() string { return a.Name }

// ObjectSetAssignment: `NAME CLASSREF ::= { ObjectSet }`.
type ObjectSetAssignment struct {
	Span
	Name     string
	Params   *ParameterList
	ClassRef *TypeRef
	Set      *ObjectSet
}

func (a *ObjectSetAssignment) assignmentName() string { return a.Name }

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// Type is the interface for any ASN.1 type expression.
type Type interface {
	Node
	typeNode()
}


// BuiltinKind enumerates X.680 builtin types.
type BuiltinKind int

const (
	UnknownBuiltin BuiltinKind = iota
	Boolean
	Integer
	Real
	Null
	BitString
	OctetString
	ObjectIdentifier
	RelativeOID
	OIDIRI
	RelativeOIDIRI
	Enumerated
	UTCTime
	GeneralizedTime
	External
	EmbeddedPDV
	CharacterString
	Date
	TimeOfDay
	DateTime
	Duration
	Time
	// Restricted string types - X.680 §41.
	BMPString
	GeneralString
	GraphicString
	IA5String
	ISO646String
	NumericString
	PrintableString
	TeletexString
	T61String
	UniversalString
	UTF8String
	VideotexString
	VisibleString
	ObjectDescriptor
)

// BuiltinType is a simple non-parameterised builtin (boolean, integer,
// real, ...). Types that carry data (BIT STRING with named bits,
// ENUMERATED with members, INTEGER with named numbers, SEQUENCE OF X)
// have their own dedicated node types below.
type BuiltinType struct {
	TypeBase
	Kind BuiltinKind
	Name string // raw keyword text for round-trip ("INTEGER", "BIT STRING")
}

// IntegerType: `INTEGER` optionally followed by `{ name(value), ... }`.
type IntegerType struct {
	TypeBase
	NamedNumbers []NamedNumber
}

// NamedNumber is `name(value)` inside INTEGER / BIT STRING / ENUMERATED.
type NamedNumber struct {
	Span
	Name  string
	Value Value
}

// BitStringType: `BIT STRING` optionally followed by named bits.
type BitStringType struct {
	TypeBase
	NamedBits []NamedNumber
}

// EnumeratedType: `ENUMERATED { name(val), ..., name }` with extension.
type EnumeratedType struct {
	TypeBase
	Items      []EnumItem
	Extensible bool
	Extensions []EnumItem // entries after the extension marker
}

// EnumItem is a single ENUMERATED entry, either named (with optional value).
type EnumItem struct {
	Span
	Name  string
	Value Value // optional
}

// SequenceType: `SEQUENCE { components } [...] [, components]`.
type SequenceType struct {
	TypeBase
	Components []Component
	Extensible bool
	Extensions []ExtensionAddition
}

// SetType: `SET { ... }`, otherwise structurally identical to SEQUENCE.
type SetType struct {
	TypeBase
	Components []Component
	Extensible bool
	Extensions []ExtensionAddition
}

// ChoiceType: `CHOICE { alternatives ... }`.
type ChoiceType struct {
	TypeBase
	Alternatives []Component
	Extensible   bool
	Extensions   []ExtensionAddition
}

// SequenceOfType: `SEQUENCE [(size)] OF Type`.
type SequenceOfType struct {
	TypeBase
	Element    Type
	Constraint *Constraint // optional size constraint
}

// SetOfType: `SET [(size)] OF Type`.
type SetOfType struct {
	TypeBase
	Element    Type
	Constraint *Constraint
}

// Component is one named member of a SEQUENCE / SET / CHOICE.
type Component struct {
	Span
	Name     string
	Type     Type
	Optional bool
	Default  Value
	// COMPONENTS OF support: when true, Type is a referenced type
	// whose own components should be spliced in here.
	ComponentsOf bool
}

// ExtensionAddition wraps one or more components added between [[ ]]
// or directly after the extension marker.
type ExtensionAddition struct {
	Span
	Group      int        // 0 means "ungrouped, immediately after ...". 1+ = [[ N: ...
	Components []Component
}

// TaggedType: `[Tag] Type`.
type TaggedType struct {
	TypeBase
	Tag      Tag
	Underlying Type
}

// TagClass enumerates the four ASN.1 tag classes.
type TagClass int

const (
	ContextSpecificTag TagClass = iota // [N] with no class keyword
	UniversalTag                       // [UNIVERSAL N]
	ApplicationTag                     // [APPLICATION N]
	PrivateTag                         // [PRIVATE N]
)

// TagMode is IMPLICIT, EXPLICIT or unspecified (use module default).
type TagMode int

const (
	TagModeUnspecified TagMode = iota
	TagModeImplicit
	TagModeExplicit
)

// Tag is `[CLASS N]` with optional IMPLICIT/EXPLICIT mode.
type Tag struct {
	Span
	Class  TagClass
	Number Value // typically an integer literal but may be a reference
	Mode   TagMode
}

// ReferencedType wraps a use of a previously declared type.
type ReferencedType struct {
	TypeBase
	Ref     *TypeRef
	// Actuals carries actual parameters for X.683 instantiation, if any.
	Actuals *ActualParameterList
}

// TypeRef is a possibly qualified reference like `Foo` or `Mod.Foo`.
type TypeRef struct {
	Span
	Module string // optional
	Name   string
}

// ConstrainedType wraps an inner type with a constraint.
type ConstrainedType struct {
	TypeBase
	Inner      Type
	Constraint *Constraint
}

// AnyType is used as a placeholder when the parser cannot identify a
// type but recovers to continue parsing. Diagnostics will explain.
type AnyType struct {
	TypeBase
	Raw string
}

// OpenTypeFieldType is a use of `CLASS.&Field` where the field is a
// type field, producing an open type at the use site.
type OpenTypeFieldType struct {
	TypeBase
	ClassRef *TypeRef
	Field    string // e.g. "&Type"
}

// ---------------------------------------------------------------------------
// Values
// ---------------------------------------------------------------------------

// Value is the interface for any ASN.1 value expression.
type Value interface {
	Node
	valueNode()
}


// IntegerValue holds a signed integer literal.
type IntegerValue struct {
	ValueBase
	Text string // source text (lets callers re-parse big numbers)
}

// RealValue holds a real-number literal.
type RealValue struct {
	ValueBase
	Text string
}

// BooleanValue is TRUE or FALSE.
type BooleanValue struct {
	ValueBase
	Value bool
}

// NullValue represents the NULL value of NULL type.
type NullValue struct{ ValueBase }

// StringValue covers CSTRING, BSTRING, HSTRING literals; Kind preserves
// which form was used.
type StringValue struct {
	ValueBase
	Kind StringKind
	Text string // raw text including delimiters
}

// StringKind disambiguates the three ASN.1 string literal flavours.
type StringKind int

const (
	StringCString StringKind = iota
	StringBString
	StringHString
)

// OIDValue is an object identifier literal `{ a b(2) c(3) }`.
type OIDValue struct {
	ValueBase
	OID *OID
}

// ReferenceValue is a bare name resolving to another value.
type ReferenceValue struct {
	ValueBase
	Module string
	Name   string
}

// ChoiceValue: `name : value`.
type ChoiceValue struct {
	ValueBase
	Alternative string
	Value       Value
}

// SequenceValue: `{ name v, name v, ... }` for SEQUENCE/SET.
type SequenceValue struct {
	ValueBase
	Fields []NamedValue
}

// NamedValue is one entry inside a SEQUENCE/SET value.
type NamedValue struct {
	Span
	Name  string
	Value Value
}

// SequenceOfValue: `{ v1, v2, v3, ... }`.
type SequenceOfValue struct {
	ValueBase
	Elements []Value
}

// ---------------------------------------------------------------------------
// Constraints
// ---------------------------------------------------------------------------

// Constraint wraps the outer parens of `(...)`. The body is an
// ElementSet (X.680 §49).
type Constraint struct {
	Span
	Set        *ElementSet
	Exception  *Exception // optional `! ExceptionSpec`
}

// Exception captures the optional `! errorValue` annotation after a
// constraint.
type Exception struct {
	Span
	Raw string
}

// ElementSet is a (possibly extensible) tree of unions/intersections of
// constraint elements.
type ElementSet struct {
	Span
	Root       UnionExpr
	Extensible bool
	Extension  UnionExpr // optional set after `, ...`
}

// UnionExpr is a list of intersections joined by `|` / UNION.
type UnionExpr []IntersectionExpr

// IntersectionExpr is a list of elements joined by `^` / INTERSECTION.
type IntersectionExpr []ConstraintElement

// ConstraintElement is one atom in a constraint expression.
type ConstraintElement interface {
	Node
	constraintElementNode()
}


// SingleValueConstraint: literal value.
type SingleValueConstraint struct {
	ConstraintElementBase
	Value Value
}

// ValueRangeConstraint: `(lo..hi)`, with optional open endpoints.
type ValueRangeConstraint struct {
	ConstraintElementBase
	Lower      Value
	LowerOpen  bool
	Upper      Value
	UpperOpen  bool
	LowerIsMin bool
	UpperIsMax bool
}

// SizeConstraint: `SIZE (N)` or `SIZE (lo..hi)`.
type SizeConstraint struct {
	ConstraintElementBase
	Constraint *Constraint
}

// AlphabetConstraint: `FROM ("abc")` and friends.
type AlphabetConstraint struct {
	ConstraintElementBase
	Constraint *Constraint
}

// TypeConstraint constrains to subtype `Type`.
type TypeConstraint struct {
	ConstraintElementBase
	Type Type
}

// ContainedSubtype: `INCLUDES Type` or `(Type)` shorthand.
type ContainedSubtype struct {
	ConstraintElementBase
	Type Type
}

// PatternConstraint: `PATTERN value`.
type PatternConstraint struct {
	ConstraintElementBase
	Pattern Value
}

// PropertySettings: `SETTINGS "..."`.
type PropertySettings struct {
	ConstraintElementBase
	Settings string
}

// InnerTypeConstraint: `WITH COMPONENT(S) { ... }`.
type InnerTypeConstraint struct {
	ConstraintElementBase
	Single     bool // false = "WITH COMPONENTS"
	Constraint *Constraint
	Components []InnerComponent
	PartialFlag bool // true when the component list includes "..."
}

// InnerComponent is one entry inside a `WITH COMPONENTS` block.
type InnerComponent struct {
	Span
	Name       string
	Constraint *Constraint
	Presence   Presence
}

// Presence is the optional presence keyword in a `WITH COMPONENTS`.
type Presence int

const (
	PresenceUnspecified Presence = iota
	PresencePresent
	PresenceAbsent
	PresenceOptional
)

// TableConstraint: `({ObjectSet})` or `({ObjectSet}{@field.path})`.
type TableConstraint struct {
	ConstraintElementBase
	ObjectSet *ObjectSet
	AtNotation *AtNotation
}

// AtNotation captures `{@component.field}` style field references that
// drive component-relation constraints.
type AtNotation struct {
	Span
	Level int      // number of leading `.` characters (0 = root, 1 = `.x`, ...)
	Path  []string // dotted path components
}

// UserDefinedConstraint: `CONSTRAINED BY { ... }`. We preserve the raw
// text since the body is implementation defined.
type UserDefinedConstraint struct {
	ConstraintElementBase
	Raw string
}

// ExceptConstraint: `A EXCEPT B`.
type ExceptConstraint struct {
	ConstraintElementBase
	Base    ConstraintElement
	Exclude ConstraintElement
}

// AllExceptConstraint: `ALL EXCEPT B`.
type AllExceptConstraint struct {
	ConstraintElementBase
	Exclude ConstraintElement
}

// ---------------------------------------------------------------------------
// X.681 information object classes
// ---------------------------------------------------------------------------

// ObjectClass is the body of an X.681 class definition.
type ObjectClass struct {
	Span
	Fields     []FieldSpec
	WithSyntax *WithSyntaxSpec // optional
}

// FieldSpec is the interface for one field inside an ObjectClass.
type FieldSpec interface {
	Node
	fieldName() string
}

// FieldName returns the &Foo/&foo name of a field spec.
func FieldName(f FieldSpec) string { return f.fieldName() }

// TypeFieldSpec: `&Foo [OPTIONAL] [DEFAULT Type]`.
type TypeFieldSpec struct {
	Span
	Name     string // includes leading "&"
	Optional bool
	Default  Type
}

func (f *TypeFieldSpec) fieldName() string { return f.Name }

// FixedTypeValueFieldSpec: `&foo Type [UNIQUE] [OPTIONAL] [DEFAULT Value]`.
type FixedTypeValueFieldSpec struct {
	Span
	Name     string
	Type     Type
	Unique   bool
	Optional bool
	Default  Value
}

func (f *FixedTypeValueFieldSpec) fieldName() string { return f.Name }

// VariableTypeValueFieldSpec: `&foo &Field [OPTIONAL] [DEFAULT Value]`.
type VariableTypeValueFieldSpec struct {
	Span
	Name      string
	FieldName string // referenced &Type field on same class
	Optional  bool
	Default   Value
}

func (f *VariableTypeValueFieldSpec) fieldName() string { return f.Name }

// FixedTypeValueSetFieldSpec: `&Foo Type [OPTIONAL] [DEFAULT {Set}]`.
type FixedTypeValueSetFieldSpec struct {
	Span
	Name     string
	Type     Type
	Optional bool
	Default  *ElementSet
}

func (f *FixedTypeValueSetFieldSpec) fieldName() string { return f.Name }

// VariableTypeValueSetFieldSpec: `&Foo &Field [OPTIONAL] [DEFAULT {Set}]`.
type VariableTypeValueSetFieldSpec struct {
	Span
	Name      string
	FieldName string
	Optional  bool
	Default   *ElementSet
}

func (f *VariableTypeValueSetFieldSpec) fieldName() string { return f.Name }

// ObjectFieldSpec: `&foo CLASS [OPTIONAL] [DEFAULT object]`.
type ObjectFieldSpec struct {
	Span
	Name     string
	ClassRef *TypeRef
	Optional bool
	Default  *Object
}

func (f *ObjectFieldSpec) fieldName() string { return f.Name }

// ObjectSetFieldSpec: `&Foo CLASS [OPTIONAL] [DEFAULT { Set }]`.
type ObjectSetFieldSpec struct {
	Span
	Name     string
	ClassRef *TypeRef
	Optional bool
	Default  *ObjectSet
}

func (f *ObjectSetFieldSpec) fieldName() string { return f.Name }

// WithSyntaxSpec is the literal-or-field stream that drives X.681
// object literal parsing.
type WithSyntaxSpec struct {
	Span
	Tokens []WithSyntaxToken
}

// WithSyntaxToken is either a literal WORD, an optional `[ ... ]`
// group, or a field reference (&Foo / &foo). We keep them as a flat
// stream; the ClassObjectParser walks it.
type WithSyntaxToken struct {
	Span
	Kind   WithSyntaxKind
	Text   string             // for Word / FieldRef
	Group  []WithSyntaxToken  // for OptionalGroup
}

// WithSyntaxKind enumerates the three template token forms.
type WithSyntaxKind int

const (
	WSKWord WithSyntaxKind = iota
	WSKFieldRef
	WSKOptionalGroup
)

// Object is the parsed body of an object literal. We retain the
// per-field settings as `(fieldRef -> Setting)`.
type Object struct {
	Span
	Settings []ObjectSetting
}

// ObjectSetting is one filled-in entry inside an Object literal.
type ObjectSetting struct {
	Span
	FieldRef string // includes leading "&"
	Type     Type   // either Type or Value will be non-nil (mutually exclusive)
	Value    Value
}

// ObjectSet is the parsed `{ ObjectSetElement | ... }` body.
type ObjectSet struct {
	Span
	Root       UnionElements // simplified ElementSet for object sets
	Extensible bool
	Extension  UnionElements
}

// UnionElements is a flat union of ObjectSetElements; intersection
// support is omitted in this MVP and parsed as a single union.
type UnionElements []ObjectSetElement

// ObjectSetElement is one atom in an object set body.
type ObjectSetElement interface {
	Node
	objectSetElementNode()
}


// ObjectLiteralElement: a `{ ... }` object literal in an object set.
type ObjectLiteralElement struct {
	ObjectSetElementBase
	Object *Object
}

// ObjectReferenceElement: bare object reference.
type ObjectReferenceElement struct {
	ObjectSetElementBase
	Ref *TypeRef
}

// ObjectSetReferenceElement: reference to a named object set.
type ObjectSetReferenceElement struct {
	ObjectSetElementBase
	Ref *TypeRef
}

// ---------------------------------------------------------------------------
// X.683 parameterisation
// ---------------------------------------------------------------------------

// ParameterList is the `{ Param1, Param2, ... }` that follows a
// parameterised type/value/class/object/set name.
type ParameterList struct {
	Span
	Params []Parameter
}

// Parameter is one entry inside a ParameterList, with an optional
// governor (the type the actual parameter must conform to).
type Parameter struct {
	Span
	Governor  Type // optional
	Reference string
}

// ActualParameterList is the `{ Type1, value1, ... }` carrying the
// actuals at an instantiation site.
type ActualParameterList struct {
	Span
	Params []ActualParameter
}

// ActualParameter wraps either a Type or a Value.
type ActualParameter struct {
	Span
	Type  Type
	Value Value
}

// ---------------------------------------------------------------------------
// Diagnostics
// ---------------------------------------------------------------------------

// Diagnostic is a parser- or resolver-produced message with a byte
// range. The legacy {Line, Column, Message} shape lives on the public
// `asn1.Diagnostic` type for API back-compat; this AST-level diagnostic
// is the one new code should use.
type Diagnostic struct {
	Pos      int
	End      int
	Severity Severity
	Code     string
	Message  string
}

// Severity classifies a diagnostic.
type Severity int

const (
	SeverityError Severity = iota
	SeverityWarning
	SeverityInfo
	SeverityHint
)

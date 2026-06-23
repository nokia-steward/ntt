// Package ast provides TTCN-3 syntax tree nodes and functions for tree
// traversal.
package syntax

import "github.com/hashicorp/go-multierror"

//go:generate go run ./internal/gen

// All node types implement the Node interface.
type Node interface {
	Kind() Kind
	Pos() int
	End() int
	FirstTok() Token
	LastTok() Token
	Children() []Node
	Inspect(func(Node) bool)
}

type Token interface {
	Node
	String() string
	PrevTok() Token
	NextTok() Token
}

// All expression nodes implement the Expr interface.
type Expr interface {
	Node
	exprNode()
}

// All statement nodes implement the Stmt interface.
type Stmt interface {
	Node
	stmtNode()
}

// All declaration nodes implement the Decl interface.
type Decl interface {
	Node
	declNode()
}

type Root struct {
	NodeList
	*Scanner
	Filename string
	tokens   []token
	errs     []error

	// lineCache memoises the line lookup result from the previous
	// Position() call. Most LSP traversals visit tokens in source
	// order, so the next position lands either on the same line or
	// shortly after - both cases short-circuit the binary search.
	// Concurrent reads are tolerated because the cache only stores
	// a hint; a stale read produces the correct answer via the
	// fallback search below.
	lineCacheLine int
	lineCacheLo   int // first byte of the cached line
	lineCacheHi   int // first byte of the next line (or len(src))
	lineCacheOk   bool
}

func (n *Root) Err() error {
	return multierror.Append(nil, n.errs...).ErrorOrNil()
}

func (n *Root) Position(offset int) Position {
	if offset < 0 {
		return Position{}
	}
	if l := n.searchLines(offset); l >= 0 {
		return Position{
			Line:   l + 1,
			Column: offset - n.lines[l] + 1,
		}
	}
	return Position{}
}

// searchLines returns the index of the line that contains the byte
// offset pos. The result is cached so that the common access pattern
// (looking up positions in monotonically non-decreasing order, as
// happens when the LSP walks a tree from start to end) collapses to a
// single bounds check on the cache.
func (n *Root) searchLines(pos int) int {
	if n.lineCacheOk && pos >= n.lineCacheLo && pos < n.lineCacheHi {
		return n.lineCacheLine
	}

	i, j := 0, len(n.lines)
	for i < j {
		h := int(uint(i+j) >> 1) // avoid overflow when computing h
		if n.lines[h] <= pos {
			i = h + 1
		} else {
			j = h
		}
	}
	idx := i - 1

	if idx >= 0 {
		n.lineCacheLine = idx
		n.lineCacheLo = n.lines[idx]
		if idx+1 < len(n.lines) {
			n.lineCacheHi = n.lines[idx+1]
		} else {
			// We don't know the buffer length here, but any
			// position past the last newline still belongs to
			// the last line. Mark hi as a sentinel.
			n.lineCacheHi = 1 << 62
		}
		n.lineCacheOk = true
	}
	return idx
}

func (n *Root) PosFor(line, col int) int {
	line--
	if line < 0 {
		return -1
	}
	if line >= len(n.lines) {
		line = len(n.lines) - 1
	}
	return n.lines[line] + col - 1
}

func (n *Root) FirstTok() Token {
	if len(n.tokens) == 0 {
		return nil
	}
	return &tokenNode{n, 0}
}

func (n *Root) LastTok() Token {
	if len(n.tokens) == 0 {
		return nil
	}
	return &tokenNode{n, len(n.tokens) - 1}
}

func (n *Root) Inspect(f func(Node) bool) { n.NodeList.Inspect(f) }
func (n *Root) Children() []Node          { return n.NodeList.Children() }

type tokenNode struct {
	*Root
	idx int
}

func (n *tokenNode) Kind() Kind {
	return n.tokens[n.idx].Kind
}

func (n *tokenNode) Pos() int {
	tok := n.tokens[n.idx]
	return tok.Begin
}

func (n *tokenNode) End() int {
	tok := n.tokens[n.idx]
	return tok.End
}

func (n *tokenNode) LastTok() Token   { return n }
func (n *tokenNode) FirstTok() Token  { return n }
func (n *tokenNode) Children() []Node { return nil }
func (n *tokenNode) PrevTok() Token {
	if n.idx <= 0 {
		return nil
	}
	return &tokenNode{idx: n.idx - 1, Root: n.Root}
}

func (n *tokenNode) NextTok() Token {
	if n.idx >= len(n.tokens)-1 {
		return nil
	}
	return &tokenNode{idx: n.idx + 1, Root: n.Root}
}

func (n *tokenNode) String() string {
	tok := n.tokens[n.idx]
	if tok.IsLiteral() {
		return string(n.Root.src[tok.Begin:tok.End])
	}
	return tok.String()
}

func (n *tokenNode) Inspect(fn func(Node) bool) {
	fn(n)
}

type ErrorNode struct {
	From, To Token
}

func (x ErrorNode) exprNode()     {}
func (x ErrorNode) stmtNode()     {}
func (x ErrorNode) declNode()     {}
func (x ErrorNode) typeSpecNode() {}

type NodeList struct {
	Nodes []Node
}

type (
	// Ident represents an identifier.
	Ident struct {
		IsName bool  // true if this is a name, false if it is a reference
		Tok    Token // first identifier token
		Tok2   Token `json:",omitempty"` // optional second identifier token, e.g. for "any port"
	}

	// ParametrizedIdent represents a paremetrized identifier, e.g. "f<charstring>".
	ParametrizedIdent struct {
		Ident  *Ident     // Identifier
		Params *ParenExpr // Parameter list
	}

	// A ValueLiteral represents simple literals, like integers, charstrings, ...
	ValueLiteral struct {
		Tok Token
	}

	// A CompositeLiteral represents composite literals, e.g. "{x:=23, y:=5}".
	CompositeLiteral struct {
		LBrace Token  // Position of "{"
		List   []Expr // Expression list
		RBrace Token  // Position of "{"
	}

	// An ObjidLiteral represents an OBJECT IDENTIFIER value notation,
	// e.g. `objid { itu_t question(1) 7 }`. Components are whitespace
	// separated name forms, number forms, or name-and-number forms.
	ObjidLiteral struct {
		Tok    Token  // Position of "objid"
		LBrace Token  // Position of "{"
		List   []Expr // OID components
		RBrace Token  // Position of "}"
	}

	// A UnaryExpr represents a unary expresions.
	UnaryExpr struct {
		Op Token // Operator token, like "+", "-", "!", ...
		X  Expr
	}

	// A postfix expresseion (i++, i--)
	PostExpr struct {
		X  Expr
		Op Token
	}

	// A BinaryExpr represents a binary expression.
	// Possible operands are all tokens with a precedence value, TO and FROM.
	BinaryExpr struct {
		X  Expr  // First operand
		Op Token // Operator token
		Y  Expr  // Second operand
	}

	// A ParenExpr represents parenthized expression lists.
	ParenExpr struct {
		LParen Token  // Position of "(", "<", "["
		List   []Expr // Expression list
		RParen Token  // Position of ")", ">", "]"
	}

	// A SelectorExpr represents an expression followed by a selector.
	SelectorExpr struct {
		X   Expr  // Preceding expression (might be nil)
		Dot Token // Position of "."
		Sel Expr  // Literal, identifier or reference.
	}

	// A IndexExpr represents an expression followed by an index.
	IndexExpr struct {
		X      Expr  // Preceding expression (might be nil)
		LBrack Token // Position of "["
		Index  Expr  // Actuall index expression (might be "-")
		RBrack Token // Position of "]"
	}

	// A CallExpr represents a regular function call.
	CallExpr struct {
		Fun  Expr       // Function expression
		Args *ParenExpr // Function arguments
	}

	// A LengthExpr represents a length expression.
	LengthExpr struct {
		X    Expr       // Preceding expression
		Len  Token      // Position of "length" keyword
		Size *ParenExpr // Size expression
	}

	// A RedirectExpr represents various redirect expressions
	RedirectExpr struct {
		X             Expr   // Preceding redirected expression
		Tok           Token  // Position of "->"
		ValueTok      Token  // Position of "value" or nil
		Value         []Expr // Value expression
		VerdictTok    Token  // Position of "verdict" or nil
		Verdict       []Expr // Verdict expression (call/done redirect)
		ParamTok      Token  // Position of "param" or nil
		Param         []Expr // Param expression
		SenderTok     Token  // Position of "sender" or nil
		Sender        Expr   // Sender expression
		IndexTok      Token  // Position of "@index" or nil
		IndexValueTok Token  // Position of "value" or nil
		Index         Expr   // Index expression
		TimestampTok  Token  // Position of "timestamp" or nil
		Timestamp     Expr   // Timestamp expression
	}

	// A ValueExpr represents the return value used by signature based communication.
	ValueExpr struct {
		X   Expr  // Preceding template expression
		Tok Token // Position of "value"
		Y   Expr  // Value expression
	}

	// A ParamExpr represents parametrized map and unmap statements.
	ParamExpr struct {
		X   Expr  // map or unmap statement
		Tok Token // Position "param"
		Y   Expr  // Additional arguments for map/unmap
	}

	// A FromExpr represents a "from" expression, like "any from a".
	FromExpr struct {
		KindTok Token // ANY or ALL
		FromTok Token // Position of "from"
		X       Expr  // Expression
	}

	// A ModifiesExpr represents a "modifies" expression.
	ModifiesExpr struct {
		Tok    Token // Position of "modifies"
		X      Expr  // Base template expression
		Assign Token // Position of ":="
		Y      Expr  // Modifying expression
	}

	// A RegexExpr represents a "regexp" expression.
	RegexpExpr struct {
		Tok    Token // Position of "regexp"
		NoCase Token // Position of "@nocase" or nil
		X      Expr  // Regex expression
	}

	// A PatternExpr represents a "pattern" expression.
	PatternExpr struct {
		Tok    Token // Position of "pattern"
		NoCase Token // Position of "@nocase" of nil
		X      Expr  // Pattern expression
	}

	// A DecmatchExpr represents a "decmatch" expression.
	DecmatchExpr struct {
		Tok    Token // Position of "decmatch"
		Params Expr  // Parameter list or nil
		X      Expr  // Template expression
	}

	// A DecodedExpr represents a "@decoded" expression.
	DecodedExpr struct {
		Tok    Token // Position of "decoded"
		Params Expr  // Parameter list or nil
		X      Expr  // Template expression
	}

	// A DynamicExpr represents a "@dynamic" expression.
	DynamicExpr struct {
		Tok  Token      // Position of "dynamic"
		Body *BlockStmt // Template body
	}

	// A DefKindExpr represents a definition kind expression, used by imports
	// and with-attributes.
	DefKindExpr struct {
		KindTok Token  // Definition kind, "type", "group", ...
		List    []Expr // List of identifiers or except-expressions
	}

	// A ExceptExpr is used by DefKindExpr to express exlusion of specific
	// defintions.
	ExceptExpr struct {
		X         Expr   // (Qualified) identifier or "all"
		ExceptTok Token  // Position of "except"
		LBrace    Token  // Position of "{" or nil
		List      []Expr // List of identifiers or DefKindExprs to exclude
		RBrace    Token  // Position of "}" or nil
	}
)

func (x *Ident) String() string {
	if x.Tok2 != nil {
		return x.Tok.String() + " " + x.Tok2.String()
	}
	return x.Tok.String()
}

func (x *Ident) exprNode()             {}
func (x *ParametrizedIdent) exprNode() {}
func (x *ValueLiteral) exprNode()      {}
func (x *CompositeLiteral) exprNode()  {}
func (x *ObjidLiteral) exprNode()      {}
func (x *UnaryExpr) exprNode()         {}
func (x *PostExpr) exprNode()          {}
func (x *BinaryExpr) exprNode()        {}
func (x *ParenExpr) exprNode()         {}
func (x *SelectorExpr) exprNode()      {}
func (x *IndexExpr) exprNode()         {}
func (x *CallExpr) exprNode()          {}
func (x *LengthExpr) exprNode()        {}
func (x *RedirectExpr) exprNode()      {}
func (x *ValueExpr) exprNode()         {}
func (x *ParamExpr) exprNode()         {}
func (x *FromExpr) exprNode()          {}
func (x *ModifiesExpr) exprNode()      {}
func (x *RegexpExpr) exprNode()        {}
func (x *PatternExpr) exprNode()       {}
func (x *DecmatchExpr) exprNode()      {}
func (x *DecodedExpr) exprNode()       {}
func (x *DynamicExpr) exprNode()       {}
func (x *DefKindExpr) exprNode()       {}
func (x *ExceptExpr) exprNode()        {}

type (
	// A BlockStmt represents a curly braces enclosed list of statements.
	BlockStmt struct {
		LBrace Token  // Position of "{"
		Stmts  []Stmt // List of statements
		RBrace Token  // Position of "}"
	}

	// A DeclStmt represents a value declaration used as statement, lika a
	// local variable declaration.
	DeclStmt struct {
		Decl Decl
	}

	// An ExprStmt represents a expression used as statement, like an
	// assignment or function call.
	ExprStmt struct {
		Expr Expr
	}

	// A BranchStmt represents a branch statement.
	BranchStmt struct {
		Tok   Token  // REPEAT, BREAK, CONTINUE, LABEL, GOTO
		Label *Ident // Label literal or nil
	}

	// A ReturnStmt represents a return statement.
	ReturnStmt struct {
		Tok    Token // Position of "return"
		Result Expr  // Resulting expression of nil
	}

	// A RaiseStmt represents the object-oriented `raise <expr>`
	// statement that throws an exception value to the nearest
	// enclosing catch clause (ETSI ES 201 873-1 clause 5.2.3).
	RaiseStmt struct {
		Tok Token // Position of "raise"
		X   Expr  // Raised value
	}

	// A AltStmt represents an alternative statement.
	AltStmt struct {
		Tok       Token // ALT or INTERLEAVE
		NoDefault Token
		Body      *BlockStmt // Block statement with alternations
	}

	// A CallStmt represents a "call" statement with communication-block.
	CallStmt struct {
		Stmt Stmt       // "call" statement
		Body *BlockStmt // Block statement with alternations
	}

	// A ForStmt represents a "for" statement.
	ForStmt struct {
		Tok      Token      // Position of "for"
		LParen   Token      // Position of "("
		Init     Stmt       // Initialization statement
		InitSemi Token      // Position of ";"
		Cond     Expr       // Conditional expression
		CondSemi Token      // Position of ";"
		Post     Stmt       // Post iteration statement
		RParen   Token      // Position of ")"
		Body     *BlockStmt // Loop-Body
	}

	ForRangeStmt struct {
		Tok    Token
		LParen Token
		Init   Stmt
		InTok  Token
		Range  Expr
		RParen Token
		Body   *BlockStmt
	}

	// A WhilStmt represents a "while" statement.
	WhileStmt struct {
		Tok    Token      // Position of "while"
		LParen Token      // Position of "("
		Cond   Expr       // Conditional expression
		RParen Token      // Position of ")"
		Body   *BlockStmt // Loop-body
	}

	// A DoWhileStmt represents a do-while statement.
	DoWhileStmt struct {
		DoTok    Token      // Position of "do"
		Body     *BlockStmt // Loop-Body
		WhileTok Token      // Position of "while"
		LParen   Token      // Position of "("
		Cond     Expr       // Conditional expression
		RParen   Token      // Position of ")"
	}

	// A IfStmt represents a conditional statement.
	IfStmt struct {
		Tok     Token // Position of "if"
		LParen  Token
		Cond    Expr // Conditional expression
		RParen  Token
		Then    *BlockStmt // True branch
		ElseTok Token      // Position of "else" or nil
		Else    Stmt       // Else branch
	}

	// A SelectStmt represents a select statements.
	SelectStmt struct {
		Tok    Token         // Position of "select"
		Union  Token         // Position of "union" or nil
		Class  Token         // Position of "class" or nil
		Tag    *ParenExpr    // Tag expression
		LBrace Token         // Position of "{"
		Body   []*CaseClause // List of case clauses
		RBrace Token         // Position of "}"
	}

	// A CaseClause represents a case clause.
	CaseClause struct {
		Tok  Token      // Position of "case"
		Case *ParenExpr // nil means else-case
		Body *BlockStmt // Case body
	}

	// A CommClause represents communication clauses used by alt, interleave or check.
	CommClause struct {
		LBrack Token      // Position of '['
		X      Expr       // Conditional guard expression or nil
		Else   Token      // Else-clause of nil
		RBrack Token      // Position of ']'
		Comm   Stmt       // Communication statement
		Body   *BlockStmt // Body of nil
	}
)

func (x *BlockStmt) stmtNode()    {}
func (x *DeclStmt) stmtNode()     {}
func (x *ExprStmt) stmtNode()     {}
func (x *BranchStmt) stmtNode()   {}
func (x *ReturnStmt) stmtNode()   {}
func (x *RaiseStmt) stmtNode()    {}
func (x *CallStmt) stmtNode()     {}
func (x *AltStmt) stmtNode()      {}
func (x *ForStmt) stmtNode()      {}
func (x *ForRangeStmt) stmtNode() {}
func (x *WhileStmt) stmtNode()    {}
func (x *DoWhileStmt) stmtNode()  {}
func (x *IfStmt) stmtNode()       {}
func (x *SelectStmt) stmtNode()   {}
func (x *CaseClause) stmtNode()   {}
func (x *CommClause) stmtNode()   {}

// All nested types implement TypeSpec interface.
type TypeSpec interface {
	Node
	typeSpecNode()
}

type (
	// A Field represents a named struct member or sub type definition
	Field struct {
		DefaultTok       Token        // Position of "@default" or nil
		Type             TypeSpec     // Type
		Name             *Ident       // Name
		ArrayDef         []*ParenExpr // Array definitions
		TypePars         *FormalPars
		ValueConstraint  *ParenExpr  // Value constraint or nil
		LengthConstraint *LengthExpr // Length constraint or nil
		Optional         Token       // Position of "optional" or nil
	}

	// A RefSpec represents a type reference.
	RefSpec struct {
		X Expr
	}

	// A StructSpec represents a struct type specification.
	StructSpec struct {
		KindTok Token    // RECORD, SET, UNION
		LBrace  Token    // Position of "{"
		Fields  []*Field // Member list
		RBrace  Token    // Position of "}"
	}

	// A ListSpec represents a list type specification.
	ListSpec struct {
		KindTok  Token       // RECORD, SET
		Length   *LengthExpr // Length constraint or nil
		OfTok    Token       // Position of "of"
		ElemType TypeSpec    // Element type specification
	}

	// A MapSpec represents a map type specification.
	MapSpec struct {
		MapTok   Token
		FromTok  Token
		FromType TypeSpec
		ToTok    Token
		ToType   TypeSpec
	}

	// A EnumSpec represents a enumeration type specification.
	EnumSpec struct {
		Tok    Token  // Position of "enumerated"
		LBrace Token  // Position of "{"
		Enums  []Expr // Enum list
		RBrace Token  // Position of "}"
	}

	// A BehaviourSpec represents a behaviour type specification.
	BehaviourSpec struct {
		KindTok    Token       // TESTCASE, FUNCTION, ALTSTEP
		Interleave Token       // INTERLEAVE or nil
		Params     *FormalPars // Parameter list or nil
		RunsOn     *RunsOnSpec // runs on spec or nil
		System     *SystemSpec // system spec or nil
		Return     *ReturnSpec // return value spec or nil
	}
)

func (x *Field) typeSpecNode()         {}
func (x *RefSpec) typeSpecNode()       {}
func (x *StructSpec) typeSpecNode()    {}
func (x *ListSpec) typeSpecNode()      {}
func (x *MapSpec) typeSpecNode()       {}
func (x *EnumSpec) typeSpecNode()      {}
func (x *BehaviourSpec) typeSpecNode() {}

type (
	// A ValueDecl represents a value declaration.
	ValueDecl struct {
		KindTok             Token // VAR, CONST, TIMER, PORT, TEMPLATE, MODULEPAR
		TemplateRestriction *RestrictionSpec
		Modif               Token // "@lazy", "@fuzzy" or nil
		Type                Expr
		Decls               []*Declarator
		With                *WithSpec
	}

	// A Declarator represents a single varable declaration
	Declarator struct {
		Name      *Ident
		ArrayDef  []*ParenExpr
		AssignTok Token
		Value     Expr
	}

	TemplateDecl struct {
		*RestrictionSpec
		Modif       Token // "@lazy", "@fuzzy" or nil
		Type        Expr
		Name        *Ident
		TypePars    *FormalPars
		Params      *FormalPars
		ModifiesTok Token
		Base        Expr
		AssignTok   Token
		Value       Expr
		With        *WithSpec
	}

	// A ModuleParameterGroup represents a deprecated module parameter list
	ModuleParameterGroup struct {
		Tok    Token        // Position of "modulepar"
		LBrace Token        // Position of "{"
		Decls  []*ValueDecl // Module parameter list
		RBrace Token        // Position of "}"
		With   *WithSpec
	}

	// A FuncDecl represents a behaviour definition.
	FuncDecl struct {
		External     Token // Position of "external" or nil
		KindTok      Token // TESTCASE, ALTSTEP, FUNCTION
		Interleave   Token // INTERLEAVE or nil
		Name         *Ident
		Modif        Token // Position of "@deterministic" or nil
		TypePars     *FormalPars
		Params       *FormalPars    // Formal parameter list or nil
		RunsOn       *RunsOnSpec    // Optional runs-on-spec
		Mtc          *MtcSpec       // Optional mtc-spec
		System       *SystemSpec    // Optional system-spec
		Return       *ReturnSpec    // Optional return-spec
		ExceptionTok Token          // Position of "exception" or nil (OO exception clause)
		Exception    *ParenExpr     // Exception type list or nil
		Body         *BlockStmt     // Body or nil
		Catch        []*CatchClause // OO exception handlers after the body
		FinallyTok   Token          // Position of "finally" or nil
		Finally      *BlockStmt     // OO finally block or nil
		With         *WithSpec
	}

	// A CatchClause represents an object-oriented exception handler
	// attached after a function, altstep or testcase body:
	// `catch (Type [name]) { ... }` (ETSI ES 201 873-1 clause 5.2).
	CatchClause struct {
		CatchTok Token      // Position of "catch"
		LParen   Token      // Position of "("
		Type     Expr       // Caught exception type
		Var      *Ident     // Optional bound variable name or nil
		RParen   Token      // Position of ")"
		Body     *BlockStmt // Handler body
	}

	// A ConstructorDecl represents a class constructor definition.
	ConstructorDecl struct {
		Name     *Ident      // CREATE
		Params   *FormalPars // Formal parameter list
		ColonTok Token       // Position of ":" before the super-call or nil
		Init     Expr        // Super-constructor init call (`: Super(args)`) or nil
		Body     *BlockStmt  // Body
	}

	// A SignatureDecl represents a signature type for procedure based communication.
	SignatureDecl struct {
		Tok          Token // Position of "signature"
		Name         *Ident
		TypePars     *FormalPars
		Params       *FormalPars
		NoBlock      Token       // Optional "noblock"
		Return       *ReturnSpec // Optional return-spec
		ExceptionTok Token       // Position of "exeception" or nil
		Exception    *ParenExpr  // Exception list
		With         *WithSpec
	}

	// A SubTypeDecl represents a named sub type declaration
	SubTypeDecl struct {
		TypeTok Token  // Position of "type"
		Field   *Field // Field spec
		With    *WithSpec
	}

	// A StructTypeDecl represents a name struct type.
	StructTypeDecl struct {
		TypeTok  Token  // Position of "type"
		KindTok  Token  // RECORD, SET, UNION
		Name     *Ident // Name
		TypePars *FormalPars
		LBrace   Token    // Position of "{"
		Fields   []*Field // Member list
		RBrace   Token    // Position of }"
		With     *WithSpec
	}

	// A StructTypeDecl represents a name struct type.
	ClassTypeDecl struct {
		TypeTok     Token  // Position of "type"
		ExternalTok Token  // Position of "external" or nil
		KindTok     Token  // CLASS
		Modif       Token  // "@abstract" or nil
		Name        *Ident // Name
		ExtendsTok  Token
		Extends     []Expr
		RunsOn      *RunsOnSpec // Optional runs-on-spec
		Mtc         *MtcSpec    // Optional mtc-spec
		System      *SystemSpec // Optional system-spec
		LBrace      Token       // Position of "{"
		Defs        []*ModuleDef
		RBrace      Token      // Position of }"
		FinallyTok  Token      // Position of "finally" or nil
		Finally     *BlockStmt // Destructor body or nil
		With        *WithSpec
	}

	MapTypeDecl struct {
		TypeTok  Token
		Spec     *MapSpec
		Name     *Ident
		TypePars *FormalPars
		With     *WithSpec
	}

	// A EnumTypeDecl represents a named enum type.
	EnumTypeDecl struct {
		TypeTok  Token // Position of "type"
		EnumTok  Token // Position of "ENUMERATED"
		Name     *Ident
		TypePars *FormalPars
		LBrace   Token  // Position of "{"
		Enums    []Expr // Enum list
		RBrace   Token  // Position of "}"
		With     *WithSpec
	}

	// A BehaviourTypeDecl represents a named behaviour type.
	BehaviourTypeDecl struct {
		TypeTok    Token // Position of "type"
		KindTok    Token // TESTCASE, ALTSTEP, FUNCTION
		Interleave Token // INTERLEAVE or nil
		Name       *Ident
		TypePars   *FormalPars
		Params     *FormalPars // Formal parameter list
		RunsOn     *RunsOnSpec // Optional runs-on spec
		System     *SystemSpec // Optional system spec
		Return     *ReturnSpec // Optional return spec
		With       *WithSpec
	}

	PortTypeDecl struct {
		TypeTok  Token
		PortTok  Token
		Name     *Ident
		TypePars *FormalPars
		KindTok  Token // MIXED, MESSAGE, PROCEDURE
		Realtime Token
		LBrace   Token
		Attrs    []Node
		RBrace   Token
		With     *WithSpec
	}

	PortAttribute struct {
		KindTok Token // IN, OUT, INOUT, ADDRESS
		Types   []Expr
	}

	PortMapAttribute struct {
		MapTok   Token // MAP, UNMAP
		ParamTok Token
		Params   *FormalPars
	}

	ComponentTypeDecl struct {
		TypeTok    Token
		CompTok    Token
		Name       *Ident
		TypePars   *FormalPars
		ExtendsTok Token
		Extends    []Expr
		Body       *BlockStmt
		With       *WithSpec
	}
)

func (x *FuncDecl) IsControl() bool {
	return x.Modif != nil && x.Modif.String() == "@control"
}

func (x *FuncDecl) IsTest() bool {
	return x.KindTok.Kind() == TESTCASE
}

func (x *ValueDecl) declNode()            {}
func (x *Declarator) declNode()           {}
func (x *TemplateDecl) declNode()         {}
func (x *ModuleParameterGroup) declNode() {}
func (x *FuncDecl) declNode()             {}
func (x *SignatureDecl) declNode()        {}
func (x *SubTypeDecl) declNode()          {}
func (x *StructTypeDecl) declNode()       {}
func (x *ClassTypeDecl) declNode()        {}
func (x *MapTypeDecl) declNode()          {}
func (x *EnumTypeDecl) declNode()         {}
func (x *BehaviourTypeDecl) declNode()    {}
func (x *PortTypeDecl) declNode()         {}
func (x *PortAttribute) declNode()        {}
func (x *PortMapAttribute) declNode()     {}
func (x *ComponentTypeDecl) declNode()    {}

// ------------------------------------------------------------------------
// Modules and Module Definitions

type (
	Module struct {
		Tok      Token
		Name     *Ident
		Language *LanguageSpec
		LBrace   Token
		Defs     []*ModuleDef
		RBrace   Token
		With     *WithSpec
	}

	ModuleDef struct {
		Visibility Token
		Def        Node
	}

	ControlPart struct {
		Name *Ident
		Body *BlockStmt
		With *WithSpec
	}

	ImportDecl struct {
		ImportTok Token
		FromTok   Token
		Module    *Ident
		Language  *LanguageSpec
		LBrace    Token
		List      []*DefKindExpr
		RBrace    Token
		With      *WithSpec
	}

	GroupDecl struct {
		Tok    Token
		Name   *Ident
		LBrace Token
		Defs   []*ModuleDef
		RBrace Token
		With   *WithSpec
	}

	FriendDecl struct {
		FriendTok Token
		ModuleTok Token
		Module    *Ident
		With      *WithSpec
	}
)

// ------------------------------------------------------------------------
// Miscellaneous

type (
	LanguageSpec struct {
		Tok  Token
		List []Token
	}

	RestrictionSpec struct {
		TemplateTok Token
		LParen      Token
		Tok         Token
		RParen      Token
	}

	RunsOnSpec struct {
		RunsTok Token
		OnTok   Token
		Comp    Expr
	}

	SystemSpec struct {
		Tok  Token
		Comp Expr
	}

	MtcSpec struct {
		Tok  Token
		Comp Expr
	}

	ReturnSpec struct {
		Tok         Token
		Restriction *RestrictionSpec
		Modif       Token
		Type        Expr
	}

	FormalPars struct {
		LParen Token
		List   []*FormalPar
		RParen Token
	}

	FormalPar struct {
		Direction           Token
		TemplateRestriction *RestrictionSpec
		Modif               Token
		Modif2              Token
		Type                Expr
		Name                *Ident
		ArrayDef            []*ParenExpr
		AssignTok           Token
		Value               Expr
	}

	WithSpec struct {
		Tok    Token
		LBrace Token
		List   []*WithStmt
		RBrace Token
	}

	WithStmt struct {
		KindTok  Token
		Override Token
		LParen   Token
		List     []Expr
		RParen   Token
		Value    Expr
	}
)

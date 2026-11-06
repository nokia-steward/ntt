package t3xf

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"runtime/trace"
	"strconv"
	"strings"

	"github.com/nokia/ntt/internal/t3xf/opcode"

	"github.com/nokia/ntt/ttcn3/syntax"
)

type callKind uint8

const (
	callPlain callKind = iota
	callCollect
	callList
	callBlock
)

var universe = &scope{
	store: map[string]symbol{
		"log":         {op: opcode.LOG, call: callCollect},
		"int2str":     {op: opcode.INT2STR},
		"lengthof":    {op: opcode.LENGTHOF},
		"valueof":     {op: opcode.VALUEOF},
		"match":       {op: opcode.MATCH},
		"complement":  {op: opcode.COMPLEMENT, call: callList},
		"subset":      {op: opcode.SUBSET, call: callList},
		"superset":    {op: opcode.SUPERSET, call: callList},
		"permutation": {op: opcode.PERMUTATION, call: callList},
		"infinity":    {op: opcode.INFINITYP},
		"setverdict":  {op: opcode.SETVERDICT, call: callCollect},
		"execute":     {op: opcode.EXECUTE, callOp: opcode.EXECUTEL, call: callBlock},
		"activate":    {op: opcode.ACTIVATE, call: callBlock},
		"deactivate":  {op: opcode.DEACTIVATEA, callOp: opcode.DEACTIVATE},
		"isbound":     {op: opcode.ISBOUND, call: callBlock},
		"ispresent":   {op: opcode.ISPRESENT, call: callBlock},
		"ischosen":    {op: opcode.ISCHOSEN, call: callBlock},
		"isvalue":     {op: opcode.ISVALUE, call: callBlock},
		"getverdict":  {op: opcode.GETVERDICT},
		"action":      {op: opcode.ACTION},
		"self":        {op: opcode.SELF},
		"mtc":         {op: opcode.MTC},
		"system":      {op: opcode.SYSTEM},
		"stop":        {op: opcode.STOPI},
		"testcase": {scope: &scope{store: map[string]symbol{
			"stop": {op: opcode.TCSTOP, callOp: opcode.TCSTOP, call: callList},
		}}},
		"any timer": {scope: &scope{store: map[string]symbol{
			"timeout": {op: opcode.TIMEOUT1, step: true},
			"running": {op: opcode.RUNNING1T},
		}}},
		"all timer": {scope: &scope{store: map[string]symbol{
			"stop": {op: opcode.STOPAT},
		}}},
		"any port": {scope: &scope{store: map[string]symbol{
			"receive":    {op: opcode.RECEIVE1, callOp: opcode.RECEIVE1, step: true},
			"trigger":    {op: opcode.TRIGGER1, callOp: opcode.TRIGGER1, step: true},
			"check":      {op: opcode.CHECK1, callOp: opcode.CHECK1, step: true},
			"checkstate": {callOp: opcode.CHECKSTATEAN},
		}}},
		"all port": {scope: &scope{store: map[string]symbol{
			"start":      {op: opcode.STARTAP},
			"stop":       {op: opcode.STOPAP},
			"clear":      {op: opcode.CLEARA},
			"halt":       {op: opcode.HALTA},
			"checkstate": {callOp: opcode.CHECKSTATEAL},
		}}},
		"any component":        {op: opcode.ANY},
		"int2char":             {op: opcode.INT2CHAR},
		"int2bit":              {op: opcode.INT2BIT},
		"int2hex":              {op: opcode.INT2HEX},
		"int2oct":              {op: opcode.INT2OCT},
		"int2float":            {op: opcode.INT2FLOAT},
		"float2int":            {op: opcode.FLOAT2INT},
		"char2int":             {op: opcode.CHAR2INT},
		"char2oct":             {op: opcode.CHAR2OCT},
		"bit2int":              {op: opcode.BIT2INT},
		"bit2hex":              {op: opcode.BIT2HEX},
		"bit2oct":              {op: opcode.BIT2OCT},
		"bit2str":              {op: opcode.BIT2STR},
		"hex2int":              {op: opcode.HEX2INT},
		"hex2bit":              {op: opcode.HEX2BIT},
		"hex2oct":              {op: opcode.HEX2OCT},
		"hex2str":              {op: opcode.HEX2STR},
		"oct2int":              {op: opcode.OCT2INT},
		"oct2bit":              {op: opcode.OCT2BIT},
		"oct2hex":              {op: opcode.OCT2HEX},
		"oct2str":              {op: opcode.OCT2STR},
		"oct2char":             {op: opcode.OCT2CHR},
		"str2int":              {op: opcode.STR2INT},
		"str2oct":              {op: opcode.STR2OCT},
		"str2float":            {op: opcode.STR2FLOAT},
		"str2hex":              {op: opcode.STR2HEX},
		"enum2int":             {op: opcode.ENUM2INT},
		"int2enum":             {op: opcode.INT2ENUM},
		"substr":               {op: opcode.SUBSTR},
		"replace":              {op: opcode.REPLACE},
		"rnd":                  {op: opcode.RND},
		"sizeof":               {op: opcode.SIZEOF},
		"encvalue":             {op: opcode.ENCVALUE},
		"decvalue":             {op: opcode.DECVALUE},
		"testcasename":         {op: opcode.TESTCASENAME},
		"integer":              {op: opcode.INTEGER},
		"float":                {op: opcode.FLOAT},
		"bitstring":            {op: opcode.BITSTRING},
		"hexstring":            {op: opcode.HEXSTRING},
		"octetstring":          {op: opcode.OCTETSTRING},
		"boolean":              {op: opcode.BOOLEAN},
		"charstring":           {op: opcode.CHARSTRING},
		"universal charstring": {op: opcode.CHARSTRINGU},
		"verdicttype":          {op: opcode.VERDICTTYPE},
		"default":              {op: opcode.DEFAULT},
		"timer":                {op: opcode.TIMER},
	}}

// methods holds the operations applied to a timer or component with a dot.
var methods = &scope{
	store: map[string]symbol{
		"create":  {op: opcode.CREATE, callOp: opcode.CREATEN},
		"start":   {op: opcode.START, callOp: opcode.STARTD},
		"stop":    {op: opcode.STOP},
		"timeout": {op: opcode.TIMEOUT, step: true},
		"done":    {op: opcode.DONE, step: true},
		"read":    {op: opcode.READ},
		"running": {op: opcode.RUNNING},
	}}

var componentScope = &scope{
	parent: methods,
	store: map[string]symbol{
		"start": {callOp: opcode.STARTC, call: callBlock},
	},
}

var portScope = &scope{
	parent: methods,
	store: map[string]symbol{
		"send":       {callOp: opcode.SEND},
		"receive":    {op: opcode.RECEIVE, callOp: opcode.RECEIVE, step: true},
		"trigger":    {op: opcode.TRIGGER, callOp: opcode.TRIGGER, step: true},
		"check":      {op: opcode.CHECK, callOp: opcode.CHECK, step: true},
		"clear":      {op: opcode.CLEAR},
		"halt":       {op: opcode.HALT},
		"checkstate": {callOp: opcode.CHECKSTATE},
	},
}

type scope struct {
	parent *scope
	store  map[string]symbol
}

func newScope(parent *scope) *scope {
	return &scope{parent: parent, store: make(map[string]symbol)}
}

func (s *scope) lookup(name string) (symbol, bool) {
	if obj, ok := s.store[name]; ok {
		return obj, true
	}
	if s.parent != nil {
		if obj, ok := s.parent.lookup(name); ok {
			return obj, true
		}
	}
	return symbol{}, false
}

func (s *scope) define(name string, sym symbol) {
	s.store[name] = sym
}

type symbol struct {
	op     opcode.Opcode
	callOp opcode.Opcode
	call   callKind
	step   bool
	isType bool
	arg    any
	ref    Reference
	scope  *scope      // for struct and component types: holds field/method symbols
	typ    syntax.Node // declared type; resolves scope after forward declarations
	init   syntax.Expr // initializer of a var declared without a type
	params int         // formal parameter count of a template
}

type patchSymbol struct {
	id    *syntax.Ident
	name  string
	scope *scope
	addr  int
}

// Compiler implements a simplistic, single pass T3XF compiler for TTCN-3.
//
// It is mostly just a generator, since it does not perform any type checks or
// similar. Intended use is as a helper for testing k3r.
type Compiler struct {
	scope       *scope
	targetScope *scope
	cur         syntax.Node
	err         error
	e           *Encoder
	lastLine    int
	fields      map[string]int
	patches     []patchSymbol
	file        string
	module      string
	name        string // enclosing definition, for __SCOPE__
	temps       int
	nparams     map[string]int // template parameter counts, for forward modifies
}

func NewCompiler() *Compiler {
	return &Compiler{
		e:      NewEncoder(),
		fields: make(map[string]int),
		scope:  newScope(universe),
	}
}

// NewImageCompiler creates a compiler whose output can be loaded as a T3XF
// image.
func NewImageCompiler() *Compiler {
	c := NewCompiler()
	c.emit(opcode.NOP, 0)
	c.emit(opcode.NATLONG, 2)
	c.emit(opcode.VERSION, 0)
	return c
}

func (c *Compiler) Err() error {
	return c.err
}

// Assemble puts all compiled modules into one coherent T3XF file.
func (c *Compiler) Assemble() ([]byte, error) {
	region := trace.StartRegion(context.Background(), "t3xf.Compiler.Assemble")
	defer region.End()

	if c.err != nil {
		return nil, c.err
	}

	c.applyPatches()
	if c.err != nil {
		return nil, c.err
	}

	return c.e.Assemble()
}

func (c *Compiler) applyPatches() {
	for _, p := range c.patches {
		sym, ok := p.scope.lookup(p.name)
		if !ok {
			c.cur = p.id
			c.errorf("%q undefined", p.name)
			continue
		}
		c.e.Patch(p.addr, sym.op, sym.arg)
	}
}

// CompileImage parses the given TTCN-3 files and returns one T3XF image.
func CompileImage(files ...string) ([]byte, error) {
	c := NewImageCompiler()
	for _, file := range files {
		src, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		root := syntax.Parse(src, syntax.WithFilename(file))
		if err := root.Err(); err != nil {
			return nil, err
		}
		if err := c.Compile(root); err != nil {
			return nil, err
		}
	}
	return c.Assemble()
}

// Compile given TTCN-3 syntax tree and append resulting bytecode to the
// internal output buffer.
func (c *Compiler) Compile(n syntax.Node) error {
	region := trace.StartRegion(context.Background(), "t3xf.Compiler.Compile")
	defer region.End()
	c.lastLine = 0
	c.compile(n)
	return c.err
}

func (c *Compiler) compile(n syntax.Node) error {
	if n == nil {
		return nil
	}
	prev := c.cur
	c.cur = n
	defer func() { c.cur = prev }()

	if line := syntax.Begin(n).Line; line != c.lastLine {
		c.emit(opcode.LINE, line)
		c.lastLine = line
	}

	switch n := n.(type) {
	case *syntax.Root:
		c.file = n.Filename
		if n.Filename != "" {
			c.emit(opcode.SCAN, 0)
		}
		for _, child := range n.Nodes {
			c.compile(child)
		}
		if n.Filename != "" {
			c.emit(opcode.BLOCK, 0)
			c.emit(opcode.NAME, n.Filename)
			c.emit(opcode.SOURCE, 0)
		}

	case *syntax.Module:
		c.scope = newScope(c.scope)
		op := opcode.MODULE
		if attrs := n.With; attrs != nil {
			op = opcode.MODULEW
			c.compile(attrs)
		}
		c.module, c.name = n.Name.String(), n.Name.String()
		c.nparams = map[string]int{}
		for _, child := range n.Defs {
			td, ok := child.Def.(*syntax.TemplateDecl)
			if !ok || td.Name == nil {
				continue
			}
			if td.Params != nil {
				c.nparams[td.Name.String()] = len(td.Params.List)
			} else {
				c.nparams[td.Name.String()] = 0
			}
		}
		c.emit(opcode.SCAN, 0)
		// Types go first so that behaviour declared before a type sees its fields.
		// The module-wide address type applies to every port regardless of
		// declaration order, so make it available before other types.
		for _, child := range n.Defs {
			if isAddressTypeDecl(child.Def) {
				c.compile(child.Def)
			}
		}
		for _, types := range []bool{true, false} {
			for _, child := range n.Defs {
				if isAddressTypeDecl(child.Def) {
					continue
				}
				if isTypeDecl(child.Def) == types {
					c.compile(child.Def)
				}
			}
		}
		c.emit(opcode.BLOCK, 0)

		c.emit(opcode.NAME, n.Name.String())
		addr := c.emit(op, 0)
		c.scope = c.scope.parent
		c.scope.define(n.Name.String(), symbol{op: opcode.REF, arg: Reference(addr)})

	case *syntax.ModuleDef:
		c.compile(n.Def)

	case *syntax.FuncDecl:
		switch k := n.KindTok.Kind(); {
		case k == syntax.FUNCTION && n.External == nil:
			c.compileFunction(n)
		case k == syntax.FUNCTION && n.External != nil:
			c.compileExtFunc(n)
		case k == syntax.TESTCASE:
			c.compileTestcase(n)
		case k == syntax.ALTSTEP:
			c.compileAltstep(n)
		default:
			c.errorf("unsupported behaviour %s", k)
		}

	case *syntax.TemplateDecl:
		c.compileTemplate(n)

	case *syntax.ValueDecl:
		k := syntax.VAR
		if n.KindTok != nil {
			k = n.KindTok.Kind()
		}

		fn := c.compileVar
		switch k {
		case syntax.CONST:
			fn = c.compileConst
		case syntax.MODULEPAR:
			fn = c.compileModulePar
		}

		for _, decl := range n.Decls {
			fn(k, n.TemplateRestriction, n.Modif, n.Type, decl, n.With)
		}

	case *syntax.ControlPart:
		op := opcode.CONTROL
		if attrs := n.With; attrs != nil {
			op = opcode.CONTROLW
			c.compile(attrs)
		}
		leave := c.enter("control", nil)
		c.compile(n.Body)
		leave()

		addr := c.emit(op, 0)
		c.scope.define("control", symbol{op: opcode.REF, arg: Reference(addr)})

	case *syntax.BlockStmt:
		c.compileBlock(n, newScope(c.scope))

	case *syntax.ReturnStmt:
		if n.Result != nil {
			c.compile(n.Result)
		}
		c.emit(opcode.RETURN, 0)

	case *syntax.BranchStmt:
		switch n.Tok.Kind() {
		case syntax.BREAK:
			c.emit(opcode.BREAK, 0)
		case syntax.CONTINUE:
			c.emit(opcode.CONTINUE, 0)
		case syntax.REPEAT:
			c.emit(opcode.REPEAT, 0)
		case syntax.LABEL:
			c.scope.define("label "+n.Label.String(), symbol{op: opcode.GOTO, arg: Reference(c.e.Len())})
		case syntax.GOTO:
			c.emitRef(n.Label, "label "+n.Label.String(), opcode.GOTO)
		default:
			c.errorf("unsupported branch %s", n.Tok)
		}

	case *syntax.IfStmt:
		op := opcode.IF
		c.compile(n.Cond)
		c.compile(n.Then)
		if n.Else != nil {
			els := n.Else
			if _, ok := els.(*syntax.BlockStmt); !ok {
				els = &syntax.BlockStmt{Stmts: []syntax.Stmt{els}}
			}
			c.compile(els)
			op = opcode.IFELSE
		}
		c.emit(op, 0)

	case *syntax.WhileStmt:
		c.emit(opcode.SCAN, 0)
		c.compile(n.Cond)
		c.emit(opcode.BLOCK, 0)
		c.compile(n.Body)
		c.emit(opcode.WHILE, 0)

	case *syntax.DoWhileStmt:
		c.compile(n.Body)
		c.emit(opcode.SCAN, 0)
		c.compile(n.Cond)
		c.emit(opcode.BLOCK, 0)
		c.emit(opcode.DOWHILE, 0)

	case *syntax.ForStmt:
		c.scope = newScope(c.scope)
		c.emit(opcode.SCAN, 0)
		c.compile(n.Init)
		c.emit(opcode.BLOCK, 0)
		c.emit(opcode.SCAN, 0)
		c.compile(n.Cond)
		c.emit(opcode.BLOCK, 0)
		c.compile(n.Body)
		c.emit(opcode.SCAN, 0)
		c.compile(n.Post)
		c.emit(opcode.BLOCK, 0)
		c.emit(opcode.FOR, 0)
		c.scope = c.scope.parent

	case *syntax.ForRangeStmt:
		c.scope = newScope(c.scope)
		// mtc numbers a loop after the range-fors in its body, so the
		// inner loop gets the lower hidden-variable numbers.
		base := c.temps
		c.temps += rangeForTemps(n.Body)
		coll := func() { c.compile(n.Range) }
		if _, ok := n.Range.(*syntax.CallExpr); ok {
			tmp := c.tempVar("_collection", func() { c.compileTypeOf(n.Range) })
			c.compile(n.Range)
			c.emit(opcode.REF, tmp)
			c.emit(opcode.ASSIGN, 0)
			coll = func() { c.emit(opcode.REF, tmp) }
		}
		c.emit(opcode.SCAN, 0)
		coll()
		c.emit(opcode.LENGTHOF, 0)
		c.emit(opcode.BLOCK, 0)
		c.emit(opcode.INTEGER, 0)
		c.emit(opcode.NAME, c.tempName("_collection_len"))
		length := c.emit(opcode.CONST, 0)
		c.emit(opcode.SCAN, 0)
		i := c.tempVar("_i", func() { c.emit(opcode.INTEGER, 0) })
		c.emit(opcode.NATLONG, 0)
		c.emit(opcode.REF, i)
		c.emit(opcode.ASSIGN, 0)
		c.emit(opcode.BLOCK, 0)
		next := c.temps
		c.temps = base
		c.emit(opcode.SCAN, 0)
		c.emit(opcode.REF, i)
		c.emit(opcode.REF, Reference(length))
		c.emit(opcode.LT, 0)
		c.emit(opcode.BLOCK, 0)
		c.emit(opcode.SCAN, 0)
		target := syntax.Expr(nil)
		switch init := n.Init.(type) {
		case *syntax.DeclStmt:
			c.compile(init)
			if vd, ok := init.Decl.(*syntax.ValueDecl); ok && len(vd.Decls) == 1 {
				target = vd.Decls[0].Name
			}
		case *syntax.ExprStmt:
			target = init.Expr
		}
		if target == nil {
			c.errorf("unsupported range loop variable %T", n.Init)
		}
		coll()
		c.emit(opcode.REF, i)
		c.emit(opcode.GET, 0)
		c.assignTarget(target)
		c.emit(opcode.ASSIGN, 0)
		for _, stmt := range n.Body.Stmts {
			c.compile(stmt)
		}
		c.emit(opcode.BLOCK, 0)
		c.emit(opcode.SCAN, 0)
		c.emit(opcode.REF, i)
		c.emit(opcode.NATLONG, 1)
		c.emit(opcode.ADD, 0)
		c.emit(opcode.REF, i)
		c.emit(opcode.ASSIGN, 0)
		c.emit(opcode.BLOCK, 0)
		c.emit(opcode.FOR, 0)
		if c.temps < next {
			c.temps = next
		}
		c.scope = c.scope.parent

	case *syntax.SelectStmt:
		if n.Tag == nil {
			c.errorf("select expects one expression")
			break
		}
		c.compileCases(n, n.Body)

	case *syntax.ExprStmt:
		if _, ok := c.portComm(n.Expr); ok {
			c.emit(opcode.SCAN, 0)
			c.compileStep(n.Expr, nil)
			c.emit(opcode.BLOCK, 0)
			if n.NoDefault != nil {
				c.emit(opcode.ALTND, 0)
			} else {
				c.emit(opcode.ALT, 0)
			}
			break
		}
		if id, ok := n.Expr.(*syntax.Ident); ok && (id.String() == "disconnect" || id.String() == "unmap") {
			c.compilePortConfig(n.Expr)
			break
		}
		if _, ok := n.Expr.(*syntax.SelectorExpr); ok {
			if sym, ok := c.resolve(n.Expr); ok && sym.step {
				c.emit(opcode.SCAN, 0)
				c.compileStep(n.Expr, nil)
				c.emit(opcode.BLOCK, 0)
				c.emit(opcode.ALT, 0)
				break
			}
		}
		c.compile(n.Expr)

	case *syntax.AltStmt:
		c.emit(opcode.SCAN, 0)
		if n.Body != nil {
			for _, stmt := range n.Body.Stmts {
				c.compile(stmt)
			}
		}
		c.emit(opcode.BLOCK, 0)
		if n.Tok != nil && n.Tok.Kind() == syntax.INTERLEAVE {
			c.emit(opcode.INTERLEAVE, 0)
		} else if n.NoDefault != nil {
			c.emit(opcode.ALTND, 0)
		} else {
			c.emit(opcode.ALT, 0)
		}

	case *syntax.CommClause:
		if n.Else != nil {
			c.compile(n.Body)
			c.emit(opcode.ELSE, 0)
			break
		}
		es, ok := n.Comm.(*syntax.ExprStmt)
		if !ok {
			c.errorf("unsupported communication %T", n.Comm)
			break
		}
		if n.X != nil {
			c.emit(opcode.SCAN, 0)
			c.compile(n.X)
			c.emit(opcode.BLOCK, 0)
			c.emit(opcode.SPECPLC, 0)
			c.emit(opcode.SCAN, 0)
			c.compileStep(es.Expr, n.Body)
			c.emit(opcode.BLOCK, 0)
			c.emit(opcode.IF, 0)
			break
		}
		c.compileStep(es.Expr, n.Body)

	case *syntax.DeclStmt:
		c.compile(n.Decl)

	case *syntax.CallExpr:
		if c.compilePortCall(n) {
			break
		}
		var args []syntax.Expr
		if n.Args != nil {
			args = n.Args.List
		}
		sym, ok := c.resolve(n.Fun)
		sel, isSel := n.Fun.(*syntax.SelectorExpr)
		if isSel && (!ok || sym.callOp == 0 || len(args) != 1) {
			c.errorf("unsupported call %T", n.Fun)
			return nil
		}
		switch sym.call {
		case callPlain, callCollect, callList:
			collect := sym.call == callList || sym.call == callCollect && len(args) > 1
			if collect {
				c.emit(opcode.MARK, 0)
			}
			for i, arg := range args {
				var sc *scope
				if sym.op == opcode.MATCH && len(args) == 2 {
					sc = c.typeScope(args[1-i])
				}
				if from, ok := arg.(*syntax.FromExpr); ok && sym.op == opcode.PERMUTATION {
					c.compile(from.X)
					c.emit(opcode.ALLFROMP, 0)
					continue
				}
				c.compileScoped(arg, sc)
			}
			if collect {
				c.emit(opcode.COLLECT, 0)
			}
		case callBlock:
			if len(args) == 0 {
				c.errorf("missing argument")
				return nil
			}
			for _, arg := range args[1:] {
				c.compile(arg)
			}
			c.emit(opcode.SCAN, 0)
			c.compile(args[0])
			c.emit(opcode.BLOCK, 0)
		}
		if sel != nil {
			c.compileRecv(sel.X)
			c.emit(sym.callOp, 0)
		} else if sym.callOp != 0 && (len(args) > 1 || sym.call == callPlain) {
			c.emit(sym.callOp, 0)
		} else {
			c.compile(n.Fun)
		}

	case *syntax.ParenExpr:
		switch len(n.List) {
		case 0:
			c.errorf("empty parenthesized expression")
		case 1:
			c.compile(n.List[0])
		default:
			c.emit(opcode.MARK, 0)
			for _, x := range n.List {
				c.compile(x)
			}
			c.emit(opcode.COLLECT, 0)
		}

	case *syntax.UnaryExpr:
		switch n.Op.Kind() {
		case syntax.ADD:
			c.compile(n.X)
			c.emit(opcode.CLONE, 0)
		case syntax.SUB:
			if id, ok := n.X.(*syntax.Ident); ok && id.String() == "infinity" {
				c.emit(opcode.INFINITYN, 0)
				break
			}
			c.compile(n.X)
			c.emit(opcode.NEG, 0)
		case syntax.NOT, syntax.EXCL:
			c.compile(n.X)
			c.emit(opcode.NOT, 0)
		case syntax.IFPRESENT:
			c.compile(n.X)
			c.emit(opcode.IFPRESENT, 0)
		default:
			c.errorf("unsupported unary operator %s", n.Op)
		}

	case *syntax.PostExpr:
		c.compile(n.X)
		c.emit(opcode.NEG, 0)
		c.emit(opcode.NEG, 0)
		c.assignTarget(n.X)
		c.emit(opcode.NATLONG, 1)
		if n.Op.Kind() == syntax.INC {
			c.emit(opcode.ADD, 0)
		} else {
			c.emit(opcode.SUB, 0)
		}
		c.assignTarget(n.X)
		c.emit(opcode.ASSIGN, 0)
		c.emit(opcode.DROP, 0)

	case *syntax.BinaryExpr:
		if n.Op.Kind() == syntax.TO && c.compilePortSend(n) {
			return nil
		}
		switch n.Op.Kind() {
		case syntax.ASSIGN:
			c.compileScoped(n.Y, c.typeScope(n.X))
			c.assignTarget(n.X)
			c.emit(opcode.ASSIGN, 0)
			return nil
		case syntax.COLON:
			c.compileScoped(n.Y, c.typeScope(n.X))
			c.compile(n.X)
			c.emit(opcode.PERMITT, 0)
			c.emit(opcode.ASSIGND, 0)
			return nil
		}

		c.compile(n.X)
		// and/or leave a call or other compound right-hand side unevaluated
		// until the left-hand side is known. A literal or a variable is a
		// single value, which is what mtc pushes directly.
		if (n.Op.Kind() == syntax.AND || n.Op.Kind() == syntax.OR) && !plainOperand(n.Y) {
			c.emit(opcode.SCAN, 0)
			c.compile(n.Y)
			c.emit(opcode.BLOCK, 0)
		} else if n.Op.Kind() == syntax.EQ || n.Op.Kind() == syntax.NE {
			c.compileScoped(n.Y, c.typeScope(n.X))
			if _, ok := n.Y.(*syntax.CompositeLiteral); ok {
				if lhs, ok := c.resolve(n.X); ok && lhs.typ != nil {
					c.compile(lhs.typ)
					c.emit(opcode.PERMITT, 0)
					c.emit(opcode.ASSIGND, 0)
				}
			}
		} else {
			c.compile(n.Y)
		}
		switch n.Op.Kind() {
		case syntax.ADD:
			c.emit(opcode.ADD, 0)
		case syntax.SUB:
			c.emit(opcode.SUB, 0)
		case syntax.MUL:
			c.emit(opcode.MUL, 0)
		case syntax.DIV:
			c.emit(opcode.DIV, 0)
		case syntax.MOD:
			c.emit(opcode.MOD, 0)
		case syntax.REM:
			c.emit(opcode.REM, 0)
		case syntax.EQ:
			c.emit(opcode.EQ, 0)
		case syntax.NE:
			c.emit(opcode.NE, 0)
		case syntax.LT:
			c.emit(opcode.LT, 0)
		case syntax.GT:
			c.emit(opcode.GT, 0)
		case syntax.LE:
			c.emit(opcode.LE, 0)
		case syntax.GE:
			c.emit(opcode.GE, 0)
		case syntax.AND:
			c.emit(opcode.AND, 0)
		case syntax.OR:
			c.emit(opcode.OR, 0)
		case syntax.XOR:
			c.emit(opcode.XOR, 0)
		case syntax.CONCAT:
			c.emit(opcode.CAT, 0)
		case syntax.SHL:
			c.emit(opcode.SHL, 0)
		case syntax.SHR:
			c.emit(opcode.SHR, 0)
		case syntax.ROL:
			c.emit(opcode.ROL, 0)
		case syntax.ROR:
			c.emit(opcode.ROR, 0)
		case syntax.RANGE:
			c.emit(opcode.RANGE, 0)
		default:
			c.errorf("unsupported binary operator %s", n.Op)
		}

	case *syntax.ValueLiteral:
		switch n.Tok.Kind() {
		case syntax.INT:
			s := n.Tok.String()
			bi, ok := big.NewInt(0).SetString(s, 10)
			if !ok {
				c.errorf("invalid integer %s", s)
			}
			if i := bi.Int64(); bi.IsInt64() && math.MinInt32 <= i && i <= math.MaxInt32 {
				c.emit(opcode.NATLONG, int(i))
			} else {
				c.emit(opcode.ISTR, s)
			}

		case syntax.FLOAT:
			f, err := strconv.ParseFloat(n.Tok.String(), 64)
			if err != nil {
				c.errorf("invalid float %s", n.Tok)
			}
			c.emit(opcode.IEEE754DP, f)

		case syntax.STRING:
			// TTCN-3 has no backslash escapes outside patterns; mtc keeps them as written.
			s := n.Tok.String()
			c.emit(opcode.UTF8, strings.ReplaceAll(s[1:len(s)-1], `""`, `"`))

		case syntax.TRUE:
			c.emit(opcode.TRUE, 0)
		case syntax.FALSE:
			c.emit(opcode.FALSE, 0)

		case syntax.NULL:
			c.emit(opcode.NULL, 0)
		case syntax.ANY:
			c.emit(opcode.ANY, 0)
		case syntax.MUL:
			c.emit(opcode.ANYN, 0)
		case syntax.SUB:
			c.emit(opcode.SKIP, 0)
		case syntax.OMIT:
			c.emit(opcode.OMIT, 0)

		case syntax.ERROR:
			c.emit(opcode.ERROR, 0)
		case syntax.FAIL:
			c.emit(opcode.FAIL, 0)
		case syntax.INCONC:
			c.emit(opcode.INCONC, 0)
		case syntax.PASS:
			c.emit(opcode.PASS, 0)
		case syntax.NONE:
			c.emit(opcode.NONE, 0)

		case syntax.BSTRING:
			c.compileBinary(n.Tok.String())

		default:
			c.errorf("unsupported literal %s", n.Tok)
		}

	case *syntax.LengthExpr:
		c.compile(n.X)
		for _, x := range n.Size.List {
			c.compile(x)
		}
		c.emit(opcode.LENGTH, 0)

	case *syntax.PatternExpr:
		if n.NoCase != nil {
			c.errorf("@nocase not supported")
		}
		c.compile(n.X)
		c.emit(opcode.PATTERN, 0)

	case *syntax.RegexpExpr:
		if n.NoCase != nil {
			c.errorf("@nocase not supported")
		}
		if args, ok := n.X.(*syntax.ParenExpr); ok {
			for _, x := range args.List {
				c.compile(x)
			}
		} else {
			c.compile(n.X)
		}
		c.emit(opcode.REGEXP, 0)

	case *syntax.CompositeLiteral:
		if len(n.List) > 0 {
			if be, ok := n.List[0].(*syntax.BinaryExpr); ok && be.Op.Kind() == syntax.ASSIGN {
				c.emit(opcode.SCAN, 0)
				for _, elem := range n.List {
					be, ok := elem.(*syntax.BinaryExpr)
					if !ok || be.Op.Kind() != syntax.ASSIGN {
						c.errorf("expected assignment in composite literal")
						continue
					}
					if idx, ok := be.X.(*syntax.IndexExpr); ok && idx.X == nil {
						c.compile(be.Y)
						c.emit(opcode.VALUE, 0)
						if lit, ok := idx.Index.(*syntax.ValueLiteral); ok && lit.Tok.Kind() == syntax.INT {
							if i, err := strconv.Atoi(lit.Tok.String()); err == nil && 0 <= i && i <= 0xffff {
								c.emit(opcode.IDEF, i)
								c.emit(opcode.ASSIGN, 0)
								continue
							}
						}
						c.compile(idx.Index)
						c.emit(opcode.DEF, 0)
						c.emit(opcode.ASSIGN, 0)
					} else if id, ok := be.X.(*syntax.Ident); ok {
						var sym symbol
						if c.targetScope != nil {
							sym, _ = c.targetScope.lookup(id.String())
							sym = c.typed(sym)
						}
						if sym.op != opcode.IGET {
							c.errorf("unknown field %s", id)
							continue
						}
						c.compileScoped(be.Y, sym.scope)
						c.emit(opcode.VALUE, 0)
						c.emit(opcode.IDEF, sym.arg)
						c.emit(opcode.ASSIGN, 0)
					} else {
						c.errorf("unsupported composite literal element %T", be.X)
					}
				}
				c.emit(opcode.BLOCK, 0)
				break
			}
		}
		c.emit(opcode.MARK, 0)
		for _, elem := range n.List {
			c.compile(elem)
		}
		c.emit(opcode.VLIST, 0)

	case *syntax.RefSpec:
		c.compile(n.X)

	case *syntax.ParamExpr:
		if !c.compilePortConfig(n) {
			c.errorf("unsupported param expression")
		}

	case *syntax.FromExpr:
		if n.KindTok.Kind() != syntax.ALL {
			c.errorf("unsupported %s from", n.KindTok)
		}
		c.compile(n.X)
		c.emit(opcode.ALLFROM, 0)

	case *syntax.Ident:
		name := n.String()
		if c.targetScope != nil {
			if sym, ok := c.targetScope.lookup(name); ok && sym.op == opcode.REF {
				c.emit(sym.op, sym.arg)
				break
			}
		}
		switch name {
		case "__SCOPE__":
			c.emit(opcode.UTF8, c.name)
		case "__MODULE__":
			c.emit(opcode.UTF8, c.module)
		case "__FILE__":
			file := c.file
			if file != "" {
				if abs, err := filepath.Abs(file); err == nil {
					file = abs
				}
			}
			c.emit(opcode.UTF8, file)
		case "__BFILE__":
			c.emit(opcode.UTF8, filepath.Base(c.file))
		case "__LINE__":
			c.emit(opcode.NATLONG, syntax.Begin(n).Line)
		default:
			c.emitRef(n, name, opcode.REF)
		}

	case *syntax.WithSpec:
		c.emit(opcode.SCAN, 0)
		for _, child := range n.List {
			c.compile(child)
		}
		c.emit(opcode.BLOCK, 0)

	case *syntax.WithStmt:
		c.compile(n.Value)
		switch n.KindTok.Kind() {
		case syntax.EXTENSION:
			c.emit(opcode.EXTENSION, 0)
		case syntax.ENCODE:
			c.emit(opcode.ENCODE, 0)
		case syntax.VARIANT:
			c.emit(opcode.VARIANT, 0)
		default:
			c.errorf("unsupported attribute %s", n.KindTok.Kind())
		}

	case *syntax.FormalPars:
		if len(n.List) == 0 {
			c.emit(opcode.SKIP, 0)
			break
		}
		c.emit(opcode.SCAN, 0)
		for _, child := range n.List {
			c.compile(child)
		}
		c.emit(opcode.BLOCK, 0)

	case *syntax.FormalPar:
		c.compile(n.Type)
		if n.TemplateRestriction != nil {
			c.compile(n.TemplateRestriction)
		}
		c.emit(opcode.NAME, n.Name.String())
		dir := opcode.IN
		if n.Direction != nil {
			switch n.Direction.Kind() {
			case syntax.OUT:
				dir = opcode.OUT
			case syntax.INOUT:
				dir = opcode.INOUT
			}
		}
		addr := c.emit(dir, 0)
		c.scope.define(n.Name.String(), symbol{op: opcode.REF, arg: Reference(addr), typ: n.Type})

	case *syntax.RestrictionSpec:
		op := opcode.PERMITT
		if n.Tok != nil {
			switch n.Tok.Kind() {
			case syntax.VALUE:
				return nil
			case syntax.OMIT:
				op = opcode.PERMITO
			case syntax.PRESENT:
				op = opcode.PERMITP
			}
		}
		c.emit(op, 0)

	case *syntax.ReturnSpec:
		c.compile(n.Type)
		if n.Restriction != nil {
			c.compile(n.Restriction)
		}

	case *syntax.SelectorExpr:
		c.compileSelector(n, false)

	case *syntax.IndexExpr:
		c.compile(n.X)
		c.compile(n.Index)
		c.emit(opcode.GET, 0)

	case *syntax.StructTypeDecl:
		op := opcode.TYPE
		if n.With != nil {
			op = opcode.TYPEW
			c.compile(n.With)
		}
		structScope := c.compileStruct(n.KindTok.Kind(), n.Fields)
		c.emit(opcode.NAME, n.Name.String())
		addr := c.emit(op, 0)
		c.scope.define(n.Name.String(), symbol{op: opcode.REF, arg: Reference(addr), scope: structScope, isType: true})

	case *syntax.ComponentTypeDecl:
		op := opcode.TYPE
		if n.With != nil {
			op = opcode.TYPEW
			c.compile(n.With)
		}
		// The body scope holds the component's own and inherited definitions.
		body := newScope(c.scope)
		kind := opcode.COMPONENT
		if len(n.Extends) > 0 {
			kind = opcode.COMPONENTX
			c.emit(opcode.MARK, 0)
			for _, base := range n.Extends {
				c.compile(base)
				if sym, _ := c.resolve(base); sym.scope != nil {
					for name, s := range sym.scope.store {
						body.define(name, s)
					}
				}
			}
			c.emit(opcode.COLLECT, 0)
		}
		c.compileBlock(n.Body, body)
		c.emit(kind, 0)
		c.emit(opcode.NAME, n.Name.String())
		addr := c.emit(op, 0)
		c.scope.define(n.Name.String(), symbol{op: opcode.REF, arg: Reference(addr), scope: &scope{parent: componentScope, store: body.store}})

	case *syntax.SubTypeDecl:
		op := opcode.TYPE
		if n.With != nil {
			op = opcode.TYPEW
			c.compile(n.With)
		}
		sc := c.compileField(n.Field)
		c.emit(opcode.NAME, n.Field.Name.String())
		addr := c.emit(op, 0)
		c.scope.define(n.Field.Name.String(), symbol{op: opcode.REF, arg: Reference(addr), typ: n.Field.Type, scope: sc, isType: true})

	case *syntax.MapTypeDecl:
		op := opcode.TYPE
		if n.With != nil {
			op = opcode.TYPEW
			c.compile(n.With)
		}
		values := c.compileMapSpec(n.Spec)
		name := n.Name.String()
		c.emit(opcode.NAME, name)
		addr := c.emit(op, 0)
		// m.from and m.to take the set-of type of their result as operand.
		// Like a record of, the map's scope also resolves the fields of its values.
		sc := newScope(values)
		setof := func(field string, op opcode.Opcode, typ syntax.TypeSpec) {
			c.compile(typ)
			c.emit(opcode.SETOF, 0)
			c.emit(opcode.NAME, name+"."+field)
			sc.define(field, symbol{op: op, ref: Reference(c.emit(opcode.TYPE, 0))})
		}
		setof("from", opcode.FROM, n.Spec.FromType)
		setof("to", opcode.TO, n.Spec.ToType)
		c.scope.define(name, symbol{op: opcode.REF, arg: Reference(addr), scope: sc, isType: true})

	case *syntax.MapSpec:
		c.compileMapSpec(n)

	case *syntax.ListSpec:
		c.compileNestedType(n, nil, nil)

	case *syntax.StructSpec:
		c.compileStruct(n.KindTok.Kind(), n.Fields)

	case *syntax.EnumTypeDecl:
		op := opcode.TYPE
		if n.With != nil {
			op = opcode.TYPEW
			c.compile(n.With)
		}
		enumScope := c.compileEnum(n.Enums)
		c.emit(opcode.NAME, n.Name.String())
		addr := c.emit(op, 0)
		c.scope.define(n.Name.String(), symbol{op: opcode.REF, arg: Reference(addr), scope: enumScope, isType: true})

	case *syntax.EnumSpec:
		c.compileEnum(n.Enums)

	case *syntax.PortTypeDecl:
		if n.KindTok == nil || n.KindTok.Kind() != syntax.MESSAGE {
			c.errorf("unsupported port kind %v", n.KindTok)
			break
		}
		op := opcode.TYPE
		if n.With != nil {
			op = opcode.TYPEW
			c.compile(n.With)
		}
		var in, out []syntax.Expr
		var address syntax.Expr
		var mapParams, unmapParams *syntax.FormalPars
		for _, a := range n.Attrs {
			switch a := a.(type) {
			case *syntax.PortMapAttribute:
				if a.MapTok.Kind() == syntax.MAP {
					mapParams = a.Params
				} else {
					unmapParams = a.Params
				}
			case *syntax.PortAttribute:
				switch a.KindTok.Kind() {
				case syntax.IN:
					in = append(in, a.Types...)
				case syntax.OUT:
					out = append(out, a.Types...)
				case syntax.INOUT:
					in = append(in, a.Types...)
					out = append(out, a.Types...)
				case syntax.ADDRESS:
					if len(a.Types) == 1 {
						address = a.Types[0]
					} else {
						c.errorf("address expects one type")
					}
				default:
					c.errorf("unsupported port attribute %s", a.KindTok)
				}
			default:
				c.errorf("unsupported port attribute %T", a)
			}
		}
		for _, list := range [][]syntax.Expr{in, out} {
			if len(list) == 0 {
				c.emit(opcode.SKIP, 0)
			} else {
				c.emit(opcode.SCAN, 0)
				for _, t := range list {
					if id, ok := t.(*syntax.Ident); ok && id.String() == "all" {
						// T3XF has no "all message types" type object.
						// default is not a legal message type and serves as
						// the wildcard sentinel understood by this runtime.
						c.emit(opcode.DEFAULT, 0)
					} else {
						c.compile(t)
					}
				}
				c.emit(opcode.BLOCK, 0)
			}
		}
		addressSym, globalAddress := c.scope.lookup("address")
		if address != nil {
			c.compile(address)
		} else if globalAddress && addressSym.isType {
			c.emit(addressSym.op, addressSym.arg)
		}
		compileParams := func(params *syntax.FormalPars) {
			if params == nil {
				c.emit(opcode.SKIP, 0)
				return
			}
			scope := c.scope
			c.scope = newScope(scope)
			c.compile(params)
			c.scope = scope
		}
		compileParams(mapParams)
		compileParams(unmapParams)
		hasAddress := address != nil || globalAddress && addressSym.isType
		if hasAddress {
			c.emit(opcode.PORTMA, 0)
		} else {
			c.emit(opcode.PORTM, 0)
		}
		name := n.Name.String()
		c.emit(opcode.NAME, name)
		addr := c.emit(op, 0)
		sc := newScope(portScope)
		if hasAddress {
			if address != nil {
				c.compile(address)
			} else {
				c.emit(addressSym.op, addressSym.arg)
			}
			c.emit(opcode.NAME, name+".address")
			addressRef := c.emit(opcode.TYPE, 0)
			addressScope := addressSym.scope
			var addressType syntax.Node
			if address != nil {
				addressScope = c.typeScope(address)
				addressType = address
			}
			sc.define("address", symbol{
				op: opcode.REF, arg: Reference(addressRef), isType: true,
				scope: addressScope, typ: addressType,
			})
		}
		c.scope.define(name, symbol{op: opcode.REF, arg: Reference(addr), scope: sc, isType: true})

	default:
		c.errorf("unexpected node type %T", n)
	}

	return nil
}

func (c *Compiler) resolve(n syntax.Node) (symbol, bool) {
	switch n := n.(type) {
	case *syntax.ReturnSpec:
		if n != nil {
			return c.resolve(n.Type)
		}
	case *syntax.RefSpec:
		return c.resolve(n.X)
	case *syntax.ListSpec:
		return c.resolve(n.ElemType)
	case *syntax.IndexExpr:
		return c.resolve(n.X)
	case *syntax.Ident:
		sym, ok := c.scope.lookup(n.String())
		return c.typed(sym), ok
	case *syntax.CallExpr:
		fun, _ := c.resolve(n.Fun)
		if fun.op == opcode.VALUEOF && n.Args != nil && len(n.Args.List) == 1 {
			fun, _ = c.resolve(n.Args.List[0])
		}
		return symbol{scope: fun.scope, typ: fun.typ}, true
	case *syntax.SelectorExpr:
		sel, ok := n.Sel.(*syntax.Ident)
		if !ok {
			return symbol{}, false
		}
		recv, _ := c.resolve(n.X)
		sc := recv.scope
		if sc == nil {
			sc = methods
		}
		sym, ok := sc.lookup(sel.String())
		if !ok {
			return symbol{}, false
		}
		if sym.op != opcode.IGET && sym.scope == nil {
			sym.scope = recv.scope
		}
		sym.isType = recv.isType
		return c.typed(sym), true
	}
	return symbol{}, false
}

// typed fills in the scope of a symbol from its declared type.
func (c *Compiler) typed(sym symbol) symbol {
	if sym.scope == nil && sym.typ != nil {
		sym.scope = c.typeScope(sym.typ)
	}
	return sym
}

func (c *Compiler) compileSelector(n *syntax.SelectorExpr, def bool) {
	c.cur = n
	sym, ok := c.resolve(n)
	if !ok || sym.op == 0 && sym.arg == nil {
		c.errorf("unsupported selector %s", n.Sel)
		return
	}
	if sym.isType && sym.op == opcode.REF {
		c.emit(sym.op, sym.arg)
		return
	}
	if sym.isType && sym.op == opcode.IGET {
		c.emit(opcode.REF, sym.ref)
		return
	}
	switch {
	case def && sym.op == opcode.IGET:
		c.assignTarget(n.X)
		sym.op = opcode.IDEF
	case sym.op == opcode.IGET:
		c.compile(n.X)
	default:
		c.compileRecv(n.X)
		if sym.ref != 0 {
			c.emit(opcode.REF, sym.ref) // the result type of map.from and map.to
		}
		if sym.call == callList {
			c.emit(opcode.SKIP, 0) // omitted argument list
		}
	}
	c.emit(sym.op, sym.arg)
}

// compileRecv compiles the receiver of a method. Receivers such as
// `any timer` or `testcase` are names only and emit nothing.
func (c *Compiler) compileRecv(x syntax.Expr) {
	if id, ok := x.(*syntax.Ident); ok {
		if sym, ok := c.scope.lookup(id.String()); ok && sym.op == 0 && sym.arg == nil {
			return
		}
	}
	c.compile(x)
}

func (c *Compiler) assignTarget(e syntax.Expr) {
	switch e := e.(type) {
	case *syntax.SelectorExpr:
		c.compileSelector(e, true)
	case *syntax.IndexExpr:
		c.assignTarget(e.X)
		c.compile(e.Index)
		c.emit(opcode.DEF, 0)
	default:
		c.compile(e)
	}
}

func (c *Compiler) typeScope(typ syntax.Node) *scope {
	sym, _ := c.resolve(typ)
	return sym.scope
}

func (c *Compiler) compileScoped(n syntax.Node, sc *scope) {
	prev := c.targetScope
	c.targetScope = sc
	c.compile(n)
	c.targetScope = prev
}

func (c *Compiler) compileStep(expr syntax.Expr, body *syntax.BlockStmt) {
	c.emit(opcode.SCAN, 0)
	if spec, ok := c.portComm(expr); ok {
		c.compilePortComm(spec)
	} else {
		c.compile(expr)
	}
	c.emit(opcode.BLOCK, 0)
	if body == nil {
		c.emit(opcode.SKIP, 0)
	} else {
		c.compile(body)
	}
	c.emit(opcode.STEP, 0)
}

type portCommSpec struct {
	op       opcode.Opcode
	receiver syntax.Expr
	match    syntax.Expr
	from     syntax.Expr
	redirect *syntax.RedirectExpr
	any      bool
	anyFrom  string
}

func selectorOperation(expr syntax.Expr) (receiver syntax.Expr, name string, args []syntax.Expr, ok bool) {
	switch expr := expr.(type) {
	case *syntax.CallExpr:
		if expr.Args != nil {
			args = expr.Args.List
		}
		switch fun := expr.Fun.(type) {
		case *syntax.SelectorExpr:
			id, yes := fun.Sel.(*syntax.Ident)
			if !yes {
				return nil, "", nil, false
			}
			return fun.X, id.String(), args, true
		case *syntax.Ident:
			return nil, fun.String(), args, true
		}
	case *syntax.SelectorExpr:
		id, yes := expr.Sel.(*syntax.Ident)
		if yes {
			return expr.X, id.String(), nil, true
		}
	case *syntax.Ident:
		return nil, expr.String(), nil, true
	}
	return nil, "", nil, false
}

func peelPortComm(expr syntax.Expr) (base, from syntax.Expr, redirect *syntax.RedirectExpr, anyFrom bool) {
	base = expr
	if r, ok := base.(*syntax.RedirectExpr); ok {
		redirect, base = r, r.X
	}
	if f, ok := base.(*syntax.FromExpr); ok && f.KindTok.Kind() == syntax.ANYKW {
		anyFrom, base = true, f.X
	}
	if b, ok := base.(*syntax.BinaryExpr); ok && b.Op.Kind() == syntax.FROM {
		base = b.X
		var r *syntax.RedirectExpr
		from, r = detachRedirect(b.Y)
		if r != nil {
			if redirect == nil {
				redirect = r
			}
		}
	}
	return
}

func detachRedirect(expr syntax.Expr) (syntax.Expr, *syntax.RedirectExpr) {
	switch expr := expr.(type) {
	case *syntax.RedirectExpr:
		return expr.X, expr
	case *syntax.BinaryExpr:
		right, redirect := detachRedirect(expr.Y)
		if redirect == nil {
			return expr, nil
		}
		copy := *expr
		copy.Y = right
		return &copy, redirect
	default:
		return expr, nil
	}
}

func (c *Compiler) portComm(expr syntax.Expr) (portCommSpec, bool) {
	base, from, redirect, anyFrom := peelPortComm(expr)
	receiver, name, args, ok := selectorOperation(base)
	if !ok {
		return portCommSpec{}, false
	}
	spec := portCommSpec{receiver: receiver, from: from, redirect: redirect, any: anyFrom}
	if anyFrom {
		spec.anyFrom = portBaseName(receiver)
	}
	switch name {
	case "receive":
		spec.op = opcode.RECEIVE
	case "trigger":
		spec.op = opcode.TRIGGER
	case "check":
		spec.op = opcode.CHECK
	default:
		return portCommSpec{}, false
	}
	if len(args) > 0 {
		if name == "check" {
			inner, innerFrom, innerRedirect, _ := peelPortComm(args[0])
			_, innerName, innerArgs, innerOK := selectorOperation(inner)
			if innerOK && innerName == "receive" {
				spec.op = opcode.RECEIVEC
				if len(innerArgs) > 0 {
					spec.match = innerArgs[0]
				}
				if innerFrom != nil {
					spec.from = innerFrom
				}
				if innerRedirect != nil {
					spec.redirect = innerRedirect
				}
			} else {
				if innerFrom != nil {
					spec.from = innerFrom
				}
				if innerRedirect != nil {
					spec.redirect = innerRedirect
				}
			}
		} else {
			spec.match = args[0]
		}
	}
	if id, ok := receiver.(*syntax.Ident); ok && id.String() == "any port" {
		spec.any = true
	}
	if spec.any {
		switch spec.op {
		case opcode.RECEIVE:
			spec.op = opcode.RECEIVE1
		case opcode.RECEIVEC:
			spec.op = opcode.RECEIVEC1
		case opcode.TRIGGER:
			spec.op = opcode.TRIGGER1
		case opcode.CHECK:
			spec.op = opcode.CHECK1
		}
	}
	return spec, true
}

func (c *Compiler) compilePortComm(spec portCommSpec) {
	if spec.match == nil && spec.from == nil && spec.anyFrom == "" {
		c.emit(opcode.SKIP, 0)
	} else {
		c.emit(opcode.SCAN, 0)
		matched := false
		if spec.match != nil {
			c.emit(opcode.VALUE, 0)
			c.emit(opcode.SCAN, 0)
			c.compile(spec.match)
			c.emit(opcode.BLOCK, 0)
			c.emit(opcode.SPECPLC, 0)
			c.emit(opcode.MATCH, 0)
			matched = true
		}
		if spec.from != nil {
			c.emit(opcode.SENDER, 0)
			c.compile(spec.from)
			c.emit(opcode.MATCH, 0)
			if matched {
				c.emit(opcode.AND, 0)
			}
			matched = true
		}
		if spec.anyFrom != "" {
			c.emit(opcode.NAME, "__port_from:"+spec.anyFrom)
			c.emit(opcode.TIMESTAMP, 0)
			if matched {
				c.emit(opcode.AND, 0)
			}
		}
		c.emit(opcode.BLOCK, 0)
	}

	redirect := spec.redirect
	if redirect == nil || len(redirect.Value) == 0 && redirect.Sender == nil && redirect.Index == nil && redirect.Timestamp == nil {
		c.emit(opcode.SKIP, 0)
	} else {
		c.emit(opcode.SCAN, 0)
		assign := opcode.ASSIGN
		if spec.op == opcode.RECEIVE || spec.op == opcode.RECEIVE1 {
			assign = opcode.MOVE
		}
		values := redirect.Value
		if len(values) == 1 {
			if list, ok := values[0].(*syntax.ParenExpr); ok {
				values = list.List
			}
		}
		for _, target := range values {
			c.emit(opcode.VALUE, 0)
			if field, ok := target.(*syntax.BinaryExpr); ok && field.Op.Kind() == syntax.ASSIGN {
				source := field.Y
				if decoded, ok := source.(*syntax.DecodedExpr); ok {
					c.compileCommField(spec.match, decoded.X)
					c.assignTarget(field.X)
					c.emit(opcode.DECVALUE, 0)
					c.emit(opcode.DROP, 0)
					continue
				}
				c.compileCommField(spec.match, source)
				target = field.X
			}
			c.assignTarget(target)
			c.emit(assign, 0)
		}
		if redirect.Sender != nil {
			c.emit(opcode.SENDER, 0)
			c.assignTarget(redirect.Sender)
			c.emit(opcode.ASSIGN, 0)
		}
		if redirect.Index != nil {
			// Current T3XF has no dedicated @index opcode. The marker is
			// consumed by TIMESTAMP and interpreted by this VM as the
			// selected port's array index.
			c.emit(opcode.NAME, "__port_index")
			c.emit(opcode.TIMESTAMP, 0)
			c.assignTarget(redirect.Index)
			c.emit(opcode.ASSIGN, 0)
		}
		if redirect.Timestamp != nil {
			c.emit(opcode.TIMESTAMP, 0)
			c.assignTarget(redirect.Timestamp)
			c.emit(opcode.ASSIGN, 0)
		}
		c.emit(opcode.BLOCK, 0)
	}
	if !spec.any {
		c.compileRecv(spec.receiver)
	} else if spec.anyFrom != "" {
		c.emit(opcode.NAME, "__port_from:"+spec.anyFrom)
	}
	c.emit(spec.op, 0)
}

func portBaseName(expr syntax.Expr) string {
	switch expr := expr.(type) {
	case *syntax.Ident:
		return expr.String()
	case *syntax.IndexExpr:
		return portBaseName(expr.X)
	}
	return ""
}

func (c *Compiler) compileCommField(match, field syntax.Expr) {
	if typed, ok := match.(*syntax.BinaryExpr); ok && typed.Op.Kind() == syntax.COLON {
		if scope := c.typeScope(typed.X); scope != nil {
			if c.compileCommFieldIn(scope, field) {
				return
			}
		}
	}
	c.errorf("unsupported communication field redirect %T", field)
}

func (c *Compiler) compileCommFieldIn(scope *scope, field syntax.Expr) bool {
	switch field := field.(type) {
	case *syntax.Ident:
		sym, ok := scope.lookup(field.String())
		if !ok || sym.op != opcode.IGET {
			return false
		}
		c.emit(sym.op, sym.arg)
		return true
	case *syntax.IndexExpr:
		if !c.compileCommFieldIn(scope, field.X) {
			return false
		}
		c.compile(field.Index)
		c.emit(opcode.GET, 0)
		return true
	}
	return false
}

func (c *Compiler) compilePortCall(call *syntax.CallExpr) bool {
	if c.compilePortConfig(call) {
		return true
	}
	receiver, name, args, ok := selectorOperation(call)
	if !ok {
		return false
	}
	switch name {
	case "send":
		return c.compilePortSend(call)
	case "checkstate":
		if len(args) != 1 {
			c.errorf("checkstate expects one argument")
			return true
		}
		if id, ok := receiver.(*syntax.Ident); ok {
			switch id.String() {
			case "any port":
				c.compile(args[0])
				c.emit(opcode.CHECKSTATEAN, 0)
				return true
			case "all port":
				c.compile(args[0])
				c.emit(opcode.CHECKSTATEAL, 0)
				return true
			}
		}
		c.compile(receiver)
		c.compile(args[0])
		c.emit(opcode.CHECKSTATE, 0)
		return true
	}
	return false
}

func (c *Compiler) compilePortSend(expr syntax.Expr) bool {
	base := expr
	var destination syntax.Expr
	if binary, ok := expr.(*syntax.BinaryExpr); ok {
		if binary.Op.Kind() != syntax.TO {
			return false
		}
		base, destination = binary.X, binary.Y
	}
	receiver, name, args, ok := selectorOperation(base)
	if !ok || name != "send" {
		return false
	}
	if len(args) != 1 {
		c.errorf("send expects one message")
		return true
	}
	op := opcode.SEND
	if id, ok := destination.(*syntax.Ident); ok && id.String() == "all component" {
		op = opcode.SENDA
		destination = nil
	} else if list, ok := destination.(*syntax.ParenExpr); ok {
		c.emit(opcode.MARK, 0)
		for _, item := range list.List {
			c.compile(item)
		}
		c.emit(opcode.COLLECT, 0)
		op = opcode.SENDN
	} else if destination != nil {
		c.compile(destination)
		op = opcode.SEND1
	}
	c.compile(args[0])
	c.compile(receiver)
	c.emit(op, 0)
	return true
}

func (c *Compiler) compilePortConfig(expr syntax.Expr) bool {
	base := expr
	var actuals []syntax.Expr
	if params, ok := expr.(*syntax.ParamExpr); ok {
		base = params.X
		if list, ok := params.Y.(*syntax.ParenExpr); ok {
			actuals = list.List
		}
	}
	var name string
	var args []syntax.Expr
	switch base := base.(type) {
	case *syntax.Ident:
		name = base.String()
	case *syntax.CallExpr:
		id, ok := base.Fun.(*syntax.Ident)
		if !ok {
			return false
		}
		name = id.String()
		if base.Args != nil {
			args = base.Args.List
		}
	default:
		return false
	}
	if name != "connect" && name != "disconnect" && name != "map" && name != "unmap" {
		return false
	}
	emitActuals := func() {
		c.emit(opcode.MARK, 0)
		for _, actual := range actuals {
			c.compile(actual)
		}
		c.emit(opcode.VLIST, 0)
	}
	switch name {
	case "connect":
		if len(args) != 2 {
			c.errorf("connect expects two ports")
			return true
		}
		c.compilePortRef(args[0])
		c.compilePortRef(args[1])
		c.emit(opcode.CONNECT, 0)
	case "map":
		if len(args) != 2 {
			c.errorf("map expects two ports")
			return true
		}
		c.compilePortRef(args[0])
		c.compilePortRef(args[1])
		emitActuals()
		c.emit(opcode.MAP, 0)
	case "disconnect", "unmap":
		mapped := name == "unmap"
		switch len(args) {
		case 0:
			c.emit(opcode.SELF, 0)
			if mapped {
				emitActuals()
				c.emit(opcode.UNMAPA, 0)
			} else {
				c.emit(opcode.DISCONNECTA, 0)
			}
		case 1:
			component, port, ok := splitPortRef(args[0])
			if ok {
				if componentID, yes := component.(*syntax.Ident); yes && componentID.String() == "all component" {
					if portID, yes := port.(*syntax.Ident); yes && portID.String() == "all port" {
						if mapped {
							c.emit(opcode.UNMAPAA, 0)
						} else {
							c.emit(opcode.DISCONNECTAA, 0)
						}
						return true
					}
				}
				if portID, yes := port.(*syntax.Ident); yes && portID.String() == "all port" {
					c.compile(component)
				} else {
					c.compilePortRef(args[0])
				}
			} else {
				c.compile(args[0])
			}
			if mapped {
				emitActuals()
				c.emit(opcode.UNMAPA, 0)
			} else {
				c.emit(opcode.DISCONNECTA, 0)
			}
		case 2:
			c.compilePortRef(args[0])
			c.compilePortRef(args[1])
			if mapped {
				emitActuals()
				c.emit(opcode.UNMAP, 0)
			} else {
				c.emit(opcode.DISCONNECT, 0)
			}
		default:
			c.errorf("%s expects zero, one, or two port arguments", name)
		}
	}
	return true
}

func splitPortRef(expr syntax.Expr) (component, port syntax.Expr, ok bool) {
	binary, ok := expr.(*syntax.BinaryExpr)
	if !ok || binary.Op.Kind() != syntax.COLON {
		return nil, nil, false
	}
	return binary.X, binary.Y, true
}

func (c *Compiler) compilePortRef(expr syntax.Expr) {
	component, port, ok := splitPortRef(expr)
	if !ok {
		c.compile(expr)
		return
	}
	c.compile(component)
	c.compilePortMember(port)
}

func (c *Compiler) compilePortMember(expr syntax.Expr) {
	switch expr := expr.(type) {
	case *syntax.Ident:
		c.emit(opcode.NAME, expr.String())
		c.emit(opcode.GET, 0)
	case *syntax.IndexExpr:
		c.compilePortMember(expr.X)
		c.compile(expr.Index)
		c.emit(opcode.GET, 0)
	default:
		c.errorf("unsupported port reference %T", expr)
	}
}

func (c *Compiler) compileStruct(k syntax.Kind, fields []*syntax.Field) *scope {
	s := newScope(nil)
	if len(fields) == 0 {
		c.emit(opcode.SKIP, 0)
	} else {
		c.emit(opcode.SCAN, 0)
		for i, field := range fields {
			nested := c.compileField(field)
			c.emit(opcode.NAME, field.Name.String())
			var addr int
			idx := i
			switch {
			case field.Optional != nil:
				addr = c.emit(opcode.FIELDO, 0)
			case k == syntax.UNION:
				idx = c.fieldIndex(field.Name.String())
				addr = c.emit(opcode.IFIELD, idx)
			default:
				addr = c.emit(opcode.FIELD, 0)
			}
			s.define(field.Name.String(), symbol{
				op:    opcode.IGET,
				arg:   idx,
				ref:   Reference(addr),
				scope: nested,
				typ:   field.Type,
			})
		}
		c.emit(opcode.BLOCK, 0)
	}
	switch k {
	case syntax.RECORD:
		c.emit(opcode.RECORD, 0)
	case syntax.SET:
		c.emit(opcode.SET, 0)
	case syntax.UNION:
		c.emit(opcode.UNION, 0)
	default:
		c.errorf("unsupported struct type %s", k)
	}
	return s
}

func (c *Compiler) compileEnum(enums []syntax.Expr) *scope {
	s := newScope(nil)
	c.emit(opcode.SCAN, 0)
	for i, e := range enums {
		id, ok := e.(*syntax.Ident)
		if !ok {
			c.errorf("unsupported enum item %T", e)
			continue
		}
		name := id.String()
		c.emit(opcode.NATLONG, i)
		c.emit(opcode.NAME, name)
		addr := c.emit(opcode.TERM, 0)
		sym := symbol{op: opcode.REF, arg: Reference(addr), scope: s}
		s.define(name, sym)
		if _, exists := c.scope.store[name]; !exists {
			c.scope.define(name, sym)
		}
	}
	c.emit(opcode.BLOCK, 0)
	c.emit(opcode.ENUMERATED, 0)
	return s
}

// compileMapSpec emits a map type and returns the field scope of its values.
func (c *Compiler) compileMapSpec(n *syntax.MapSpec) *scope {
	var values *scope
	for _, t := range []syntax.TypeSpec{n.FromType, n.ToType} {
		c.emit(opcode.SCAN, 0)
		values = c.compileNestedType(t, nil, nil)
		c.emit(opcode.BLOCK, 0)
	}
	c.emit(opcode.MAPT, 0)
	if values == nil {
		values = c.typeScope(n.ToType)
	}
	return values
}

// compileField emits a field type and returns the field scope of an inline
// struct type.
func (c *Compiler) compileField(f *syntax.Field) *scope {
	sc := c.compileNestedType(f.Type, f.ValueConstraint, f.LengthConstraint)
	c.compileArrayDef(f.ArrayDef)
	return sc
}

func (c *Compiler) compileNestedType(ty syntax.TypeSpec, vc *syntax.ParenExpr, le *syntax.LengthExpr) *scope {
	if ls, ok := ty.(*syntax.ListSpec); ok {
		sc := c.compileNestedType(ls.ElemType, vc, le)
		switch ls.KindTok.Kind() {
		case syntax.RECORD:
			c.emit(opcode.RECORDOF, 0)
		case syntax.SET:
			c.emit(opcode.SETOF, 0)
		default:
			c.errorf("unsupported list type %s", ls.KindTok.Kind())

		}
		if ls.Length != nil {
			c.compileSubtype(nil, ls.Length)
		}
		return sc
	}

	var sc *scope
	if ss, ok := ty.(*syntax.StructSpec); ok {
		sc = c.compileStruct(ss.KindTok.Kind(), ss.Fields)
	} else {
		c.compile(ty)
	}
	if vc != nil || le != nil {
		c.compileSubtype(vc, le)
	}
	return sc
}

func (c *Compiler) compileSubtype(vc *syntax.ParenExpr, le *syntax.LengthExpr) {
	c.emit(opcode.SCAN, 0)
	if vc == nil {
		c.emit(opcode.ANY, 0)
	} else {
		c.compile(vc)
	}
	if le != nil {
		c.compile(le)
	}
	c.emit(opcode.BLOCK, 0)
	c.emit(opcode.SUBTYPE, 0)
}

func (c *Compiler) compileTemplate(n *syntax.TemplateDecl) error {
	if n.Modif != nil {
		c.errorf("modifiers not supported")
	}
	if n.With != nil {
		c.errorf("attributes not supported")
	}
	// A module-level template's __SCOPE__ is its own name. A template
	// declared inside a function keeps that function's name, and a
	// module-level const keeps the module name.
	scopeName := c.name
	if scopeName == "" || scopeName == c.module {
		scopeName = n.Name.String()
	}
	leave := c.enter(scopeName, nil)
	emitType := func() {
		c.compile(n.Type)
		if n.RestrictionSpec != nil {
			c.compile(n.RestrictionSpec)
		} else {
			c.emit(opcode.PERMITT, 0)
		}
	}
	emitType()
	if n.Params != nil {
		c.compile(n.Params)
	} else {
		c.emit(opcode.SKIP, 0)
	}
	c.emit(opcode.SCAN, 0)
	c.compileScoped(n.Value, c.typeScope(n.Type))
	if n.Base != nil {
		// mtc passes the base's parameters by position, not every parameter
		// of the modifying template.
		if n.Params != nil {
			list := n.Params.List
			npass := c.templateParams(n.Base)
			if npass > len(list) {
				npass = len(list)
			}
			for _, p := range list[:npass] {
				c.compile(p.Name)
			}
		}
		c.compile(n.Base)
		c.emit(opcode.ASSIGND, 0)
	}
	emitType()
	c.emit(opcode.ASSIGND, 0)
	c.emit(opcode.BLOCK, 0)
	c.emit(opcode.NAME, n.Name.String())
	addr := c.emit(opcode.TEMPLATE, 0)
	leave()
	npass := 0
	if n.Params != nil {
		npass = len(n.Params.List)
	}
	c.scope.define(n.Name.String(), symbol{op: opcode.REF, arg: Reference(addr), typ: n.Type, params: npass})
	return nil
}

// templateParams is the number of formals of the base template. A base that
// is declared later is counted from the module pre-scan.
func (c *Compiler) templateParams(base syntax.Expr) int {
	id, ok := base.(*syntax.Ident)
	if !ok {
		return 0
	}
	if sym, ok := c.scope.lookup(id.String()); ok && sym.arg != nil {
		return sym.params
	}
	return c.nparams[id.String()]
}

// enter opens the scope of definition name: its locals, then the body of the
// runs-on component, then the enclosing scope. The returned func leaves it.
func (c *Compiler) enter(name string, runsOn *syntax.RunsOnSpec) func() {
	prevScope, prevName := c.scope, c.name
	if runsOn != nil {
		if comp, _ := c.resolve(runsOn.Comp); comp.scope != nil {
			c.scope = &scope{parent: c.scope, store: comp.scope.store}
		}
	}
	c.scope, c.name = newScope(c.scope), name
	return func() { c.scope, c.name = prevScope, prevName }
}

func returnType(n *syntax.FuncDecl) syntax.Expr {
	if n.Return == nil {
		return nil
	}
	return n.Return.Type
}

func (c *Compiler) compileFunction(n *syntax.FuncDecl) error {
	leave := c.enter(n.Name.String(), n.RunsOn)

	if n.With != nil {
		c.errorf("function attributes not supported")
	}
	if n.Mtc != nil {
		c.errorf("MTC clause not supported")
	}

	op := opcode.FUNCTION
	switch {
	case n.Return == nil && n.RunsOn != nil:
		op = opcode.FUNCTIONB
		c.compile(n.RunsOn.Comp)

	case n.Return != nil && n.RunsOn == nil:
		op = opcode.FUNCTIONV
		c.compile(n.Return)

	case n.Return != nil && n.RunsOn != nil:
		op = opcode.FUNCTIONVB
		c.compile(n.Return)
		c.compile(n.RunsOn.Comp)
	}

	c.compile(n.Params)
	c.compileScoped(n.Body, c.typeScope(n.Return))
	c.emit(opcode.NAME, n.Name.String())
	addr := c.emit(op, 0)

	leave()
	c.scope.define(n.Name.String(), symbol{op: opcode.REF, arg: Reference(addr), typ: returnType(n)})
	return nil
}

func (c *Compiler) compileExtFunc(n *syntax.FuncDecl) error {
	leave := c.enter(n.Name.String(), nil)

	if n.Mtc != nil {
		c.errorf("MTC clause not supported")
	}

	if n.RunsOn != nil {
		c.errorf("runs on clause not supported")
	}

	op := opcode.FUNCTIONX
	switch {
	case n.Return == nil && n.With != nil:
		op = opcode.FUNCTIONXW
		c.compile(n.With)

	case n.Return != nil && n.With == nil:
		op = opcode.FUNCTIONXV
		c.compile(n.Return)

	case n.Return != nil && n.With != nil:
		op = opcode.FUNCTIONXVW
		c.compile(n.Return)
		c.compile(n.With)
	}
	c.compile(n.Params)
	c.emit(opcode.NAME, n.Name.String())
	addr := c.emit(op, 0)

	leave()
	c.scope.define(n.Name.String(), symbol{op: opcode.REF, arg: Reference(addr), typ: returnType(n)})
	return nil
}

func (c *Compiler) compileTestcase(n *syntax.FuncDecl) error {
	leave := c.enter(n.Name.String(), n.RunsOn)

	op := opcode.TESTCASE
	if n.System != nil {
		op = opcode.TESTCASES
		c.compile(n.System.Comp)
	}
	c.compile(n.RunsOn.Comp)
	c.compile(n.Params)
	c.compile(n.Body)
	c.emit(opcode.NAME, n.Name.String())
	addr := c.emit(op, 0)

	leave()
	c.scope.define(n.Name.String(), symbol{op: opcode.REF, arg: Reference(addr)})
	return nil
}

func (c *Compiler) compileAltstep(n *syntax.FuncDecl) error {
	leave := c.enter(n.Name.String(), n.RunsOn)

	if n.Mtc != nil {
		c.errorf("MTC clause not supported")
	}

	op := opcode.ALTSTEP
	switch {
	case n.RunsOn == nil && n.With != nil:
		op = opcode.ALTSTEPW
		c.compile(n.With)

	case n.RunsOn != nil && n.With == nil:
		op = opcode.ALTSTEPB
		c.compile(n.RunsOn.Comp)

	case n.RunsOn != nil && n.With != nil:
		op = opcode.ALTSTEPBW
		c.compile(n.RunsOn.Comp)
		c.compile(n.With)
	}

	c.compile(n.Params)
	c.compile(n.Body)
	c.emit(opcode.NAME, n.Name.String())
	addr := c.emit(op, 0)
	leave()
	c.scope.define(n.Name.String(), symbol{op: opcode.REF, arg: Reference(addr)})

	return nil
}

func (c *Compiler) compileVar(kind syntax.Kind, restr *syntax.RestrictionSpec, modif syntax.Token, typ syntax.Expr, decl *syntax.Declarator, attrs *syntax.WithSpec) error {
	if attrs != nil {
		c.errorf("attributes not supported")
	}
	var declared syntax.Node = typ
	var init syntax.Expr
	switch sym, ok := c.scope.lookup(kind.String()); {
	case typ != nil:
		c.compile(typ)
	case ok:
		c.emit(sym.op, sym.arg)
	case decl.Value != nil:
		c.compileTypeOf(decl.Value)
		val, _ := c.resolve(decl.Value)
		declared, init = val.typ, decl.Value
	default:
		c.errorf("unsupported declaration %s", kind)
	}
	c.compileArrayDef(decl.ArrayDef)
	if restr != nil {
		c.compile(restr)
	}

	c.emit(opcode.NAME, decl.Name.String())
	if modif != nil {
		c.errorf("modifiers not supported")
	}
	addr := c.emit(opcode.VAR, 0)
	if decl.Value != nil {
		c.compileScoped(decl.Value, c.typeScope(declared))
		c.emit(opcode.REF, Reference(addr))
		c.emit(opcode.ASSIGN, 0)
	}
	c.scope.define(decl.Name.String(), symbol{op: opcode.REF, arg: Reference(addr), typ: declared, init: init})
	return nil
}

func (c *Compiler) compileConst(kind syntax.Kind, restr *syntax.RestrictionSpec,
	modif syntax.Token, typ syntax.Expr, decl *syntax.Declarator, attrs *syntax.WithSpec) error {
	op := opcode.CONST
	if attrs != nil {
		op = opcode.CONSTW
		c.compile(attrs)
	}

	if decl.Value != nil {
		c.emit(opcode.SCAN, 0)
		c.compileScoped(decl.Value, c.typeScope(typ))
		c.emit(opcode.BLOCK, 0)
	} else {
		c.errorf("constant declaration without value")
	}

	c.compile(typ)
	c.compileArrayDef(decl.ArrayDef)
	c.emit(opcode.NAME, decl.Name.String())
	if modif != nil {
		c.errorf("modifiers not supported")
	}
	addr := c.emit(op, 0)
	c.scope.define(decl.Name.String(), symbol{op: opcode.REF, arg: Reference(addr), typ: typ})
	return nil
}

func (c *Compiler) compileArrayDef(a []*syntax.ParenExpr) {
	for i := len(a) - 1; i >= 0; i-- {
		c.emit(opcode.SCAN, 0)
		for _, x := range a[i].List {
			c.compile(x)
		}
		c.emit(opcode.BLOCK, 0)
		c.emit(opcode.ARRAY, 0)
	}
}

func (c *Compiler) compileModulePar(kind syntax.Kind, restr *syntax.RestrictionSpec,
	modif syntax.Token, typ syntax.Expr, decl *syntax.Declarator, attrs *syntax.WithSpec) error {

	op := opcode.MPAR
	if decl.Value != nil {
		op = opcode.MPARD
		c.emit(opcode.SCAN, 0)
		c.compileScoped(decl.Value, c.typeScope(typ))
		c.emit(opcode.BLOCK, 0)
	}

	if attrs != nil {
		c.errorf("attributes not supported")
	}
	if modif != nil {
		c.errorf("modifiers not supported")
	}

	c.compile(typ)
	c.compileArrayDef(decl.ArrayDef)
	if restr != nil {
		c.compile(restr)
	}

	c.emit(opcode.NAME, decl.Name.String())
	addr := c.emit(op, 0)
	c.scope.define(decl.Name.String(), symbol{op: opcode.REF, arg: Reference(addr), typ: typ})
	return nil
}

func (c *Compiler) compileBlock(b *syntax.BlockStmt, sc *scope) {
	if b == nil || len(b.Stmts) == 0 {
		c.emit(opcode.SKIP, 0)
		return
	}
	prev := c.scope
	c.scope = sc
	c.emit(opcode.SCAN, 0)
	for _, child := range b.Stmts {
		c.compile(child)
	}
	c.emit(opcode.BLOCK, 0)
	c.scope = prev
}

// emitRef emits op referencing the symbol name, patching forward references
// once all definitions are known.
func (c *Compiler) emitRef(id *syntax.Ident, name string, op opcode.Opcode) {
	if sym, ok := c.scope.lookup(name); ok {
		c.emit(sym.op, sym.arg)
		return
	}
	addr := c.emit(op, Reference(0x3FFFFFFC))
	c.patches = append(c.patches, patchSymbol{id: id, name: name, scope: c.scope, addr: addr})
}

// compileTypeOf emits the type of expression e, as mtc does where the source
// leaves it implicit.
func (c *Compiler) compileTypeOf(e syntax.Expr) {
	switch e := e.(type) {
	case *syntax.ValueLiteral:
		switch k := e.Tok.Kind(); k {
		case syntax.INT:
			c.emit(opcode.INTEGER, 0)
		case syntax.FLOAT:
			c.emit(opcode.FLOAT, 0)
		case syntax.STRING:
			c.emit(opcode.CHARSTRING, 0)
		case syntax.TRUE, syntax.FALSE:
			c.emit(opcode.BOOLEAN, 0)
		case syntax.NONE, syntax.PASS, syntax.INCONC, syntax.FAIL, syntax.ERROR:
			c.emit(opcode.VERDICTTYPE, 0)
		case syntax.BSTRING:
			switch s := e.Tok.String(); s[len(s)-1] | 0x20 {
			case 'b':
				c.emit(opcode.BITSTRING, 0)
			case 'h':
				c.emit(opcode.HEXSTRING, 0)
			default:
				c.emit(opcode.OCTETSTRING, 0)
			}
		default:
			c.errorf("cannot infer type of %s", e.Tok)
		}
	case *syntax.ParenExpr:
		c.compileTypeOf(e.List[0])
	case *syntax.UnaryExpr:
		c.compileTypeOf(e.X)
	case *syntax.BinaryExpr:
		switch e.Op.Kind() {
		case syntax.EQ, syntax.NE, syntax.LT, syntax.GT, syntax.LE, syntax.GE, syntax.AND, syntax.OR, syntax.XOR:
			c.emit(opcode.BOOLEAN, 0)
		default:
			c.compileTypeOf(e.X)
		}
	case *syntax.Ident:
		switch e.String() {
		case "__LINE__":
			c.emit(opcode.INTEGER, 0)
		case "__SCOPE__", "__MODULE__", "__FILE__", "__BFILE__":
			c.emit(opcode.CHARSTRING, 0)
		case "getverdict":
			c.emit(opcode.VERDICTTYPE, 0)
		default:
			c.compileDeclaredType(e)
		}
	case *syntax.SelectorExpr:
		if sym, _ := c.resolve(e); sym.op == opcode.CREATE {
			c.compile(e.X)
			break
		}
		c.compileDeclaredType(e)
	case *syntax.CallExpr:
		name := ""
		if id, ok := e.Fun.(*syntax.Ident); ok {
			name = id.String()
		}
		results := map[string]opcode.Opcode{"execute": opcode.VERDICTTYPE, "activate": opcode.DEFAULT}
		conversions := map[string]opcode.Opcode{
			"int": opcode.INTEGER, "float": opcode.FLOAT, "str": opcode.CHARSTRING, "char": opcode.CHARSTRING,
			"bit": opcode.BITSTRING, "hex": opcode.HEXSTRING, "oct": opcode.OCTETSTRING,
		}
		_, builtin := universe.store[name]
		_, to, _ := strings.Cut(name, "2")
		switch {
		case !builtin:
			if _, ok := e.Fun.(*syntax.SelectorExpr); ok {
				c.compileTypeOf(e.Fun)
			} else {
				c.compileDeclaredType(e)
			}
		case results[name] != 0:
			c.emit(results[name], 0)
		case conversions[to] != 0:
			c.emit(conversions[to], 0)
		case (name == "substr" || name == "replace" || name == "valueof") && e.Args != nil && len(e.Args.List) > 0:
			c.compileTypeOf(e.Args.List[0])
		default:
			c.errorf("cannot infer type of %s", name)
		}
	default:
		c.compileDeclaredType(e)
	}
}

func (c *Compiler) compileDeclaredType(e syntax.Expr) {
	switch sym, _ := c.resolve(e); {
	case sym.typ != nil:
		c.compile(sym.typ)
	case sym.init != nil:
		c.compileTypeOf(sym.init)
	default:
		c.errorf("cannot infer type of %T", e)
	}
}

// rangeForTemps counts the hidden variables of the range-fors in n: two per
// loop, plus one when the range expression is a call.
func rangeForTemps(n syntax.Node) int {
	if n == nil {
		return 0
	}
	total := 0
	n.Inspect(func(x syntax.Node) bool {
		fr, ok := x.(*syntax.ForRangeStmt)
		if !ok {
			return true
		}
		total += 2
		if _, ok := fr.Range.(*syntax.CallExpr); ok {
			total++
		}
		return true
	})
	return total
}

// tempName numbers the hidden variables mtc introduces, like _i0.
func (c *Compiler) tempName(prefix string) string {
	c.temps++
	return fmt.Sprintf("%s%d", prefix, c.temps-1)
}

func (c *Compiler) tempVar(prefix string, emitType func()) Reference {
	emitType()
	c.emit(opcode.NAME, c.tempName(prefix))
	return Reference(c.emit(opcode.VAR, 0))
}

// compileCases lowers the clauses of a select statement to an if-else chain.
// mtc uses a silent match once there is more than one case to try.
func (c *Compiler) compileCases(n *syntax.SelectStmt, clauses []*syntax.CaseClause) {
	tag := n.Tag
	match := opcode.MATCH
	cases := 0
	for _, cl := range n.Body {
		if cl.Case != nil {
			cases++
		}
	}
	if cases > 1 {
		match = opcode.SMATCH
	}
	cl := clauses[0]
	if cl.Case == nil {
		c.compile(cl.Body)
		return
	}
	for i, x := range cl.Case.List {
		if i > 0 {
			c.emit(opcode.SCAN, 0)
		}
		if n.Union != nil {
			if i == 0 {
				c.emit(opcode.SCAN, 0)
			}
			c.compile(&syntax.SelectorExpr{X: tag, Sel: x})
			if i == 0 {
				c.emit(opcode.BLOCK, 0)
			}
			c.emit(opcode.ISCHOSEN, 0)
		} else {
			c.compile(tag)
			c.compileScoped(x, c.typeScope(tag))
			c.compileTypeOf(tag)
			c.emit(opcode.PERMITT, 0)
			c.emit(opcode.ASSIGND, 0)
			c.emit(match, 0)
		}
		if i > 0 {
			c.emit(opcode.BLOCK, 0)
			c.emit(opcode.OR, 0)
		}
	}
	c.compile(cl.Body)
	switch rest := clauses[1:]; {
	case len(rest) == 0:
		c.emit(opcode.IF, 0)
	case rest[0].Case == nil:
		c.compile(rest[0].Body)
		c.emit(opcode.IFELSE, 0)
	default:
		c.emit(opcode.SCAN, 0)
		c.compileCases(n, rest)
		c.emit(opcode.BLOCK, 0)
		c.emit(opcode.IFELSE, 0)
	}
}

func isTypeDecl(n syntax.Node) bool {
	switch n.(type) {
	case *syntax.StructTypeDecl, *syntax.SubTypeDecl, *syntax.EnumTypeDecl, *syntax.ComponentTypeDecl, *syntax.PortTypeDecl, *syntax.MapTypeDecl:
		return true
	}
	return false
}

func isAddressTypeDecl(n syntax.Node) bool {
	decl, ok := n.(*syntax.SubTypeDecl)
	return ok && decl.Field != nil && decl.Field.Name != nil && decl.Field.Name.String() == "address"
}

// plainOperand reports whether e is a literal or a variable, after
// parentheses. and/or push those directly and wrap everything else in a block.
func plainOperand(e syntax.Expr) bool {
	for {
		p, ok := e.(*syntax.ParenExpr)
		if !ok || len(p.List) != 1 {
			break
		}
		e = p.List[0]
	}
	switch e.(type) {
	case *syntax.Ident, *syntax.ValueLiteral:
		return true
	default:
		return false
	}
}

// compileBinary emits a bitstring, hexstring, or octetstring literal.
// A literal that contains ? or * is a pattern, as mtc emits it.
func (c *Compiler) compileBinary(lit string) {
	if len(lit) < 3 || lit[0] != '\'' || lit[len(lit)-2] != '\'' {
		c.errorf("invalid binary string %s", lit)
		return
	}
	unit := lit[len(lit)-1]
	if unit >= 'a' && unit <= 'z' {
		unit -= 'a' - 'A'
	}
	body := lit[1 : len(lit)-2]
	s := make([]byte, 0, len(body))
	for i := 0; i < len(body); i++ {
		switch body[i] {
		case ' ', '\t', '\n', '\r', '\v', '\f':
		default:
			s = append(s, body[i])
		}
	}

	hex := false
	var op opcode.Opcode
	switch unit {
	case 'B':
		op = opcode.BITS
	case 'H':
		op = opcode.NIBBLES
		hex = true
	case 'O':
		op = opcode.OCTETS
		hex = true
	default:
		c.errorf("invalid binary string %s", lit)
		return
	}

	pattern := false
	digits := 0
	for _, ch := range s {
		if ch == '?' || ch == '*' {
			pattern = true
			continue
		}
		if hex {
			if _, ok := hexValue(ch); !ok {
				c.errorf("invalid binary string %s", lit)
				return
			}
		} else if ch != '0' && ch != '1' {
			c.errorf("invalid binary string %s", lit)
			return
		}
		digits++
	}
	if unit == 'O' && digits%2 != 0 {
		c.errorf("invalid octetstring %s", lit)
		return
	}
	if pattern {
		c.emit(opcode.UTF8, string(s))
		c.emit(opcode.PATTERN, 0)
		return
	}

	var n int
	var data []byte
	switch unit {
	case 'B':
		n = len(s)
		data = make([]byte, (n+7)/8)
		for i, ch := range s {
			if ch == '1' {
				data[i/8] |= 1 << (7 - i%8)
			}
		}
	case 'H':
		n = len(s)
		data = make([]byte, (n+1)/2)
		for i, ch := range s {
			v, _ := hexValue(ch)
			if i%2 == 0 {
				data[i/2] |= v << 4
			} else {
				data[i/2] |= v
			}
		}
	default:
		n = len(s) / 2
		data = make([]byte, n)
		for i := 0; i < n; i++ {
			hi, _ := hexValue(s[i*2])
			lo, _ := hexValue(s[i*2+1])
			data[i] = hi<<4 | lo
		}
	}
	c.emit(op, NewString(n, data))
}

func hexValue(ch byte) (byte, bool) {
	switch {
	case ch >= '0' && ch <= '9':
		return ch - '0', true
	case ch >= 'A' && ch <= 'F':
		return ch - 'A' + 10, true
	case ch >= 'a' && ch <= 'f':
		return ch - 'a' + 10, true
	default:
		return 0, false
	}
}

func (c *Compiler) emit(op opcode.Opcode, arg any) int {
	pos := c.e.Len()
	if err := c.e.Encode(op, arg); err != nil {
		c.errorf("%w", err)
	}
	return pos

}

func (c *Compiler) errorf(format string, args ...any) {
	err := fmt.Errorf(format, args...)
	if spn := syntax.SpanOf(c.cur); spn.Begin.IsValid() {
		err = fmt.Errorf("%s: %w", spn, err)
	}
	c.err = errors.Join(c.err, err)
}

// fieldIndex numbers union field names module-wide, as mtc does. Ids 0-8
// belong to the fields of the anytype union mtc inserts into every module.
func (c *Compiler) fieldIndex(name string) int {
	i, ok := c.fields[name]
	if !ok {
		i = 9 + len(c.fields)
		c.fields[name] = i
	}
	return i
}

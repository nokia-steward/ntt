package syntax

import (
	"fmt"
	"testing"
)

type Expect int

const (
	pass Expect = iota
	fail
)

type Test struct {
	expect Expect
	input  string
}

func TestModules(t *testing.T) {
	modules := []Test{
		{pass, `module m {}`},
		{pass, `module m language "str1", "str2" {}`},
		{pass, `module m language "str1", "str2", {}`},
	}

	testParse(t, modules, func(p *parser) { p.parseModule() })
}

func TestWithStmts(t *testing.T) {
	withStmts := []Test{
		{pass, `encode    "str";`},
		{pass, `variant   "str";`},
		{pass, `display   "str";`},
		{pass, `extension "str";`},
		{pass, `optional  "str";`},
		{pass, `stepsize  "str";`},
		{pass, `encode override        "str";`},
		{pass, `encode @local          "str";`},
		{pass, `encode @local          "str"."ruleA";`},
		{pass, `variant {"a"}."ruleA";`},
		{pass, `variant {"a", "b",}."ruleA";`},
		{pass, `variant (f) {"a", "b"}."ruleA";`},
		{pass, `variant (type all,) "str";`},
		{fail, `variant {"a", "b"} "ruleA";`},
		{fail, `variant {"a", "b"};`},
		{fail, `variant {}."ruleA";`},
		{fail, `variant {a}."ruleA";`},
		{pass, `encode ([-])           "str";`},
		{pass, `encode ([-].bar)       "str";`},
		{pass, `encode (a[-])          "str";`},
		{pass, `encode (group all)     "str";`},
		{pass, `encode (type all)      "str";`},
		{pass, `encode (template all)  "str";`},
		{pass, `encode (const all)     "str";`},
		{pass, `encode (altstep all)   "str";`},
		{pass, `encode (testcase all)  "str";`},
		{pass, `encode (function all)  "str";`},
		{pass, `encode (signature all) "str";`},
		{pass, `encode (modulepar all) "str";`},
		{pass, `encode (type all except {a,b}) "str";`},
	}

	testParse(t, withStmts, func(p *parser) { p.parseWithStmt() })
}

func TestExprs(t *testing.T) {
	exprs := []Test{
		{pass, `-`},
		{pass, `m.to`},
		{pass, `1..23`},
		{pass, `a[-]`},
		{pass, `-1 * x`},
		{pass, `-x * y`},
		{pass, `{x := (1+2)*3, y:=a.f()}`},
		{pass, `{[0] := 1, [1] := 2 }`},
		{pass, `x[i][j] := 1, y[k] := 2 }`},
		{pass, `{(1+2)*3, a.f()}`},
		{pass, `x.class := 23`},
		{pass, `R.encode`},
		{pass, `R.field1.encode`},
		{pass, `R.variant`},
		{pass, `Int2.variant("CodecA")`},
		{pass, `MyType.display`},
		{pass, `int.extension`},
		{pass, `m_msg.optional`},
		{pass, `m.b.present`},
		{pass, `present(t)`},
		{pass, `present(t).f`},
		{fail, `present`},
		{fail, `present t`},
		{fail, `R.override`},
		{pass, `{-,-}`},
		{pass, `(1,*,?,-,2)`},
		{pass, `t length(5..23)`},
		{pass, `t length(5..23) ifpresent`},
		{pass, `t ifpresent`},
		{pass, `system:p`},
		{pass, `modifies t:=23`},
		{pass, `complement(all from t)`},
		{pass, `b := any from c.running -> @index value i`},
		{pass, `p := decmatch M: {f1:= 10, f2 := '1001'B}`},
		{pass, `p := decmatch ("UTF-8") M: {f1:= 10, f2 := '1001'B}`},
		{pass, `decmatch(x)`},
		{pass, `{f := decmatch(x)}`},
		{pass, `{f := decmatch(x), g := 1}`},
		{pass, `decmatch(x) ifpresent`},
		{pass, `p := @decoded payload`},
		{pass, `p := pattern ".*" & p & "$"`},
		{pass, `p := pattern @nocase ".*" & p & "$"`},
		{pass, `regexp @nocase(x,charstring:"?+(text)?+",0)`},
		{pass, `regexp @nocase(x,charstring:"?+(text)?+",0,)`},
		{pass, `{x := 1,}`},
		{pass, `{a := 1, b := 2,}`},
		{pass, `{1, 2, 3,}`},
		{pass, `match(ptc.alive, false)`},
		{pass, `f(r...)`},
		{pass, `f({1, 2, 3}...)`},
		{pass, `x.universal charstring := "FF80"`},
		{pass, `::E + NS::E`},
		{pass, `???`},
		{pass, `-???`},
		{pass, `"Hello " & ???`},
		{pass, `???("reason")`},
	}

	testParse(t, exprs, func(p *parser) { p.parseExprList() })
}

func TestFuncDecls(t *testing.T) {
	funcDecls := []Test{
		{pass, `testcase f() {}`},
		{pass, `testcase f() runs on A[-] {}`},
		{pass, `testcase f() runs on C system C {}`},
		{pass, `function f() {}`},
		{pass, `function f() return int {}`},
		{pass, `function f() return template int {}`},
		{pass, `function f() return template(value) int {}`},
		{pass, `function f() return value int {}`},
		{pass, `function @deterministic f() {}`},
		{pass, `function f() return record of integer {}`},
		{pass, `function f() return record of record {}[3] {}`},
		{pass, `function f() return template set { integer a } {}`},
		{pass, `function f(@lazy @deterministic integer p) {}`},
		{pass, `function f(in integer x, out integer y,) {}`},
		{pass, `function f(record { integer a, } x,) {}`},
		{pass, `signature f(integer x, charstring y,);`},
		{pass, `altstep as1(in integer items...) {}`},
		{pass, `testcase tc(in verdicttype verdicts...) runs on C {}`},
		{pass, `function f() runs on A[-] {}`},
		{pass, `function f() mtc C {}`},
		{pass, `function f() runs on C mtc C system C {}`},
		{pass, `altstep as() { var roi[-] a[4][4]; [] receive; [else] {}}`},
		{pass, `altstep as1() { [] a1() [] a2() }`},
		{pass, `altstep interleave interleaveStep() {  [] p1.receive; [] p2.check{}}`},
		{pass, `signature f();`},
		{pass, `signature f() exception (integer);`},
		{pass, `signature f() return int;`},
		{pass, `signature f() return int exception (integer, a.b[0]);`},
		{pass, `signature f() noblock;`},
		{pass, `signature f() noblock exception (integer, a.b[0]);`},
	}

	testParse(t, funcDecls, func(p *parser) { p.parseFuncDecl() })
}

func TestModuleDefs(t *testing.T) {
	moduleDefs := []Test{
		{pass, `import from m all;`},
		{pass, `import from m language "str1", "str2" all;`},
		{pass, `import from m -> n all;`},
		{pass, `import from m -> n {type t};`},
		{pass, `import from m language "str1" -> n all;`},
		{fail, `import from m -> all;`},
		{fail, `import from m -> n.x all;`},
		{pass, `import from m all except {}`},
		{pass, `import from m all except {
                        template  all;
                        const     all;
                        altstep   all;
                        testcase  all;
                        function  all;
                        signature all;
                        modulepar all;
                        import    all;
                        type      all }`},
		{pass, `import from m all except { group all }`},
		{pass, `import from m all except { group x,y }`},
		{pass, `import from m all except { group x,y, }`},
		{pass, `import from m {
                        template  all;
                        const     all;
                        altstep   all;
                        testcase  all;
                        function  all;
                        signature all;
                        modulepar all;
                        import    all;
                        type      all }`},
		{pass, `import from m {
                        group x except { group all }, y }`},

		{pass, `friend module m;`},
		{pass, `public modulepar integer x;`},
		{pass, `modulepar mpar := 1;`},
		{pass, `modulepar a := 1, b := "x";`},
		{pass, `modulepar a := 1, b := "x",;`},
		{pass, `modulepar record of charstring IDs := {"a", "b"};`},
		{pass, `modulepar template record { integer a } t := ?;`},
		{pass, `modulepar { mpar := 1; record of integer r; }`},
		{pass, `signature A() return record of integer;`},
		{pass, `signature B(record of integer p);`},
		{pass, `signature C() exception (record {}, integer);`},
		{pass, `signature D() exception (integer, a.b[0],);`},
		{fail, `modulepar := 1;`},
		{pass, `private function fn() {}`},
		{pass, `group foo { group bar { import from m all; } }`},
	}
	testParse(t, moduleDefs, func(p *parser) { p.parseModuleDef() })
}

func TestValueDecls(t *testing.T) {
	valueDecls := []Test{
		{pass, `const integer x;`},
		{pass, `const int x := 1;`},
		{pass, `const int x := 1, yi := 2;`},
		{pass, `const int x[len] := 1, y := 2;`},
		{pass, `const a[-] x := 1;`},
		{pass, `const a[1] x[2][3] := x[4];`},
		{pass, `var int x := {1,2};`},
		{pass, `var int x, y := 2, z;`},
		{pass, `var template          int x;`},
		{pass, `var template(omit)    int x;`},
		{pass, `var template(value)   int x;`},
		{pass, `var template(present) int x;`},
		{pass, `var omit    int x;`},
		{pass, `var value   int x;`},
		{pass, `var present int x;`},
		{pass, `var value @lazy int x;`},
		{pass, `var value @lazy int x, y := ?;`},
		{pass, `var @fuzzy @deterministic integer x := 1;`},
		{pass, `var template @lazy @deterministic U t := ?;`},
		{pass, `var template record of integer t := {1, *, 3};`},
		{pass, `var record {} v[3];`},
		{pass, `var set of enumerated { a, b } x;`},
		{pass, `var map from integer to charstring m;`},
		{pass, `const record of integer c := {1, 2, 3};`},
		{fail, `const record of integer := {1, 2, 3};`},
		{fail, `var @deterministic @lazy integer x;`},
		{fail, `var @lazy @fuzzy integer x;`},
		{pass, `timer x, y := 1.0, y;`},
		{pass, `port P x[len], y := 1, z := 2 ;`},
		{pass, `modulepar RoI[-] x, y:=23, z;`},
		{pass, `const x := 23`},
		{pass, `var template t := 23`},
		{pass, `var t := true`},
		{pass, `var x := 23, y := 10`},
		{pass, `var r := {1,2,3}`},
		{pass, `var integer i := ???;`},
		{pass, `modulepar charstring reason := ???;`},
	}

	testParse(t, valueDecls, func(p *parser) { p.parseValueDecl() })
}

func TestTemplateDecls(t *testing.T) {
	templateDecls := []Test{
		{pass, `template int x := ?;`},
		{pass, `template int x modifies y := ?;`},
		{pass, `template int x modifies y.z := ?;`},
		{pass, `template int x(int i,) := i;`},
		{pass, `template int x(int i) := i;`},
		{pass, `template @lazy int x := ?;`},
		{pass, `template @lazy int x(int i) := i;`},
		{pass, `template @lazy int  x(int i) modifies y := ?;`},
		{pass, `template @lazy a[-] x(int i) modifies y := ?;`},
		{pass, `template @fuzzy @deterministic integer x := ?;`},
		{pass, `template RoI x(in integer numbers...) := {};`},
		{fail, `template @deterministic @fuzzy integer x := ?;`},
		{pass, `template(omit)    int x := ?;`},
		{pass, `template(value)   int x := ?;`},
		{pass, `template(present) int x := ?;`},
		{pass, `template T t := { f := decmatch(x) };`},
	}

	testParse(t, templateDecls, func(p *parser) { p.parseTemplateDecl() })
}

func TestFormalPars(t *testing.T) {
	formalPars := []Test{
		{pass, `()`},
		{pass, `(int y)`},
		{pass, `(int x, int y)`},
		{pass, `(int x, int y,)`},
		{pass, `(in int x, out int y, inout int z)`},
		{pass, `(in template(value) @fuzzy timer x := 1, out timer y)`},
		{pass, `(out timer y, in template(value) @fuzzy timer x := 1)`},
		{pass, `(out timer y := -, in value @fuzzy timer x := 1)`},
		{pass, `(out timer y := -, in value timer x := (1,2,3))`},
		{pass, `(@lazy @deterministic integer x)`},
		{pass, `(template @fuzzy @deterministic integer x)`},
		{pass, `(in integer x...)`},
		{pass, `(integer x[3]...)`},
		{pass, `(in integer x... := {1, 2, 3})`},
		{pass, `(int x, template int y...)`},
		{pass, `(in record of integer x)`},
		{pass, `(template record of integer x)`},
		{pass, `(set length(2) of record { integer a } x)`},
		{pass, `(union { integer a, boolean } x)`},
		{pass, `(enumerated { a, b } x)`},
		{pass, `(map from integer to charstring x)`},
		{fail, `(integer... x)`},
		{fail, `(@lazy @fuzzy integer x)`},
		{fail, `(record of integer)`},
		{pass, `(in integer i := ???)`},
	}
	testParse(t, formalPars, func(p *parser) { p.parseFormalPars() })
}

func TestFormalParNodes(t *testing.T) {
	p := NewParser([]byte(`(in template @lazy @deterministic record of integer x[2]... := {})`))
	pars := p.parseFormalPars()
	if err := p.Err(); err != nil {
		t.Fatal(err)
	}
	x := pars.List[0]
	if x.Modif == nil || x.Modif.String() != "@lazy" {
		t.Errorf("Modif = %v, want @lazy", x.Modif)
	}
	if x.Deterministic == nil || x.Deterministic.String() != "@deterministic" {
		t.Errorf("Deterministic = %v, want @deterministic", x.Deterministic)
	}
	if _, ok := x.Type.(*ListSpec); !ok {
		t.Errorf("Type = %T, want *ListSpec", x.Type)
	}
	if x.Variadic == nil || x.Variadic.Kind() != ELIPSIS {
		t.Errorf("Variadic = %v, want ...", x.Variadic)
	}
	if x.Value == nil {
		t.Errorf("Value = nil, want default value")
	}
}

func TestTypes(t *testing.T) {
	types := []Test{
		// Subtypes
		{pass, `type integer t;`},
		{pass, `type int t (0..255)`},
		{pass, `type int t length(2)`},
		{pass, `type a[0] t (0,1) length(2)`},

		// List Types
		{pass, `type set of int s`},
		{pass, `type set length(2) of int s`},
		{pass, `type set length(2) of int s length(2)`},
		{pass, `type set length(2) of int s (0,1,2) length(2)`},
		{pass, `type set of set of int s`},
		{pass, `type set length(1) of set length(2) of int() s length(3)`},

		// Struct Types
		{pass, `type set address {}`},
		{pass, `type set s {}`},
		{pass, `type set s {int a optional }`},
		{pass, `type set s {set length(1) of set length(2) of int() f1[-][-] length(3) optional}`},
		{pass, `type set s {function () runs on self return template int callback}`},
		{pass, `type set s {altstep (integer p_i) runs on self as_callback}`},
		{pass, `type set s {altstep interleave(integer p_i) runs on C is_callback}`},
		{pass, `type union s {@default set of int f1 optional}`},
		{pass, `type union s {enumerated { e(1) } foo}`},
		{pass, `type enumerated a {e, e(1), e(1)}`},
		{pass, `type record r {int a, int b,}`},
		{pass, `type record r {integer a optional, charstring b,}`},
		{pass, `type record r {int a (0, 1,), int b,}`},
		{pass, `type enumerated a {e, e(1, 2,),}`},
		{pass, `type record r {record { int i, float f, } nr}`},

		// Embedded Fields
		{pass, `type record r {int, r optional, m.t}`},
		{pass, `type record r {int a, b<int>}`},
		{pass, `type set s {int}`},
		{pass, `type union u {int, @default boolean, r,}`},
		{pass, `type record r {record { int i } x, set { b }}`},
		{fail, `type int optional`},
		{fail, `type int`},
		{fail, `type record r {int a; int b}`},

		// Map Types
		{pass, `type map from charstring to integer m`},
		{pass, `type map from record { int a, int y } to integer m`},
		{pass, `type map from universal charstring to map from integer to charstring m`},

		// Port Types
		{pass, `type port p message {address a.b[-]}`},
		{pass, `type port p message {inout all}`},
		{pass, `type port p message {inout float, a.b[-]}`},
		{pass, `type port p message {map param (out int i:=1,)}`},
		{pass, `type port p message {inout integer, charstring,}`},
		{pass, `type port p message {map param (out int i:=1)}`},
		{pass, `type port p message {unmap param (out int i:=1)}`},
		{pass, `type port p procedure {}`},
		{pass, `type port p mixed {}`},

		// Component Types
		{pass, `type component C {}`},
		{pass, `type component C extends C[-], m.Base {}`},

		// Behaviour Types
		{pass, `type function fn() runs on self return template int`},
		{pass, `type altstep  as() runs on self return int`},
		{pass, `type altstep interleave iStep() runs on C`},
		{pass, `type testcase tc() runs on C system TSI`},

		// Class Types
		{pass, `type class cl {}`},
		{pass, `type class cl runs on C { create() {} }`},
		{pass, `type class cl runs on C { create() {}; private function fn() {}; var integer i; }`},
	}
	testParse(t, types, func(p *parser) { p.parseTypeDecl() })
}

func TestStmts(t *testing.T) {
	stmts := []Test{
		// Structural Statements
		{pass, `repeat;`},
		{pass, `break;`},
		{pass, `continue;`},
		{pass, `return;`},
		{pass, `return x() * 1;`},
		{pass, `label L1;`},
		{pass, `goto L2;`},
		{pass, `for (var int i := 0; i<23; i := i+1) {}`},
		{pass, `for (i:=x; i<23; i:=i+1) {}`},
		{pass, `for (x in {1,2,3}) {}`},
		{pass, `for (var x in a) {}`},
		{pass, `for (var integer x in a) {}`},
		{pass, `for (var template integer x in a) {}`},
		{pass, `while (23) {}`},
		{pass, `do {} while (23);`},
		{pass, `if (1) {}`},
		{pass, `if (1) {} else {}`},
		{pass, `if (1) {} else if (2) {} else {}`},
		{pass, `select union (p.x()) { case(1) {} case else {}}`},
		{pass, `select  (23) {case(1) {} case else {}}`},
		{pass, `interleave {}`},
		{pass, `alt {}`},
		{pass, `alt { [] receive; [23<foo()] p.timeout { var i x:=23; } [else] {}}`},
		{pass, `alt { [] a1() [] a2() }`},
		{pass, `alt { [] p.receive [] t.timeout {} }`},
		{pass, `alt { [] p.receive(x) -> value v [] a2() {} }`},
		{pass, `alt { [] a1() [else] {} }`},
		{pass, `alt { [] as1() [x > 1] p.receive {} }`},
		{pass, `alt { [] p[0].receive(a[1]) {} }`},
		{pass, `alt { [] p.receive { x := g()[0]; } }`},
		{pass, `x := g()[0];`},

		// Init Statements
		{pass, `if (var integer i := 1; match(i, 2)) {}`},
		{pass, `if (i := 1; i > 0) {} else if (var x := 2; x > i) {}`},
		{pass, `while (var integer j := 1; j < 10) {}`},
		{pass, `while (j := 1; j < 10) {}`},
		{pass, `select (var integer i := 2; i) { case(1) {} case else {}}`},
		{pass, `select union (var U v := {f1 := 1}; v) { case(f1) {} }`},
		{fail, `if (var integer i := 1) {}`},
		{fail, `while (var integer i := 1;) {}`},
		{fail, `do {} while (var integer i := 1; i < 10);`},

		// Not Implemented
		{pass, `???;`},
		{pass, `???("reason");`},
		{pass, `???(s);`},
		{fail, `??;`},

		// NoDefault Modifier
		{pass, `@nodefault AltSet1();`},
		{pass, `@nodefault all component.done;`},
		{pass, `@nodefault ptc.killed;`},
		{pass, `@nodefault p.receive(MyInt);`},
		{pass, `@nodefault p.trigger(integer:?) -> value v;`},
		{pass, `@nodefault p.getcall;`},
		{pass, `@nodefault p[0].getreply(S:{});`},
		{pass, `@nodefault p[0].catch;`},
		{pass, `@nodefault p.check(receive(integer:?) -> value v);`},
		{pass, `@nodefault t.timeout;`},
		{pass, `@nodefault any timer.timeout;`},
		{fail, `@nodefault alt {}`},
		{fail, `@nodefault var integer x;`},

		// Titan try/catch extension
		{pass, `@try {} @catch (e) {} @catch(r2) {}`},

		// Value Declaration Statements
		{pass, `var comp C := C.create;`},
		{pass, `var comp C := C.create("han solo") alive;`},

		// Expr Statements
		{pass, `send() to 80;`},
		{pass, `send() to v_dst;`},
		{pass, `receive from ip.address:?;`},
		{pass, `receive from ip.address:? -> @index x;`},
		{pass, `testcase.stop;`},
		{pass, `stop;`},
		{pass, `map (system:p1, c:p);`},
		{pass, `map (p1, p2) param ("localhost", 80);`},
		{pass, `unmap;`},
		{pass, `unmap (true);`},
		{pass, `unmap (true) param (-,-);`},
		{pass, `p.getreply(23);`},
		{pass, `p.reply(23 value x);`},
		{pass, `x.universal charstring := "FF80";`},

		// Check Statement
		{pass, `any port.check;`},
		{pass, `p.check(receive);`},
		{pass, `p.check(from x -> timestamp bar);`},
		{pass, `p.check(-> @index value i);`},
		{pass, `p.check(receive from x -> value ("foo"));`},
		{pass, `p.check(getreply(23 value x) from x -> sender(foo));`},

		// Call Statement
		{pass, `p.call(foo) to 80;`},
		{pass, `p[i].call(S:{});`},
		{pass, `p.call(S:{}) {[] receive; [else] {}}`},
	}
	testParse(t, stmts, func(p *parser) { p.parseStmt() })
}

func TestStmtFields(t *testing.T) {
	parseStmt := func(input string) Stmt {
		p := NewParser([]byte(input))
		s := p.parseStmt()
		if err := p.Err(); err != nil {
			t.Fatalf("Parse(%#q): %v", input, err)
		}
		return s
	}

	if s := parseStmt(`@nodefault p.receive;`).(*ExprStmt); s.NoDefault == nil || s.NoDefault.String() != "@nodefault" {
		t.Errorf("ExprStmt.NoDefault = %v, want @nodefault", s.NoDefault)
	}
	if s := parseStmt(`p.receive;`).(*ExprStmt); s.NoDefault != nil {
		t.Errorf("ExprStmt.NoDefault = %v, want nil", s.NoDefault)
	}
	if s := parseStmt(`???("reason");`).(*ExprStmt); s.Expr.(*CallExpr).Fun.(*ValueLiteral).Tok.Kind() != NOTIMPL {
		t.Errorf("expected call of ???")
	}

	if s := parseStmt(`if (var integer i := 1; i > 0) {}`).(*IfStmt); s.Init == nil || s.InitSemi == nil || s.Cond == nil {
		t.Errorf("IfStmt: Init=%v InitSemi=%v Cond=%v", s.Init, s.InitSemi, s.Cond)
	}
	if s := parseStmt(`if (i > 0) {}`).(*IfStmt); s.Init != nil || s.InitSemi != nil {
		t.Errorf("IfStmt: Init=%v InitSemi=%v, want nil", s.Init, s.InitSemi)
	}
	if s := parseStmt(`while (i := 1; i < 10) {}`).(*WhileStmt); s.Init == nil || s.InitSemi == nil || s.Cond == nil {
		t.Errorf("WhileStmt: Init=%v InitSemi=%v Cond=%v", s.Init, s.InitSemi, s.Cond)
	}
	if s := parseStmt(`select union (var U v := {}; v) {}`).(*SelectStmt); s.Init == nil || s.InitSemi == nil || s.Tag == nil || s.Union == nil {
		t.Errorf("SelectStmt: Init=%v InitSemi=%v Tag=%v", s.Init, s.InitSemi, s.Tag)
	}
}

func TestTypeParametrization(t *testing.T) {
	tests := []Test{
		// Formal Type Parameters
		{pass, `type set x<type T> {}`},
		{pass, `type set x<in type T> {}`},
		{pass, `type set x<in type T := integer> {}`},
		{pass, `type set x<in signature T> {}`},
		{pass, `type set x<in Comp C> {}`},
		{pass, `type set x<in Comp C := a[-]> {}`},
		{pass, `type set x<in Comp C := a<integer,boolean>[-]> {}`},
		{pass, `type component C<in type T> {}`},
		{pass, `type component D<in type T> extends C<T> {}`},
		{pass, `type component D<in type T,> extends C<T,> {}`},
		{pass, `type function f<type T> ()`},

		// Actual Type Parameters
		{pass, `const int x := a(b<x, y>(1+2));`},
		{pass, `const int x := a(b<x, y> 1+2);`},
		{pass, `const int x := a<b<c,d> >;`},
		{pass, `const int x := a<>;`},
		{pass, `const int x := a<>[-];`},
		{pass, `const int x := a<a[-]>;`},
		{pass, `const int x := a<a.b[-], c>;`},
		{pass, `const int x := a<a.b[-], c<d> >;`},
		{pass, `const int x := f(a<b, c<d>3);`},
		{pass, `const int x := a.b<>();`},
		{pass, `const int<t> x;`},
		{pass, `const int<charstring> x;`},
		{pass, `const int<universal charstring> x;`},
		{pass, `const int<anytype> x;`},
		{fail, `const int x := a(b<x, y> X+2);`},
	}
	testParse(t, tests, func(p *parser) { p.parseModuleDef() })
}

func TestAltGuardSplit(t *testing.T) {
	p := NewParser([]byte(`function f() { alt { [] as1() [x > 1] p.receive {} [] p[0].receive(a[1]) {} } }`))
	def := p.parseFuncDecl()
	if err := p.Err(); err != nil {
		t.Fatal(err)
	}
	alt := def.Body.Stmts[0].(*AltStmt)
	if len(alt.Body.Stmts) != 3 {
		t.Fatalf("got %d guards, want 3", len(alt.Body.Stmts))
	}

	g0 := alt.Body.Stmts[0].(*CommClause)
	if g0.X != nil || g0.Body != nil {
		t.Fatalf("first guard: cond=%v body=%v", g0.X, g0.Body)
	}
	if _, ok := g0.Comm.(*ExprStmt).Expr.(*CallExpr); !ok {
		t.Fatalf("first guard operation: %T", g0.Comm.(*ExprStmt).Expr)
	}

	g1 := alt.Body.Stmts[1].(*CommClause)
	if g1.X == nil {
		t.Fatal("second guard condition was parsed as an index")
	}
	if _, ok := g1.Comm.(*ExprStmt).Expr.(*SelectorExpr); !ok {
		t.Fatalf("second guard operation: %T", g1.Comm.(*ExprStmt).Expr)
	}

	call := alt.Body.Stmts[2].(*CommClause).Comm.(*ExprStmt).Expr.(*CallExpr)
	sel := call.Fun.(*SelectorExpr)
	if _, ok := sel.X.(*IndexExpr); !ok {
		t.Fatalf("port index: %T", sel.X)
	}
	if _, ok := call.Args.List[0].(*IndexExpr); !ok {
		t.Fatalf("argument index: %T", call.Args.List[0])
	}
}

func TestDecmatchParenTemplate(t *testing.T) {
	p := NewParser([]byte(`decmatch(x)`))
	e := p.parseExpr().(*DecmatchExpr)
	if err := p.Err(); err != nil {
		t.Fatal(err)
	}
	if e.Params != nil {
		t.Fatalf("Params = %#v, want nil", e.Params)
	}
	if _, ok := e.X.(*ParenExpr); !ok {
		t.Fatalf("X = %T, want *ParenExpr", e.X)
	}

	p = NewParser([]byte(`decmatch ("UTF-8") M`))
	e = p.parseExpr().(*DecmatchExpr)
	if err := p.Err(); err != nil {
		t.Fatal(err)
	}
	if e.Params == nil {
		t.Fatal("decoding parameter was consumed as the template")
	}
	if _, ok := e.X.(*Ident); !ok {
		t.Fatalf("X = %T, want *Ident", e.X)
	}
}

func TestExprBinding(t *testing.T) {
	exprs := []struct {
		in, want string
	}{
		{"not a == b", "not((a==b))"},
		{"not a < b", "not((a<b))"},
		{"not a and b", "(not(a)andb)"},
		{"a and not b", "(aandnot(b))"},
		{"not4b a + b", "not4b((a+b))"},
		{"not4b a & b", "not4b((a&b))"},
		{"not4b a << b", "(not4b(a)<<b)"},
		{"not4b a * b", "not4b((a*b))"},
		{"-a * b", "(-(a)*b)"},
		{"y => C == z", "((y=>C)==z)"},
		{"a.b => e.f", "(a.b=>e).f"},
		{"a.b[0] => e.f", "((a.b[0])=>e).f"},
		{`v => (T, "UTF-8").f`, `(v=>(T,"UTF-8")).f`},
		{"pattern \"a\" length(3) ifpresent", "ifpresent(length(pattern(\"a\")))"},
		{"pattern \"a\" & \"b\" length(3)", "length(pattern((\"a\"&\"b\")))"},
	}
	for _, tt := range exprs {
		p := NewParser([]byte(tt.in))
		got := exprShape(p.parseExpr())
		if err := p.Err(); err != nil {
			t.Errorf("Parse(%q): %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("Parse(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}

	stmts := []struct {
		in, want string
	}{
		{"p.receive(x) from c -> value v sender s", "(-> (p.receive(x)fromc) value v sender s)"},
		{"p.send(x) to c -> timestamp t", "(-> (p.send(x)toc) timestamp t)"},
		{"p.getreply(S:{}) -> value v param(a) sender s", "(-> p.getreply((S:{})) value v param sender s)"},
		{"p.getreply(S:{} value 5)", "p.getreply(value((S:{})))"},
		{"p.reply(S:{} value 5)", "p.reply(value((S:{})))"},
		{"p.check(from c -> sender s)", "p.check((-> (fromc) sender s))"},
		{"p.call(S:{}) to c { [] p.getreply {} }", "call{ (p.call((S:{}))toc) }"},
		{"p.call(S:{}) -> timestamp t { [] p.getreply {} }", "call{ (-> p.call((S:{})) timestamp t) }"},
		{"p.call(S:{}) { [] p.getreply {} }", "call{ p.call((S:{})) }"},
	}
	for _, tt := range stmts {
		p := NewParser([]byte(tt.in))
		got := exprShape(p.parseStmt())
		if err := p.Err(); err != nil {
			t.Errorf("Parse(%q): %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("Parse(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
}

func exprShape(n Node) string {
	if n == nil {
		return ""
	}
	switch n := n.(type) {
	case *ExprStmt:
		return exprShape(n.Expr)
	case *CallStmt:
		return "call{ " + exprShape(n.Stmt) + " }"
	case *UnaryExpr:
		return n.Op.String() + "(" + exprShape(n.X) + ")"
	case *BinaryExpr:
		return "(" + exprShape(n.X) + n.Op.String() + exprShape(n.Y) + ")"
	case *Ident:
		return n.String()
	case *ValueLiteral:
		return n.Tok.String()
	case *SelectorExpr:
		return exprShape(n.X) + "." + exprShape(n.Sel)
	case *IndexExpr:
		return "(" + exprShape(n.X) + "[" + exprShape(n.Index) + "])"
	case *CallExpr:
		args := ""
		if n.Args != nil {
			for i, a := range n.Args.List {
				if i > 0 {
					args += ","
				}
				args += exprShape(a)
			}
		}
		return exprShape(n.Fun) + "(" + args + ")"
	case *ParenExpr:
		s := ""
		for i, a := range n.List {
			if i > 0 {
				s += ","
			}
			s += exprShape(a)
		}
		return "(" + s + ")"
	case *CompositeLiteral:
		return "{}"
	case *RedirectExpr:
		s := "(-> " + exprShape(n.X)
		if len(n.Value) > 0 {
			s += " value"
			for _, v := range n.Value {
				s += " " + exprShape(v)
			}
		}
		if len(n.Param) > 0 {
			s += " param"
		}
		if n.Sender != nil {
			s += " sender " + exprShape(n.Sender)
		}
		if n.Timestamp != nil {
			s += " timestamp " + exprShape(n.Timestamp)
		}
		return s + ")"
	case *ValueExpr:
		return "value(" + exprShape(n.X) + ")"
	case *PatternExpr:
		return "pattern(" + exprShape(n.X) + ")"
	case *LengthExpr:
		return "length(" + exprShape(n.X) + ")"
	case *ParamExpr:
		return "param(" + exprShape(n.X) + ")"
	default:
		return fmt.Sprintf("%T", n)
	}
}

func testParse(t *testing.T, tests []Test, f func(p *parser)) {
	for _, tt := range tests {
		err := anyParse(tt.input, f)
		if tt.expect == pass && err != nil {
			t.Errorf("Parse(%#q):\n\t%v\n\n", tt.input, err)
		}
		if tt.expect == fail && err == nil {
			t.Errorf("breakage vanished: Parse(%#q)", tt.input)
		}
	}
}

func anyParse(input string, f func(p *parser)) error {
	p := NewParser([]byte(input))
	f(p)
	// TODO(5nord) temporary hack until we have proper error handling
	return p.Err()
}

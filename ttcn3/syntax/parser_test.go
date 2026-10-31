package syntax

import (
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
		{pass, `p := @decoded payload`},
		{pass, `p := pattern ".*" & p & "$"`},
		{pass, `p := pattern @nocase ".*" & p & "$"`},
		{pass, `regexp @nocase(x,charstring:"?+(text)?+",0)`},
		{pass, `match(ptc.alive, false)`},
		{pass, `x.universal charstring := "FF80"`},
		{pass, `::E + NS::E`},
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
		{pass, `function f() runs on A[-] {}`},
		{pass, `function f() mtc C {}`},
		{pass, `function f() runs on C mtc C system C {}`},
		{pass, `altstep as() { var roi[-] a[4][4]; [] receive; [else] {}}`},
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
		{pass, `timer x, y := 1.0, y;`},
		{pass, `port P x[len], y := 1, z := 2 ;`},
		{pass, `modulepar RoI[-] x, y:=23, z;`},
		{pass, `const x := 23`},
		{pass, `var template t := 23`},
		{pass, `var t := true`},
		{pass, `var x := 23, y := 10`},
		{pass, `var r := {1,2,3}`},
	}

	testParse(t, valueDecls, func(p *parser) { p.parseValueDecl() })
}

func TestTemplateDecls(t *testing.T) {
	templateDecls := []Test{
		{pass, `template int x := ?;`},
		{pass, `template int x modifies y := ?;`},
		{pass, `template int x modifies y.z := ?;`},
		{pass, `template int x(int i) := i;`},
		{pass, `template @lazy int x := ?;`},
		{pass, `template @lazy int x(int i) := i;`},
		{pass, `template @lazy int  x(int i) modifies y := ?;`},
		{pass, `template @lazy a[-] x(int i) modifies y := ?;`},
		{pass, `template(omit)    int x := ?;`},
		{pass, `template(value)   int x := ?;`},
		{pass, `template(present) int x := ?;`},
	}

	testParse(t, templateDecls, func(p *parser) { p.parseTemplateDecl() })
}

func TestFormalPars(t *testing.T) {
	formalPars := []Test{
		{pass, `()`},
		{pass, `(int y)`},
		{pass, `(int x, int y)`},
		{pass, `(in int x, out int y, inout int z)`},
		{pass, `(in template(value) @fuzzy timer x := 1, out timer y)`},
		{pass, `(out timer y, in template(value) @fuzzy timer x := 1)`},
		{pass, `(out timer y := -, in value @fuzzy timer x := 1)`},
		{pass, `(out timer y := -, in value timer x := (1,2,3))`},
	}
	testParse(t, formalPars, func(p *parser) { p.parseFormalPars() })
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

		// Map Types
		{pass, `type map from charstring to integer m`},
		{pass, `type map from record { int a, int y } to integer m`},
		{pass, `type map from universal charstring to map from integer to charstring m`},

		// Port Types
		{pass, `type port p message {address a.b[-]}`},
		{pass, `type port p message {inout all}`},
		{pass, `type port p message {inout float, a.b[-]}`},
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

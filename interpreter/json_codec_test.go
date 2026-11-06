package interpreter_test

import (
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// TestJSONCodec: encvalue / decvalue on a JSON-encoded type (ES 201
// 873-11) — a record's fields in declaration order, an omitted optional
// left out, a record of, a union, an enumerated value, an octetstring —
// and decoding text that did not come from encvalue, as a service sends
// it: members the type does not declare ignored, a missing optional
// omitted, a malformed or mistyped one a failure.
func TestJSONCodec(t *testing.T) {
	src := `module M {
		type component C {}
		type enumerated State { up, down }
		type union Addr { charstring name, integer ip }
		type record of integer Ports;
		type record Nic {
			charstring ifName,
			integer mtu optional,
			State state,
			Ports ports,
			Addr addr,
			octetstring mac,
			boolean bonded
		} with { encode "JSON" }
		testcase tc() runs on C {
			var Nic n := { ifName := "ens3", mtu := omit, state := up, ports := { 80, 443 },
			               addr := { name := "h" }, mac := 'A0FF'O, bonded := false };
			var universal charstring e := encvalue_unichar(n);
			if (e != "{""ifName"":""ens3"",""state"":""up"",""ports"":[80,443],""addr"":{""name"":""h""},""mac"":""A0FF"",""bonded"":false}") {
				setverdict(fail, "encoded ", e); stop
			}
			var Nic d;
			var universal charstring s := "{ ""mtu"": 1500, ""ifName"": ""eth0"", ""extra"": [1, 2], ""state"": ""down"", ""ports"": [], ""addr"": { ""ip"": 7 }, ""mac"": ""00"", ""bonded"": true }";
			if (decvalue_unichar(s, d) != 0) { setverdict(fail, "decode failed"); stop }
			if (d.ifName != "eth0" or d.mtu != 1500 or d.state != down or lengthof(d.ports) != 0 or d.addr.ip != 7 or d.mac != '00'O or not d.bonded) {
				setverdict(fail, "decoded ", d); stop
			}
			var Nic d2;
			if (decvalue_unichar("{""ifName"":""x"",""state"":""up"",""ports"":[1],""addr"":{""name"":""n""},""mac"":"""",""bonded"":false}", d2) != 0 or ispresent(d2.mtu)) {
				setverdict(fail, "optional ", d2); stop
			}
			var Nic bad;
			if (decvalue_unichar("{""ifName"": 5}", bad) == 0) { setverdict(fail, "a mistyped member decoded"); stop }
			if (decvalue_unichar("{not json", bad) == 0) { setverdict(fail, "malformed text decoded"); stop }
			var octetstring o := encvalue_o(n);
			var Nic r;
			if (decvalue_o(o, r) != 0 or r != n) { setverdict(fail, "octetstring round trip ", r); stop }
			setverdict(pass);
		}
	}`
	for _, k := range clocks {
		v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
		if err != nil || v != runtime.PassVerdict {
			t.Errorf("%s clock: %s (%s) %v", k.name, v, reason, err)
		}
	}
}

// TestJSONTypesOfTheDeclaringModule: a JSON type's fields name types of
// the module that declares it, even where the caller's module has
// like-named types of its own.
func TestJSONTypesOfTheDeclaringModule(t *testing.T) {
	lib := parse(t, `module Lib {
		type record Inner { integer n }
		type record Outer { Inner inner } with { encode "JSON" }
	}`)
	m := parse(t, `module M {
		import from Lib { type Outer };
		type component C {}
		type record Inner { charstring other }
		testcase tc() runs on C {
			var Outer o := { inner := { n := 3 } };
			var universal charstring e := encvalue_unichar(o);
			if (e != "{""inner"":{""n"":3}}") { setverdict(fail, "encoded ", e); stop }
			var Outer d;
			if (decvalue_unichar("{""inner"":{""n"":4}}", d) != 0 or d.inner.n != 4) { setverdict(fail, "decoded ", d); stop }
			setverdict(pass);
		}
	}`)
	for _, order := range [][]*ttcn3.Tree{{m, lib}, {lib, m}} {
		for _, k := range clocks {
			v, reason, err := interpreter.RunTestcaseWith(order, "M.tc", k.opts)
			if err != nil || v != runtime.PassVerdict {
				t.Errorf("%s clock: %s (%s) %v", k.name, v, reason, err)
			}
		}
	}
}

// TestJSONTypeOfAnExpression: the type of a parameter, a function's
// result, a valueof, an element and a field are told as a variable's is;
// and "JSON" given as the dynamic encoding selects the codec.
func TestJSONTypeOfAnExpression(t *testing.T) {
	src := `module M {
		type component C {}
		type record P { integer x, integer y } with { encode "JSON" }
		type record of P Ps with { encode "JSON" }
		type record Box { P p } with { encode "JSON" }
		type record Plain { integer x }
		template P t_p := { x := 1, y := 2 };
		function origin() return P { return { x := 0, y := 0 } }
		function enc(P p) return universal charstring { return encvalue_unichar(p) }
		function dec(universal charstring s, out P p) return integer { return decvalue_unichar(s, p) }
		testcase tc() runs on C {
			var Ps ps := { { x := 5, y := 6 } };
			var Box b := { p := { x := 7, y := 8 } };
			if (enc({ x := 1, y := 1 }) != "{""x"":1,""y"":1}") { setverdict(fail, "parameter"); stop }
			if (encvalue_unichar(origin()) != "{""x"":0,""y"":0}") { setverdict(fail, "result"); stop }
			if (encvalue_unichar(valueof(t_p)) != "{""x"":1,""y"":2}") { setverdict(fail, "valueof"); stop }
			if (encvalue_unichar(ps[0]) != "{""x"":5,""y"":6}") { setverdict(fail, "element"); stop }
			if (encvalue_unichar(b.p) != "{""x"":7,""y"":8}") { setverdict(fail, "field"); stop }
			var P d;
			if (dec("{""x"":3,""y"":4}", d) != 0 or d.y != 4) { setverdict(fail, "out parameter ", d); stop }
			var Plain pl := { x := 9 };
			var universal charstring e := encvalue_unichar(pl, "", "", "JSON");
			if (e != "{""x"":9}") { setverdict(fail, "dynamic encoding ", e); stop }
			var Plain pd;
			if (decvalue_unichar("{""x"":10}", pd, "", "", "JSON") != 0 or pd.x != 10) { setverdict(fail, "dynamic decoding ", pd); stop }
			setverdict(pass);
		}
	}`
	for _, k := range clocks {
		v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
		if err != nil || v != runtime.PassVerdict {
			t.Errorf("%s clock: %s (%s) %v", k.name, v, reason, err)
		}
	}
}

// TestJSONStrictness: text with more than one value, a member named twice,
// or an odd number of octet digits is no encoding; the special floats are
// their names; an unbound mandatory field cannot be encoded; a verdict,
// a bitstring and a standalone enumerated type are coded; <, > and & are
// written as they are.
func TestJSONStrictness(t *testing.T) {
	src := `module M {
		type component C {}
		type record R { octetstring o, float f } with { encode "JSON" }
		type record V { verdicttype v, bitstring b, charstring s } with { encode "JSON" }
		type enumerated E { red, green } with { encode "JSON" }
		testcase tc() runs on C {
			var R r;
			if (decvalue_unichar("{""o"":""00"",""f"":1.0} {}", r) == 0) { setverdict(fail, "trailing value"); stop }
			if (decvalue_unichar("{""o"":""00"",""o"":""01"",""f"":1.0}", r) == 0) { setverdict(fail, "member twice"); stop }
			if (decvalue_unichar("{""o"":""ABC"",""f"":1.0}", r) == 0) { setverdict(fail, "odd octets"); stop }
			if (decvalue_unichar("{""o"":""A?"",""f"":1.0}", r) == 0) { setverdict(fail, "a wildcard digit"); stop }
			var R inf := { o := ''O, f := infinity };
			if (encvalue_unichar(inf) != "{""o"":"""",""f"":""infinity""}") { setverdict(fail, "infinity ", encvalue_unichar(inf)); stop }
			if (decvalue_unichar("{""o"":"""",""f"":""-infinity""}", r) != 0 or r.f != -infinity) { setverdict(fail, "-infinity"); stop }
			var V v := { v := inconc, b := '101'B, s := "<a&b>" };
			var universal charstring e := encvalue_unichar(v);
			if (e != "{""v"":""inconc"",""b"":""101"",""s"":""<a&b>""}") { setverdict(fail, "encoded ", e); stop }
			var V vd;
			if (decvalue_unichar(e, vd) != 0 or vd != v) { setverdict(fail, "decoded ", vd); stop }
			var E c := green;
			if (encvalue_unichar(c) != """green""") { setverdict(fail, "enumerated ", encvalue_unichar(c)); stop }
			var E cd;
			if (decvalue_unichar("""red""", cd) != 0 or cd != red) { setverdict(fail, "enumerated decoded"); stop }
			setverdict(pass);
		}
	}`
	for _, k := range clocks {
		v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
		if err != nil || v != runtime.PassVerdict {
			t.Errorf("%s clock: %s (%s) %v", k.name, v, reason, err)
		}
	}
}

// TestJSONTypeOfTheBindingScope: a name's type is the one its own
// declaration gives — a parameter's, a component variable's — not a
// like-named definition's of another module; a qualified type name is
// that module's; JSON that does not fit the type does not decode, though
// another type's value encoded to the same text.
func TestJSONTypeOfTheBindingScope(t *testing.T) {
	a := parse(t, `module A {
		type record R { integer a } with { encode "JSON" }
		const R x := { a := 1 };
	}`)
	m := parse(t, `module M {
		type record S { charstring b } with { encode "JSON" }
		type component C { var S cv := { b := "c" } }
		function enc(S x) return universal charstring { return encvalue_unichar(x) }
		function dec(universal charstring s, out S x) return integer { return decvalue_unichar(s, x) }
		function encv() runs on C return universal charstring { return encvalue_unichar(cv) }
		testcase tc() runs on C {
			if (enc({ b := "p" }) != "{""b"":""p""}") { setverdict(fail, "parameter ", enc({ b := "p" })); stop }
			var S d;
			if (dec("{""b"":""q""}", d) != 0 or d.b != "q") { setverdict(fail, "out parameter"); stop }
			if (encv() != "{""b"":""c""}") { setverdict(fail, "component variable ", encv()); stop }
			var A.R q := { a := 2 };
			if (encvalue_unichar(q) != "{""a"":2}") { setverdict(fail, "qualified ", encvalue_unichar(q)); stop }
			var universal charstring u := encvalue_unichar(q);
			var S s;
			if (decvalue_unichar(u, s) == 0) { setverdict(fail, "decoded another type's value ", s); stop }
			setverdict(pass);
		}
	}`)
	for _, order := range [][]*ttcn3.Tree{{m, a}, {a, m}} {
		for _, k := range clocks {
			v, reason, err := interpreter.RunTestcaseWith(order, "M.tc", k.opts)
			if err != nil || v != runtime.PassVerdict {
				t.Errorf("%s clock: %s (%s) %v", k.name, v, reason, err)
			}
		}
	}
}

// TestUnionAlternativeReplaced: choosing another alternative of a union
// drops the one it carried, so it encodes as the new one.
func TestUnionAlternativeReplaced(t *testing.T) {
	src := `module M {
		type component C {}
		type union U { integer i, charstring s } with { encode "JSON" }
		testcase tc() runs on C {
			var U u; u.i := 1; u.s := "x";
			if (not ischosen(u.s) or ischosen(u.i) or u != { s := "x" }) { setverdict(fail, u); stop }
			if (encvalue_unichar(u) != "{""s"":""x""}") { setverdict(fail, encvalue_unichar(u)); stop }
			setverdict(pass);
		}
	}`
	for i := 0; i < 5; i++ {
		v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", clocks[0].opts)
		if err != nil || v != runtime.PassVerdict {
			t.Fatalf("%s (%s) %v", v, reason, err)
		}
	}
}

// TestUnionAlternativeReplacedEverywhere: in a parameter, an inout
// parameter, a list element and an anytype too.
func TestUnionAlternativeReplacedEverywhere(t *testing.T) {
	src := `module M {
		type component C {}
		type union U { integer i, charstring s } with { encode "JSON" }
		type record of U Us;
		function f(U p) return universal charstring { p.s := "x"; return encvalue_unichar(p) }
		function g(inout U p) { p.s := "y" }
		testcase tc() runs on C {
			var U u; u.i := 1;
			if (f(u) != "{""s"":""x""}") { setverdict(fail, "parameter ", f(u)); stop }
			g(u);
			if (ischosen(u.i) or u.s != "y") { setverdict(fail, "inout ", u); stop }
			var Us l := { { i := 1 } };
			l[0].s := "z";
			if (ischosen(l[0].i)) { setverdict(fail, "element ", l); stop }
			var anytype a; a.integer := 1; a.charstring := "w";
			if (ischosen(a.integer)) { setverdict(fail, "anytype ", a); stop }
			setverdict(pass);
		}
	}`
	for _, k := range clocks {
		v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
		if err != nil || v != runtime.PassVerdict {
			t.Errorf("%s clock: %s (%s) %v", k.name, v, reason, err)
		}
	}
}

// TestJSONTypesOfAnotherModulesDefinitions: a component variable and an
// imported constant are of their own module's types, though the module
// using them declares like-named ones; a variant on a field's enumerated
// type is not ignored; a wrapper naming another module's type is no
// wrapper of this one.
func TestJSONTypesOfAnotherModulesDefinitions(t *testing.T) {
	a := parse(t, `module XA {
		type record R { integer n } with { encode "JSON" }
		type union U { integer i, charstring s } with { encode "JSON" }
		type component C { var R v := { n := 1 }; var U u := { i := 1 } }
		const R kc := { n := 9 };
	}`)
	b := parse(t, `module XB {
		import from XA { type C; const kc };
		type record R { charstring other } with { encode "JSON" }
		type record U { integer i, charstring s optional }
		type enumerated E { a, b } with { variant "text 'a' as 'AA'" }
		type record RE { E e } with { encode "JSON" }
		testcase tc() runs on C {
			if (encvalue_unichar(v) != "{""n"":1}") { setverdict(fail, "component variable ", encvalue_unichar(v)); stop }
			u.s := "x";
			if (ischosen(u.i)) { setverdict(fail, "component union ", u); stop }
			if (encvalue_unichar(kc) != "{""n"":9}") { setverdict(fail, "imported constant ", encvalue_unichar(kc)); stop }
			var RE re := { e := a };
			if (encvalue_unichar(re) == "{""e"":""a""}") { setverdict(fail, "enumerated variant ignored"); stop }
			var R r;
			if (decvalue_unichar("{""Other.R"":{""other"":""o""}}", r) == 0) { setverdict(fail, "another module's wrapper"); stop }
			if (decvalue_unichar("{""XB.R"":{""other"":""o""}}", r) != 0 or r.other != "o") { setverdict(fail, "own wrapper"); stop }
			setverdict(pass);
		}
	}`)
	for _, order := range [][]*ttcn3.Tree{{b, a}, {a, b}} {
		for _, k := range clocks {
			v, reason, err := interpreter.RunTestcaseWith(order, "XB.tc", k.opts)
			if err != nil || v != runtime.PassVerdict {
				t.Errorf("%s clock: %s (%s) %v", k.name, v, reason, err)
			}
		}
	}
}

// TestRecordFieldOrder: a record value is written with its fields in its
// type's declaration order, however it was written.
func TestRecordFieldOrder(t *testing.T) {
	src := `module M {
		type component C {}
		type record R { integer field1, integer field2, charstring field3 optional }
		testcase tc() runs on C {
			var R r := { field3 := "c", field2 := 2, field1 := 1 };
			var R s;
			s.field2 := 4; s.field1 := 3; s.field3 := omit;
			var R u;
			u := { field2 := 6, field3 := "e", field1 := 5 };
			setverdict(pass, r, s, u);
		}
	}`
	for i := 0; i < 5; i++ {
		v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", clocks[0].opts)
		if err != nil || v != runtime.PassVerdict {
			t.Fatalf("%s (%s) %v", v, reason, err)
		}
		want := `{field1 := 1, field2 := 2, field3 := "c"} {field1 := 3, field2 := 4, field3 := omit} {field1 := 5, field2 := 6, field3 := "e"}`
		if reason != want {
			t.Fatalf("reason %q, want %q", reason, want)
		}
	}
}

// TestSharedConstantIsNotReordered: components receiving with a module
// constant of another record type, at once, leave it as it is — written
// in its own type's order, and not written at all (go test -race).
func TestSharedConstantIsNotReordered(t *testing.T) {
	src := `module M {
		type record A { integer x, integer y }
		type record B { integer y, integer x }
		type port P message { inout A }
		type component C { port P p }
		const B c_b := { y := 1, x := 2 };
		template A t_a := c_b;
		function loop() runs on C {
			for (var integer i := 0; i < 50; i := i + 1) { p.send(t_a); p.receive(A:t_a) }
			setverdict(pass, c_b);
		}
		testcase tc() runs on C {
			var C a := C.create, b := C.create, c := C.create;
			connect(a:p, a:p); connect(b:p, b:p); connect(c:p, c:p);
			a.start(loop()); b.start(loop()); c.start(loop());
			all component.done;
			if (getverdict == pass) { setverdict(pass, c_b) }
		}
	}`
	for _, k := range clocks {
		v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
		if err != nil || v != runtime.PassVerdict || reason != "{y := 1, x := 2}" {
			t.Errorf("%s clock: %s (%s) %v", k.name, v, reason, err)
		}
	}
}

// TestJSONVariants: the JSON encoding instructions of ES 201 873-11 Annex
// B a type or field carries — name as (a text, a case change, name all
// as), omit as null, asValue, fractionDigits, escape as, noType and the
// JSON type identifications at encoding; default, name as, asValue and
// useMinus at decoding — each as the annex's examples have them; and
// decoding a bitstring of JSON text, as encvalue gives it.
func TestJSONVariants(t *testing.T) {
	src := `module M {
type component C {}
type record PhoneNumber {
  integer countryPrefix optional,
  integer networkPrefix,
  integer localNumber
} with { variant(countryPrefix) "omit as null"; variant(networkPrefix) "name as 'net'"; variant(localNumber) "name as uppercased" }
type union PersonID { integer numericID, charstring email, charstring name }
  with { variant(numericID) "name as 'ID'"; variant(email) "name as 'Email'"; variant(name) "name as 'Name'" }
type record of PersonID PersonIDs;
type union U1 { integer i, float f, octetstring os, charstring cs } with { variant "asValue" }
type record of U1 RoU1;
type record Product {
  charstring name,
  float price,
  octetstring id optional,
  charstring origin,
  universal charstring text
} with { variant(id) "default (FFFF)"; variant(origin) "default(Hungary)"; variant(text) "default (char(0,0,0,65) & ""?"")" }
type record Lc { integer fieldOne, charstring fieldTwo } with { variant "name all as capitalized" }
type float Num with { variant "fractionDigits 3" }
type record Nums { Num a, Num b, float c } with { variant(c) "fractionDigits 0" }
type charstring Esc with { variant "escape as usi" }
type record Strs { Esc u, charstring s, charstring t } with { variant(s) "escape as short"; variant(t) "escape as transparent" }
type float MZ with { variant "useMinus" }
type record Z { float plain, MZ minus }
type charstring NT with { variant "noType"; variant "JSON:string" }
function enc(charstring what, charstring got, charstring want) {
  if (got != want) { setverdict(fail, what, ": ", got, " /= ", want) }
}
testcase tc() runs on C {
  enc("omit as null, name as", oct2char(bit2oct(encvalue(PhoneNumber:{ omit, 20, 1234567 }))), "{""countryPrefix"":null,""net"":20,""LOCALNUMBER"":1234567}");
  enc("union names", oct2char(bit2oct(encvalue(PersonIDs:{ { numericID := 189249214 }, { email := "jdoe@mail.com" }, { name := "John Doe" } }))), "[{""ID"":189249214},{""Email"":""jdoe@mail.com""},{""Name"":""John Doe""}]");
  enc("asValue", oct2char(bit2oct(encvalue(RoU1:{ { i := 10 }, { f := 6.4 }, { os := '1ED5'O }, { cs := "hello" } }))), "[10,6.4,""1ED5"",""hello""]");
  enc("name all as", oct2char(bit2oct(encvalue(Lc:{ 1, "x" }))), "{""FieldOne"":1,""FieldTwo"":""x""}");
  enc("fractionDigits", oct2char(bit2oct(encvalue(Nums:{ 3.1415, 3.14, 3.14 }))), "{""a"":31.415E-1,""b"":3.14,""c"":314E-2}");
  var charstring bsl := char(0,0,0,92);
  enc("escape", oct2char(bit2oct(encvalue(Strs:{ "a""b", "x/y", "q""r" }))), "{""u"":""a" & bsl & "u0022b"",""s"":""x" & bsl & "/y"",""t"":""q""r""}");
  enc("noType", oct2char(bit2oct(encvalue(NT:"abc"))), """abc""");
  var Product p;
  var bitstring bs := oct2bit(char2oct("{ ""name"" : ""Shoe"", ""price"" : 29.50, ""text"" : ""available"" }"));
  if (decvalue(bs, p) != 0 or p != { name := "Shoe", price := 29.5, id := 'FFFF'O, origin := "Hungary", text := "available" }) { setverdict(fail, "default: ", p) }
  bs := oct2bit(char2oct("{ ""name"" : ""Shirt"", ""price"" : 12.99, ""id"" : null }"));
  if (decvalue(bs, p) != 0 or p != { name := "Shirt", price := 12.99, id := omit, origin := "Hungary", text := char(0,0,0,65) & "?" }) { setverdict(fail, "default 2: ", p) }
  var RoU1 r;
  bs := oct2bit(char2oct("[10,6.4,""1ED5"",""hello""]"));
  if (decvalue(bs, r) != 0 or r != { { i := 10 }, { f := 6.4 }, { os := '1ED5'O }, { cs := "hello" } }) { setverdict(fail, "asValue dec: ", r) }
  var PhoneNumber pn;
  bs := oct2bit(char2oct("{""net"":20,""LOCALNUMBER"":7}"));
  var PhoneNumber want := { countryPrefix := omit, networkPrefix := 20, localNumber := 7 };
  if (decvalue(bs, pn) != 0 or pn != want) { setverdict(fail, "name as dec: ", pn) }
  var Z z;
  bs := oct2bit(char2oct("{""plain"":-0.0,""minus"":-0.0}"));
  if (decvalue(bs, z) != 0) { setverdict(fail, "useMinus dec") }
  log(z, " ", 1.0 / z.minus);
  setverdict(pass)
}
} with { encode "JSON" }`
	for _, k := range clocks {
		v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
		if err != nil || v != runtime.PassVerdict {
			t.Errorf("%s clock: %s (%s) %v", k.name, v, reason, err)
		}
	}
}

// TestJSONVariantNotApplied: a type with an instruction the codec does not
// apply goes as before there was a JSON codec — its value round-trips —
// and another codec's instruction is not the JSON codec's.
func TestJSONVariantNotApplied(t *testing.T) {
	src := `module M {
		type component C {}
		type record R { integer a } with { variant "useOrder" }
		type record S { integer a } with { variant "XER"."name as 'X'"; variant(a) "name as 'A'" }
		testcase tc() runs on C {
			var R r := { a := 1 }, r2;
			var bitstring b := encvalue(r);
			if (decvalue(b, r2) != 0 or r2 != r) { setverdict(fail, "round trip ", r2) }
			var universal charstring e := encvalue_unichar(S:{ a := 2 });
			if (e != "{""A"":2}") { setverdict(fail, "encoded ", e) }
			setverdict(pass)
		}
	} with { encode "JSON" }`
	for _, k := range clocks {
		v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
		if err != nil || v != runtime.PassVerdict {
			t.Errorf("%s clock: %s (%s) %v", k.name, v, reason, err)
		}
	}
}

// TestJSONVariantsWhereTheyBelong: an instruction applies to what it is
// written for — a field qualifier to the type's own field, or to the field
// of the type an enclosing scope names (`variant (Q.x)`), not to a
// like-named field of a type it was not written on, nor to a field named
// so but for its case; a rename to another member's name leaves the type
// to the placeholder codec; JSON that does not fit the type does not
// decode; a default's value is the declaring module's, a
// structured one written as a value, whoever decodes. The type identifications
// the codec does not apply (JSON:literal) leave the type to the placeholder
// codec; a bitstring encvalue gave decodes to its value whichever
// encoding is named; empty strings and an integral float round-trip.
func TestJSONVariantsWhereTheyBelong(t *testing.T) {
	src := `module M {
		type component C { const integer c_k := 99 }
		const integer c_k := 7;
		type record P { integer a, integer b }
		type record R { integer foo, integer b }
		type record Two { integer aB, integer ab } with { variant(aB) "name as 'X'" }
		type record D { integer x, integer k, charstring s, P p } with {
			variant(k) "default (c_k)"; variant(s) "default (abc)"; variant(p) "default ({3, 4})"
		}
		type enumerated Null { null_ } with { variant "JSON:literal" }
		type integer I with { variant "normalize" }
		type record E { octetstring o, hexstring h, bitstring b, float f }
		type record Clash { integer a, integer b } with { variant(a) "name as 'b'" }
		type record Q { integer x, integer y }
		function dec(universal charstring s) return D {
			const integer c_k := 42; var charstring abc := "shadow";
			var D d;
			if (decvalue_unichar(s, d) != 0) { setverdict(fail, "decode ", s) }
			return d
		}
		testcase tc() runs on C {
			var universal charstring e := encvalue_unichar(R:{ foo := 1, b := 2 });
			if (e != "{""foo"":1,""b"":2}") { setverdict(fail, "R ", e) }
			var Two two := { aB := 1, ab := 2 }, two2;
			if (decvalue(encvalue(two), two2) != 0 or two2 != two) { setverdict(fail, "case ", two2) }
			var D d := dec("{""x"":1}");
			if (d.k != 7 or d.s != "abc" or d.p.a != 3 or d.p.b != 4) { setverdict(fail, "defaults ", d) }
			var Null n := null_, n2;
			if (decvalue(encvalue(n), n2) != 0 or n2 != n) { setverdict(fail, "literal ", n2) }
			var I i := 53, i2;
			if (decvalue(encvalue(i), i2, "", "JSON") != 0 or i2 != 53) { setverdict(fail, "cache ", i2) }
			var E x := { o := ''O, h := ''H, b := ''B, f := 1.0 }, x2;
			if (decvalue(encvalue(x), x2) != 0 or x2 != x) { setverdict(fail, "empty ", x2) }
			if (encvalue_unichar(x) != "{""o"":"""",""h"":"""",""b"":"""",""f"":1.0}") { setverdict(fail, encvalue_unichar(x)) }
			var Clash cl := { a := 1, b := 2 }, cl2;
			if (decvalue(encvalue(cl), cl2) != 0 or cl2 != cl) { setverdict(fail, "clash ", cl2) }
			if (encvalue_unichar(Q:{ x := 1, y := 2 }) != "{""X"":1,""y"":2}") { setverdict(fail, "Q ", encvalue_unichar(Q:{ x := 1, y := 2 })) }
			var P pp;
			if (decvalue(encvalue(R:{ foo := 1, b := 2 }), pp) == 0) { setverdict(fail, "an R decoded as a P ", pp) }
			setverdict(pass)
		}
	} with { encode "JSON"; variant (Foo) "name as 'ZZ'"; variant (Q.x) "name as 'X'" }`
	for _, k := range clocks {
		v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
		if err != nil || v != runtime.PassVerdict {
			t.Errorf("%s clock: %s (%s) %v", k.name, v, reason, err)
		}
	}
}

package interpreter_test

import "testing"

// A field of a synonym keeps the field-qualified attribute of the record
// the synonym refers to, over the synonym's own; an @local attribute of
// a module or group is not inherited by the definitions inside it (ETSI
// 27.1.2.0).
func TestAttributeScopes(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type record of universal charstring RUC;
		type record SourceType { integer field1, integer field2 } with { encode (field1) "Rule1" }
		type SourceType SynonymType with { encode "Rule2" }
		type integer MyIntModule;
		group myGroup {
			type integer MyIntGroup;
		} with { display @local "group" }
		testcase tc() runs on C {
			var SynonymType v_syn;
			var RUC v_f1 := v_syn.field1.encode, v_f2 := v_syn.field2.encode;
			if (v_f1 == { "Rule1" } and v_f2 == { "Rule2" } and
			    MyIntModule.display == "" and MyIntGroup.display == "") { setverdict(pass); }
			else { setverdict(fail, v_f1, v_f2, MyIntModule.display, MyIntGroup.display); }
		}
	} with { display @local "module" }`)
}

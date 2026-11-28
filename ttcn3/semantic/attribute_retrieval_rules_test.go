package semantic

import (
	"slices"
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// Only variant takes a parameter, a universal charstring naming an encode
// attribute of the type (27.8 a-c); setencode applies to a port (27.9 c);
// a decoded field reference is not assigned to (A.1.6.8.3).
func TestAttributeRetrievalRules(t *testing.T) {
	cases := []struct {
		name, stmt, want string
	}{
		{"encode with a parameter", `v_ruc := MyInt.encode("Codec1");`, "attribute-parameter-not-variant"},
		{"display with a parameter", `v_uc := MyInt.display("Codec1");`, "attribute-parameter-not-variant"},
		{"variant with an integer", `v_ruc := MyInt.variant(1);`, "attribute-parameter-type"},
		{"variant of an unknown encoding", `v_ruc := MyInt.variant("Codec3");`, "attribute-parameter-unknown-encoding"},
		{"variant of a value of an unknown encoding", `v_ruc := v_my.variant("Codec3");`, "attribute-parameter-unknown-encoding"},
		{"setencode on an integer", `v_int.setencode(MyInt, "Codec1");`, "setencode-not-a-port"},
		{"decoded field reference assigned", `v_pdu.payload => MyInt := 1;`, "decoded-field-reference-assigned"},

		{"encode", `v_ruc := MyInt.encode;`, ""},
		{"variant", `v_ruc := MyInt.variant;`, ""},
		{"variant of an encoding", `v_ruc := MyInt.variant("Codec2");`, ""},
		{"variant of a module encoding", `v_ruc := MyInt.variant("ModCodec");`, ""},
		{"variant of a value", `v_ruc := v_my.variant("Codec1");`, ""},
		{"variant of a type without encodes", `v_ruc := Plain.variant("Any");`, ""},
		{"variant of a variable", `v_ruc := MyInt.variant(v_uc);`, ""},
		{"setencode on a port", `p.setencode(MyInt, "Codec1");`, ""},
		{"decoded field reference read", `v_int := v_pdu.payload => MyInt;`, ""},
	}
	ruleCodes := []string{"attribute-parameter-not-variant", "attribute-parameter-type", "attribute-parameter-unknown-encoding",
		"setencode-not-a-port", "decoded-field-reference-assigned"}
	for _, c := range cases {
		tree := parse(t, `module M {
			type integer MyInt with { encode "Codec1"; encode "Codec2" }
			type integer Plain;
			type record PDU { octetstring payload }
			type record of universal charstring RUC;
			type port P message { inout MyInt }
			type component C { port P p }
			testcase TC() runs on C {
				var RUC v_ruc;
				var universal charstring v_uc;
				var integer v_int;
				var MyInt v_my;
				var PDU v_pdu;
				`+c.stmt+`
			}
		} with { encode "ModCodec" }`)
		got := codes(NewAnalyzer(&ttcn3.DB{}).Analyze(tree))
		var hit []string
		for _, code := range got {
			if slices.Contains(ruleCodes, code) {
				hit = append(hit, code)
			}
		}
		switch {
		case c.want == "" && len(hit) > 0:
			t.Errorf("%s: unexpected %v", c.name, hit)
		case c.want != "" && !slices.Contains(got, c.want):
			t.Errorf("%s: expected %s, got %v", c.name, c.want, got)
		}
	}
}

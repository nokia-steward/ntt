package lower_test

import (
	"strings"
	"testing"

	"github.com/nokia/ntt/ir"
	"github.com/nokia/ntt/ir/lower"
	"github.com/nokia/ntt/ttcn3"
)

func parse(t *testing.T, src string) *ttcn3.Tree {
	t.Helper()
	tree := ttcn3.Parse(src)
	if tree == nil || tree.Err != nil {
		if tree != nil {
			t.Fatalf("parse: %v\n%s", tree.Err, src)
		}
		t.Fatalf("parse returned nil for:\n%s", src)
	}
	return tree
}

func TestModule_LowersSetverdictTestcase(t *testing.T) {
	tree := parse(t, `module M {
        testcase tc_pass() runs on C {
            setverdict(pass);
        }
    }`)
	m, diags := lower.Module(tree)
	if m == nil {
		t.Fatalf("Module returned nil")
	}
	if m.Name != "M" {
		t.Errorf("module name = %q, want M", m.Name)
	}
	if len(m.Functions) != 1 {
		t.Fatalf("want 1 function, got %d", len(m.Functions))
	}
	fn := m.Functions[0]
	if fn.Name != "tc_pass" {
		t.Errorf("fn.Name = %q", fn.Name)
	}
	dump := ir.Dump(m)
	if !strings.Contains(dump, "setverdict") {
		t.Errorf("missing setverdict in dump:\n%s", dump)
	}
	for _, d := range diags {
		t.Logf("diag: %s", d.Message)
	}
}

func TestModule_LowersIfStatement(t *testing.T) {
	tree := parse(t, `module M {
        testcase tc() runs on C {
            if (1 == 1) { setverdict(pass); } else { setverdict(fail); }
        }
    }`)
	m, _ := lower.Module(tree)
	if m == nil {
		t.Fatalf("Module returned nil")
	}
	dump := ir.Dump(m)
	for _, want := range []string{"const.int", "eq", "cond_br", "setverdict"} {
		if !strings.Contains(dump, want) {
			t.Errorf("missing %q in dump:\n%s", want, dump)
		}
	}
}

func TestModule_LowersFunctionWithReturn(t *testing.T) {
	tree := parse(t, `module M {
        function add(integer a, integer b) return integer { return a + b; }
    }`)
	m, _ := lower.Module(tree)
	if m == nil || len(m.Functions) != 1 {
		t.Fatalf("want 1 function")
	}
	fn := m.Functions[0]
	if fn.Name != "add" {
		t.Errorf("fn.Name = %q", fn.Name)
	}
	if len(fn.Params) != 2 {
		t.Errorf("want 2 params, got %d", len(fn.Params))
	}
	if fn.Return != ir.TypeInt {
		t.Errorf("return type = %s, want int", fn.Return)
	}
	dump := ir.Dump(m)
	if !strings.Contains(dump, "add") || !strings.Contains(dump, "return") {
		t.Errorf("dump missing add/return:\n%s", dump)
	}
}

func TestModule_SkipsUnsupportedDeclsWithDiagnostic(t *testing.T) {
	tree := parse(t, `module M {
        type component C { var integer v }
        testcase tc() runs on C { setverdict(pass); }
    }`)
	m, diags := lower.Module(tree)
	if m == nil || len(m.Functions) != 1 {
		t.Fatalf("expected at least the testcase to survive lowering")
	}
	if len(diags) == 0 {
		t.Errorf("expected a diagnostic for the unsupported component type decl")
	}
}

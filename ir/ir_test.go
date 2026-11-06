package ir_test

import (
	"strings"
	"testing"

	"github.com/nokia/ntt/ir"
)

func TestBuilder_SimpleFunction(t *testing.T) {
	m := &ir.Module{Name: "M"}
	fn := m.AddFunction(&ir.Function{Name: "add", Return: ir.TypeInt})
	a := fn.NewValue(ir.TypeInt)
	b := fn.NewValue(ir.TypeInt)
	fn.Params = []*ir.Value{a, b}

	bld := ir.NewBuilder(fn)
	sum := bld.Add(a, b)
	bld.Return(sum)

	dump := ir.Dump(m)
	for _, want := range []string{"module M", "func add(", "add %1 %2", "return %3"} {
		if !strings.Contains(dump, want) {
			t.Errorf("missing %q in:\n%s", want, dump)
		}
	}
}

func TestBuilder_CondBranch(t *testing.T) {
	m := &ir.Module{Name: "M"}
	fn := m.AddFunction(&ir.Function{Name: "f", Return: ir.TypeVoid})
	bld := ir.NewBuilder(fn)
	cond := bld.ConstBool(true)
	thn := fn.NewBlock("yes")
	els := fn.NewBlock("no")
	bld.CondBranch(cond, thn, els)
	bld.SetBlock(thn)
	bld.Return(nil)
	bld.SetBlock(els)
	bld.Return(nil)

	dump := ir.Dump(m)
	if !strings.Contains(dump, "@yes @no") {
		t.Errorf("missing branch targets: %s", dump)
	}
}

func TestType_String(t *testing.T) {
	cases := map[ir.Type]string{
		ir.TypeInt:    "int",
		ir.TypeBool:   "bool",
		ir.TypeString: "string",
		ir.TypeVoid:   "void",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Errorf("Type(%d).String() = %q, want %q", k, got, want)
		}
	}
}

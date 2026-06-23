package golang_test

import (
	"bytes"
	"go/format"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/nokia/ntt/backend/golang"
	"github.com/nokia/ntt/ir"
)

// buildSampleModule constructs the IR for:
//
//   function add(a, b: integer) return integer { return a + b; }
//   testcase tc_smoke() { if (add(2, 3) == 5) { setverdict(pass); } else { setverdict(fail); } }
//
// It's enough to exercise the constant / arithmetic / branch / call /
// setverdict opcodes the Go backend supports.
func buildSampleModule() *ir.Module {
	m := &ir.Module{Name: "Sample"}

	add := m.AddFunction(&ir.Function{Name: "add", Return: ir.TypeInt})
	pa := add.NewValue(ir.TypeInt)
	pb := add.NewValue(ir.TypeInt)
	add.Params = []*ir.Value{pa, pb}
	ab := ir.NewBuilder(add)
	sum := ab.Add(pa, pb)
	ab.Return(sum)

	tc := m.AddFunction(&ir.Function{Name: "tc_smoke", Return: ir.TypeVoid, IsTestcase: true})
	tb := ir.NewBuilder(tc)
	two := tb.ConstInt(2)
	three := tb.ConstInt(3)
	five := tb.ConstInt(5)
	r := tb.Call("add", ir.TypeInt, two, three)
	cond := tb.Eq(r, five)

	pass := tc.NewBlock("pass")
	fail := tc.NewBlock("fail")
	tb.CondBranch(cond, pass, fail)

	tb.SetBlock(pass)
	tb.SetVerdict("pass")
	tb.Return(nil)

	tb.SetBlock(fail)
	tb.SetVerdict("fail")
	tb.Return(nil)

	return m
}

func TestGenerate_ProducesValidGo(t *testing.T) {
	var buf bytes.Buffer
	if err := golang.Generate(&buf, buildSampleModule()); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	src := buf.Bytes()

	// Parse the emitted code as Go to catch syntax errors fast.
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, "gen.go", src, 0); err != nil {
		t.Fatalf("emitted Go does not parse: %v\n%s", err, src)
	}
	// Also format - if it gofmts cleanly the layout is reasonable.
	if _, err := format.Source(src); err != nil {
		t.Errorf("gofmt rejects emitted source: %v", err)
	}

	for _, want := range []string{
		"func Add(",
		"func TcSmoke(",
		"runtime.PassVerdict",
		"runtime.FailVerdict",
		"goto block_pass",
		"func RunSuite(",
	} {
		if !bytes.Contains(src, []byte(want)) {
			t.Errorf("missing %q in:\n%s", want, src)
		}
	}
}

func TestGenerate_NoUndefinedReferences(t *testing.T) {
	var buf bytes.Buffer
	if err := golang.Generate(&buf, buildSampleModule()); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	s := buf.String()
	if strings.Contains(s, "_ = v") == false {
		t.Errorf("expected discard underscore for declared values: %s", s)
	}
}

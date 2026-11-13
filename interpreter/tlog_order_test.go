package interpreter

import (
	"testing"

	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// TestTypedRecordInDeclarationOrder: a record a codec made without its
// type, its fields in no particular order, is logged with them in the
// order its type declares them.
func TestTypedRecordInDeclarationOrder(t *testing.T) {
	tree := ttcn3.Parse(`module M { type record R { integer status, charstring body } }`)
	var decl *syntax.StructTypeDecl
	tree.Inspect(func(n syntax.Node) bool {
		if d, ok := n.(*syntax.StructTypeDecl); ok {
			decl = d
		}
		return decl == nil
	})
	if decl == nil {
		t.Fatal("no record type")
	}
	rec := runtime.NewRecord()
	rec.Fields["body"] = runtime.NewCharstring("ok")
	rec.Fields["status"] = runtime.NewInt(200)
	v := tlTyped(rec, &runtime.TypeDesc{Name: "R", Struct: decl}, runtime.NewEnv(nil))
	var got []string
	for _, e := range v.Elems {
		got = append(got, e.Name)
	}
	if len(got) != 2 || got[0] != "status" || got[1] != "body" {
		t.Fatalf("fields %v, want [status body]", got)
	}
}

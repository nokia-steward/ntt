package nodes

import "testing"

// nullVisitor verifies that every generated node carries a working
// Accept method that dispatches to the matching Visit* hook.
type nullVisitor struct {
	visited map[NodeKind]int
}

func newNullVisitor() *nullVisitor {
	return &nullVisitor{visited: make(map[NodeKind]int)}
}

func (v *nullVisitor) VisitBlock(n *Block)         { v.visited[n.Kind()]++ }
func (v *nullVisitor) VisitDecl(n *Decl)           { v.visited[n.Kind()]++ }
func (v *nullVisitor) VisitModule(n *Module)       { v.visited[n.Kind()]++ }
func (v *nullVisitor) VisitModuleDef(n *ModuleDef) { v.visited[n.Kind()]++ }
func (v *nullVisitor) VisitParam(n *Param)         { v.visited[n.Kind()]++ }
func (v *nullVisitor) VisitParamList(n *ParamList) { v.visited[n.Kind()]++ }
func (v *nullVisitor) VisitStmt(n *Stmt)           { v.visited[n.Kind()]++ }

func TestVisitorDispatch(t *testing.T) {
	v := newNullVisitor()
	nodes := []Node{
		&Module{}, &ModuleDef{}, &Decl{}, &ParamList{}, &Param{}, &Block{}, &Stmt{},
	}
	for _, n := range nodes {
		n.Accept(v)
	}
	if got, want := len(v.visited), len(nodes); got != want {
		t.Fatalf("visited %d kinds, want %d", got, want)
	}
}

func TestChildrenSkipsNil(t *testing.T) {
	m := &Module{} // no children
	if got := m.Children(); len(got) != 0 {
		t.Fatalf("empty module should have no children, got %v", got)
	}

	m2 := &Module{Defs: []*ModuleDef{nil, {}, nil, {}}}
	got := m2.Children()
	if len(got) != 2 {
		t.Fatalf("expected 2 non-nil children, got %d", len(got))
	}
}

func TestKindString(t *testing.T) {
	cases := map[NodeKind]string{
		KindModule:    "Module",
		KindParamList: "ParamList",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", k, got, want)
		}
	}
}

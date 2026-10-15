package interpreter

import (
	"testing"

	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func TestPortReceiveJSONDefaultMaterialisesOmittedField(t *testing.T) {
	tree := ttcn3.Parse(`module M {
		type record MyRecord {
			integer int optional,
			charstring MyCharString
		} with { encode "JSON"; variant(int) "default (15)" }

		template MyRecord m_msg := { omit, "abcdef" }
		template MyRecord m_rec := { 15, "abcdef" }
	}`)
	if tree == nil || tree.Root == nil {
		t.Fatal("parse returned nil tree")
	}
	mods := tree.Modules()
	if len(mods) != 1 {
		t.Fatalf("got %d modules, want 1", len(mods))
	}
	mod, ok := mods[0].Node.(*syntax.Module)
	if !ok {
		t.Fatalf("module node has type %T", mods[0].Node)
	}

	env := runtime.NewEnv(nil)
	initModuleDefs(env, mod)

	head, ok := env.Get("m_msg")
	if !ok {
		t.Fatal("m_msg was not initialised")
	}
	call := receiveCallExpr(t, "receive(m_rec)")
	if !portReceiveMatches(head, call, env) {
		td := receiveTemplateTypeDesc(call.Args.List[0], env)
		t.Fatalf("receive(m_rec) did not match omitted JSON default field; declared=%q td=%#v head=%s", declaredTypeName(env, "m_rec"), td, head.Inspect())
	}

	plain := receiveCallExpr(t, "receive({ 15, \"abcdef\" })")
	if portReceiveMatches(head, plain, env) {
		t.Fatal("untyped receive unexpectedly materialised JSON default field")
	}
}

func receiveCallExpr(t *testing.T, src string) *syntax.CallExpr {
	t.Helper()
	nodes := syntax.Parse([]byte(src))
	if err := nodes.Err(); err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var call *syntax.CallExpr
	syntax.Inspect(nodes, func(n syntax.Node) bool {
		if c, ok := n.(*syntax.CallExpr); ok && call == nil {
			call = c
			return false
		}
		return true
	})
	if call == nil {
		t.Fatalf("no call expression in %q", src)
	}
	return call
}

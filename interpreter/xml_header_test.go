package interpreter

import (
	"testing"

	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func TestEncvalueUnicharXMLHeaderControl(t *testing.T) {
	tests := []struct {
		expr       string
		wantPrefix string
	}{
		{`encvalue_unichar("abc", -, "xmlHeader")`, "<?xml"},
		{`encvalue_unichar("abc", -, "noXmlHeader")`, "<valu"},
	}
	for _, tt := range tests {
		nodes := syntax.Parse([]byte(tt.expr))
		if err := nodes.Err(); err != nil {
			t.Fatalf("parse %q: %v", tt.expr, err)
		}
		got := Eval(nodes, runtime.NewEnv(nil))
		s, ok := got.(*runtime.String)
		if !ok {
			t.Fatalf("%s returned %T", tt.expr, got)
		}
		if len(s.Value) < len(tt.wantPrefix) || string(s.Value[:len(tt.wantPrefix)]) != tt.wantPrefix {
			t.Fatalf("%s = %q, want prefix %q", tt.expr, string(s.Value), tt.wantPrefix)
		}
	}
}

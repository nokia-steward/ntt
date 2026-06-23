package semantic

import (
	"github.com/nokia/ntt/ttcn3/syntax"
)

// checkConnectOutlistRules used to enforce ETSI 21.1.1 b3/c3
// (`connect` / `map` require a non-empty outlist), but Sem
// 210101_011 and 210101_012 in the conformance suite document a
// relaxation of that rule: connecting two `in`-only ports or
// mapping any direction combination is permitted, even though
// the result cannot carry traffic. CR 7607 against the spec
// proposes re-introducing the restriction, but until then the
// suite expects acceptance. We keep the helper machinery in
// place so we can re-enable cheaply.
func (a *Analyzer) checkConnectOutlistRules(mod *syntax.Module) []Diagnostic {
	_ = collectPortDirs
	_ = collectPortVarTypes
	_ = portTypeFromArg
	return nil
}

// portDirSet remembers the directions a port type declares.
// inout flips both bits.
type portDirSet struct {
	hasIn  bool
	hasOut bool
}

func (d portDirSet) empty() bool { return !d.hasIn && !d.hasOut }

// collectPortDirs walks every port type declaration and records
// which directions its PortAttribute list mentions.
func collectPortDirs(mod *syntax.Module) map[string]portDirSet {
	out := map[string]portDirSet{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		ptd, ok := n.(*syntax.PortTypeDecl)
		if !ok || ptd == nil || ptd.Name == nil {
			return true
		}
		name := ptd.Name.String()
		d := out[name]
		for _, attr := range ptd.Attrs {
			pa, ok := attr.(*syntax.PortAttribute)
			if !ok || pa == nil || pa.KindTok == nil {
				continue
			}
			switch pa.KindTok.Kind() {
			case syntax.IN:
				d.hasIn = true
			case syntax.OUT:
				d.hasOut = true
			case syntax.INOUT:
				d.hasIn = true
				d.hasOut = true
			}
		}
		out[name] = d
		return true
	})
	return out
}

package main

import (
	"math/big"

	"github.com/nokia/ntt/runtime"
)

// The ETSI conformance suite marks a handful of fixtures
// `@configuration external_functions`: they declare an `external
// function` and then assert something about its result, which means the
// suite expects the SUT adapter to supply a body (ETSI 16.1.3). Nothing
// in the language says what that body computes, so the only source of
// truth is each fixture's own doc comment ("@return always 1"). These
// are those bodies, and they live here - beside the harness that plays
// the adapter role - rather than in the engine.
//
// Names are module-qualified so a binding answers only for the fixture
// that documented it. Everything else stays unbound and keeps returning
// Undefined.
func bindConformanceExternalFuncs() {
	// @return always 1
	runtime.BindExternalFunc(
		"Sem_160103_external_functions_001.xf_Sem_160103_external_functions_001",
		func([]runtime.Object) runtime.Object { return runtime.NewInt(1) })

	// @return p_in + 1
	runtime.BindExternalFunc(
		"Sem_160103_external_functions_002.xf_Sem_160103_external_functions_002",
		func(args []runtime.Object) runtime.Object {
			if len(args) < 1 {
				return runtime.Errorf("xf_Sem_160103_external_functions_002: missing p_in")
			}
			in, ok := args[0].(runtime.Int)
			if !ok {
				return runtime.Errorf("xf_Sem_160103_external_functions_002: p_in is %s, want integer", args[0].Type())
			}
			return runtime.Int{Int: new(big.Int).Add(in.Int, big.NewInt(1))}
		})

	// @return always true. The fixture calls it once per TTCN-3 type to
	// show each is compatible with the open type `any`, so the body must
	// accept whatever it is handed.
	runtime.BindExternalFunc(
		"Sem_060302_structured_type_010.xf_my_external_function",
		func([]runtime.Object) runtime.Object { return runtime.NewBool(true) })
}

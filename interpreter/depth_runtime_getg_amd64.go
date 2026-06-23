// Declaration of the getg assembly helper. The body lives in
// depth_runtime_amd64.s (build-constrained). On non-amd64-linux
// builds we provide a stub that returns nil so the init() in
// depth_runtime.go leaves useFastGoid==false and the slow stack-
// parsing fallback stays active.

//go:build amd64 && linux

package interpreter

import "unsafe"

func getg() unsafe.Pointer

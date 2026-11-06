// Declaration of the getg assembly helper. The body lives in
// depth_runtime_amd64.s or depth_runtime_arm64.s. On other
// architectures we provide a stub that returns nil so the init() in
// depth_runtime.go leaves useFastGoid==false and the slow stack-
// parsing fallback stays active.

//go:build amd64 || arm64

package interpreter

import "unsafe"

func getg() unsafe.Pointer

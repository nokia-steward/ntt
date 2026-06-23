// Fallback for platforms without the assembly helper. getg returns
// nil; depth_runtime.go's init() then leaves useFastGoid==false and
// goroutineID() routes to the slow stack-parsing form.

//go:build !(amd64 && linux)

package interpreter

import "unsafe"

func getg() unsafe.Pointer { return nil }

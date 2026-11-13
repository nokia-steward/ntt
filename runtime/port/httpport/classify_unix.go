//go:build !windows

package httpport

// platformReason has nothing to add away from Windows: the portable
// syscall constants in classify() already match what the network stack
// returns. See classify_windows.go for why Windows needs its own table.
func platformReason(error) (string, bool) { return "", false }

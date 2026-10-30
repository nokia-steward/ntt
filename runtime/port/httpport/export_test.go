package httpport

// SetMaxPendingStream lowers the cap on a stream's unreceived values for a
// test; call the result to restore it.
func SetMaxPendingStream(n int) func() {
	old := maxPendingStream
	maxPendingStream = n
	return func() { maxPendingStream = old }
}

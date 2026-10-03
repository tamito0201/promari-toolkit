//go:build windows

package platform

// Alive cannot ask Windows without a process handle API this package does not
// use; the caller falls back to how recently the process was seen.
func (*OS) Alive(int) (alive, ok bool) { return false, false }

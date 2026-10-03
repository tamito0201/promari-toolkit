//go:build !darwin && !linux

package platform

// Parent is not asked on this system; the caller keeps the direct parent.
func (*OS) Parent(int) (ppid int, name string, ok bool) { return 0, "", false }

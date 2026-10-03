//go:build unix

package platform

import (
	"errors"
	"syscall"
)

// Alive sends signal 0, which checks the process without touching it. A
// process of another user answers EPERM: it exists all the same.
func (*OS) Alive(pid int) (alive, ok bool) {
	if pid <= 0 {
		return false, true
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM), true
}

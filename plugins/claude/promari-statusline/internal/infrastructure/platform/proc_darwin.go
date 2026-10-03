//go:build darwin

package platform

import "golang.org/x/sys/unix"

// Parent asks the kernel for a process's parent and command name, without
// starting a process.
func (*OS) Parent(pid int) (ppid int, name string, ok bool) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || kp.Proc.P_pid == 0 {
		return 0, "", false
	}
	return int(kp.Eproc.Ppid), unix.ByteSliceToString(kp.Proc.P_comm[:]), true
}

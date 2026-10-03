//go:build linux

package platform

import (
	"os"
	"strconv"
	"strings"
)

// Parent reads a process's parent and command name from /proc/<pid>/stat,
// without starting a process.
func (*OS) Parent(pid int) (ppid int, name string, ok bool) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, "", false
	}
	return parseStat(string(data))
}

// parseStat reads "pid (comm) state ppid ...". The name is between the first
// "(" and the last ")": a name may itself hold spaces and parentheses.
func parseStat(stat string) (ppid int, name string, ok bool) {
	open, end := strings.IndexByte(stat, '('), strings.LastIndexByte(stat, ')')
	if open < 0 || end < open {
		return 0, "", false
	}
	fields := strings.Fields(stat[end+1:])
	if len(fields) < 2 {
		return 0, "", false
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, "", false
	}
	return ppid, stat[open+1 : end], true
}

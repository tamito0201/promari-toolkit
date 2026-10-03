// Package host reads the computer the session runs on: its load, memory,
// disk and battery, the terminal, the clock and the song that is playing.
package host

import (
	"context"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/domain/repository"
	"promari-statusline/internal/infrastructure/platform"
)

const (
	// lstartLayout is how ps prints a start time, once its padding is collapsed.
	lstartLayout = "Mon Jan 2 15:04:05 2006"
	// ancestorLimit bounds the walk from the status line up to the login shell.
	ancestorLimit = 12
	// kibibyte converts the kibibytes df prints to bytes.
	kibibyte = 1024
	// defaultPageBytes is the page size of Apple silicon, used when vm_stat
	// does not print one.
	defaultPageBytes = 16384
	// availableColumn is the column of df -P that holds the free kibibytes.
	availableColumn = 3
)

// Machine reads the state of the computer with the tools every macOS and most
// Linux systems have. A tool that is missing leaves its reading absent.
type Machine struct {
	Sys platform.System
}

var _ repository.MachineReader = Machine{}

// Machine implements repository.MachineReader. The readings are independent,
// so they are taken at once: each goroutine runs one bounded command (or a
// bounded walk of them) and writes one field of its own.
func (m Machine) Machine(ctx context.Context, dir string) model.Machine {
	machine := model.Machine{CPUs: m.Sys.NumCPU()}
	var wg sync.WaitGroup
	wg.Go(func() { machine.TerminalStart = m.terminalStart(ctx) })
	wg.Go(func() { machine.Load = m.load(ctx) })
	wg.Go(func() { machine.FreeMemory = m.freeMemory(ctx) })
	wg.Go(func() { machine.FreeDisk = m.freeDisk(ctx, dir) })
	wg.Go(func() { machine.Battery = m.battery(ctx) })
	wg.Go(func() { machine.Sessions = len(strings.Fields(m.run(ctx, "pgrep", "-x", "claude"))) })
	wg.Wait()
	return machine
}

// run returns a command's output, or "" when it cannot be run. The tools read
// here exit with an error where there is nothing to print (pgrep without a
// match), so only the output is looked at.
func (m Machine) run(ctx context.Context, name string, args ...string) string {
	out, _ := m.Sys.Run(ctx, platform.Cmd{Name: name, Args: args})
	return out
}

// terminalStart returns when the terminal was opened: the start of the oldest
// process on the status line's terminal, or, for a status line without a
// terminal of its own, the start of the login shell among its ancestors.
func (m Machine) terminalStart(ctx context.Context) model.Optional[time.Time] {
	pid := strconv.Itoa(m.Sys.Pid())
	if tty := strings.TrimSpace(m.run(ctx, "ps", "-o", "tty=", "-p", pid)); tty != "" && !strings.HasPrefix(tty, "?") {
		var oldest model.Optional[time.Time]
		for line := range strings.Lines(m.run(ctx, "ps", "-o", "lstart=", "-t", tty)) {
			if start, ok := parseStart(line); ok && (!oldest.Present() || start.Before(oldest.Or(start))) {
				oldest = model.Some(start)
			}
		}
		if oldest.Present() {
			return oldest
		}
	}
	for range ancestorLimit {
		parent, command, ok := strings.Cut(strings.TrimSpace(m.run(ctx, "ps", "-o", "ppid=,comm=", "-p", pid)), " ")
		if !ok {
			break
		}
		// A login shell is named "-zsh"; a terminal starts it through "login".
		if name := filepath.Base(strings.TrimSpace(command)); name == "login" || strings.HasPrefix(name, "-") {
			if start, ok := parseStart(m.run(ctx, "ps", "-o", "lstart=", "-p", pid)); ok {
				return model.Some(start)
			}
			break
		}
		if n, err := strconv.Atoi(parent); err != nil || n <= 1 {
			break
		}
		pid = parent
	}
	return model.Optional[time.Time]{}
}

func parseStart(lstart string) (time.Time, bool) {
	t, err := time.ParseInLocation(lstartLayout, strings.Join(strings.Fields(lstart), " "), time.Local)
	return t, err == nil
}

// load returns the one-minute load average: from /proc on Linux, from sysctl
// on macOS.
func (m Machine) load(ctx context.Context) model.Optional[float64] {
	text := strings.Trim(m.run(ctx, "sysctl", "-n", "vm.loadavg"), "{} \n")
	if data, err := m.Sys.ReadFile("/proc/loadavg"); err == nil {
		text = string(data)
	}
	first, _, _ := strings.Cut(strings.TrimSpace(text), " ")
	load, err := strconv.ParseFloat(first, 64)
	if err != nil {
		return model.Optional[float64]{}
	}
	return model.Some(load)
}

var (
	pageSizeRe = regexp.MustCompile(`page size of (\d+) bytes`)
	freeRe     = regexp.MustCompile(`Pages (?:free|inactive):\s+(\d+)`)
	batteryRe  = regexp.MustCompile(`(\d+)%; (\w+)`)
)

// freeMemory returns the memory that is free or can be reclaimed at once
// (vm_stat, macOS).
func (m Machine) freeMemory(ctx context.Context) model.Optional[float64] {
	stat := m.run(ctx, "vm_stat")
	pageBytes := defaultPageBytes
	if match := pageSizeRe.FindStringSubmatch(stat); match != nil {
		if n, err := strconv.Atoi(match[1]); err == nil {
			pageBytes = n
		}
	}
	pages := 0
	for _, match := range freeRe.FindAllStringSubmatch(stat, -1) {
		if n, err := strconv.Atoi(match[1]); err == nil {
			pages += n
		}
	}
	if pages == 0 {
		return model.Optional[float64]{}
	}
	return model.Some(float64(pages) * float64(pageBytes))
}

// freeDisk returns the free space of the file system that holds dir (df).
func (m Machine) freeDisk(ctx context.Context, dir string) model.Optional[float64] {
	lines := strings.Split(strings.TrimSpace(m.run(ctx, "df", "-Pk", dir)), "\n")
	fields := strings.Fields(lines[len(lines)-1])
	if len(lines) < 2 || len(fields) <= availableColumn {
		return model.Optional[float64]{}
	}
	available, err := strconv.ParseFloat(fields[availableColumn], 64)
	if err != nil {
		return model.Optional[float64]{}
	}
	return model.Some(available * kibibyte)
}

// battery returns the charge of the battery (pmset, macOS).
func (m Machine) battery(ctx context.Context) model.Optional[model.Battery] {
	match := batteryRe.FindStringSubmatch(m.run(ctx, "pmset", "-g", "batt"))
	if match == nil {
		return model.Optional[model.Battery]{}
	}
	pct, err := strconv.Atoi(match[1])
	if err != nil {
		return model.Optional[model.Battery]{}
	}
	onPower := match[2] == "charging" || match[2] == "charged" || match[2] == "finishing"
	return model.Some(model.Battery{Percent: pct, OnPower: onPower})
}

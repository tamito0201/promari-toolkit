package host_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/domain/repository"
	"promari-statusline/internal/infrastructure/host"
	"promari-statusline/internal/infrastructure/platform/platformtest"
)

var t0 = time.Date(2026, 10, 3, 4, 9, 0, 0, time.UTC)

var errMissing = errors.New("executable file not found")

func TestClock(t *testing.T) {
	t.Parallel()
	if got := (host.Clock{Sys: platformtest.New(t0)}).Now(); !got.Equal(t0) {
		t.Errorf("Now() = %v", got)
	}
}

func TestTerminalWidth(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		columns string
		tty     int
		cells   int
		source  string
	}{
		{"COLUMNS wins", "87", 120, 87, "COLUMNS"},
		{"COLUMNS at the smallest width that is believed", "40", 0, 40, "COLUMNS"},
		{"a COLUMNS too small to believe falls back to the terminal", "39", 120, 120, "tty"},
		{"a COLUMNS that is not a number", "wide", 120, 120, "tty"},
		{"no COLUMNS", "", 96, 96, "tty"},
		{"a terminal too narrow to believe", "", 20, 100, "fallback"},
		{"nothing to measure", "", 0, 100, "fallback"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sys := platformtest.New(t0)
			sys.Env["COLUMNS"] = tt.columns
			sys.Width = tt.tty
			if cells, source := (host.Terminal{Sys: sys}).Width(); cells != tt.cells || source != tt.source {
				t.Errorf("Width() = %d, %q; want %d, %q", cells, source, tt.cells, tt.source)
			}
		})
	}
}

func TestTools(t *testing.T) {
	t.Parallel()
	sys := platformtest.New(t0)
	sys.Path["gh"] = "/opt/homebrew/bin/gh"
	tools := host.Tools{Sys: sys}
	if path, ok := tools.Find("gh"); path != "/opt/homebrew/bin/gh" || !ok {
		t.Errorf("Find(gh) = %q, %v", path, ok)
	}
	if _, ok := tools.Find("ccusage"); ok {
		t.Error("Find(ccusage) found a tool that is not there")
	}
}

func TestNowPlaying(t *testing.T) {
	t.Parallel()
	const command = "nowplaying-cli get title artist playbackRate"
	tests := []struct {
		name string
		out  platformtest.Result
		want model.Track
		err  error
	}{
		{"playing", platformtest.Result{Out: "Take Five\nThe Dave Brubeck Quartet\n1\n"}, model.Track{Title: "Take Five", Artist: "The Dave Brubeck Quartet"}, nil},
		{"paused", platformtest.Result{Out: "Take Five\nnull\n0\n"}, model.Track{Title: "Take Five", Paused: true}, nil},
		{"paused, written as a fraction", platformtest.Result{Out: "Take Five\nSomeone\n0.0\nextra\n"}, model.Track{Title: "Take Five", Artist: "Someone", Paused: true}, nil},
		{"only a title", platformtest.Result{Out: "Take Five\n"}, model.Track{Title: "Take Five"}, nil},
		{"nothing playing", platformtest.Result{Out: "null\nnull\nnull\n"}, model.Track{}, repository.ErrNone},
		{"no output", platformtest.Result{}, model.Track{}, repository.ErrNone},
		{"nowplaying-cli is not installed", platformtest.Result{Err: errMissing}, model.Track{}, errMissing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sys := platformtest.New(t0)
			sys.Cmds[command] = tt.out
			got, err := host.NowPlaying{Sys: sys}.Track(context.Background())
			if got != tt.want || !errors.Is(err, tt.err) {
				t.Errorf("Track() = %+v, %v; want %+v, %v", got, err, tt.want, tt.err)
			}
		})
	}
}

const vmStat = `Mach Virtual Memory Statistics: (page size of 16384 bytes)
Pages free:                               10000.
Pages active:                            400000.
Pages inactive:                           90000.
Pages speculative:                         5000.
`

func TestMachine(t *testing.T) {
	t.Parallel()
	local := func(year int, month time.Month, day, hour, minute, second int) time.Time {
		return time.Date(year, month, day, hour, minute, second, 0, time.Local)
	}
	some := model.Some[float64]
	tests := []struct {
		name  string
		cmds  map[string]platformtest.Result
		files map[string]string
		want  model.Machine
	}{
		{
			"macOS with a terminal of its own",
			map[string]platformtest.Result{
				"ps -o tty= -p 100":        {Out: "ttys003 \n"},
				"ps -o lstart= -t ttys003": {Out: "Thu Oct  1 09:00:00 2026\nnot a date\nWed Sep 30 08:00:00 2026\nFri Oct  2 10:00:00 2026\n"},
				"sysctl -n vm.loadavg":     {Out: "{ 3.94 3.50 3.20 }\n"},
				"vm_stat":                  {Out: vmStat},
				"df -Pk /work":             {Out: "Filesystem 1024-blocks Used Available Capacity Mounted on\n/dev/disk3s5 971350180 800000000 84302236 91% /System/Volumes/Data\n"},
				"pmset -g batt":            {Out: "Now drawing from 'AC Power'\n -InternalBattery-0 (id=1)\t100%; charged; 0:00 remaining present: true\n"},
				"pgrep -x claude":          {Out: "101\n102\n103\n"},
			},
			nil,
			model.Machine{
				TerminalStart: model.Some(local(2026, time.September, 30, 8, 0, 0)), Load: some(3.94), CPUs: 10,
				FreeMemory: some(100_000 * 16384), FreeDisk: some(84302236 * 1024),
				Battery: model.Some(model.Battery{Percent: 100, OnPower: true}), Sessions: 3,
			},
		},
		{
			"a status line without a terminal: the login shell among its ancestors",
			map[string]platformtest.Result{
				"ps -o tty= -p 100":        {Out: "??\n"},
				"ps -o ppid=,comm= -p 100": {Out: "  200 /usr/local/bin/claude\n"},
				"ps -o ppid=,comm= -p 200": {Out: "  300 -zsh\n"},
				"ps -o lstart= -p 200":     {Out: "Fri Oct  2 10:00:00 2026\n"},
				"pmset -g batt":            {Out: " -InternalBattery-0\t15%; discharging; 1:02 remaining\n"},
				"vm_stat":                  {Out: "Pages free: 2.\nPages inactive: 3.\n"},
				"df -Pk /work":             {Out: "Filesystem\n"},
				"pgrep -x claude":          {Err: errMissing},
			},
			map[string]string{"/proc/loadavg": "0.52 0.40 0.30 1/200 12345\n"},
			model.Machine{
				TerminalStart: model.Some(local(2026, time.October, 2, 10, 0, 0)), Load: some(0.52), CPUs: 10,
				FreeMemory: some(5 * 16384), Battery: model.Some(model.Battery{Percent: 15}),
			},
		},
		{
			"started by login rather than a login shell",
			map[string]platformtest.Result{
				"ps -o ppid=,comm= -p 100": {Out: "200 /usr/bin/login\n"},
				"ps -o lstart= -p 100":     {Out: "Fri Oct  2 10:00:00 2026\n"},
			},
			nil,
			model.Machine{TerminalStart: model.Some(local(2026, time.October, 2, 10, 0, 0)), CPUs: 10},
		},
		{
			"the walk stops at init, at a parent that is not a number, and at a start that cannot be read",
			map[string]platformtest.Result{
				"ps -o tty= -p 100":        {Out: "ttys001\n"},
				"ps -o lstart= -t ttys001": {Out: "\n"},
				"ps -o ppid=,comm= -p 100": {Out: "200 node\n"},
				"ps -o ppid=,comm= -p 200": {Out: "1 launchd\n"},
				"sysctl -n vm.loadavg":     {Out: "{ high }\n"},
				"df -Pk /work":             {Out: "Filesystem 1024-blocks Used Available\n/dev/disk1 100 50 lots\n"},
				"pmset -g batt":            {Out: "No battery\n"},
			},
			nil,
			model.Machine{CPUs: 10},
		},
		{
			"a login shell whose start cannot be read",
			map[string]platformtest.Result{
				"ps -o ppid=,comm= -p 100": {Out: "x node\n"},
			},
			nil,
			model.Machine{CPUs: 10},
		},
		{"a machine with none of the tools", nil, nil, model.Machine{CPUs: 10}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sys := platformtest.New(t0)
			sys.CPUs = 10
			sys.Cmds = tt.cmds
			if sys.Cmds == nil {
				sys.Cmds = map[string]platformtest.Result{}
			}
			for name, content := range tt.files {
				sys.Files[name] = []byte(content)
			}
			if got := (host.Machine{Sys: sys}).Machine(context.Background(), "/work"); got != tt.want {
				t.Errorf("Machine() =\n  %+v\nwant\n  %+v", got, tt.want)
			}
		})
	}
	t.Run("a login shell without a readable start time", func(t *testing.T) {
		t.Parallel()
		sys := platformtest.New(t0)
		sys.Cmds["ps -o ppid=,comm= -p 100"] = platformtest.Result{Out: "200 -zsh\n"}
		if got := (host.Machine{Sys: sys}).Machine(context.Background(), "/work"); got.TerminalStart.Present() {
			t.Errorf("a terminal start out of nowhere: %+v", got.TerminalStart)
		}
	})
	t.Run("a page size other than the default, and a battery percentage that is no number", func(t *testing.T) {
		t.Parallel()
		sys := platformtest.New(t0)
		sys.Cmds["vm_stat"] = platformtest.Result{Out: "(page size of 4096 bytes)\nPages free: 10.\n"}
		sys.Cmds["pmset -g batt"] = platformtest.Result{Out: "99999999999999999999%; charging;"}
		got := host.Machine{Sys: sys}.Machine(context.Background(), "/work")
		if got.FreeMemory.Or(0) != 10*4096 || got.Battery.Present() {
			t.Errorf("Machine() = %+v", got)
		}
	})
}

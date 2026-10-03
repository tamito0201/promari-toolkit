// Package di is the composition root: the one place that knows every layer.
// It builds the adapters, hands them to the use cases through the ports, and
// hands the use cases to the command line. Nothing else names an adapter.
package di

import (
	"io"
	"runtime"

	"promari-statusline/internal/application/usecase"
	"promari-statusline/internal/infrastructure/claude"
	"promari-statusline/internal/infrastructure/filecache"
	"promari-statusline/internal/infrastructure/host"
	"promari-statusline/internal/infrastructure/platform"
	"promari-statusline/internal/infrastructure/state"
	"promari-statusline/internal/infrastructure/usage"
	"promari-statusline/internal/infrastructure/vcs"
	"promari-statusline/internal/interfaces/cli"
	"promari-statusline/internal/interfaces/statusline"
)

// Streams are the standard streams of the process.
type Streams struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

// New wires the application on top of sys.
func New(sys platform.System, streams Streams) *cli.App {
	store := filecache.NewStore(sys)
	terminal := host.Terminal{Sys: sys}
	settings := claude.Settings{Sys: sys}
	binary := claude.Binary{Sys: sys, Suffix: executableSuffix()}
	projects := claude.Projects{Sys: sys}
	install := usecase.InstallDeps{Settings: settings, Binary: binary, Projects: projects}

	render := usecase.NewRenderStatusLine(usecase.RenderDeps{
		Clock:      host.Clock{Sys: sys},
		Sources:    sources(sys, store),
		Activities: state.Activities{Store: store},
		Limits:     state.Limits{Store: store},
		Board:      usage.Board{Sys: sys},
		Terminal:   terminal,
		Recorder:   state.Recorder{Store: store},
		Switches:   state.Switches{Store: store},
		Peers:      state.Peers{Store: store, Sys: sys},
	})

	return &cli.App{
		Render:    statusline.Handler{Render: render},
		Install:   usecase.NewInstall(install),
		Global:    usecase.NewInstallGlobal(install),
		Uninstall: usecase.NewUninstall(install),
		Refresh:   usecase.NewRefresh(binary),
		Diagnose: usecase.NewDiagnose(usecase.DiagnoseDeps{
			Settings: settings,
			Binary:   binary,
			Tools:    host.Tools{Sys: sys},
			Terminal: terminal,
			Launcher: claude.Launcher{Sys: sys},
			Projects: projects,
		}),
		In:  streams.In,
		Out: streams.Out,
		Err: streams.Err,
	}
}

// sources builds the readers of a render. Every reader that starts a process,
// asks the network or scans a large file is wrapped in the decorator that
// remembers its answer; the use case sees the port either way.
func sources(sys platform.System, store *filecache.Store) usecase.Sources {
	return usecase.Sources{
		Git:        vcs.Git{Sys: sys, History: filecache.Histories{Store: store, Next: vcs.History{Sys: sys}}},
		Pulls:      filecache.PullRequests{Store: store, Next: vcs.GitHub{Sys: sys}},
		Reviews:    filecache.ReviewQueues{Store: store, Next: vcs.GitHub{Sys: sys}},
		Spend:      usage.CCUsage{Sys: sys},
		Codex:      filecache.Codex{Store: store, Next: usage.Codex{Sys: sys}},
		Transcript: filecache.Transcripts{Store: store, Next: claude.Transcript{Sys: sys}},
		Todos:      claude.Todos{Sys: sys},
		Track:      filecache.Tracks{Store: store, Next: host.NowPlaying{Sys: sys}},
		Incident:   filecache.Incidents{Store: store, Next: claude.StatusPage{Sys: sys}},
		Release:    filecache.Releases{Store: store, Next: claude.Registry{Sys: sys}},
		Account:    filecache.Accounts{Store: store, Next: claude.Account{Sys: sys}},
		Machine:    host.Machine{Sys: sys},
	}
}

func executableSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

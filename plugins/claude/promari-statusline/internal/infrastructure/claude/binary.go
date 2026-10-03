package claude

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"promari-statusline/internal/domain/repository"
	"promari-statusline/internal/infrastructure/platform"
)

const (
	// installDir is the plugin's directory under Claude Code's configuration.
	installDir = "promari-statusline"
	binaryName = "psl"
	// renderCommand is the subcommand the settings run.
	renderCommand = "render"
)

// Binary keeps a copy of the running binary under Claude Code's configuration
// directory. Claude Code installs every version of a plugin into a directory
// of its own, so the settings cannot point into the plugin; they point here.
type Binary struct {
	Sys platform.System
	// Suffix is the executable suffix of the platform: ".exe" on Windows.
	Suffix string
}

var _ repository.BinaryStore = Binary{}

// Path implements repository.BinaryStore.
func (b Binary) Path() string {
	return filepath.Join(ConfigDir(b.Sys), installDir, binaryName+b.Suffix)
}

// Command implements repository.BinaryStore. The path is written with ~ when
// it lies under the home directory, so that settings shared between machines
// (a project's settings.json in a repository) work for every user.
func (b Binary) Command() string {
	path := b.Path()
	if home := b.Sys.HomeDir(); home != "" {
		if rest, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
			path = "~/" + filepath.ToSlash(rest)
		}
	}
	if strings.ContainsAny(path, " \t") {
		path = `"` + path + `"`
	}
	return path + " " + renderCommand
}

// InSync implements repository.BinaryStore.
func (b Binary) InSync() (bool, error) {
	installed, err := b.Sys.ReadFile(b.Path())
	if errors.Is(err, fs.ErrNotExist) {
		return false, repository.ErrNone
	}
	if err != nil {
		return false, fmt.Errorf("read the installed binary: %w", err)
	}
	running, err := b.running()
	if err != nil {
		return false, err
	}
	return bytes.Equal(installed, running), nil
}

// Install implements repository.BinaryStore. The copy replaces the installed
// file in one rename, so a status line that is being drawn keeps the file it
// opened.
func (b Binary) Install() error {
	running, err := b.running()
	if err != nil {
		return err
	}
	return b.Sys.WriteFile(b.Path(), running, platform.Executable)
}

// Remove implements repository.BinaryStore.
func (b Binary) Remove() error { return b.Sys.Remove(b.Path()) }

func (b Binary) running() ([]byte, error) {
	path, err := b.Sys.Executable()
	if err != nil {
		return nil, err
	}
	data, err := b.Sys.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read the running binary: %w", err)
	}
	return data, nil
}

// launcherErrorFile is where bin/psl records a failure to provide a binary.
const launcherErrorFile = "launcher_error"

// Launcher reads what the launcher script left behind.
type Launcher struct {
	Sys platform.System
}

var _ repository.LauncherLog = Launcher{}

// LastError implements repository.LauncherLog. The file's first line is the
// time of the failure, the rest its message.
func (l Launcher) LastError() (string, error) {
	dir := l.Sys.Getenv("CLAUDE_PLUGIN_DATA")
	if dir == "" {
		dir = filepath.Join(ConfigDir(l.Sys), "plugins", "data", installDir)
	}
	data, err := l.Sys.ReadFile(filepath.Join(dir, launcherErrorFile))
	if err != nil || len(bytes.TrimSpace(data)) == 0 {
		return "", repository.ErrNone
	}
	return strings.Join(strings.Fields(string(data)), " "), nil
}

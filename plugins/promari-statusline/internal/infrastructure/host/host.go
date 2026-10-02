package host

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/domain/repository"
	"promari-statusline/internal/infrastructure/platform"
)

// Clock is the machine's clock.
type Clock struct {
	Sys platform.System
}

var _ repository.Clock = Clock{}

// Now implements repository.Clock.
func (c Clock) Now() time.Time { return c.Sys.Now() }

const (
	// minWidth is the narrowest width that is believed; anything smaller is a
	// terminal that did not report its size.
	minWidth = 40
	// fallbackWidth is used when no width can be measured.
	fallbackWidth = 100
)

// Terminal measures the terminal's width.
type Terminal struct {
	Sys platform.System
}

var _ repository.Terminal = Terminal{}

// Width implements repository.Terminal. Claude Code passes the width in
// COLUMNS; without it the controlling terminal is asked, and without one of
// those a default is assumed. (tput reports nothing useful to a status line.)
func (t Terminal) Width() (cells int, source string) {
	if columns, err := strconv.Atoi(t.Sys.Getenv("COLUMNS")); err == nil && columns >= minWidth {
		return columns, "COLUMNS"
	}
	if width, ok := t.Sys.TermWidth(); ok && width >= minWidth {
		return width, "tty"
	}
	return fallbackWidth, "fallback"
}

// Tools looks for executables on PATH.
type Tools struct {
	Sys platform.System
}

var _ repository.ToolFinder = Tools{}

// Find implements repository.ToolFinder.
func (t Tools) Find(name string) (string, bool) { return t.Sys.LookPath(name) }

// nothing is what nowplaying-cli prints for a field that has no value.
const nothing = "null"

// NowPlaying reads the song that is playing with nowplaying-cli (macOS).
type NowPlaying struct {
	Sys platform.System
}

var _ repository.TrackReader = NowPlaying{}

// Track implements repository.TrackReader.
func (n NowPlaying) Track(ctx context.Context) (model.Track, error) {
	out, err := n.Sys.Run(ctx, platform.Cmd{Name: "nowplaying-cli", Args: []string{"get", "title", "artist", "playbackRate"}})
	if err != nil {
		return model.Track{}, fmt.Errorf("nowplaying-cli: %w", err)
	}
	// One line per field, in the order asked for.
	var fields [3]string
	i := 0
	for line := range strings.Lines(out) {
		if i == len(fields) {
			break
		}
		if value := strings.TrimSpace(line); value != nothing {
			fields[i] = value
		}
		i++
	}
	title, artist, rate := fields[0], fields[1], fields[2]
	if title == "" {
		return model.Track{}, repository.ErrNone
	}
	return model.Track{Title: title, Artist: artist, Paused: rate == "0" || rate == "0.0"}, nil
}

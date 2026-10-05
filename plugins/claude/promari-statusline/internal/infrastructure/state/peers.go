package state

import (
	"path/filepath"
	"slices"
	"strings"
	"time"

	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/domain/repository"
	"promari-statusline/internal/infrastructure/filecache"
	"promari-statusline/internal/infrastructure/platform"
)

const (
	peersDir = "peers"
	// peerQuiet is how long a session whose Claude Code cannot be asked about
	// (Windows, or a process the kernel would not describe) still counts as
	// running after its last post.
	peerQuiet = 10 * time.Minute
	// peerKept bounds even a session whose owner answers: a process id is
	// reused, and a day-old post is not news.
	peerKept = 24 * time.Hour
	// ownerHops bounds the walk up from the status line to its Claude Code.
	ownerHops = 4
)

// shells are the command names skipped on the way up to the Claude Code that
// runs the status line: a shell that runs the command, unless it execs it,
// stands between them and exits with every render.
var shells = []string{"sh", "bash", "zsh", "dash", "fish", "ksh"}

// Peers shares each session's summary through a file per session in the
// cache directory.
type Peers struct {
	Store *filecache.Store
	Sys   platform.System
}

var _ repository.PeerBoard = Peers{}

// Post implements repository.PeerBoard.
func (p Peers) Post(peer model.Peer) error {
	peer.Owner = p.owner()
	return filecache.Save(p.Store, peer, peersDir, peer.Key+".json")
}

// owner returns the process id of the Claude Code that started this status
// line: the parent, or the first ancestor that is not a shell.
func (p Peers) owner() int {
	pid := p.Sys.Ppid()
	for range ownerHops {
		parent, name, ok := p.Sys.Parent(pid)
		if !ok || parent <= 1 || !slices.Contains(shells, filepath.Base(name)) {
			return pid
		}
		pid = parent
	}
	return pid
}

// Roster implements repository.PeerBoard. One Claude Code runs one session at
// a time: after /clear or a resume, the session it ran before is left behind
// under its old id, and only the newest post of each owner is kept.
func (p Peers) Roster() model.Roster {
	now := p.Sys.Now()
	var roster model.Roster
	// paths[i] is the file roster[i] was read from. Only a path the glob found
	// is ever removed; a key read from a file never makes a path.
	var paths []string
	byOwner := map[int]int{}
	for _, path := range p.Sys.Glob(p.Store.Path(peersDir, "*.json*")) {
		if !strings.HasSuffix(path, ".json") {
			// The temporary file of a write: one a killed render left behind
			// (a dead session's key is never written again, so no write would
			// sweep it), or one of a write in flight, told apart by age.
			if at, err := p.Sys.ModTime(path); err == nil && now.Sub(at) >= peerQuiet {
				p.drop(path)
			}
			continue
		}
		peer, ok := filecache.Load[model.Peer](p.Store, peersDir, filepath.Base(path))
		if !ok || !p.running(peer, now) {
			p.drop(path)
			continue
		}
		i, seen := byOwner[peer.Owner]
		switch {
		case peer.Owner <= 0 || !seen:
			if peer.Owner > 0 {
				byOwner[peer.Owner] = len(roster)
			}
			roster = append(roster, peer)
			paths = append(paths, path)
		case peer.At.After(roster[i].At):
			p.drop(paths[i])
			roster[i], paths[i] = peer, path
		default:
			p.drop(path)
		}
	}
	return roster
}

// running reports whether the session of a post still runs: its Claude Code is
// alive, or, where that cannot be asked, it posted lately.
func (p Peers) running(peer model.Peer, now time.Time) bool {
	age := now.Sub(peer.At)
	if age >= peerKept {
		return false
	}
	if peer.Owner > 0 {
		if alive, ok := p.Sys.Alive(peer.Owner); ok {
			return alive
		}
	}
	return age < peerQuiet
}

// drop removes the file of a session that no longer runs. A file that cannot
// be removed is dropped again by the next render that sees it.
func (p Peers) drop(path string) { _ = p.Sys.Remove(path) }

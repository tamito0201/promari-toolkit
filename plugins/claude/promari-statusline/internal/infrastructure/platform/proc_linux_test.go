//go:build linux

package platform

import "testing"

func TestParseStat(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		stat string
		ppid int
		name string
		ok   bool
	}{
		{"42 (claude) S 7 42 42 0", 7, "claude", true},
		{"42 (a (weird) name) S 9 42", 9, "a (weird) name", true},
		{"42 claude S 7", 0, "", false},
		{"42 (claude) S", 0, "", false},
		{"42 (claude) S x", 0, "", false},
	} {
		ppid, name, ok := parseStat(c.stat)
		if ppid != c.ppid || name != c.name || ok != c.ok {
			t.Errorf("parseStat(%q) = %d %q %v, want %d %q %v", c.stat, ppid, name, ok, c.ppid, c.name, c.ok)
		}
	}
}

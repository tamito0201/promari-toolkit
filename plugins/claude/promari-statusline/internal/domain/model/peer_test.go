package model_test

import (
	"slices"
	"testing"

	"promari-statusline/internal/domain/model"
)

func TestPeerOf(t *testing.T) {
	t.Parallel()
	s := model.Session{
		ID: "s1", Name: "docs", Dir: "/work/promari", Model: "Opus",
		Cost:    model.Cost{TotalUSD: model.Some(1.5)},
		Context: model.ContextWindow{Size: 200_000, UsedPct: model.Some(42.0)},
	}
	got, ok := model.PeerOf(&s, "a@example.com", "develop", t0)
	want := model.Peer{
		Key: "s1", At: t0, Account: "a@example.com", Name: "docs", Project: "promari", Branch: "develop", Model: "Opus",
		ContextPct: model.Some(42.0), CostUSD: model.Some(1.5),
	}
	if !ok || got != want {
		t.Errorf("PeerOf = %+v, %v\nwant %+v", got, ok, want)
	}

	bare := model.Session{ID: "s2"}
	if got, ok := model.PeerOf(&bare, "", "", t0); !ok || got.Project != "" || got.ContextPct.Present() {
		t.Errorf("a bare session = %+v, %v", got, ok)
	}
	unsafe := model.Session{ID: "../x"}
	if _, ok := model.PeerOf(&unsafe, "", "", t0); ok {
		t.Error("an id that cannot name a file was posted")
	}
}

func TestPeerLabel(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		peer model.Peer
		want string
	}{
		{model.Peer{Key: "k", Name: "n", Project: "p"}, "n"},
		{model.Peer{Key: "k", Project: "p"}, "p"},
		{model.Peer{Key: "k"}, "k"},
	} {
		if got := c.peer.Label(); got != c.want {
			t.Errorf("Label(%+v) = %q, want %q", c.peer, got, c.want)
		}
	}
}

func TestRoster(t *testing.T) {
	t.Parallel()
	r := model.Roster{
		{Key: "z", Project: "beta", Account: "a"},
		{Key: "self", Project: "alpha", Account: "a"},
		{Key: "y", Project: "alpha", Account: "a"},
		{Key: "x", Project: "alpha", Account: "b"},
		{Key: "w", Project: "alpha"},
	}
	keys := func(r model.Roster) []string {
		out := make([]string, 0, len(r))
		for _, p := range r {
			out = append(out, p.Key)
		}
		return out
	}
	if got, want := keys(r.Of("a")), []string{"z", "self", "y"}; !slices.Equal(got, want) {
		t.Errorf("Of(a) = %v, want %v", got, want)
	}
	if got, want := keys(r.Of("")), []string{"w"}; !slices.Equal(got, want) {
		t.Errorf("Of(\"\") = %v, want %v", got, want)
	}
	if got, want := keys(r.Of("a").Others("self")), []string{"y", "z"}; !slices.Equal(got, want) {
		t.Errorf("Others = %v, want %v (by label, then id)", got, want)
	}
	if len(r) != 5 || r[0].Key != "z" {
		t.Errorf("the roster was changed: %v", keys(r))
	}
}

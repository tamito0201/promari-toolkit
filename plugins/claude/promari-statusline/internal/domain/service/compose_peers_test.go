package service

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"promari-statusline/internal/domain/model"
)

func peerGroup(v *View) []string {
	for _, line := range render(Compose(v)) {
		if len(line) > len("👥 Sessions") && line[:len("👥 Sessions")] == "👥 Sessions" {
			return []string{line}
		}
	}
	return nil
}

func TestComposePeers(t *testing.T) {
	t.Parallel()
	many := make(model.Roster, 0, maxPeers+2)
	for i := range maxPeers + 2 {
		many = append(many, model.Peer{Key: "k" + strconv.Itoa(i), Project: "p" + strconv.Itoa(i), At: now})
	}
	tests := []struct {
		name string
		view View
		want []string
	}{
		{"alone, there is no group", View{}, nil},
		{
			"each other session with what its status line last showed",
			View{Running: 3, Peers: model.Roster{
				{Key: "a", Project: "web-app", Branch: "feat/a-very-long-branch-name", ContextPct: model.Some(18.4), CostUSD: model.Some(0.425), At: now},
				{Key: "b", Name: "docs", Project: "portal", At: now.Add(-90 * time.Second)},
			}},
			[]string{"👥 Sessions: Live ×3 | web-app feat/a-very-long-… Ctx 18% $0.42 | docs Idle 1m"},
		},
		{"an unknown count is at least the sessions shown and this one", View{Peers: model.Roster{{Key: "a", At: now}}}, []string{"👥 Sessions: Live ×2 | a"}},
		{
			"past the limit the rest are counted",
			View{Running: maxPeers + 3, Peers: many},
			[]string{"👥 Sessions: Live ×11 | p0 | p1 | p2 | p3 | p4 | p5 | p6 | p7 | +2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			v := tt.view
			v.Now = now
			if got := peerGroup(&v); !slices.Equal(got, tt.want) {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestComposePeerAlarm(t *testing.T) {
	t.Parallel()
	v := View{Now: now, Peers: model.Roster{
		{Key: "full", Project: "full", ContextPct: model.Some(93.0), At: now},
		{Key: "fine", Project: "fine", ContextPct: model.Some(40.0), At: now},
	}}
	if got, want := alarms(Compose(&v)), []string{"full Ctx 93%"}; !slices.Equal(got, want) {
		t.Errorf("alarms = %q, want %q", got, want)
	}
}

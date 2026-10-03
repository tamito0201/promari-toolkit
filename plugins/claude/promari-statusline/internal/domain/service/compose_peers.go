package service

import (
	"strconv"
	"time"

	"promari-statusline/internal/domain/model"
)

const (
	// The longest name and branch of another session shown before they are cut.
	peerLabelCharacters  = 20
	peerBranchCharacters = 18
	// maxPeers is how many other sessions get a chip of their own; the rest are
	// counted in one more.
	maxPeers = 8
	// peerIdleAfter is how long another session may be quiet before its chip
	// says for how long: its numbers are then that old.
	peerIdleAfter = time.Minute
)

// peerChips shows the other sessions running on this machine, one chip each:
// name, branch, context and cost, as their own status lines last showed them.
// Every status line posts its session on each render, so all of them show the
// same sessions.
func peerChips(v *View) []model.Chip {
	if len(v.Peers) == 0 {
		return nil
	}
	chips := []model.Chip{chip(model.ToneMuted, "Live ×"+strconv.Itoa(max(v.Running, len(v.Peers)+1)))}
	for i := range v.Peers {
		if i == maxPeers {
			chips = append(chips, chip(model.ToneMuted, "+"+strconv.Itoa(len(v.Peers)-maxPeers)))
			break
		}
		chips = append(chips, peerChip(v, &v.Peers[i]))
	}
	return chips
}

func peerChip(v *View, p *model.Peer) model.Chip {
	c := model.Chip{{Text: truncate(p.Label(), peerLabelCharacters), Tone: model.ToneNote, Bold: true}}
	if p.Branch != "" {
		c = append(c, space(), text(model.ToneMuted, truncate(p.Branch, peerBranchCharacters)))
	}
	alarm := false
	if pct, ok := p.ContextPct.Get(); ok {
		c = append(c, space(), text(severity(pct, warnPct, badPct), "Ctx "+fixed(pct, 0)+"%"))
		alarm = pct >= v.alarmAt()
	}
	if cost, ok := p.CostUSD.Get(); ok {
		c = append(c, space(), text(model.ToneMoney, "$"+fixed(cost, 2)))
	}
	if idle := v.Now.Sub(p.At); idle >= peerIdleAfter {
		c = append(c, space(), text(model.ToneMuted, "Idle "+span(idle)))
	}
	return alarmIf(alarm, c)
}

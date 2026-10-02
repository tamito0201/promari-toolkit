package service

import (
	"strconv"
	"strings"

	"promari-statusline/internal/domain/model"
)

const (
	// The load per CPU, in percent, where the load turns yellow and red.
	loadWarnPct = 70.0
	loadBadPct  = 100.0

	// The battery is green from batteryOKPct, yellow from batteryLowPct, red below.
	batteryOKPct  = 50
	batteryLowPct = model.LowBatteryPct
)

// systemChips shows the clock and the state of the machine.
func systemChips(v *View) []model.Chip {
	machine := v.Facts.Machine
	chips := []model.Chip{chip(model.ToneInfo, "🕐 "+clock(v.Now))}
	if start, ok := machine.TerminalStart.Get(); ok {
		chips = append(chips, chip(model.ToneMuted, "Term up "+span(v.Now.Sub(start))))
	}
	if load, ok := machine.Load.Get(); ok {
		cpus := max(1, machine.CPUs)
		chips = append(chips, model.Chip{
			text(model.ToneMuted, "CPU"),
			space(),
			text(severity(load/float64(cpus)*percent, loadWarnPct, loadBadPct), fixed(load, 1)),
			text(model.ToneMuted, "/"+strconv.Itoa(cpus)+"c"),
		})
	}
	if free, ok := machine.FreeMemory.Get(); ok {
		chips = append(chips, chip(model.ToneMuted, "🧮 Mem "+gib(free, 1)))
	}
	if free, ok := machine.FreeDisk.Get(); ok {
		chips = append(chips, chip(model.ToneMuted, "💾 Disk "+gib(free, 0)))
	}
	if battery, ok := machine.Battery.Get(); ok {
		chips = append(chips, alarmIf(battery.Low() || v.AlarmAll, batteryChip(battery)))
	}
	return chips
}

func batteryChip(b model.Battery) model.Chip {
	icon := "🔋"
	if b.OnPower {
		icon = "🔌"
	}
	tone := model.ToneDanger
	switch {
	case b.Percent >= batteryOKPct:
		tone = model.ToneGood
	case b.Percent >= batteryLowPct:
		tone = model.ToneCaution
	}
	return model.Chip{
		text(model.TonePlain, icon+" "),
		text(model.ToneMuted, "Bat "),
		text(tone, strconv.Itoa(b.Percent)+"%"),
	}
}

// metaChips shows what surrounds the session: parallel sessions, the account,
// the version and whether a newer one exists.
func metaChips(v *View) []model.Chip {
	var chips []model.Chip
	if n := v.Facts.Machine.Sessions; n > 1 {
		chips = append(chips, chip(model.ToneAccent, "⛵ Proc ×"+strconv.Itoa(n)))
	}
	if account, ok := v.Facts.Account.Get(); ok && account != "" {
		name, _, _ := strings.Cut(account, "@")
		chips = append(chips, chip(model.ToneInfo, "👤 "+name))
	}
	if v.Session.Version != "" {
		chips = append(chips, chip(model.ToneMuted, "v"+v.Session.Version))
	}
	if latest, ok := v.Facts.Latest.Get(); ok && Newer(latest, v.Session.Version) {
		chips = append(chips, chip(model.ToneCaution, "🆙 Update v"+latest))
	}
	return chips
}

// musicChips shows the song that is playing.
func musicChips(v *View) []model.Chip {
	track, ok := v.Facts.Track.Get()
	if !ok || track.Title == "" {
		return nil
	}
	title := track.Title
	if track.Artist != "" {
		title += " — " + track.Artist
	}
	c := chip(model.ToneAccent, truncate(title, trackCharacters))
	if track.Paused {
		c = append(c, text(model.ToneMuted, " (Paused)"))
	}
	return []model.Chip{c}
}

// Newer reports whether latest is a newer version than current. Both are
// dotted numbers; anything else (a pre-release tag, an empty string) is not
// compared and gives false.
func Newer(latest, current string) bool {
	a, okA := versionParts(latest)
	b, okB := versionParts(current)
	if !okA || !okB {
		return false
	}
	for i := range min(len(a), len(b)) {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return len(a) > len(b)
}

func versionParts(v string) ([]int, bool) {
	if v == "" {
		return nil, false
	}
	var parts []int
	for part := range strings.SplitSeq(v, ".") {
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil, false
		}
		parts = append(parts, n)
	}
	return parts, true
}

package service

import (
	"strconv"

	"promari-statusline/internal/domain/model"
)

// traceChips shows how the agent works through the session: how much it
// explores per edit, what it reads again, how long it edits without reading,
// how much of the context its tools' output takes, how many prompts went by
// since the last compaction and how unevenly the prompts cost.
//
// The studies behind these measure where trajectories that fail differ from
// those that succeed; except for the run of edits, none gives a threshold for
// one session, so only that run is coloured.
func traceChips(v *View) []model.Chip {
	t, ok := v.Facts.Transcript.Get()
	if !ok {
		return nil
	}
	tr := t.Trace
	var chips []model.Chip
	// Passing trajectories browse more before they act (Oderinwale, "Agent
	// trajectories as programs", 2026).
	if ratio, ok := tr.ExploreRatio(t.Quality.Edits); ok {
		chips = append(chips, chip(model.ToneInfo, "Explore "+fixed(ratio, 1)+"/edit"))
	}
	if tr.Rereads+tr.Researches > 0 {
		chips = append(chips, model.Chip{
			text(model.ToneCaution, "Reread ×"+strconv.Itoa(tr.Rereads+tr.Researches)),
			text(model.ToneMuted, " (read "+strconv.Itoa(tr.Rereads)+" · search "+strconv.Itoa(tr.Researches)+")"),
		})
	}
	if tr.MaxEditRun > 1 {
		tone := model.ToneMuted
		if tr.MaxEditRun >= model.LongEditRun {
			tone = model.ToneCaution
		}
		chips = append(chips, chip(tone, "EditRun max "+strconv.Itoa(tr.MaxEditRun)))
	}
	// Observations took 62-84% of an agent's context in the studies of
	// SWE-agent's trajectories (Lindenbauer et al., "The Complexity Trap",
	// NeurIPS 2025 DL4C workshop).
	if all, largest := tr.ObservedTokens(); all > 0 {
		c := model.Chip{text(model.ToneNote, "Obs ≈"+tokens(all))}
		if usage, ok := v.Usage.Get(); ok && usage.Used > 0 {
			c = append(c, text(model.ToneMuted, " ("+fixed(min(all/usage.Used, 1)*percent, 0)+"% ctx)"))
		}
		c = append(c, text(model.ToneMuted, " max "+tokens(largest)))
		chips = append(chips, c)
	}
	// Models answered 39% worse over a multi-turn conversation than in one
	// turn (Laban et al., "LLMs Get Lost In Multi-Turn Conversation", 2025).
	if tr.PromptsSinceCompact > 0 && t.Compactions > 0 {
		chips = append(chips, chip(model.ToneMuted, "Since compact "+strconv.Itoa(tr.PromptsSinceCompact)+" prompts"))
	}
	// Runs of the same task differed by up to 30 times in tokens (Bai et al.,
	// "How Do AI Agents Spend Your Money?", 2026).
	if median, largest, ok := tr.TurnSpread(); ok && median > 0 {
		chips = append(chips, model.Chip{
			text(model.ToneNote, "PromptTok p50 "+tokens(median)),
			text(model.ToneMuted, " max "+tokens(largest)+" (×"+fixed(largest/median, 0)+")"),
		})
	}
	return chips
}

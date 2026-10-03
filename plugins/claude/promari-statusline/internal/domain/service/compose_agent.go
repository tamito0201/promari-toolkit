package service

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"promari-statusline/internal/domain/model"
)

const (
	// modelsShown is how many models the mix names.
	modelsShown = 3
	// modelNameCharacters bounds a model's name in the mix.
	modelNameCharacters = 12
)

// agentChips shows how the agent and the human work together: how many steps
// the agent takes per prompt, how often the human steps in, what the model
// refused or cut short, and which models and subagents answered.
//
// Autonomy follows METR's view of agents by the length of the tasks they finish
// on their own ("Measuring AI Ability to Complete Long Tasks", 2025); the
// interventions are its reverse side: the prompts a human had to interrupt or
// whose tool calls they refused.
func agentChips(v *View) []model.Chip {
	t, ok := v.Facts.Transcript.Get()
	if !ok {
		return nil
	}
	var chips []model.Chip
	if t.Prompts > 0 {
		chips = append(chips, chip(model.ToneMuted, "Prompts ×"+strconv.Itoa(t.Prompts)))
	}
	if auto, ok := t.Autonomy(); ok {
		chips = append(chips, chip(model.ToneAccent, "Auto ×"+fixed(auto, 1)+"/prompt"))
	}
	if rate, ok := t.Interventions(); ok && t.Interrupts+t.Denials > 0 {
		chips = append(chips, model.Chip{
			text(model.ToneCaution, "Interv "+fixed(rate, 0)+"%"),
			text(model.ToneMuted, " (✋"+strconv.Itoa(t.Interrupts)+" ⛔"+strconv.Itoa(t.Denials)+")"),
		})
	}
	for _, count := range []struct {
		n     int
		tone  model.Tone
		label string
	}{
		{t.Refusals, model.ToneDanger, "Refuse ×"},
		{t.Truncated, model.ToneCaution, "MaxTok ×"},
	} {
		if count.n > 0 {
			chips = append(chips, chip(count.tone, count.label+strconv.Itoa(count.n)))
		}
	}
	if t.Queued > 0 {
		chips = append(chips, chip(model.ToneMuted, "Queued ×"+strconv.Itoa(t.Queued)))
	}
	if t.WebSearches+t.WebFetches > 0 {
		chips = append(chips, chip(model.ToneInfo, "Web search "+strconv.Itoa(t.WebSearches)+" fetch "+strconv.Itoa(t.WebFetches)))
	}
	if t.SideRequests > 0 {
		chips = append(chips, chip(model.ToneNote, "Sub ×"+strconv.Itoa(t.SideRequests)+" req"))
	}
	if len(t.Models) > 1 {
		chips = append(chips, modelMix(t))
	}
	return chips
}

// modelMix names the models that answered, the busiest first, with their
// share of the responses.
func modelMix(t model.Transcript) model.Chip {
	type share struct {
		name string
		n    int
	}
	shares := make([]share, 0, len(t.Models))
	total := 0
	for name, n := range t.Models {
		shares = append(shares, share{name, n})
		total += n
	}
	slices.SortFunc(shares, func(a, b share) int {
		if a.n != b.n {
			return b.n - a.n
		}
		return strings.Compare(a.name, b.name)
	})
	parts := make([]string, 0, modelsShown)
	for _, s := range shares[:min(len(shares), modelsShown)] {
		name := truncate(strings.TrimPrefix(s.name, "claude-"), modelNameCharacters)
		parts = append(parts, name+" "+fixed(float64(s.n)/float64(total)*percent, 0)+"%")
	}
	return chip(model.ToneAccent, "Models "+strings.Join(parts, " · "))
}

// turnChip shows the durations of the turns: the last, the median and the 90th
// percentile. A turn is the feedback loop of the DevEx framework (Noda,
// Storey, Forsgren and Greiler, ACM Queue 21(2), 2023): the time from asking
// to having the answer.
func turnChip(t model.Transcript) (model.Chip, bool) {
	times, ok := t.TurnTimes()
	if !ok {
		return nil, false
	}
	return model.Chip{
		text(model.ToneNote, "Turn "+brief(times.Last)),
		text(model.ToneMuted, " (p50 "+brief(times.P50)+" · p90 "+brief(times.P90)+")"),
	}, true
}

// brief formats a duration that may be shorter than a minute: 42s, then as span.
func brief(d time.Duration) string {
	if d < time.Minute {
		return strconv.Itoa(int(max(0, d)/time.Second)) + "s"
	}
	return span(d)
}

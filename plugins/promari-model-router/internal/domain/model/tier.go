// Package model holds the value objects and entities of the routing domain.
// Value objects are immutable: every operation returns a new value.
package model

import (
	"regexp"
	"slices"
	"strings"
)

// Tier is a Claude model family ordered by capability and cost.
type Tier string

// The known tiers, cheapest first.
const (
	TierUnknown Tier = ""
	TierHaiku   Tier = "haiku"
	TierSonnet  Tier = "sonnet"
	TierOpus    Tier = "opus"
	TierFable   Tier = "fable"
)

// TierOrder is the capability order, cheapest first.
var TierOrder = []Tier{TierHaiku, TierSonnet, TierOpus, TierFable}

var familyRE = regexp.MustCompile(`(?i)(haiku|sonnet|opus|fable)`)

// TierOf maps an alias or full model ID (claude-opus-5-5[1m]) to a Tier.
// Strings without a family name ("inherit", "default") give TierUnknown.
func TierOf(model string) Tier {
	return Tier(strings.ToLower(familyRE.FindString(model)))
}

// Rank is the position in TierOrder, or -1 for an unknown tier.
func (t Tier) Rank() int { return slices.Index(TierOrder, t) }

// Known reports whether the tier is one of TierOrder.
func (t Tier) Known() bool { return t.Rank() >= 0 }

// Above reports whether t is strictly above other. Unknown tiers never compare.
func (t Tier) Above(other Tier) bool {
	return t.Known() && other.Known() && t.Rank() > other.Rank()
}

// Min returns the cheaper tier; an unknown side yields the other one.
func (t Tier) Min(other Tier) Tier {
	switch {
	case !t.Known():
		return other
	case !other.Known():
		return t
	case t.Rank() <= other.Rank():
		return t
	default:
		return other
	}
}

func (t Tier) String() string { return string(t) }

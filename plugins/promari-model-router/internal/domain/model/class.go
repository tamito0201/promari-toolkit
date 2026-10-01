package model

import (
	"maps"
	"slices"
)

// Class is the kind of work a prompt asks for.
type Class string

// The routing classes, from the cheapest to the most demanding.
const (
	ClassNone         Class = "" // abstain
	ClassLookup       Class = "lookup"
	ClassMechanical   Class = "mechanical"
	ClassStandard     Class = "standard"
	ClassComplex      Class = "complex"
	ClassArchitecture Class = "architecture"
)

// ClassOrder lists the classes from the cheapest to the most demanding.
var ClassOrder = []Class{ClassLookup, ClassMechanical, ClassStandard, ClassComplex, ClassArchitecture}

// ParseClass returns the class for a name, or ClassNone.
func ParseClass(name string) Class {
	if c := Class(name); slices.Contains(ClassOrder, c) {
		return c
	}
	return ClassNone
}

// Rank is the position in ClassOrder, or -1 for ClassNone.
func (c Class) Rank() int { return slices.Index(ClassOrder, c) }

// Abstained reports whether no class was chosen.
func (c Class) Abstained() bool { return c == ClassNone }

// Cheap reports whether the class may route below sonnet.
func (c Class) Cheap() bool { return c == ClassLookup || c == ClassMechanical }

// Deep reports whether the class needs judgement (debugging or design).
func (c Class) Deep() bool { return c == ClassComplex || c == ClassArchitecture }

// DowngradeSensitive reports whether a danger flag must block routing this class.
func (c Class) DowngradeSensitive() bool {
	return c == ClassLookup || c == ClassMechanical || c == ClassStandard
}

func (c Class) String() string { return string(c) }

// TierSpec is the model and effort a class or an agent maps to.
type TierSpec struct {
	Model   Tier   `json:"model" toml:"model"`
	Effort  string `json:"effort" toml:"effort"`
	Summary string `json:"summary" toml:"summary"`
}

// TierTable is the single source of truth loaded from data/tiers.toml.
type TierTable struct {
	Order         []Tier               `json:"order" toml:"order"`
	Ceiling       Tier                 `json:"auto_route_ceiling" toml:"auto_route_ceiling"`
	Classes       map[Class]TierSpec   `json:"classes" toml:"classes"`
	Agents        map[string]AgentSpec `json:"agents" toml:"agents"`
	BuiltinAgents []string             `json:"builtin_agents" toml:"builtin_agents"`
}

// AgentSpec describes a fixed-tier agent shipped in agents/.
type AgentSpec struct {
	Model  Tier   `json:"model" toml:"model"`
	Effort string `json:"effort" toml:"effort"`
	Class  Class  `json:"class" toml:"class"`
}

// Target returns the tier for a class, capped at the auto-route ceiling
// (fable may bill usage credits, so it is never chosen automatically).
func (t TierTable) Target(c Class) Tier {
	spec, ok := t.Classes[c]
	if !ok || !spec.Model.Known() {
		return TierUnknown
	}
	return spec.Model.Min(t.Ceiling)
}

// IsBuiltinAgent reports whether a subagent type is a Claude Code built-in.
func (t TierTable) IsBuiltinAgent(name string) bool {
	return slices.Contains(t.BuiltinAgents, name)
}

// Clone returns a deep copy (the provider hands the same table to every caller).
func (t TierTable) Clone() TierTable {
	t.Order, t.BuiltinAgents = slices.Clone(t.Order), slices.Clone(t.BuiltinAgents)
	t.Classes, t.Agents = maps.Clone(t.Classes), maps.Clone(t.Agents)
	return t
}

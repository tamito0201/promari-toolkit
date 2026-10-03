package service

import (
	"strconv"

	"promari-statusline/internal/domain/model"
)

// ruleLabels name the rules on the status line, with the tone of a break: a
// danger for what lets data leak, be lost or be attacked, a caution for what
// hides a failure or breaks a rule the course states, muted for layout.
var ruleLabels = [...]struct {
	label string
	tone  model.Tone
}{
	model.RuleSQLConcat:   {"SQL+str", model.ToneDanger},
	model.RuleNoWhere:     {"No WHERE", model.ToneDanger},
	model.RuleSecret:      {"Cred in code", model.ToneDanger},
	model.RuleThrowEx:     {"throw ex", model.ToneDanger},
	model.RuleEmptyCatch:  {"Empty catch", model.ToneDanger},
	model.RuleCatchAll:    {"Catch-all", model.ToneCaution},
	model.RuleNoCSRF:      {"No CSRF", model.ToneCaution},
	model.RuleHTMLRaw:     {"Html.Raw", model.ToneCaution},
	model.RuleSingletonDB: {"Singleton DB", model.ToneCaution},
	model.RuleNullCompare: {"= NULL", model.ToneCaution},
	model.RuleJSVar:       {"JS var", model.ToneCaution},
	model.RuleBrRun:       {"<br><br>", model.ToneCaution},
	model.RuleNaming:      {"Naming", model.ToneCaution},
	model.RuleNoBraces:    {"No {}", model.ToneCaution},
	model.RuleLongLine:    {">120 col", model.ToneMuted},
	model.RuleTabIndent:   {"Tab indent", model.ToneMuted},
}

// rulesChips shows the coding rules of a training course for new engineers
// that the added lines of the working tree break, the most serious first. A
// break that lets data leak or be lost blinks.
func rulesChips(v *View) []model.Chip {
	git, ok := v.Facts.Git.Get()
	if !ok || git.Rules.Total() == 0 && git.RulesUnchecked == 0 {
		return nil
	}
	var chips []model.Chip
	for rule, n := range git.Rules {
		if n == 0 {
			continue
		}
		l := ruleLabels[rule]
		chips = append(chips, alarmIf(l.tone == model.ToneDanger, chip(l.tone, l.label+" ×"+strconv.Itoa(n))))
	}
	if git.RulesUnchecked > 0 {
		// No break shown is not the same as none: say what was not read.
		chips = append(chips, chip(model.ToneMuted, "Unchecked "+grouped(float64(git.RulesUnchecked))+" lines"))
	}
	return chips
}

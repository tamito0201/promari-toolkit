// Package usage reads how much is being used and spent outside the session's
// own report: the estimates of ccusage, and the usage windows Codex writes to
// its session logs.
package usage

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/domain/repository"
	"promari-statusline/internal/infrastructure/platform"
)

// ccusageTimeout bounds ccusage, a Node.js program that reads every session
// log on its first run.
const ccusageTimeout = 15 * time.Second

// CCUsage reads the estimated spending by running `ccusage statusline`, which
// keeps its own cache for ten seconds.
type CCUsage struct {
	Sys platform.System
}

var _ repository.SpendReader = CCUsage{}

// The parts of ccusage's one-line summary, for example
// "💰 $1.23 session / $45.67 today / $8.90 block (2h 15m left) | 🔥 $3.21/hr".
var (
	sgr      = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	todayRe  = regexp.MustCompile(`\$([\d,.]+) today`)
	blockRe  = regexp.MustCompile(`\$([\d,.]+) block(?: \(([^)]+) left\))?`)
	burnRe   = regexp.MustCompile(`([\d,.]+)/hr`)
	leftRe   = regexp.MustCompile(`^(?:(\d+)h)?(?:(\d+)m)?`)
	thousand = strings.NewReplacer(",", "")
)

// Spend implements repository.SpendReader.
func (c CCUsage) Spend(ctx context.Context, input []byte) (model.Spend, error) {
	out, err := c.Sys.Run(ctx, platform.Cmd{
		Name:    "ccusage",
		Args:    []string{"statusline", "--refresh-interval", "10"},
		Stdin:   input,
		Timeout: ccusageTimeout,
	})
	if err != nil {
		return model.Spend{}, fmt.Errorf("ccusage: %w", err)
	}
	spend := parseSpend(sgr.ReplaceAllString(out, ""))
	if !spend.Today.Present() && !spend.Block.Present() && !spend.BurnPerHour.Present() {
		return model.Spend{}, repository.ErrNone
	}
	return spend, nil
}

func parseSpend(line string) model.Spend {
	var spend model.Spend
	if m := todayRe.FindStringSubmatch(line); m != nil {
		spend.Today = amount(m[1])
	}
	if m := blockRe.FindStringSubmatch(line); m != nil {
		spend.Block = amount(m[1])
		spend.BlockLeftText = strings.ReplaceAll(m[2], " ", "")
		spend.BlockLeft = parseLeft(spend.BlockLeftText)
	}
	if m := burnRe.FindStringSubmatch(line); m != nil {
		spend.BurnPerHour = amount(m[1])
	}
	return spend
}

// amount keeps the text as ccusage printed it and parses its value. A text
// that is not a number is not an amount.
func amount(s string) model.Optional[model.Amount] {
	v, err := strconv.ParseFloat(thousand.Replace(s), 64)
	if err != nil {
		return model.Optional[model.Amount]{}
	}
	return model.Some(model.Amount{Text: s, Value: v})
}

// parseLeft reads "2h15m", "2h" or "45m".
func parseLeft(s string) time.Duration {
	m := leftRe.FindStringSubmatch(s)
	hours, _ := strconv.Atoi(m[1])
	minutes, _ := strconv.Atoi(m[2])
	return time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute
}

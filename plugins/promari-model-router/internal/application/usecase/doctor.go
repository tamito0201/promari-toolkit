package usecase

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/repository"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/pkg/fp"
)

// CheckStatus is the verdict of one doctor check.
type CheckStatus string

// Doctor verdicts; any CheckFail makes `pmr doctor` exit non-zero.
const (
	CheckOK   CheckStatus = "ok"
	CheckWarn CheckStatus = "warn"
	CheckFail CheckStatus = "fail"
)

// Check names that carry a hint in the CLI (the hint text is the CLI's).
const (
	CheckArtifact = "artifact"
	CheckForce    = model.ForceEnv
)

// Check is one doctor line.
type Check struct {
	Status CheckStatus `json:"status"`
	Name   string      `json:"name"`
	Detail string      `json:"detail"`
}

// DoctorConfig is the configuration the doctor inspects.
type DoctorConfig interface {
	repository.SettingsProvider
	repository.ConfigDiagnostics
	repository.TierProvider
}

// DoctorLedger is the ledger access the doctor needs: the chain and the
// recent entries.
type DoctorLedger interface {
	repository.LedgerReader
	repository.LedgerVerifier
}

// DoctorUseCase inspects the environment.
type DoctorUseCase struct {
	Config    DoctorConfig
	Env       repository.EnvReader
	Resolver  SessionResolver
	Usage     repository.UsageReader
	Artifacts repository.ArtifactLoader
	Agents    repository.AgentSource
	Ledger    DoctorLedger
	// HookFailures is the failure a hook left when it could not write the
	// ledger; LauncherFailures is the one bin/pmr left when it could not
	// start the binary (it deletes the file after a successful start).
	HookFailures     repository.FailureReader
	LauncherFailures repository.FailureReader
	Clock            repository.Clock
	DataDir          string
	LedgerPath       string
}

// LauncherSourceEnv is set by bin/pmr to say which binary it started.
const LauncherSourceEnv = "PMR_LAUNCHER_SOURCE"

// Execute returns the checks; any CheckFail should exit non-zero.
func (u DoctorUseCase) Execute(ctx context.Context, cwd string) []Check {
	var out []Check
	add := func(status CheckStatus, name, detail string) { out = append(out, Check{status, name, detail}) }
	st := u.Config.Settings(cwd)
	add(okIf(slices.Contains([]model.Mode{model.ModeEnforce, model.ModeShadow, model.ModeOff}, st.Routing.Mode)), "mode", string(st.Routing.Mode))
	for _, p := range u.Config.Problems(cwd) {
		add(CheckFail, "config file rejected", p)
	}
	add(CheckOK, "data dir", u.DataDir)
	add(CheckOK, "ledger", u.LedgerPath)
	for _, s := range u.Config.Sources(cwd) {
		add(CheckOK, "config file", s)
	}
	switch u.Env.Getenv(LauncherSourceEnv) {
	case "":
	case "dist":
		add(CheckWarn, "launcher", "running the development build in dist/ (may lag behind the source)")
	default:
		add(CheckOK, "launcher", u.Env.Getenv(LauncherSourceEnv))
	}
	if model.SubagentModelForced(u.Env.Getenv) {
		add(CheckFail, CheckForce, "set: routing is disabled")
	}
	if v := u.Env.Getenv("CLAUDE_CODE_SUBAGENT_MODEL"); v != "" {
		add(CheckWarn, "CLAUDE_CODE_SUBAGENT_MODEL", v+" (a default only; routed models still win)")
	}
	if v := u.Env.Getenv("CLAUDE_CODE_EFFORT_LEVEL"); v != "" {
		add(CheckWarn, "CLAUDE_CODE_EFFORT_LEVEL", v+": overrides the effort in agent frontmatter")
	}
	sm := u.Resolver.Resolve(ctx, Event{Cwd: cwd})
	add(warnIf(sm.Model == ""), "session model fallback", sm.Model+" via "+string(sm.Source)+" (uncertain sources never cap a route)")
	p := u.Usage.Claude()
	add(warnIf(!p.Known), "Claude usage cache", fmt.Sprintf("known=%v high=%v 5h=%v 7d=%v", p.Known, p.High, p.FiveHour.OrElse(-1), p.SevenDay.OrElse(-1)))
	cq := u.Usage.Codex()
	add(warnIf(!cq.Available), "Codex usage", fmt.Sprintf("available=%v used=%v", cq.Available, cq.Used.OrElse(-1)))
	out = append(out, u.artifact())
	if problems := lintAgents(u.Agents, u.Config.Tiers()); len(problems) > 0 {
		add(CheckFail, "agent tiers", strings.Join(problems, "; "))
	} else {
		add(CheckOK, "agent tiers", "consistent with data/tiers.toml")
	}
	checked, broken, err := u.Ledger.Verify(ctx)
	switch {
	case err != nil:
		add(CheckFail, "ledger chain", err.Error())
	case broken != 0:
		add(CheckFail, "ledger chain", fmt.Sprintf("broken at row %d", broken))
	default:
		add(CheckOK, "ledger chain", fmt.Sprintf("%d rows intact", checked))
	}
	return append(out, u.recent(ctx, st.Doctor.WindowDays)...)
}

// artifact says which artifact routing uses, and whether a local one is broken.
func (u DoctorUseCase) artifact() Check {
	art, err := u.Artifacts.Load()
	switch {
	case errors.Is(err, model.ErrArtifactBroken):
		return Check{CheckFail, CheckArtifact, err.Error() + "; routing runs on rules only"}
	case err != nil || !art.Ready():
		return Check{CheckWarn, CheckArtifact, "no trained artifact: rules only"}
	case !art.Trusted():
		return Check{CheckWarn, CheckArtifact, fmt.Sprintf("%s artifact only (%s, %d samples): not used for routing; rules only",
			cmp.Or(art.Origin, model.OriginEmbedded), art.Source, art.Samples)}
	default:
		return Check{CheckOK, CheckArtifact, fmt.Sprintf("local, %s, %d samples, trained %s", art.Source, art.Samples, art.TrainedAt.Format(time.DateOnly))}
	}
}

// recent reads the ledger window and the failure files. "0 hook errors" is
// only reported when the ledger could be read; a failure the ledger never saw
// (last_error) fails the check until the ledger has taken a row after it.
func (u DoctorUseCase) recent(ctx context.Context, windowDays int) []Check {
	var out []Check
	add := func(status CheckStatus, name, detail string) { out = append(out, Check{status, name, detail}) }
	window := fmt.Sprintf("(%d days)", windowDays)
	since := u.Clock.Now().AddDate(0, 0, -windowDays)
	week, err := collect(u.Ledger.Since(ctx, since))
	if err != nil {
		add(CheckFail, "ledger "+window, "unreadable: "+err.Error())
	} else {
		add(warnIf(len(week) == 0), "ledger "+window, fmt.Sprintf("%d entries", len(week)))
		errs := slices.Collect(fp.Filter(slices.Values(week), ofEvent(model.EventError)))
		if len(errs) > 0 {
			last := errs[len(errs)-1]
			add(CheckFail, "hook errors "+window, fmt.Sprintf("%d; last: %s %s", len(errs), last.Reason, last.Detail))
		} else {
			add(CheckOK, "hook errors "+window, "0")
		}
	}
	newest := time.Time{}
	if len(week) > 0 {
		newest = week[len(week)-1].At
	}
	failure := func(name string, r repository.FailureReader, recovered func(model.Failure) bool) {
		f, ok, err := r.LastFailure()
		switch {
		case err != nil:
			add(CheckFail, name, "unreadable: "+err.Error())
		case !ok || f.At.Before(since):
		case recovered(f):
			add(CheckWarn, name, f.At.UTC().Format(time.RFC3339)+" (the ledger has recorded entries since): "+f.Message)
		default:
			add(CheckFail, name, f.At.UTC().Format(time.RFC3339)+": "+f.Message)
		}
	}
	failure("hook failure outside the ledger", u.HookFailures, func(f model.Failure) bool { return err == nil && newest.After(f.At) })
	// The launcher deletes its file on success: a file present is the latest start.
	failure("launcher failure", u.LauncherFailures, func(model.Failure) bool { return false })
	return out
}

func okIf(b bool) CheckStatus {
	if b {
		return CheckOK
	}
	return CheckFail
}

func warnIf(b bool) CheckStatus {
	if b {
		return CheckWarn
	}
	return CheckOK
}

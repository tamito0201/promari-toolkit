package cli

import (
	"github.com/spf13/cobra"

	"promari-model-router/internal/application/usecase"
	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/domain/repository"
	"promari-model-router/internal/interfaces/hook"
)

// Scope builds what one command runs (an abstract factory). The command line
// declares what it needs and the composition root decides how it is built, so
// this package depends on no DI library and resolves nothing by type at run
// time. Each method builds lazily: a command opens the ledger only when the
// use case it runs needs it, and a failure to build (the data directory or the
// ledger cannot be opened) is an error the command reports, not a panic.
type Scope interface {
	// Settings are the process-level settings (the defaults and the user's
	// file, never a project's), which the flags take their defaults from.
	Settings() model.Settings

	Hook() (hook.Handlers, error)
	RecordError() (usecase.RecordErrorUseCase, error)
	// FailureRecorder and Clock need no ledger: they record a failure when
	// nothing else can be built.
	FailureRecorder() (repository.FailureRecorder, error)
	Clock() (repository.Clock, error)

	Cloud() (usecase.CloudUseCase, error)
	Cost() (usecase.CostUseCase, error)
	Doctor() (usecase.DoctorUseCase, error)
	Eval() (usecase.EvalUseCase, error)
	Explain() (usecase.ExplainUseCase, error)
	Feed() (usecase.FeedUseCase, error)
	Lint() (usecase.LintUseCase, error)
	Policy() (usecase.PolicyUseCase, error)
	Query() (usecase.QueryUseCase, error)
	Report() (usecase.ReportUseCase, error)
	Train() (usecase.TrainUseCase, error)
	Verify() (usecase.VerifyUseCase, error)

	// Close releases what the scope built (the ledger).
	Close() error
}

// Opener opens a scope for one command.
type Opener func() Scope

// scoped runs a command in a scope of its own, closed when the command ends.
func scoped(open Opener, run func(cmd *cobra.Command, args []string, s Scope) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		s := open()
		defer func() { _ = s.Close() }()
		return run(cmd, args, s)
	}
}

// use runs a command with the one thing it uses, built by get (a method
// expression such as Scope.Report): the command sees neither the scope nor
// anything else in it.
func use[T any](open Opener, get func(Scope) (T, error), run func(cmd *cobra.Command, args []string, uc T) error) func(*cobra.Command, []string) error {
	return scoped(open, func(cmd *cobra.Command, args []string, s Scope) error {
		uc, err := get(s)
		if err != nil {
			return err
		}
		return run(cmd, args, uc)
	})
}

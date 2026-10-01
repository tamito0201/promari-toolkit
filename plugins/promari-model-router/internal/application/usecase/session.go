package usecase

import (
	"context"

	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/model"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/repository"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/service"
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/pkg/fp"
)

// SessionResolver finds the main conversation's model from the observations
// available to a hook (shared by the hooks and `pmr doctor`).
type SessionResolver struct {
	Transcript repository.TranscriptReader
	Sessions   repository.SessionFinder
	Settings   repository.SettingsReader
}

// Resolve returns the most reliable observation (see service.ResolveSession).
func (r SessionResolver) Resolve(ctx context.Context, ev Event) model.SessionModel {
	return service.ResolveSession(
		fp.MapOption(fp.OptionFrom(r.Transcript.LatestModel(ev.TranscriptPath)), func(m string) model.SessionModel {
			return model.SessionModel{Model: m, Source: model.SourceTranscript}
		}),
		func() fp.Option[model.SessionModel] {
			s, ok, err := r.Sessions.Find(ctx, ev.SessionID)
			if err != nil || !ok {
				return fp.None[model.SessionModel]()
			}
			return fp.Some(model.SessionModel{Model: s.Model, Source: model.SourceSessionState})
		}(),
		fp.MapOption(fp.OptionFrom(r.Settings.EnvModel()), func(m string) model.SessionModel {
			return model.SessionModel{Model: m, Source: model.SourceEnv}
		}),
		fp.MapOption(fp.OptionFrom(r.Settings.SettingsModel(ev.Cwd)), func(m string) model.SessionModel {
			return model.SessionModel{Model: m, Source: model.SourceSettings}
		}),
	)
}

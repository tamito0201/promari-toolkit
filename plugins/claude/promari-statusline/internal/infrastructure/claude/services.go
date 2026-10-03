package claude

import (
	"context"
	"fmt"
	"time"

	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/domain/repository"
	"promari-statusline/internal/infrastructure/platform"
	"promari-statusline/pkg/jsonx"
)

const (
	// The public endpoints the status line reads. Both answer without
	// authentication and receive nothing about the session.
	statusURL  = "https://status.anthropic.com/api/v2/status.json"
	releaseURL = "https://registry.npmjs.org/@anthropic-ai/claude-code/latest"
	// httpTimeout ends this side's wait. The answers are remembered, so a slow
	// endpoint delays one render in several minutes at most.
	httpTimeout = 2 * time.Second
	// operational is the status page's indicator while nothing is wrong.
	operational = "none"
)

// StatusPage reads the status page of the API.
type StatusPage struct {
	Sys platform.System
}

var _ repository.IncidentReader = StatusPage{}

// Incident implements repository.IncidentReader.
func (s StatusPage) Incident(ctx context.Context) (model.Incident, error) {
	body, err := s.Sys.Get(ctx, statusURL, httpTimeout)
	if err != nil {
		return model.Incident{}, fmt.Errorf("read the status page: %w", err)
	}
	page, _ := jsonx.Parse(body)
	status := jsonx.Child(page, "status")
	indicator := jsonx.Or[string](status, "indicator")
	if indicator == "" || indicator == operational {
		return model.Incident{}, repository.ErrNone
	}
	return model.Incident{Indicator: indicator, Description: jsonx.Or[string](status, "description")}, nil
}

// Registry reads the newest release of Claude Code from the npm registry.
type Registry struct {
	Sys platform.System
}

var _ repository.ReleaseReader = Registry{}

// Latest implements repository.ReleaseReader.
func (r Registry) Latest(ctx context.Context) (string, error) {
	body, err := r.Sys.Get(ctx, releaseURL, httpTimeout)
	if err != nil {
		return "", fmt.Errorf("read the registry: %w", err)
	}
	release, _ := jsonx.Parse(body)
	version := jsonx.Or[string](release, "version")
	if version == "" {
		return "", repository.ErrNone
	}
	return version, nil
}

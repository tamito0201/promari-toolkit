package statusline

import (
	"context"
	"fmt"
	"io"

	"promari-statusline/internal/application/usecase"
)

// maxInput bounds the session report; Claude Code sends a few kilobytes.
const maxInput = 4 << 20

// Renderer is the use case the handler calls.
type Renderer interface {
	Execute(ctx context.Context, req usecase.RenderRequest) usecase.Rendered
}

// Handler answers one render request of Claude Code.
type Handler struct {
	Render Renderer
}

// Handle reads the session report from in and writes the status line to out.
// Input that cannot be read or decoded still gives a status line: everything
// that does not depend on the report is shown.
func (h Handler) Handle(ctx context.Context, in io.Reader, out io.Writer) error {
	// A failed read leaves raw empty or cut short; either decodes to a session
	// that reported nothing.
	raw, _ := io.ReadAll(io.LimitReader(in, maxInput))
	rendered := h.Render.Execute(ctx, usecase.RenderRequest{Session: Decode(raw), Raw: raw})
	if _, err := fmt.Fprintln(out, Present(rendered.Lines, rendered.At)); err != nil {
		return fmt.Errorf("write the status line: %w", err)
	}
	return nil
}

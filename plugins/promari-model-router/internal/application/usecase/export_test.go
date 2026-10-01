package usecase

import (
	"github.com/tamito0201/promari-toolkit/plugins/promari-model-router/internal/domain/service"
)

// Test-only hooks into unexported helpers (compiled only with the tests).

// TraceDetail exposes traceDetail.
func TraceDetail(tr service.RouteTrace, decimals int, degraded []string) string {
	return traceDetail(tr, decimals, degraded)
}

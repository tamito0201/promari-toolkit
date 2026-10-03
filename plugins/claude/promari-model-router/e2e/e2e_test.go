// Package e2e runs the runn runbooks against the real binary and the real
// HTTP handler: hooks are driven exactly as Claude Code drives them (JSON on
// stdin, JSON on stdout) and assertions use runn's compare().
package e2e

import (
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/k1LoW/runn"
	"github.com/samber/do/v2"

	"promari-model-router/internal/application/usecase"
	"promari-model-router/internal/di"
	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/interfaces/web"
)

func TestRunbooks(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e builds the binary")
	}
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "pmr")
	// `go test` exports GOROOT to the test process, so the same toolchain builds the binary.
	goBin := filepath.Join(os.Getenv("GOROOT"), "bin", "go")
	if os.Getenv("GOROOT") == "" {
		goBin = "go"
	}
	args := []string{"build", "-o", bin}
	// `task coverage` sets PMR_E2E_COVERDIR: the binary is built with coverage
	// instrumentation and writes its counters there, so the paths only the E2E
	// runbooks reach (hook, cli) are counted together with the unit tests.
	coverDir := os.Getenv("PMR_E2E_COVERDIR")
	if coverDir != "" {
		args = append(args, "-cover", "-coverpkg=../...")
	}
	build := exec.CommandContext(t.Context(), goBin, append(args, "../cmd/pmr")...)
	build.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build pmr: %v\n%s", err, out)
	}

	// Isolate every piece of state: the ledger, the artifact, the user config
	// and the usage caches all live under the temporary HOME.
	home := filepath.Join(tmp, "home")
	for k, v := range map[string]string{
		"HOME": home, "CLAUDE_PLUGIN_DATA": filepath.Join(tmp, "data"), "CLAUDE_PROJECT_DIR": filepath.Join(tmp, "project"),
		"ANTHROPIC_MODEL": "", "CLAUDE_CODE_SUBAGENT_MODEL_FORCE": "", "PMR_BIN": bin,
		"GOCOVERDIR": coverDir,
	} {
		t.Setenv(k, v)
	}

	// The API runbook talks to the same handler `pmr serve` mounts.
	c := di.New()
	t.Cleanup(func() { _ = c.Shutdown() })
	h, err := web.Handler(web.Deps{
		Report: do.MustInvoke[usecase.ReportUseCase](c), Explain: do.MustInvoke[usecase.ExplainUseCase](c),
		Feed: do.MustInvoke[usecase.FeedUseCase](c), Serve: do.MustInvoke[model.Settings](c).Serve,
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)

	// Order matters: the CLI runbook reads the ledger the hook runbook wrote.
	tests := []struct {
		name string
		book string
	}{
		{name: "hooks", book: "runbooks/hooks.yml"},
		{name: "cli", book: "runbooks/cli.yml"},
		{name: "api", book: "runbooks/api.yml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o, err := runn.Load(tt.book, runn.T(t), runn.Scopes(runn.AllowRunExec), runn.Runner("req", ts.URL))
			if err != nil {
				t.Fatal(err)
			}
			if err := o.RunN(t.Context()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

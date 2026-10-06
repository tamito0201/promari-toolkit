// Package dashboard serves the status line as a web page: `psl dashboard`
// listens on the loopback address and answers the page (built from web/ with
// TypeScript and embedded at build time) and its JSON.
package dashboard

import (
	"context"
	"embed"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"

	"promari-statusline/internal/application/usecase"
)

const (
	// shutdownGrace is how long requests in flight may finish after Ctrl-C.
	shutdownGrace = 3 * time.Second
	// readHeaderTimeout bounds a client that sends its request slowly.
	readHeaderTimeout = 5 * time.Second
	// writeTimeout bounds an answer. A snapshot asks git, GitHub and ccusage;
	// each is bounded by its own timeout, well inside this one.
	writeTimeout = 30 * time.Second
	// idleTimeout closes a kept-alive connection the page no longer uses.
	idleTimeout = time.Minute
)

// assets is the page as web/ builds it (index.html, app.js, app.css).
//
//go:embed assets
var assets embed.FS

// Snapshotter is the use case the server calls.
type Snapshotter interface {
	Execute(ctx context.Context) usecase.Snapshot
}

// Server answers the dashboard's requests.
type Server struct {
	Snapshot Snapshotter
	Version  string
	// Grace is how long requests in flight may finish after Ctrl-C; zero
	// means shutdownGrace.
	Grace time.Duration
}

// static is the page, with the embedded directory as its root. fs.Sub fails
// only on a malformed name, and "assets" is a constant one.
var static, _ = fs.Sub(assets, "assets")

// Handler returns the routes of the dashboard.
func (s Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/snapshot", s.snapshot)
	mux.Handle("GET /", http.FileServerFS(static))
	return guard(mux)
}

func (s Server) snapshot(w http.ResponseWriter, r *http.Request) {
	snap := s.Snapshot.Execute(r.Context())
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	// A client that went away cannot be told about it.
	_ = json.MarshalWrite(w, toDTO(&snap, s.Version))
}

// guard refuses requests that do not name this machine and sets the headers
// every answer carries. Checking the Host header stops DNS rebinding: a page on
// another site that points its own name at 127.0.0.1 still sends that name,
// and would otherwise read this machine's facts through the visitor's browser.
func guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !local(r.Host) {
			http.Error(w, "the dashboard answers only to localhost", http.StatusMisdirectedRequest)
			return
		}
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; style-src 'self' https://fonts.googleapis.com; font-src https://fonts.gstatic.com; img-src 'self' data:; base-uri 'none'; frame-ancestors 'none'; form-action 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// local reports whether a Host header names this machine.
func local(host string) bool {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Serve listens on addr until ctx ends, and prints where to open the page.
func (s Server) Serve(ctx context.Context, addr string, out io.Writer) error {
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}
	fmt.Fprintf(out, "📊 promari-statusline dashboard: http://%s/  (Ctrl-C to stop)\n", displayAddr(ln.Addr().String()))
	return s.serve(ctx, ln)
}

// serve answers on ln until ctx ends. It closes ln.
func (s Server) serve(ctx context.Context, ln net.Listener) error {
	grace := s.Grace
	if grace == 0 {
		grace = shutdownGrace
	}
	srv := &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: readHeaderTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}
	// The shutdown goroutine ends with this context: on Ctrl-C, or when Serve
	// returns first because it failed.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stopped := make(chan error, 1)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), grace)
		defer cancel()
		stopped <- srv.Shutdown(shutdownCtx)
	}()
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	if err := <-stopped; err != nil {
		return fmt.Errorf("shut down: %w", err)
	}
	return nil
}

// displayAddr writes 127.0.0.1 as localhost, the name a reader types.
func displayAddr(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || host != "127.0.0.1" {
		return addr
	}
	return "localhost:" + port
}

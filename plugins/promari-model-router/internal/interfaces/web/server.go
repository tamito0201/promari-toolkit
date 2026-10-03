// Package web serves the read-only HTTP API described by openapi/openapi.yaml.
// The handlers implement the interface oapi-codegen generated from that
// contract; incoming requests are validated against the same contract
// (kin-openapi), so the spec and the server cannot drift apart silently.
//
// Trust boundary: the server binds to loopback only, answers only requests
// addressed to a loopback name (a DNS-rebinding page cannot reach it through
// its own host name), and never writes.
package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	middleware "github.com/oapi-codegen/nethttp-middleware"

	"promari-model-router/internal/application/usecase"
	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/interfaces/web/api"
)

// Version is reported by /healthz.
var Version = "dev"

// Deps are the use cases the API reads from and the [serve] settings.
type Deps struct {
	Report  usecase.ReportUseCase
	Explain usecase.ExplainUseCase
	Feed    usecase.FeedUseCase
	Serve   model.ServeSettings
}

// ledgerEvent is the SSE payload (LedgerEvent in openapi.yaml).
type ledgerEvent struct {
	At     time.Time `json:"at"`
	Event  string    `json:"event"`
	Action string    `json:"action,omitempty"`
	Reason string    `json:"reason,omitempty"`
	Target string    `json:"target,omitempty"`
	Class  string    `json:"class,omitempty"`
}

// maxSinceSeconds mirrors the OpenAPI contract (since_seconds maximum).
const maxSinceSeconds = 86400

// ErrNotLoopback rejects listening on a non-loopback address.
var ErrNotLoopback = errors.New("pmr serve only listens on a loopback address (127.0.0.1 or ::1)")

type strict struct{ d Deps }

var _ api.StrictServerInterface = strict{}

// convert re-shapes a use-case value into a generated API type through JSON.
func convert[T any](v any) (T, error) {
	var out T
	raw, err := json.Marshal(v)
	if err == nil {
		err = json.Unmarshal(raw, &out)
	}
	return out, err
}

func (s strict) GetHealth(context.Context, api.GetHealthRequestObject) (api.GetHealthResponseObject, error) {
	return api.GetHealth200JSONResponse{Status: api.Ok, Version: Version}, nil
}

func (s strict) GetReport(ctx context.Context, req api.GetReportRequestObject) (api.GetReportResponseObject, error) {
	days := 0.0 // the use case's default window
	if req.Params.Days != nil {
		days = *req.Params.Days
	}
	rep, err := s.d.Report.Execute(ctx, days)
	if err != nil {
		return nil, fmt.Errorf("build report: %w", err) // a server-side failure: 500
	}
	out, err := convert[api.Report](rep)
	return reportOK(out), err
}

func (s strict) PostExplain(_ context.Context, req api.PostExplainRequestObject) (api.PostExplainResponseObject, error) {
	if req.Body == nil {
		return api.PostExplain400JSONResponse{Message: "missing body"}, nil
	}
	in := usecase.ExplainInput{Prompt: req.Body.Prompt, SubagentType: model.DefaultSubagentType}
	if req.Body.SubagentType != nil {
		in.SubagentType = *req.Body.SubagentType
	}
	if req.Body.SessionModel != nil {
		in.SessionModel = *req.Body.SessionModel
	}
	out, err := convert[api.Explanation](s.d.Explain.Execute(in))
	return explanationOK(out), err
}

// The generated 200 responses are defined types (type X Report); they do not
// inherit the MarshalJSON of the schema type, so encoding them drops every
// additional property (from, to, prices_as_of, scores, continuation, ...).
// These wrappers encode through the schema type instead.
type (
	reportOK      api.Report
	explanationOK api.Explanation
)

func (r reportOK) VisitGetReportResponse(w http.ResponseWriter) error {
	return writeOK(w, api.Report(r))
}

func (e explanationOK) VisitPostExplainResponse(w http.ResponseWriter) error {
	return writeOK(w, api.Explanation(e))
}

// writeOK writes v as a 200 JSON body.
func writeOK(w http.ResponseWriter, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(append(raw, '\n'))
	return err
}

// GetEvents is served by the streaming handler below; the generated strict
// handler buffers the body, which would defeat Server-Sent Events.
func (s strict) GetEvents(context.Context, api.GetEventsRequestObject) (api.GetEventsResponseObject, error) {
	return nil, errors.New("served by the streaming handler")
}

// badSince is the 400 body (the Error schema) for a bad since_seconds.
var badSince = fmt.Sprintf(`{"message":"since_seconds must be an integer from 0 to %d"}`+"\n", maxSinceSeconds)

// events streams ledger entries as SSE, polling every [serve].sse_poll_ms.
// It follows ledger positions (FeedUseCase), so no entry is sent twice.
func (s strict) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	back := 0
	if v := r.URL.Query().Get("since_seconds"); v != "" {
		sec, err := strconv.Atoi(v)
		if err != nil || sec < 0 || sec > maxSinceSeconds {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, badSince)
			return
		}
		back = sec
	}
	cur, err := s.d.Feed.Start(r.Context(), time.Duration(back)*time.Second)
	if err != nil {
		http.Error(w, "ledger unreadable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	ticker := time.NewTicker(time.Duration(s.d.Serve.SSEPollMS) * time.Millisecond)
	defer ticker.Stop()
	send := func(e model.Entry) error {
		raw, err := json.Marshal(ledgerEvent{
			At: e.At, Event: string(e.Event), Action: string(e.Action), Reason: e.Reason, Target: string(e.Target), Class: string(e.Class),
		})
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(w, "event: ledger\ndata: %s\n\n", raw)
		return err
	}
	for {
		if cur, err = s.d.Feed.Next(r.Context(), cur, send); err != nil {
			return
		}
		flusher.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

// loopbackHost reports whether a Host header names the loopback interface
// (any port: the listener is loopback-only, the name is what a DNS-rebinding
// page cannot fake).
func loopbackHost(host string) bool {
	name := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		name = h
	}
	return name == "localhost" || name == "127.0.0.1" || name == "::1" || name == "[::1]"
}

// hostGuard answers 421 Misdirected Request to a request for another host.
func hostGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !loopbackHost(r.Host) {
			http.Error(w, "pmr serve answers only requests for 127.0.0.1, localhost or [::1]", http.StatusMisdirectedRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// loadSpec decodes the embedded contract (a variable so tests can make it fail).
var loadSpec = api.GetSpec

// Handler builds the validated HTTP handler.
func Handler(d Deps) (http.Handler, error) {
	spec, err := loadSpec()
	if err != nil {
		return nil, err
	}
	spec.Servers = nil // validate paths only, not the Host header
	s := strict{d: d}
	generated := api.HandlerWithOptions(api.NewStrictHandler(s, nil), api.StdHTTPServerOptions{})
	validated := middleware.OapiRequestValidatorWithOptions(spec, &middleware.Options{SilenceServersWarning: true})(generated)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/events", s.events)
	mux.Handle("/", validated)
	return hostGuard(mux), nil
}

// Serve listens on a loopback address until ctx is cancelled. It returns
// only after the listener has stopped and the shutdown has finished, with the
// listener's error (a busy port) or the shutdown's. Requests run under ctx
// (BaseContext), so an open event stream ends with it instead of holding the
// shutdown until its deadline.
func Serve(ctx context.Context, addr string, d Deps, log io.Writer) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return ErrNotLoopback
	}
	h, err := Handler(d)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr: addr, Handler: h, ReadHeaderTimeout: time.Duration(d.Serve.ReadHeaderTimeoutMS) * time.Millisecond,
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	fmt.Fprintf(log, "pmr serve: http://%s (OpenAPI: openapi/openapi.yaml)\n", addr)
	served := make(chan error, 1)
	go func() { served <- srv.ListenAndServe() }()
	select {
	case err := <-served:
		return err // the listener failed; ListenAndServe never returns nil
	case <-ctx.Done():
	}
	// The parent is already cancelled; keep its values but not its cancellation.
	shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Duration(d.Serve.ShutdownTimeoutMS)*time.Millisecond)
	defer cancel()
	shutdownErr := srv.Shutdown(shutdown)
	<-served // ErrServerClosed: once Shutdown is called, ListenAndServe returns it
	return shutdownErr
}

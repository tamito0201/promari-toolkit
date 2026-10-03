package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"promari-model-router/internal/application/usecase"
	"promari-model-router/internal/domain/model"
)

const cloudID = "session_017cB1aBqhcEMKwqXEK2d4A9"

type fakeMessenger struct {
	sent    []string
	page    string
	err     error
	clockAt *time.Time // the clock's time when Send ran
	clock   *fakeCloudClock
}

func (f *fakeMessenger) Send(_ context.Context, s model.CloudSessionID, m model.CloudMessage) (string, error) {
	f.sent = append(f.sent, s.String()+": "+m.Text())
	if f.clock != nil {
		at := f.clock.Now()
		f.clockAt = &at
	}
	return f.page, f.err
}

type fakeLinks struct {
	link             model.CloudLink
	ok               bool
	loadErr, saveErr error
	saved            []model.CloudLink
}

func (f *fakeLinks) Load(context.Context) (model.CloudLink, bool, error) {
	return f.link, f.ok, f.loadErr
}

func (f *fakeLinks) Save(_ context.Context, l model.CloudLink) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = append(f.saved, l)
	f.link, f.ok = l, true
	return nil
}

// fakeCloudClock moves forward a second on every reading, so a test can tell
// which reading a value came from.
type fakeCloudClock struct{ at time.Time }

func (c *fakeCloudClock) Now() time.Time {
	c.at = c.at.Add(time.Second)
	return c.at
}

func mustCloudID(t *testing.T) model.CloudSessionID {
	t.Helper()
	id, err := model.ParseCloudSessionID(cloudID)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestCloudUse(t *testing.T) {
	t.Parallel()
	errDisk := errors.New("disk full")
	tests := []struct {
		name      string
		raw       string
		saveErr   error
		wantErr   error
		wantSaved int
	}{
		{name: "URL is stored as its ID", raw: "https://claude.ai/code/" + cloudID + "?m=0", wantSaved: 1},
		{name: "invalid ID is not stored", raw: "rm -rf", wantErr: model.ErrInvalidCloudSession},
		{name: "save failure", raw: cloudID, saveErr: errDisk, wantErr: errDisk},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			links := &fakeLinks{saveErr: tt.saveErr}
			u := usecase.CloudUseCase{Messenger: &fakeMessenger{}, Links: links, Clock: &fakeCloudClock{}}
			link, err := u.Use(context.Background(), tt.raw)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if len(links.saved) != tt.wantSaved {
				t.Fatalf("saved %d times, want %d", len(links.saved), tt.wantSaved)
			}
			if tt.wantErr == nil && link.Session.String() != cloudID {
				t.Errorf("session = %q", link.Session)
			}
		})
	}
}

func TestCloudStatus(t *testing.T) {
	t.Parallel()
	errRead := errors.New("permission denied")
	tests := []struct {
		name    string
		links   *fakeLinks
		wantErr error
	}{
		{name: "never set", links: &fakeLinks{}, wantErr: model.ErrNoCloudSession},
		{name: "unreadable", links: &fakeLinks{loadErr: errRead}, wantErr: errRead},
		{name: "set", links: &fakeLinks{ok: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.wantErr == nil {
				tt.links.link = model.CloudLink{Session: mustCloudID(t)}
			}
			u := usecase.CloudUseCase{Messenger: &fakeMessenger{}, Links: tt.links, Clock: &fakeCloudClock{}}
			if _, err := u.Status(context.Background()); !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestCloudSend(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)
	errCLI := errors.New("cli failed")
	errDisk := errors.New("disk full")
	tests := []struct {
		name     string
		text     string
		unset    bool
		sendErr  error
		saveErr  error
		wantErr  error
		wantSent []string
	}{
		{name: "queues the trimmed text", text: " run the tests \n", wantSent: []string{cloudID + ": run the tests"}},
		{name: "empty text is not sent", text: "  ", wantErr: model.ErrEmptyCloudMessage},
		{name: "no session", text: "hi", unset: true, wantErr: model.ErrNoCloudSession},
		{name: "CLI failure", text: "hi", sendErr: errCLI, wantErr: errCLI, wantSent: []string{cloudID + ": hi"}},
		{name: "sent but not recorded", text: "hi", saveErr: errDisk, wantErr: errDisk, wantSent: []string{cloudID + ": hi"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			clk := &fakeCloudClock{at: start}
			links := &fakeLinks{saveErr: tt.saveErr}
			if !tt.unset {
				links.link, links.ok = model.CloudLink{Session: mustCloudID(t)}, true
			}
			msg := &fakeMessenger{page: "https://claude.ai/code/" + cloudID + "?from=cli", err: tt.sendErr, clock: clk}
			u := usecase.CloudUseCase{Messenger: msg, Links: links, Clock: clk}
			got, err := u.Send(context.Background(), tt.text)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(tt.wantSent, msg.sent); diff != "" {
				t.Errorf("sent (-want +got):\n%s", diff)
			}
			if tt.wantErr != nil {
				return
			}
			// The reading window opens before the message leaves.
			if !got.SentAt.Before(*msg.clockAt) {
				t.Errorf("SentAt %v is not before the send at %v", got.SentAt, *msg.clockAt)
			}
			if diff := cmp.Diff(got.SentAt, links.link.SentAt); diff != "" {
				t.Errorf("stored SentAt (-receipt +stored):\n%s", diff)
			}
			if got.URL != msg.page || got.Session.String() != cloudID {
				t.Errorf("receipt = %+v", got)
			}
		})
	}
}

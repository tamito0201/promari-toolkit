package model_test

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

	"promari-model-router/internal/domain/model"
)

func TestParseCloudSessionID(t *testing.T) {
	t.Parallel()
	const id = "session_017cB1aBqhcEMKwqXEK2d4A9"
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr error
	}{
		{name: "bare session ID", raw: id, want: id},
		{name: "bare cse ID with spaces", raw: "  cse_0123456789abcdef \n", want: "cse_0123456789abcdef"},
		{name: "full URL with query", raw: "https://claude.ai/code/" + id + "?from=cli&m=0", want: id},
		{name: "URL without scheme", raw: "claude.ai/code/" + id, want: id},
		{name: "another host", raw: "https://example.com/code/" + id, wantErr: model.ErrInvalidCloudSession},
		{name: "another path", raw: "https://claude.ai/chat/" + id, wantErr: model.ErrInvalidCloudSession},
		{name: "path traversal", raw: "../../etc", wantErr: model.ErrInvalidCloudSession},
		{name: "option-like word", raw: "--dangerously-skip-permissions", wantErr: model.ErrInvalidCloudSession},
		{name: "too short", raw: "session_abc", wantErr: model.ErrInvalidCloudSession},
		{name: "empty", raw: "", wantErr: model.ErrInvalidCloudSession},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := model.ParseCloudSessionID(tt.raw)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(tt.want, got.String()); diff != "" {
				t.Errorf("ID (-want +got):\n%s", diff)
			}
			if got.IsZero() != (tt.want == "") {
				t.Errorf("IsZero = %v", got.IsZero())
			}
		})
	}
}

func TestCloudSessionURL(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, raw, want string }{
		{name: "cse ID", raw: "cse_0123456789abcdef", want: "https://claude.ai/code/cse_0123456789abcdef"},
		{name: "URL keeps no query", raw: "claude.ai/code/session_0123456789?m=0", want: "https://claude.ai/code/session_0123456789"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			id, err := model.ParseCloudSessionID(tt.raw)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tt.want, id.URL()); diff != "" {
				t.Errorf("URL (-want +got):\n%s", diff)
			}
		})
	}
}

func TestNewCloudMessage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		text    string
		want    string
		wantErr error
	}{
		{name: "trimmed", text: "  run the tests\n", want: "run the tests"},
		{name: "keeps inner lines", text: "a\nb", want: "a\nb"},
		{name: "blank", text: " \n\t", wantErr: model.ErrEmptyCloudMessage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := model.NewCloudMessage(tt.text)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(tt.want, got.Text()); diff != "" {
				t.Errorf("text (-want +got):\n%s", diff)
			}
		})
	}
}

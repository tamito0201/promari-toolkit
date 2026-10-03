// Package failurelog keeps the last failure in a plain text file beside the
// ledger, for the failures the ledger cannot take: a hook that could not open
// or write it, and the launcher (bin/pmr) that could not start the binary.
//
// Format (shared with bin/pmr, which writes launcher_error): the first line is
// the time in RFC 3339 (UTC), the remaining lines are the message.
package failurelog

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/domain/repository"
	"promari-model-router/internal/infrastructure/fsutil"
)

// File implements repository.FailureRecorder and repository.FailureReader.
type File struct {
	Path string
}

var (
	_ repository.FailureRecorder = File{}
	_ repository.FailureReader   = File{}
)

// RecordFailure replaces the file with f (only the last failure is kept).
func (l File) RecordFailure(f model.Failure) error {
	if err := os.MkdirAll(filepath.Dir(l.Path), 0o700); err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(l.Path, []byte(f.At.UTC().Format(time.RFC3339)+"\n"+f.Message+"\n"))
}

// LastFailure reads the file; ok is false when there is none.
func (l File) LastFailure() (model.Failure, bool, error) {
	raw, err := os.ReadFile(l.Path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return model.Failure{}, false, nil
	case err != nil:
		return model.Failure{}, false, err
	}
	first, rest, _ := strings.Cut(string(raw), "\n")
	at, err := time.Parse(time.RFC3339, strings.TrimSpace(first))
	if err != nil {
		return model.Failure{}, false, fmt.Errorf("%s: first line is not an RFC 3339 time: %w", l.Path, err)
	}
	return model.Failure{At: at, Message: strings.TrimSpace(rest)}, true, nil
}

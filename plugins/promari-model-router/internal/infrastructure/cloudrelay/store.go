package cloudrelay

import (
	"context"
	"encoding/json/v2"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"promari-model-router/internal/domain/model"
	"promari-model-router/internal/domain/repository"
	"promari-model-router/internal/infrastructure/fsutil"
)

// Store implements repository.CloudLinkStore as a small JSON file in the
// plugin's data directory (mode 0600).
type Store struct{ Path string }

var _ repository.CloudLinkStore = Store{}

type storedLink struct {
	Session string    `json:"session"`
	SentAt  time.Time `json:"sent_at,omitzero"`
}

// Load implements repository.CloudLinkStore.
func (s Store) Load(context.Context) (model.CloudLink, bool, error) {
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return model.CloudLink{}, false, nil
	}
	if err != nil {
		return model.CloudLink{}, false, err
	}
	var v storedLink
	if err := json.Unmarshal(data, &v); err != nil {
		return model.CloudLink{}, false, err
	}
	id, err := model.ParseCloudSessionID(v.Session)
	if err != nil {
		return model.CloudLink{}, false, err
	}
	return model.CloudLink{Session: id, SentAt: v.SentAt}, true, nil
}

// Save implements repository.CloudLinkStore.
func (s Store) Save(_ context.Context, link model.CloudLink) error {
	data, err := json.Marshal(storedLink{Session: link.Session.String(), SentAt: link.SentAt})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(s.Path, data)
}

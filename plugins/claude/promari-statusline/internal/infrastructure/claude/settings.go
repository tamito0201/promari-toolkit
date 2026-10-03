package claude

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"slices"

	"promari-statusline/internal/domain/model"
	"promari-statusline/internal/domain/repository"
	"promari-statusline/internal/infrastructure/platform"
)

const (
	settingsFile  = "settings.json"
	statusLineKey = "statusLine"
	// backupStamp names a backup after the time it was made.
	backupStamp = "20060102-150405"
)

// Settings edits Claude Code's user settings file.
//
// The file belongs to the user. An edit changes the statusLine member and
// nothing else: every other member keeps its value and its place, and the file
// as it was is copied to a backup first. A file that is not a JSON object is
// never overwritten.
type Settings struct {
	Sys platform.System
}

var _ repository.SettingsStore = Settings{}

// Path implements repository.SettingsStore.
func (s Settings) Path() string { return filepath.Join(ConfigDir(s.Sys), settingsFile) }

// StatusLine implements repository.SettingsStore.
func (s Settings) StatusLine() (model.StatusLineSetting, error) {
	members, _, err := s.read()
	if err != nil {
		return model.StatusLineSetting{}, err
	}
	i := slices.IndexFunc(members, isStatusLine)
	if i < 0 {
		return model.StatusLineSetting{}, repository.ErrNone
	}
	var setting model.StatusLineSetting
	if err := json.Unmarshal(members[i].value, &setting); err != nil {
		return model.StatusLineSetting{}, fmt.Errorf("%s: statusLine: %w", s.Path(), err)
	}
	return setting, nil
}

// SetStatusLine implements repository.SettingsStore.
func (s Settings) SetStatusLine(setting model.StatusLineSetting) (string, error) {
	value, err := json.Marshal(setting)
	if err != nil {
		return "", fmt.Errorf("encode the statusLine: %w", err)
	}
	return s.edit(func(members []member) []member {
		if i := slices.IndexFunc(members, isStatusLine); i >= 0 {
			members[i].value = value
			return members
		}
		return append(members, member{name: statusLineKey, value: value})
	})
}

// RemoveStatusLine implements repository.SettingsStore.
func (s Settings) RemoveStatusLine() (string, error) {
	return s.edit(func(members []member) []member { return slices.DeleteFunc(members, isStatusLine) })
}

// member is one member of the settings object, its value undecoded.
type member struct {
	name  string
	value jsontext.Value
}

func isStatusLine(m member) bool { return m.name == statusLineKey }

// edit rewrites the settings file with the members change returns, after
// copying the file as it was to a backup. It returns the backup's path, or ""
// when there was no file to back up.
func (s Settings) edit(change func([]member) []member) (backup string, err error) {
	members, original, err := s.read()
	if err != nil {
		return "", err
	}
	updated, err := encode(change(members))
	if err != nil {
		return "", err
	}
	if original != nil {
		backup = s.Path() + ".bak-" + s.Sys.Now().Format(backupStamp)
		if err := s.Sys.WriteFile(backup, original, platform.Private); err != nil {
			return "", fmt.Errorf("back up the settings: %w", err)
		}
	}
	if err := s.Sys.WriteFile(s.Path(), updated, platform.Private); err != nil {
		return backup, fmt.Errorf("write the settings: %w", err)
	}
	return backup, nil
}

// read returns the members of the settings object in the order of the file,
// and the file's bytes. A missing file is an empty object with nil bytes.
func (s Settings) read() (members []member, original []byte, err error) {
	original, err = s.Sys.ReadFile(s.Path())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("read the settings: %w", err)
	}
	members, err = decode(original)
	if err != nil {
		return nil, nil, fmt.Errorf("%s is not a JSON object, left untouched: %w", s.Path(), err)
	}
	return members, original, nil
}

// errNotObject is returned for a settings file whose top level is not an object.
var errNotObject = errors.New("the top level is not an object")

// decode reads the members of a JSON object, keeping their order.
func decode(data []byte) ([]member, error) {
	dec := jsontext.NewDecoder(bytes.NewReader(data))
	if open, err := dec.ReadToken(); err != nil {
		return nil, err
	} else if open.Kind() != jsontext.KindBeginObject {
		return nil, errNotObject
	}
	var members []member
	for dec.PeekKind() != jsontext.KindEndObject {
		token, err := dec.ReadToken()
		if err != nil {
			return nil, err
		}
		// A token is only valid until the decoder is used again.
		name := token.String()
		value, err := dec.ReadValue()
		if err != nil {
			return nil, err
		}
		members = append(members, member{name: name, value: value.Clone()})
	}
	if _, err := dec.ReadToken(); err != nil {
		return nil, err
	}
	// Whatever follows the object would be lost when the file is written back.
	if _, err := dec.ReadToken(); !errors.Is(err, io.EOF) {
		return nil, errTrailing
	}
	return members, nil
}

// errTrailing is returned for a settings file with content after its object.
var errTrailing = errors.New("there is content after the object")

// encode writes the members as an indented JSON object.
func encode(members []member) ([]byte, error) {
	var out bytes.Buffer
	enc := jsontext.NewEncoder(&out, jsontext.WithIndent("  "), jsontext.EscapeForHTML(false))
	if err := enc.WriteToken(jsontext.BeginObject); err != nil {
		return nil, fmt.Errorf("encode the settings: %w", err)
	}
	for _, m := range members {
		if err := errors.Join(enc.WriteToken(jsontext.String(m.name)), enc.WriteValue(m.value)); err != nil {
			return nil, fmt.Errorf("encode the settings: %s: %w", m.name, err)
		}
	}
	if err := enc.WriteToken(jsontext.EndObject); err != nil {
		return nil, fmt.Errorf("encode the settings: %w", err)
	}
	return out.Bytes(), nil
}

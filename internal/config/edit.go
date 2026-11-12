package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// SetValues sets top-level keys in the JSON config file in one write,
// keeping every other key the user has set. The file is created from the
// defaults if it does not exist. Keys are written sorted and indented.
// A symlinked file (one kept with dotfiles, say) is written where the link
// points, and the link is kept.
func SetValues(path string, values map[string]any) error {
	if target, err := filepath.EvalSymlinks(path); err == nil {
		path = target
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		data = SampleConfig()
	} else if err != nil {
		return err
	}
	m := map[string]json.RawMessage{}
	if len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, &m); err != nil {
			return fmt.Errorf("%s: %w", path, describeJSONError(data, err))
		}
	}
	for key, value := range values {
		v, err := json.Marshal(value)
		if err != nil {
			return err
		}
		m[key] = v
	}
	out, err := marshalIndent(m)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// WriteSample writes the default configuration to path, creating its
// folder. It fails with an error matching fs.ErrExist if the file is
// already there, which it leaves alone.
func WriteSample(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, err = f.Write(SampleConfig())
	return errors.Join(err, f.Close())
}

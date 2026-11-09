package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// SetValues sets top-level keys in the JSON config file in one write,
// keeping every other key the user has set. The file is created from the
// defaults if it does not exist. Keys are written sorted and indented.
func SetValues(path string, values map[string]any) error {
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

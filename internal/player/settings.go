package player

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

const maxSettings = 1 << 20

// Settings persists the page's settings document. Its structure belongs to
// the page (web/src/settings.ts); Go only requires a JSON object of bounded
// size and writes it atomically.
type Settings struct {
	path string
	mu   sync.Mutex
	doc  []byte
}

// DefaultSettingsPath is %AppData%\jusplay\settings.json.
func DefaultSettingsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "jusplay", "settings.json"), nil
}

// OpenSettings loads path; a missing or unreadable file starts empty.
func OpenSettings(path string) *Settings {
	s := &Settings{path: path, doc: []byte("{}")}
	if b, err := os.ReadFile(path); err == nil && validObject(b) {
		s.doc = b
	}
	return s
}

func validObject(b []byte) bool {
	var m map[string]json.RawMessage
	return len(b) <= maxSettings && json.Unmarshal(b, &m) == nil && m != nil
}

func (s *Settings) Get() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.doc...)
}

func (s *Settings) Put(b []byte) error {
	if !validObject(b) {
		return errors.New("settings must be a JSON object under 1 MiB")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.path != "" {
		if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
			return err
		}
		tmp := s.path + ".partial"
		if err := os.WriteFile(tmp, b, 0o644); err != nil {
			return err
		}
		if err := os.Rename(tmp, s.path); err != nil {
			return err
		}
	}
	s.doc = append([]byte(nil), b...)
	return nil
}

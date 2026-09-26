package player

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Prefs are the interface choices shared by the library and player pages:
// language, colour theme and library sort order. They live in their own
// small file (ui.json beside settings.json) because the player page writes
// its settings document whole and would drop keys it does not know.
//
// The pages load them as a blocking script (prefs.js) in <head>, so the
// first paint already has the right theme and language. localStorage cannot
// do this: the loopback port, and with it the page's origin, changes on
// every run.
type Prefs struct {
	Lang  string `json:"lang"`  // "zh" or "ja"
	Theme string `json:"theme"` // "dark" or "light"
	Sort  string `json:"sort"`  // "name", "recent" or "added"
}

var prefChoices = map[string][]string{
	"lang":  {"zh", "ja"},
	"theme": {"dark", "light"},
	"sort":  {"name", "recent", "added"},
}

func (p *Prefs) field(k string) *string {
	switch k {
	case "lang":
		return &p.Lang
	case "theme":
		return &p.Theme
	case "sort":
		return &p.Sort
	}
	return nil
}

func allowed(k, v string) bool {
	for _, c := range prefChoices[k] {
		if c == v {
			return true
		}
	}
	return false
}

// normalize replaces unknown values with the defaults (first choice).
func (p *Prefs) normalize() {
	for k, cs := range prefChoices {
		if f := p.field(k); !allowed(k, *f) {
			*f = cs[0]
		}
	}
}

type prefStore struct {
	path string
	mu   sync.Mutex
	p    Prefs
}

func openPrefs(path string) *prefStore {
	s := &prefStore{path: path}
	if path != "" {
		if b, err := os.ReadFile(path); err == nil {
			json.Unmarshal(b, &s.p)
		}
	}
	s.p.normalize()
	return s
}

func (s *prefStore) get() Prefs {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.p
}

// update applies the keys present in b; an unknown key or value is an error
// and changes nothing.
func (s *prefStore) update(b []byte) (Prefs, error) {
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		return Prefs{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.p
	for k, v := range m {
		f := next.field(k)
		if f == nil || !allowed(k, v) {
			return Prefs{}, fmt.Errorf("bad preference %q=%q", k, v)
		}
		*f = v
	}
	if s.path != "" {
		out, _ := json.Marshal(next)
		if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
			return Prefs{}, err
		}
		tmp := s.path + ".partial"
		if err := os.WriteFile(tmp, out, 0o644); err != nil {
			return Prefs{}, err
		}
		if err := os.Rename(tmp, s.path); err != nil {
			return Prefs{}, errors.Join(err, os.Remove(tmp))
		}
	}
	s.p = next
	return next, nil
}

// script is prefs.js: it sets <html data-theme lang> before the body is
// parsed. Values are from the fixed choice lists, and JSON-encoded anyway.
func (p Prefs) script() []byte {
	j, _ := json.Marshal(p)
	return []byte(fmt.Sprintf("window.kpPrefs=%s;document.documentElement.dataset.theme=kpPrefs.theme;document.documentElement.lang=kpPrefs.lang===\"ja\"?\"ja\":\"zh-CN\";"+
		// The font's two faces (tokens.css), fetched alongside the page.
		"for(var w of[400,700]){var l=document.createElement(\"link\");l.rel=\"preload\";l.as=\"font\";l.type=\"font/woff2\";l.crossOrigin=\"anonymous\";"+
		"l.href=\"fonts/jus-sans-\"+w+\".woff2\";document.head.appendChild(l)}\n", j))
}

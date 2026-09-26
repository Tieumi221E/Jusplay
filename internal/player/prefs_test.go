package player

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrefs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ui.json")
	s := openPrefs(path)
	if got := s.get(); got != (Prefs{Lang: "zh", Theme: "dark", Sort: "name"}) {
		t.Fatalf("defaults: %+v", got)
	}
	if _, err := s.update([]byte(`{"lang":"ja","theme":"light"}`)); err != nil {
		t.Fatal(err)
	}
	// Rejected as a whole: nothing of it applies.
	for _, bad := range []string{`{"theme":"blue"}`, `{"lang":"en"}`, `{"sort":"name","x":"y"}`, `{"lang":"ja","theme":"<script>"}`, `[]`} {
		if _, err := s.update([]byte(bad)); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
	want := Prefs{Lang: "ja", Theme: "light", Sort: "name"}
	if got := openPrefs(path).get(); got != want {
		t.Fatalf("after reopen: %+v, want %+v", got, want)
	}
	js := string(want.script())
	if !strings.Contains(js, `"theme":"light"`) || !strings.Contains(js, `"ja":"zh-CN"`) {
		t.Errorf("script: %s", js)
	}

	// A hand-edited file with unknown values falls back to the defaults.
	os.WriteFile(path, []byte(`{"lang":"fr","theme":"light","sort":"added"}`), 0o644)
	if got := openPrefs(path).get(); got != (Prefs{Lang: "zh", Theme: "light", Sort: "added"}) {
		t.Errorf("normalize: %+v", got)
	}
}

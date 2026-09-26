package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tieumi221E/Jusplay/internal/library"
)

// The single library file of earlier versions goes into the folders'
// records once; the settings are copied; the old files stay; a second
// start imports nothing.
func TestImportLegacy(t *testing.T) {
	base := t.TempDir()
	media := filepath.Join(base, "media")
	os.MkdirAll(filepath.Join(media, "Show"), 0o755)
	video := filepath.Join(media, "Show", "Show - 01.mkv")
	os.WriteFile(video, []byte("x"), 0o644)
	config := filepath.Join(base, "config")
	old := filepath.Join(config, "kantanplay")
	os.MkdirAll(old, 0o755)
	legacy, _ := json.Marshal(map[string]any{"version": 1, "folders": []string{media}, "entries": map[string]any{
		library.IDFor(video): map[string]any{"id": library.IDFor(video), "path": video, "folder": media, "series": "Show", "position": 321.5, "watched": false},
		"loose":              map[string]any{"id": "loose", "path": filepath.Join(base, "x.mkv"), "folder": "", "series": "x", "position": 9},
	}})
	os.WriteFile(filepath.Join(old, "library.json"), legacy, 0o644)
	os.WriteFile(filepath.Join(old, "settings.json"), []byte(`{"v":2}`), 0o644)
	os.WriteFile(filepath.Join(old, "ui.json"), []byte(`{"theme":"light"}`), 0o644)

	dir := filepath.Join(base, "jusplay-data")
	os.MkdirAll(dir, 0o755)
	folders := filepath.Join(dir, "folders.json")
	msgs := importFrom(config, filepath.Join(base, "cache"), dir, folders)
	if len(msgs) == 0 || !strings.Contains(strings.Join(msgs, "\n"), "imported 1 entries") {
		t.Fatalf("messages %q", msgs)
	}
	lib, err := library.Open(folders)
	if err != nil {
		t.Fatal(err)
	}
	if e, ok := lib.Get(library.IDFor(video)); !ok || e.Position != 321.5 {
		t.Fatalf("imported entry %+v %v", e, ok)
	}
	if len(lib.Entries()) != 1 {
		t.Errorf("entries %d: a loose one was imported", len(lib.Entries()))
	}
	if _, err := os.Stat(filepath.Join(media, library.VaultDir, "library.json")); err != nil {
		t.Errorf("no records in the media folder: %v", err)
	}
	for _, f := range []string{"settings.json", "ui.json"} {
		if b, _ := os.ReadFile(filepath.Join(dir, f)); len(b) == 0 {
			t.Errorf("%s not copied", f)
		}
	}
	if _, err := os.Stat(filepath.Join(old, "library.json")); err != nil {
		t.Error("the old library file was removed")
	}
	if again := importFrom(config, filepath.Join(base, "cache"), dir, folders); again != nil {
		t.Errorf("second start imported again: %q", again)
	}
	// An older version still open saved later progress: the next start
	// takes it; progress older than the records' is not.
	later := time.Now().Add(time.Hour)
	legacy2, _ := json.Marshal(map[string]any{"version": 1, "folders": []string{media}, "entries": map[string]any{
		library.IDFor(video): map[string]any{"path": video, "folder": media, "series": "Show", "position": 600, "lastPlayed": later},
	}})
	os.WriteFile(filepath.Join(old, "library.json"), legacy2, 0o644)
	os.Chtimes(filepath.Join(old, "library.json"), later, later)
	if msgs := importFrom(config, filepath.Join(base, "cache"), dir, folders); len(msgs) == 0 {
		t.Fatal("a changed old file was not imported again")
	}
	lib, _ = library.Open(folders)
	if e, _ := lib.Get(library.IDFor(video)); e.Position != 600 {
		t.Errorf("later progress not taken: %+v", e)
	}
	earlier := time.Now().Add(-time.Hour)
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(lib.Update(library.IDFor(video), func(e *library.Entry) { e.Position, e.LastPlayed = 700, &later }))
	legacy3, _ := json.Marshal(map[string]any{"version": 1, "folders": []string{media}, "entries": map[string]any{
		library.IDFor(video): map[string]any{"path": video, "folder": media, "series": "Show", "position": 1, "lastPlayed": earlier},
	}})
	os.WriteFile(filepath.Join(old, "library.json"), legacy3, 0o644)
	os.Chtimes(filepath.Join(old, "library.json"), later.Add(time.Hour), later.Add(time.Hour))
	importFrom(config, filepath.Join(base, "cache"), dir, folders)
	lib, _ = library.Open(folders)
	if e, _ := lib.Get(library.IDFor(video)); e.Position != 700 {
		t.Errorf("older progress overwrote newer: %+v", e)
	}
}

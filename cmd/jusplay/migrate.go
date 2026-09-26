package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Tieumi221E/Jusplay/internal/library"
)

// importLegacy brings the data of earlier versions into the current layout
// when the app starts on its default data: the single library file
// (%AppData%\kantanplay\library.json, or a jusplay one) is split into each
// media folder's records (library.Import), and the settings and UI
// preferences are copied next to the app. It runs again only if that file
// changed since (an older version still open kept saving progress), and
// then takes only what was played later. The old files are left where they
// are; the returned lines say where, so the user can delete them.
func importLegacy(dir, folders string) []string {
	config, err := os.UserConfigDir()
	if err != nil {
		return nil
	}
	cache, _ := os.UserCacheDir()
	return importFrom(config, cache, dir, folders)
}

// importFrom is importLegacy with the user's config and cache folders given.
func importFrom(config, cache, dir, folders string) (msgs []string) {
	marker := filepath.Join(dir, "imported.json")
	var done struct {
		From    string    `json:"from"`
		ModTime time.Time `json:"modTime"`
	}
	if b, err := os.ReadFile(marker); err == nil {
		json.Unmarshal(b, &done)
	}
	var old string
	for _, name := range []string{"jusplay", "kantanplay"} {
		// Also when it is the fallback data folder itself: the old library
		// file is library.json, the folder list is folders.json.
		d := filepath.Join(config, name)
		if _, err := os.Stat(filepath.Join(d, "library.json")); err == nil {
			old = d
			break
		}
	}
	if old == "" {
		return nil
	}
	src, err := os.Stat(filepath.Join(old, "library.json"))
	if err != nil || (done.From == old && !src.ModTime().After(done.ModTime)) {
		return nil
	}
	var doc struct {
		Folders []string                 `json:"folders"`
		Entries map[string]library.Entry `json:"entries"`
	}
	b, err := os.ReadFile(filepath.Join(old, "library.json"))
	if err == nil {
		err = json.Unmarshal(b, &doc)
	}
	if err != nil {
		return []string{fmt.Sprintf("import from %s: %v (left as it is)", old, err)}
	}
	lib, err := library.Open(folders)
	if err != nil {
		return []string{fmt.Sprintf("import from %s: %v", old, err)}
	}
	var entries []library.Entry
	for _, e := range doc.Entries {
		entries = append(entries, e)
	}
	n, err := lib.Import(doc.Folders, entries)
	if err != nil {
		return []string{fmt.Sprintf("import from %s: %v", old, err)}
	}
	msgs = append(msgs, fmt.Sprintf("imported %d entries of %d folders from %s into the folders' records", n, len(lib.Folders()), old))
	done.From, done.ModTime = old, src.ModTime()
	if b, err := json.Marshal(done); err == nil {
		os.WriteFile(marker, b, 0o644)
	}
	for _, f := range []string{"settings.json", "ui.json"} {
		dst := filepath.Join(dir, f)
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		if b, err := os.ReadFile(filepath.Join(old, f)); err == nil {
			if err := os.WriteFile(dst, b, 0o644); err == nil {
				msgs = append(msgs, "copied "+f)
			}
		}
	}
	msgs = append(msgs, fmt.Sprintf("the old data is still in %s and %s; it is no longer used", old, filepath.Join(cache, filepath.Base(old))))
	return msgs
}

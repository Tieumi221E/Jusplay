package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/Tieumi221E/Jus/apps"
	"github.com/Tieumi221E/Jus/capreg"
	"github.com/Tieumi221E/Jusplay/internal/library"
)

// registerManifest adds the manifest (Jus contracts 11, 12): what Jusplay
// is and every place it writes — the program, the AI component beside it,
// its data folder, and in each media folder its records folder, where the
// caches (thumbnails, indexes) are the app's and the rest — progress,
// comment choices, the change log — is the user's and stays.
func registerManifest(r *capreg.Registry) {
	apps.RegisterManifest(r, func() apps.Manifest {
		exe := apps.Exe()
		data, _ := dataDir(os.Getenv("JUSPLAY_DATA"))
		m := apps.Manifest{ID: "jusplay", Name: "Jusplay", Version: version, Exe: exe,
			Links: []string{"jus://play/"},
			Formats: []apps.Format{
				{Name: "folders", Version: 2, Doc: "folders.json in the data folder: the media folders"},
				{Name: "library", Version: 2, Doc: ".jusplay/library.json in each media folder: files, progress, choices"},
				{Name: "folder-id", Version: 1, Doc: ".jusplay/folder.json: the id jus://play links name"},
			}}
		if exe != "" {
			m.Footprint = append(m.Footprint, apps.Place{Path: exe, Kind: apps.Program, What: "the program"})
			if ai := filepath.Join(filepath.Dir(exe), "components", "ai"); exists(ai) {
				m.Footprint = append(m.Footprint, apps.Place{Path: ai, Kind: apps.Cache, What: "the AI component (models, runtime): downloaded again when wanted"})
			}
		}
		if data != "" {
			m.Footprint = append(m.Footprint, apps.Place{Path: data, Kind: apps.Data, What: "settings, the media folders list, the change log of files outside them, the window profile"})
		}
		var doc struct {
			Folders []string `json:"folders"`
		}
		if b, err := os.ReadFile(filepath.Join(data, "folders.json")); err == nil {
			json.Unmarshal(b, &doc)
		}
		for _, f := range doc.Folders {
			vault := filepath.Join(f, library.VaultDir)
			if !exists(vault) {
				continue
			}
			for _, kind := range []string{"thumbs", "index"} {
				if p := filepath.Join(vault, kind); exists(p) {
					m.Footprint = append(m.Footprint, apps.Place{Path: p, Kind: apps.Cache, What: "made again when needed"})
				}
			}
			m.Footprint = append(m.Footprint, apps.Place{Path: vault, Kind: apps.User, What: "progress, comment choices and the change log of " + f})
		}
		return m
	})
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

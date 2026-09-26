package library

import (
	"testing"
	"time"
)

func TestVersions(t *testing.T) {
	ep := func(v float64) *float64 { return &v }
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	es := []Entry{
		{ID: "raw7", Folder: "D", Series: "S", Episode: ep(7), Path: "D/S/S - 07.mkv", ModTime: t0, Probe: &Probe{}, Position: 100},
		{ID: "enc7", Folder: "D", Series: "S", Episode: ep(7), Path: "D/S/anime_processed/S - 07.mkv", ModTime: t0.Add(-time.Hour), Probe: &Probe{Attached: true}},
		{ID: "gone7", Folder: "D", Series: "S", Episode: ep(7), Path: "D/S/x/S - 07.mkv", Missing: true, Probe: &Probe{Attached: true}},
		{ID: "raw8", Folder: "D", Series: "S", Episode: ep(8), Path: "D/S/S - 08.mkv", ModTime: t0, Probe: &Probe{}},
		{ID: "new8", Folder: "D", Series: "S", Episode: ep(8), Path: "D/S/b/S - 08.mkv", ModTime: t0.Add(time.Hour), Probe: &Probe{}},
		{ID: "other7", Folder: "E", Series: "S", Episode: ep(7), Path: "E/S - 07.mkv", Probe: &Probe{}},
		{ID: "s2e7", Folder: "D", Series: "S", Season: func() *int { v := 2; return &v }(), Episode: ep(7), Path: "D/S/S2 - 07.mkv", Probe: &Probe{}},
		{ID: "film", Folder: "D", Series: "S", Path: "D/S/film.mkv"},
		{ID: "film2", Folder: "D", Series: "S", Path: "D/S/film2.mkv"},
	}
	primaryOf, others := Versions(es)
	// Comments win over progress, and a missing file never stands for the episode.
	want := map[string]string{"raw7": "enc7", "gone7": "enc7", "raw8": "new8"}
	if len(primaryOf) != len(want) {
		t.Fatalf("primaryOf %v, want %v", primaryOf, want)
	}
	for k, v := range want {
		if primaryOf[k] != v {
			t.Errorf("%s -> %q, want %q", k, primaryOf[k], v)
		}
	}
	if len(others["enc7"]) != 2 || len(others["new8"]) != 1 {
		t.Errorf("others %v", others)
	}
}

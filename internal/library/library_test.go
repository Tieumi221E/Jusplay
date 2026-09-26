package library

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

)

func TestParseName(t *testing.T) {
	for _, c := range []struct {
		path, root, series string
		season             int     // 0: none
		ep                 float64 // -1: none
	}{
		{`D:\anime\-A- 空色アトリエ\空色アトリエ3期 - 13.mkv`, `D:\anime`, "空色アトリエ", 3, 13},
		{`D:\anime\-A- 空色アトリエ\空色アトリエ1期OVA.mkv`, `D:\anime`, "空色アトリエ", 1, -1},
		{`D:\anime\-A- 放課後は告らせない\放課後は告らせない2期 - 01.mkv`, `D:\anime`, "放課後は告らせない", 2, 1},
		{`D:\anime\-A- 放課後は告らせない\放課後は告らせない - 01.mkv`, `D:\anime`, "放課後は告らせない", 0, 1},
		{`D:\anime\-A- 星降る図書館\星降る図書館 - 22.mkv`, `D:\anime`, "星降る図書館", 0, 22},
		{`D:\anime\-A- 月見草\月見草 - 11_5.mkv`, `D:\anime`, "月見草", 0, 11.5},
		{`C:\dl\anime_processed\[Group One] Sora no Atelier III Mado no Mukou - 13 (CR 1920x1080 AVC AAC MKV) [A84EF89A].mkv`, `C:\dl\anime_processed`, "Sora no Atelier III Mado no Mukou", 0, 13},
		{`D:\anime\Show\Show 第05話.mkv`, `D:\anime`, "Show", 0, 5},
		{`D:\anime\Show\Show S2 - 04.mkv`, `D:\anime`, "Show", 2, 4},
		{`D:\anime\Show\Show 2nd Season - 04.mkv`, `D:\anime`, "Show", 2, 4},
		{`D:\anime\Show\Show - 12.5.mkv`, `D:\anime`, "Show", 0, 12.5},
		{`D:\anime\Movie\劇場版.mkv`, `D:\anime`, "Movie", 0, -1},
		{`C:\x\single file - 03.mkv`, "", "single file", 0, 3},
		{`D:\anime\Show\Show - 01 - 旅立ち.mkv`, `D:\anime`, "Show", 0, 1},
		// A generic parent folder (a tool's output, a season) names no show.
		{`D:\tmp\雨上がりの時計塔\anime_processed\雨上がりの時計塔 - 07.mkv`, `D:\tmp`, "雨上がりの時計塔", 0, 7},
		{`D:\anime\Show\Season 2\Show - 04.mkv`, `D:\anime`, "Show", 2, 4},
		{`D:\anime\Show\S03\[G] Show - 04 [1080p].mkv`, `D:\anime`, "Show", 3, 4},
		{`D:\anime\anime_processed\Other - 04.mkv`, `D:\anime`, "Other", 0, 4},
	} {
		n := ParseName(c.path, c.root)
		if want := map[bool]string{true: "旅立ち"}[strings.Contains(c.path, "旅立ち")]; n.Subtitle != want {
			t.Errorf("%s: subtitle %q, want %q", c.path, n.Subtitle, want)
		}
		if n.Series != c.series {
			t.Errorf("%s: series %q, want %q", c.path, n.Series, c.series)
		}
		if (n.Season == nil) != (c.season == 0) || n.Season != nil && *n.Season != c.season {
			t.Errorf("%s: season %v, want %v", c.path, n.Season, c.season)
		}
		if (n.Episode == nil) != (c.ep < 0) || n.Episode != nil && *n.Episode != c.ep {
			t.Errorf("%s: episode %v, want %v", c.path, n.Episode, c.ep)
		}
	}
}

func TestScanKeepsProgressAndMarksMissing(t *testing.T) {
	root := t.TempDir()
	show := filepath.Join(root, "-A- Show")
	os.MkdirAll(show, 0o755)
	a, b := filepath.Join(show, "Show - 01.mkv"), filepath.Join(show, "Show - 02.mkv")
	os.WriteFile(a, []byte("not really a video"), 0o644)
	os.WriteFile(b, []byte("not really a video"), 0o644)
	os.MkdirAll(filepath.Join(root, ".hidden"), 0o755)
	os.WriteFile(filepath.Join(root, ".hidden", "x - 01.mkv"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(show, "notes.txt"), []byte("x"), 0o644)

	libPath := filepath.Join(t.TempDir(), "library.json")
	l, err := Open(libPath)
	if err != nil {
		t.Fatal(err)
	}
	must(t, l.AddFolder(root))
	st, err := l.Scan()
	must(t, err)
	if st.Files != 2 || st.Added != 2 {
		t.Fatalf("first scan %+v", st)
	}
	es := l.Entries()
	if es[0].Series != "Show" || *es[0].Episode != 1 || *es[1].Episode != 2 || es[0].Probe == nil || es[0].Probe.Error == "" {
		t.Fatalf("entries %+v %+v", es[0], es[1])
	}
	must(t, l.Update(es[0].ID, func(e *Entry) { e.Position = 123; e.Watched = true }))

	// Unchanged files are not re-probed; a changed one keeps its progress.
	st, _ = l.Scan()
	if st.Added != 0 || st.Changed != 0 {
		t.Fatalf("rescan %+v", st)
	}
	later := time.Now().Add(time.Hour)
	os.Chtimes(a, later, later)
	st, _ = l.Scan()
	if st.Changed != 1 {
		t.Fatalf("changed scan %+v", st)
	}
	if e, _ := l.Get(IDFor(a)); e.Position != 123 || !e.Watched {
		t.Fatalf("progress lost: %+v", e)
	}
	// A vanished file is marked, not dropped; the library survives a reload.
	os.Remove(b)
	st, _ = l.Scan()
	if st.Missing != 1 {
		t.Fatalf("missing scan %+v", st)
	}
	l2, err := Open(libPath)
	must(t, err)
	if e, ok := l2.Get(IDFor(b)); !ok || !e.Missing {
		t.Fatalf("reloaded missing entry %+v %v", e, ok)
	}
	// An unreadable folder (drive unplugged) is reported and its files are
	// not marked missing.
	os.Rename(root, root+"-gone")
	st, _ = l.Scan()
	if len(st.FolderErrors) != 1 || st.Missing != 0 {
		t.Fatalf("unreadable folder scan %+v", st)
	}
	if e, _ := l.Get(IDFor(a)); e.Missing {
		t.Fatal("file under an unreadable folder marked missing")
	}
	os.Rename(root+"-gone", root)
	// A corrupt library file is an error, never silently replaced.
	os.WriteFile(libPath, []byte("{"), 0o644)
	if _, err := Open(libPath); err == nil {
		t.Error("corrupt library opened")
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// An empty or relative folder in the list (a hand-edited or damaged file)
// is ignored, not taken as the current directory, which would scan and
// write records into wherever the app was started.
func TestFoldersMustBeAbsolute(t *testing.T) {
	dir := t.TempDir()
	list := filepath.Join(dir, "folders.json")
	os.WriteFile(list, []byte(`{"version":2,"folders":["", "relative", "."]}`), 0o644)
	l, err := Open(list)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(l.FolderInfos()); n != 0 {
		t.Fatalf("%d folders loaded from empty/relative paths", n)
	}
	if err := l.AddFolder("relative"); err == nil {
		t.Fatal("a relative folder was added")
	}
}

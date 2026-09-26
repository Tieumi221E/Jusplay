package library

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

)

func newShow(t *testing.T, root string, names ...string) {
	t.Helper()
	os.MkdirAll(filepath.Join(root, "Show"), 0o755)
	for _, n := range names {
		os.WriteFile(filepath.Join(root, "Show", n), []byte("not really a video"), 0o644)
	}
}

// The records live in the media folder, with paths relative to it; the app
// keeps the folder list and nothing else. Moved elsewhere and added again,
// the folder brings its progress along; removed, it keeps it.
func TestRecordsLiveInTheFolder(t *testing.T) {
	root := filepath.Join(t.TempDir(), "media")
	newShow(t, root, "Show - 01.mkv", "Show - 02.mkv")
	app := filepath.Join(t.TempDir(), "folders.json")
	l, err := Open(app)
	must(t, err)
	must(t, l.AddFolder(root))
	if _, err := l.Scan(); err != nil {
		t.Fatal(err)
	}
	a := filepath.Join(root, "Show", "Show - 01.mkv")
	off := int64(-300)
	must(t, l.Update(IDFor(a), func(e *Entry) { e.Position, e.OffsetMs = 42, &off }))

	rec, err := os.ReadFile(filepath.Join(root, VaultDir, "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc vaultDoc
	must(t, json.Unmarshal(rec, &doc))
	for _, e := range doc.Entries {
		if filepath.IsAbs(e.Path) || e.ID != "" || e.Folder != "" {
			t.Errorf("stored entry %q (id %q, folder %q): want a relative path only", e.Path, e.ID, e.Folder)
		}
	}
	appDoc, _ := os.ReadFile(app)
	if strings.Contains(string(appDoc), "position") || strings.Contains(string(appDoc), "Show - 01") {
		t.Errorf("the app's file holds more than the folder list:\n%s", appDoc)
	}
	if d := l.DataDir(Entry{Folder: root}); d != filepath.Join(root, VaultDir) {
		t.Errorf("data dir %q", d)
	}

	// Removed: gone from the library, records kept; added again: back.
	must(t, l.RemoveFolder(root))
	if len(l.Entries()) != 0 {
		t.Fatal("entries of a removed folder remain")
	}
	must(t, l.AddFolder(root))
	if e, ok := l.Get(IDFor(a)); !ok || e.Position != 42 {
		t.Fatalf("re-added folder lost its records: %+v %v", e, ok)
	}

	// Moved (another drive, another computer): the same records, new paths.
	moved := filepath.Join(t.TempDir(), "elsewhere")
	must(t, os.Rename(root, moved))
	l2, err := Open(filepath.Join(t.TempDir(), "folders.json"))
	must(t, err)
	must(t, l2.AddFolder(moved))
	e, ok := l2.Get(IDFor(filepath.Join(moved, "Show", "Show - 01.mkv")))
	if !ok || e.Position != 42 || e.OffsetMs == nil || *e.OffsetMs != -300 || e.Folder != moved {
		t.Fatalf("moved folder: %+v %v", e, ok)
	}
}

// A file outside every folder is known while the app runs and written
// nowhere; a folder that cannot be written keeps its records in memory and
// says so; a records file that cannot be read is left as it is.
func TestNothingWrittenOutsideFolders(t *testing.T) {
	appDir := t.TempDir()
	app := filepath.Join(appDir, "folders.json")
	l, err := Open(app)
	must(t, err)
	loose := filepath.Join(t.TempDir(), "Other - 01.mkv")
	os.WriteFile(loose, []byte("x"), 0o644)
	e, err := l.Ensure(loose)
	must(t, err)
	must(t, l.Update(e.ID, func(e *Entry) { e.Position = 7 }))
	if d := l.DataDir(e); d != "" {
		t.Errorf("a loose file got a data dir %q", d)
	}
	if files, _ := os.ReadDir(appDir); len(files) != 0 {
		t.Errorf("app dir after a loose file: %v", files)
	}
	l2, _ := Open(app)
	if _, ok := l2.Get(e.ID); ok {
		t.Error("a loose file outlived the app")
	}

	// Unwritable: ".jusplay" is a file here, so the folder cannot be made.
	ro := filepath.Join(t.TempDir(), "ro")
	newShow(t, ro, "Show - 01.mkv")
	os.WriteFile(filepath.Join(ro, VaultDir), []byte("in the way"), 0o644)
	must(t, l.AddFolder(ro))
	if _, err := l.Scan(); err != nil {
		t.Fatal(err)
	}
	var info FolderInfo
	for _, f := range l.FolderInfos() {
		if f.Path == ro {
			info = f
		}
	}
	if info.Error == "" || l.DataDir(Entry{Folder: ro}) != "" {
		t.Errorf("unwritable folder: %+v", info)
	}
	if _, ok := l.Get(IDFor(filepath.Join(ro, "Show", "Show - 01.mkv"))); !ok {
		t.Error("unwritable folder's files not in the library")
	}

	// Unreadable records: reported, never overwritten.
	bad := filepath.Join(t.TempDir(), "bad")
	newShow(t, bad, "Show - 01.mkv")
	os.MkdirAll(filepath.Join(bad, VaultDir), 0o755)
	junk := []byte("{ not json")
	os.WriteFile(filepath.Join(bad, VaultDir, "library.json"), junk, 0o644)
	must(t, l.AddFolder(bad))
	if _, err := l.Scan(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(bad, VaultDir, "library.json")); string(b) != string(junk) {
		t.Errorf("unreadable records overwritten: %q", b)
	}
}

// The single library file of earlier versions: folders and their entries
// come in (progress included), files opened on their own do not, and
// records a folder already has win.
func TestImport(t *testing.T) {
	root := filepath.Join(t.TempDir(), "media")
	newShow(t, root, "Show - 01.mkv", "Show - 02.mkv")
	a, b := filepath.Join(root, "Show", "Show - 01.mkv"), filepath.Join(root, "Show", "Show - 02.mkv")
	l, err := Open(filepath.Join(t.TempDir(), "folders.json"))
	must(t, err)
	must(t, l.AddFolder(root))
	must(t, func() error { _, err := l.Scan(); return err }())
	must(t, l.Update(IDFor(b), func(e *Entry) { e.Position = 5 }))
	c := filepath.Join(root, "Show", "Show - 03.mkv") // not in the folder's records yet
	os.WriteFile(c, []byte("x"), 0o644)

	l2, err := Open(filepath.Join(t.TempDir(), "folders.json"))
	must(t, err)
	n, err := l2.Import([]string{root, filepath.Join(root, "gone")}, []Entry{
		{Path: c, Folder: root, Series: "Show", Position: 99, Watched: true},
		{Path: b, Folder: root, Series: "Show", Position: 1},
		{Path: `C:\x\loose.mkv`, Series: "loose", Position: 3},
	})
	must(t, err)
	if n != 1 {
		t.Errorf("imported %d, want 1", n)
	}
	if _, ok := l2.Get(IDFor(a)); !ok {
		t.Error("the folder's own records were not loaded")
	}
	if e, _ := l2.Get(IDFor(c)); e.Position != 99 || !e.Watched {
		t.Errorf("imported entry %+v", e)
	}
	if e, _ := l2.Get(IDFor(b)); e.Position != 5 {
		t.Errorf("records already in the folder should win: %+v", e)
	}
	if _, ok := l2.Get(IDFor(`C:\x\loose.mkv`)); ok {
		t.Error("a loose entry was imported")
	}
	if len(l2.Folders()) != 1 {
		t.Errorf("folders %v (a missing folder must not be added)", l2.Folders())
	}
}

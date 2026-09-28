package library

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// Two libraries on the same folder stand for two writers (a second process,
// an agent's command line, a text editor): neither loses the other's
// changes, and each sees them before its next write.
func TestWritersDoNotOverwriteEachOther(t *testing.T) {
	root := filepath.Join(t.TempDir(), "media")
	newShow(t, root, "Show - 01.mkv", "Show - 02.mkv")
	a1 := IDFor(filepath.Join(root, "Show", "Show - 01.mkv"))
	a2 := IDFor(filepath.Join(root, "Show", "Show - 02.mkv"))

	one, err := Open(filepath.Join(t.TempDir(), "folders.json"))
	must(t, err)
	must(t, one.AddFolder(root))
	_, err = one.Scan()
	must(t, err)
	two, err := Open(filepath.Join(t.TempDir(), "folders.json"))
	must(t, err)
	must(t, two.AddFolder(root))

	// Different entries.
	must(t, one.Update(a1, func(e *Entry) { e.Position = 100 }))
	tick() // a new modification time, as between two real writes
	must(t, two.Update(a2, func(e *Entry) { e.Watched = true }))
	tick()
	must(t, one.Update(a1, func(e *Entry) { e.OffsetMs = ptr(int64(250)) }))

	three, err := Open(filepath.Join(t.TempDir(), "folders.json"))
	must(t, err)
	must(t, three.AddFolder(root))
	e1, _ := three.Get(a1)
	e2, _ := three.Get(a2)
	if e1.Position != 100 || e1.OffsetMs == nil || *e1.OffsetMs != 250 || !e2.Watched {
		t.Fatalf("a write was lost: 01 %+v, 02 watched %v", e1, e2.Watched)
	}

	// The same entry: the later write works on the earlier one's result.
	tick()
	must(t, two.Update(a1, func(e *Entry) { e.Position += 5 }))
	if e, _ := two.Get(a1); e.Position != 105 || e.OffsetMs == nil {
		t.Fatalf("update did not start from the file's state: %+v", e)
	}

	// Refresh shows the other's changes without writing.
	one.Refresh()
	if e, _ := one.Get(a1); e.Position != 105 {
		t.Fatalf("refresh: position %v", e.Position)
	}
}

func TestUpdateIfConflicts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "media")
	newShow(t, root, "Show - 01.mkv")
	id := IDFor(filepath.Join(root, "Show", "Show - 01.mkv"))
	one, err := Open(filepath.Join(t.TempDir(), "folders.json"))
	must(t, err)
	must(t, one.AddFolder(root))
	_, err = one.Scan()
	must(t, err)
	e, _ := one.Get(id)
	base := e.Version()

	two, err := Open(filepath.Join(t.TempDir(), "folders.json"))
	must(t, err)
	must(t, two.AddFolder(root))
	tick()
	must(t, two.Update(id, func(e *Entry) { e.Position = 7 }))

	if err := one.UpdateIf(id, base, func(e *Entry) { e.Watched = true }); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale base accepted: %v", err)
	}
	if e, _ := one.Get(id); e.Watched || e.Position != 7 {
		t.Fatalf("a refused write changed the entry: %+v", e)
	}
	e, _ = one.Get(id)
	must(t, one.UpdateIf(id, e.Version(), func(e *Entry) { e.Watched = true }))
}

// A records file someone broke is neither overwritten nor half-read.
func TestBrokenRecordsFileIsNotOverwritten(t *testing.T) {
	root := filepath.Join(t.TempDir(), "media")
	newShow(t, root, "Show - 01.mkv")
	id := IDFor(filepath.Join(root, "Show", "Show - 01.mkv"))
	l, err := Open(filepath.Join(t.TempDir(), "folders.json"))
	must(t, err)
	must(t, l.AddFolder(root))
	_, err = l.Scan()
	must(t, err)
	tick()
	rec := filepath.Join(root, VaultDir, "library.json")
	must(t, writeAtomic(rec, []byte("{ broken")))
	must(t, l.Update(id, func(e *Entry) { e.Position = 9 }))
	if b, _, _ := readFile(rec); string(b) != "{ broken" {
		t.Fatalf("broken records file overwritten: %q", b)
	}
}

func ptr[T any](v T) *T { return &v }

// tick waits long enough for a file's modification time to differ.
func tick() { time.Sleep(20 * time.Millisecond) }

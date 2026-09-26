package embed

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tieumi221E/Jusplay/internal/bundle"
	"github.com/Tieumi221E/Jusplay/internal/comments"
	"github.com/Tieumi221E/Jusplay/internal/detzip"
	"github.com/Tieumi221E/Jusplay/internal/matroska"
	"github.com/Tieumi221E/Jusplay/internal/mkv"
)

// fixture copies the test video (1 s H.264 + AAC with a font attachment
// and two chapters) to path.
func fixture(t *testing.T, path string) {
	t.Helper()
	b, err := os.ReadFile("../../testdata/media/tiny.mkv")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestEmbedInPlace(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "Show - 01.mkv")
	fixture(t, video)
	if _, err := Embed(video, "", Options{}); !errors.Is(err, comments.ErrNone) {
		t.Fatalf("no same-name json: %v", err)
	}
	src, err := os.ReadFile("../../testdata/zouryou.json")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "Show - 01.json"), src, 0o644)

	r, err := Embed(video, "", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Replaced || r.Comments != 4 || r.Verification.Attachments != 3 {
		t.Errorf("first embed %+v", r)
	}
	files, err := mkv.Extract(video)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bundle.Check(files); err != nil {
		t.Fatalf("embedded attachments: %v", err)
	}
	// Set a verified offset, then embed again without one: the new data
	// replaces the old, the font stays, the timing is carried over.
	off := int64(-250)
	// Replacing without a backup folder is refused, and changes nothing.
	if _, err := Embed(video, "", Options{OffsetMs: &off}); err == nil {
		t.Fatal("replacing comments without a backup folder succeeded")
	}
	backups := filepath.Join(dir, ".jusplay", "backup")
	if _, err := Embed(video, "", Options{OffsetMs: &off, SyncVerified: true, BackupDir: backups}); err != nil {
		t.Fatal(err)
	}
	r, err = Embed(video, "", Options{BackupDir: backups})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Replaced || r.OffsetMs != -250 || !r.SyncVerified {
		t.Errorf("re-embed %+v", r)
	}
	// The attachments it replaced are in the backup, whole.
	kept, err := os.ReadFile(r.Backup)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	zfs, err := detzip.Open(kept)
	if err != nil {
		t.Fatal(err)
	}
	old, err := detzip.ReadAll(zfs)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bundle.Check(old); err != nil {
		t.Fatalf("backed-up attachments: %v", err)
	}
	p, _ := mkv.ProbeFile(video)
	fonts := 0
	var names []string
	for _, a := range p.Attachments {
		names = append(names, a.Name)
		if a.Name == "font.ttf" {
			fonts++
		}
	}
	if len(p.Ours()) != 3 || fonts != 1 {
		t.Errorf("attachments after re-embed: %d ours, %d fonts: %v", len(p.Ours()), fonts, names)
	}
	files, _ = mkv.Extract(video)
	var m bundle.Manifest
	json.Unmarshal(files[bundle.ManifestName], &m)
	if m.VideoID != "" || *m.Timing.OffsetMs != -250 || !m.Timing.Verified {
		t.Errorf("manifest %+v", m)
	}
	// Nothing left behind: just the video, its json and the folder's
	// records (the backups).
	entries, _ := os.ReadDir(dir)
	if len(entries) != 3 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("leftovers: %v", names)
	}
	// A non-MKV is refused before anything is written.
	mp4 := filepath.Join(dir, "x.mp4")
	os.WriteFile(mp4, []byte("x"), 0o644)
	if _, err := Embed(mp4, filepath.Join(dir, "Show - 01.json"), Options{}); err == nil || !strings.Contains(err.Error(), "MKV") {
		t.Errorf("mp4: %v", err)
	}
}

// Files embedded before the rename (kantanplay.* attachments, kantanplay-*
// schema names) are still read, verified and, on re-embedding, replaced.
func TestLegacyAttachments(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "Show - 01.mkv")
	fixture(t, video)
	src, _ := os.ReadFile("../../testdata/zouryou.json")
	os.WriteFile(filepath.Join(dir, "Show - 01.json"), src, 0o644)
	if _, err := Embed(video, "", Options{}); err != nil {
		t.Fatal(err)
	}
	files, err := mkv.Extract(video)
	if err != nil {
		t.Fatal(err)
	}
	// The same attachments as the old version wrote them.
	legacy := filepath.Join(dir, "legacy.mkv")
	var old []matroska.NewFile
	for name, b := range files {
		n := strings.Replace(name, mkv.Prefix, mkv.LegacyPrefix, 1)
		if name == bundle.ManifestName {
			b = []byte(strings.NewReplacer(`"jusplay-attachment/0"`, `"kantanplay-attachment/0"`, `"`+mkv.Prefix, `"`+mkv.LegacyPrefix).Replace(string(b)))
		}
		old = append(old, matroska.NewFile{Name: n, MIME: "application/octet-stream", Data: b})
	}
	f, c, err := matroska.Open(video)
	if err != nil {
		t.Fatal(err)
	}
	err = f.WriteFile(legacy, func(a matroska.Attachment) bool { return !strings.HasPrefix(a.Name, mkv.Prefix) }, old)
	c.Close()
	if err != nil {
		t.Fatal(err)
	}
	got, err := mkv.Extract(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got[bundle.PlaybackName]; !ok || len(got) != 3 {
		t.Fatalf("legacy names not read as current ones: %v", keys(got))
	}
	if m, err := bundle.Check(got); err != nil || m.Schema != "kantanplay-attachment/0" {
		t.Fatalf("legacy attachments do not verify: %v %+v", err, m)
	}
	r, err := Embed(legacy, filepath.Join(dir, "Show - 01.json"), Options{BackupDir: filepath.Join(dir, "backup")})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := mkv.ProbeFile(legacy)
	for _, a := range p.Ours() {
		if !strings.HasPrefix(a.Name, mkv.Prefix) {
			t.Errorf("legacy attachment kept: %s", a.Name)
		}
	}
	if !r.Replaced || len(p.Ours()) != 3 {
		t.Errorf("re-embed over legacy: replaced %v, %d attachments", r.Replaced, len(p.Ours()))
	}
}

func keys(m map[string][]byte) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

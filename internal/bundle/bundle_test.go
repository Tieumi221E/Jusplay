package bundle

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tieumi221E/Jusplay/internal/derive"
	"github.com/Tieumi221E/Jusplay/internal/detzip"
	"github.com/Tieumi221E/Jusplay/internal/mkv"
	"github.com/Tieumi221E/Jusplay/internal/snapshot"
)

func fixture(t *testing.T) *snapshot.Snapshot {
	t.Helper()
	s, err := snapshot.Load(os.DirFS("../../testdata/v1-basic"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestZipDeterministicRoundTrip(t *testing.T) {
	s := fixture(t)
	a, err := detzip.Write(s.Files())
	if err != nil {
		t.Fatal(err)
	}
	b, _ := detzip.Write(fixture(t).Files())
	if !bytes.Equal(a, b) {
		t.Fatal("zip not reproducible")
	}
	zfs, err := detzip.Open(a)
	if err != nil {
		t.Fatal(err)
	}
	back, err := snapshot.Load(zfs)
	if err != nil {
		t.Fatal(err)
	}
	for p, want := range s.Files() {
		if !bytes.Equal(back.Files()[p], want) {
			t.Errorf("%s changed through zip", p)
		}
	}
}

func buildFixture(t *testing.T, probe *mkv.Probe) []mkv.Attachment {
	t.Helper()
	atts, _, err := Build(fixture(t), probe, Options{
		Derive:    derive.Options{Forks: []string{"owner", "main"}},
		Renderer:  Renderer{Name: "niconicomments", Version: "0.4.1", Mode: "default", Fonts: "default"},
		CreatedAt: "2026-01-04T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	return atts
}

func asMap(atts []mkv.Attachment) map[string][]byte {
	m := map[string][]byte{}
	for _, a := range atts {
		m[a.Name] = append([]byte(nil), a.Data...)
	}
	return m
}

func TestCheck(t *testing.T) {
	files := asMap(buildFixture(t, &mkv.Probe{}))
	if _, err := Check(files); err != nil {
		t.Fatalf("fresh bundle should pass: %v", err)
	}
	cases := map[string]func(map[string][]byte){
		"playback byte flipped": func(m map[string][]byte) { m[PlaybackName][5] ^= 1 },
		"zip byte flipped":      func(m map[string][]byte) { m[RawName][len(m[RawName])/2] ^= 1 },
		"manifest missing":      func(m map[string][]byte) { delete(m, ManifestName) },
		"unknown attachment":    func(m map[string][]byte) { m["jusplay.other"] = []byte("x") },
		"manifest forks edited": func(m map[string][]byte) {
			m[ManifestName] = bytes.Replace(m[ManifestName], []byte(`"main"`), []byte(`"easy"`), 1)
		},
	}
	for name, mutate := range cases {
		m := asMap(buildFixture(t, &mkv.Probe{}))
		mutate(m)
		if _, err := Check(m); err == nil {
			t.Errorf("%s: Check passed", name)
		}
	}
}

// TestMKV attaches, extracts and checks on the test video (1 s H.264 +
// AAC, a font attachment, two chapters). What a damaged rewrite looks like
// to the verification is tested in internal/matroska.
func TestMKV(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.mkv")
	b, err := os.ReadFile("../../testdata/media/tiny.mkv")
	must(t, err)
	must(t, os.WriteFile(in, b, 0o644))
	probe, err := mkv.ProbeFile(in)
	must(t, err)
	atts := buildFixture(t, probe)
	out := filepath.Join(dir, "out.mkv")
	v, err := mkv.Attach(in, out, atts)
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	if v.MediaStreams != 2 || v.Chapters != 2 || v.Attachments != 3 || v.Packets == 0 {
		t.Errorf("verification %+v", v)
	}
	got, err := mkv.Extract(out)
	must(t, err)
	if _, err := Check(got); err != nil {
		t.Fatalf("check extracted: %v", err)
	}
	if orig, _ := os.ReadFile(in); len(orig) != len(b) {
		t.Fatal("input changed")
	}
	// Guards against misuse.
	if _, err := mkv.Attach(in, out, atts); err == nil {
		t.Error("attach overwrote an existing output")
	}
	if _, err := mkv.Attach(out, filepath.Join(dir, "again.mkv"), atts); err == nil ||
		!strings.Contains(err.Error(), "already has") {
		t.Errorf("attach onto an already-attached file: %v", err)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

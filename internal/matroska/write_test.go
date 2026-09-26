package matroska

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fixture = "../../testdata/media/tiny.mkv" // 1 s H.264 (B-frames) + AAC, a font attachment, 2 chapters

func writeFixture(t *testing.T, add []NewFile) (in, out string) {
	t.Helper()
	dir := t.TempDir()
	in = filepath.Join(dir, "in.mkv")
	b, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(in, b, 0o644); err != nil {
		t.Fatal(err)
	}
	f, c, err := Open(in)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	out = filepath.Join(dir, "out.mkv")
	if err := f.WriteFile(out, func(Attachment) bool { return true }, add); err != nil {
		t.Fatal(err)
	}
	return in, out
}

var ours = []NewFile{
	{Name: "jusplay.nico.raw.zip", MIME: "application/zip", Data: bytes.Repeat([]byte{1, 2, 3}, 3000)},
	{Name: "jusplay.nico.playback.json", MIME: "application/json", Data: []byte(`{"threads":[]}`)},
}

func TestWriteAndVerify(t *testing.T) {
	in, out := writeFixture(t, ours)
	keepAll := func(Attachment) bool { return true }
	v, err := Verify(in, out, keepAll, ours)
	if err != nil {
		t.Fatal(err)
	}
	if v.MediaStreams != 2 || v.Chapters != 2 || v.Attachments != 2 || v.Packets == 0 || v.ClusterBytes == 0 {
		t.Errorf("verification %+v", v)
	}
	f, c, err := Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if len(f.Attachments) != 3 || f.Attachments[0].Name != "font.ttf" {
		t.Fatalf("attachments %+v", f.Attachments)
	}
	for i, n := range ours {
		b, err := f.AttachmentData(f.Attachments[i+1], 1<<20)
		if err != nil || !bytes.Equal(b, n.Data) || f.Attachments[i+1].MIME != n.MIME {
			t.Errorf("attachment %s read back wrong", n.Name)
		}
	}
	// Replacing ours keeps the font and drops the old ones.
	out2 := filepath.Join(filepath.Dir(out), "out2.mkv")
	keepFont := func(a Attachment) bool { return a.Name == "font.ttf" }
	next := []NewFile{{Name: "jusplay.nico.playback.json", MIME: "application/json", Data: []byte(`{}`)}}
	if err := f.WriteFile(out2, keepFont, next); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(out, out2, keepFont, next); err != nil {
		t.Fatal(err)
	}
}

// Each of these outputs is wrong in one way; Verify must say so. A check
// is only worth something if it fails when it should.
func TestVerifyCatchesDamage(t *testing.T) {
	in, out := writeFixture(t, ours)
	keepAll := func(Attachment) bool { return true }
	good, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	f, c, err := Open(in)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := f.Frames(nil)
	c.Close()
	if err != nil || len(frames) < 10 {
		t.Fatalf("frames: %v", err)
	}
	patch := func(name string, edit func(b []byte) []byte, attachments []NewFile, want string) {
		t.Run(name, func(t *testing.T) {
			b := edit(append([]byte(nil), good...))
			bad := filepath.Join(t.TempDir(), "bad.mkv")
			if err := os.WriteFile(bad, b, 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := Verify(in, bad, keepAll, attachments)
			if err == nil {
				t.Fatal("accepted")
			}
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("rejected, but for %q (want %q)", err, want)
			}
		})
	}
	at := func(b []byte, s string) int {
		i := bytes.Index(b, []byte(s))
		if i < 0 {
			t.Fatalf("%q not in the file", s)
		}
		return i
	}
	patch("frame data", func(b []byte) []byte { b[frames[3].Pos+2] ^= 0x55; return b }, ours, "cluster bytes differ")
	patch("block time", func(b []byte) []byte {
		b[frames[5].Pos-2]++ // the low byte of the block's relative time
		return b
	}, ours, "frame 5 differs")
	patch("chapter title", func(b []byte) []byte { b[at(b, "Opening")] = 'o'; return b }, ours, "element 1043A770 differs")
	patch("codec id", func(b []byte) []byte { b[at(b, "V_MPEG4/ISO/AVC")+14] = 'X'; return b }, ours, "track 1 differs")
	patch("seek entry", func(b []byte) []byte {
		// The last seek entry (the attachments'): its position's last byte.
		f2, c2, _ := Read2(b)
		defer c2()
		_ = f2
		i := bytes.LastIndex(b[:f2.FirstCluster], []byte{0x53, 0xAC})
		n := int(b[i+2] & 0x0f) // size of the position (1-8 bytes, 0x8n)
		b[i+2+n]++
		return b
	}, ours, "seek entry")
	wrong := append([]NewFile(nil), ours...)
	wrong[1] = NewFile{Name: ours[1].Name, MIME: ours[1].MIME, Data: []byte("[]")}
	patch("attachment bytes", func(b []byte) []byte { return b }, wrong, "attachment jusplay.nico.playback.json differs")
}

// Read2 parses a file held in memory (tests).
func Read2(b []byte) (*File, func(), error) {
	f, err := Read(bytes.NewReader(b), int64(len(b)))
	return f, func() {}, err
}

package matroska

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestRewriteAgainstFFmpeg adds attachments to each JUSPLAY_ORACLE file
// (written to a temporary folder), replaces them again, and checks both
// results with Verify and, independently, with ffmpeg (the media streams'
// hash unchanged) and ffprobe (the attachments listed). A development check.
func TestRewriteAgainstFFmpeg(t *testing.T) {
	list := os.Getenv("JUSPLAY_ORACLE")
	if list == "" {
		t.Skip("JUSPLAY_ORACLE not set")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not found")
	}
	streamhash := func(p string) string {
		out, err := exec.Command("ffmpeg", "-v", "error", "-i", p, "-map", "0", "-map", "-0:t?", "-c", "copy", "-f", "streamhash", "-hash", "sha256", "-").Output()
		if err != nil {
			t.Fatalf("ffmpeg streamhash %s: %v", p, err)
		}
		return string(out)
	}
	for _, in := range strings.Split(list, ";") {
		ext := strings.ToLower(filepath.Ext(in))
		if ext != ".mkv" && ext != ".webm" {
			continue
		}
		t.Run(filepath.Base(in), func(t *testing.T) {
			dir := t.TempDir()
			if k := os.Getenv("JUSPLAY_KEEP"); k != "" {
				dir = k
				os.Remove(filepath.Join(dir, "one.mkv"))
				os.Remove(filepath.Join(dir, "two.mkv"))
			}
			first := []NewFile{{Name: "jusplay.nico.playback.json", MIME: "application/json", Data: []byte(`{"v":1}`)},
				{Name: "jusplay.nico.raw.zip", MIME: "application/zip", Data: bytes.Repeat([]byte("z"), 100000)}}
			out1 := filepath.Join(dir, "one.mkv")
			f, c, err := Open(in)
			if err != nil {
				t.Fatal(err)
			}
			keepAll := func(Attachment) bool { return true }
			if err := f.WriteFile(out1, keepAll, first); err != nil {
				t.Fatal(err)
			}
			c.Close()
			v, err := Verify(in, out1, keepAll, first)
			if err != nil {
				t.Fatalf("verify: %v", err)
			}
			// Replace ours, keep the others.
			second := []NewFile{{Name: "jusplay.nico.playback.json", MIME: "application/json", Data: []byte(`{"v":2}`)}}
			notOurs := func(a Attachment) bool { return !strings.HasPrefix(a.Name, "jusplay.") }
			out2 := filepath.Join(dir, "two.mkv")
			f2, c2, err := Open(out1)
			if err != nil {
				t.Fatal(err)
			}
			if err := f2.WriteFile(out2, notOurs, second); err != nil {
				t.Fatal(err)
			}
			c2.Close()
			if _, err := Verify(out1, out2, notOurs, second); err != nil {
				t.Fatalf("verify replace: %v", err)
			}
			h := streamhash(in)
			if streamhash(out1) != h || streamhash(out2) != h {
				t.Fatal("ffmpeg's media stream hash changed")
			}
			// ffprobe sees the attachments by name and type.
			pj, err := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", out2).Output()
			if err != nil {
				t.Fatal(err)
			}
			var p struct {
				Streams []struct {
					CodecType string            `json:"codec_type"`
					Tags      map[string]string `json:"tags"`
				} `json:"streams"`
			}
			json.Unmarshal(pj, &p)
			if ext == ".webm" {
				return // WebM has no attachments: players ignore them there
			}
			found := 0
			for _, s := range p.Streams {
				if s.CodecType == "attachment" && s.Tags["filename"] == "jusplay.nico.playback.json" && s.Tags["mimetype"] == "application/json" {
					found++
				}
			}
			if found != 1 {
				t.Fatalf("ffprobe lists %d jusplay attachments, want 1", found)
			}
			st1, _ := os.Stat(in)
			st2, _ := os.Stat(out2)
			t.Logf("%d frames, %d MB of clusters untouched; %d -> %d bytes", v.Packets, v.ClusterBytes>>20, st1.Size(), st2.Size())
		})
	}
}

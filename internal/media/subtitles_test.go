package media

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// TestSubtitleTracksSample reads the subtitle tracks of real files, when
// JUSPLAY_SUBS_DIR names a folder of them (not part of the repository):
// every text track must give events in time order, with valid UTF-8 text
// (Matroska requires it) and a length.
func TestSubtitleTracksSample(t *testing.T) {
	dir := os.Getenv("JUSPLAY_SUBS_DIR")
	if dir == "" {
		t.Skip("JUSPLAY_SUBS_DIR not set")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*", "*.mkv"))
	seen := 0
	for _, p := range files {
		m, err := Open(p)
		if err != nil || len(m.Subtitles) == 0 {
			continue
		}
		for _, st := range m.Subtitles {
			if !st.Text {
				t.Logf("%s: track %d %s (picture, skipped)", filepath.Base(p), st.Number, st.Codec)
				continue
			}
			head, ev, err := m.SubtitleEvents(st.Number)
			if err != nil {
				t.Errorf("%s track %d: %v", filepath.Base(p), st.Number, err)
				continue
			}
			bad, noDur := 0, 0
			for i, e := range ev {
				if !utf8.ValidString(e.Data) {
					bad++
				}
				if e.End <= e.Start {
					noDur++
				}
				if i > 0 && e.Start < ev[i-1].Start {
					t.Errorf("%s track %d: events out of order at %d", filepath.Base(p), st.Number, i)
					break
				}
			}
			t.Logf("%s: track %d %s %q %q: %d events, header %d bytes, %d invalid UTF-8, %d without length",
				filepath.Base(p), st.Number, st.Codec, st.Language, st.Name, len(ev), len(head), bad, noDur)
			if bad > 0 {
				t.Errorf("%s track %d: %d events are not UTF-8", filepath.Base(p), st.Number, bad)
			}
			seen++
		}
		if seen >= 6 {
			break
		}
	}
	if seen == 0 {
		t.Log("no text subtitle tracks found")
	}
}

// testdata/media/subs.mkv: tiny.mkv with testdata/subs/tiny.srt (chi) and
// tiny.ass (jpn, "Signs & Songs") muxed in by ffmpeg.
func TestSubtitleEventsFixture(t *testing.T) {
	m, err := Open("../../testdata/media/subs.mkv")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Subtitles) != 2 {
		t.Fatalf("tracks %+v", m.Subtitles)
	}
	srt, ass := m.Subtitles[0], m.Subtitles[1]
	if srt.Codec != "S_TEXT/UTF8" || srt.Language != "chi" || !srt.Text || ass.Codec != "S_TEXT/ASS" || ass.Language != "jpn" || ass.Name != "Signs & Songs" {
		t.Errorf("tracks %+v", m.Subtitles)
	}
	head, ev, err := m.SubtitleEvents(srt.Number)
	if err != nil || head != "" || len(ev) != 2 {
		t.Fatalf("srt: %q %+v %v", head, ev, err)
	}
	if ev[0].Data != "第一行字幕\n<i>斜体</i>" || !near(ev[0].Start, 0.1) || !near(ev[0].End, 0.45) || !near(ev[1].Start, 0.5) {
		t.Errorf("srt events %+v", ev)
	}
	head, ev, err = m.SubtitleEvents(ass.Number)
	if err != nil || len(ev) != 2 {
		t.Fatalf("ass: %+v %v", ev, err)
	}
	if !strings.Contains(head, "[V4+ Styles]") || !strings.HasSuffix(ev[0].Data, `,{\b1}こんにちは{\b0}\N二行目`) || !near(ev[1].End, 0.9) {
		t.Errorf("ass header %q, events %+v", head, ev)
	}
	if _, _, err := m.SubtitleEvents(99); err == nil {
		t.Error("track 99 read")
	}
}

func near(a, b float64) bool { return a-b < 0.002 && b-a < 0.002 }

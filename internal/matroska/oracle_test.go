package matroska

import (
	"bufio"
	"bytes"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestAgainstFFprobe compares the frame index with ffprobe's packet list for
// the files in JUSPLAY_ORACLE (paths separated by ";"): per track, every
// packet's size and key flag, and its time where ffprobe gives one. A
// development check with ffprobe as the reference; skipped without it.
func TestAgainstFFprobe(t *testing.T) {
	list := os.Getenv("JUSPLAY_ORACLE")
	if list == "" {
		t.Skip("JUSPLAY_ORACLE not set")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not found")
	}
	for _, path := range strings.Split(list, ";") {
		if path == "" || !strings.EqualFold(filepath.Ext(path), ".mkv") && !strings.EqualFold(filepath.Ext(path), ".webm") {
			continue
		}
		t.Run(filepath.Base(path), func(t *testing.T) {
			f, c, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			frames, err := f.Frames(nil)
			if err != nil {
				t.Fatal(err)
			}
			out, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "packet=stream_index,pts_time,size,flags", "-of", "csv=p=0", path).Output()
			if err != nil {
				t.Fatal(err)
			}
			type pkt struct {
				pts  float64
				size int
				key  bool
			}
			want := map[int][]pkt{}
			sc := bufio.NewScanner(bytes.NewReader(out))
			for sc.Scan() {
				fs := strings.Split(sc.Text(), ",")
				if len(fs) < 4 {
					continue
				}
				idx, _ := strconv.Atoi(fs[0])
				pts := math.NaN()
				if v, err := strconv.ParseFloat(fs[1], 64); err == nil {
					pts = v
				}
				n, _ := strconv.Atoi(fs[2])
				want[idx] = append(want[idx], pkt{pts, n, strings.Contains(fs[3], "K")})
			}
			for i, tr := range f.Tracks {
				var got []Frame
				for _, fr := range frames {
					if fr.Track == tr.Number {
						got = append(got, fr)
					}
				}
				w := want[i]
				if len(got) != len(w) {
					t.Errorf("track %d (%s): %d frames, ffprobe %d packets", tr.Number, tr.CodecID, len(got), len(w))
					continue
				}
				bad := 0
				for k := range got {
					size := int(got[k].Size) + len(tr.StripPrefix)
					timeOK := math.IsNaN(w[k].pts) || math.Abs(float64(got[k].PTS)/1e9-w[k].pts) < 0.0015
					if size != w[k].size || got[k].Key != w[k].key || !timeOK {
						if bad < 5 {
							t.Errorf("track %d (%s) frame %d: got size %d key %v pts %.6f, ffprobe size %d key %v pts %.6f",
								tr.Number, tr.CodecID, k, size, got[k].Key, float64(got[k].PTS)/1e9, w[k].size, w[k].key, w[k].pts)
						}
						bad++
					}
				}
				if bad > 0 {
					t.Errorf("track %d: %d of %d frames differ", tr.Number, bad, len(got))
				} else {
					t.Logf("track %d (%s): %d frames match", tr.Number, tr.CodecID, len(got))
				}
			}
		})
	}
}

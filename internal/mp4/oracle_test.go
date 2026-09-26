package mp4

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

// TestAgainstFFprobe compares every sample (size, sync flag, presentation
// time) with ffprobe's packets for the MP4 files in JUSPLAY_ORACLE. A
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
		if !strings.EqualFold(filepath.Ext(path), ".mp4") {
			continue
		}
		t.Run(filepath.Base(path), func(t *testing.T) {
			f, _, c, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
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
				w := want[i]
				if len(tr.Samples) != len(w) {
					t.Errorf("track %d (%s): %d samples, ffprobe %d", tr.ID, tr.Format, len(tr.Samples), len(w))
					continue
				}
				bad := 0
				for k, s := range tr.Samples {
					pts := float64(s.PTS) / float64(tr.Timescale)
					if int(s.Size) != w[k].size || s.Key != w[k].key || !(math.IsNaN(w[k].pts) || math.Abs(pts-w[k].pts) < 0.0015) {
						if bad < 5 {
							t.Errorf("track %d (%s) sample %d: size %d key %v pts %.6f; ffprobe %d %v %.6f", tr.ID, tr.Format, k, s.Size, s.Key, pts, w[k].size, w[k].key, w[k].pts)
						}
						bad++
					}
				}
				if bad == 0 {
					t.Logf("track %d (%s, %d Hz, config %v): %d samples match", tr.ID, tr.Format, tr.Timescale, keys(tr.Config), len(tr.Samples))
				}
			}
		})
	}
}

func keys(m map[string][]byte) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

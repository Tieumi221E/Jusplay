package danmaku

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestSampleFiles analyses real comment files, when JUSPLAY_DANMAKU_FILES
// lists them (separated by ";"; not part of the repository), and logs what
// it finds and how long each part took, for judging the analysis by eye.
func TestSampleFiles(t *testing.T) {
	list := os.Getenv("JUSPLAY_DANMAKU_FILES")
	if list == "" {
		t.Skip("JUSPLAY_DANMAKU_FILES not set")
	}
	for _, p := range strings.Split(list, ";") {
		f, err := os.Open(p)
		if err != nil {
			t.Fatal(err)
		}
		t0 := time.Now()
		c, err := Load(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		load := time.Since(t0)
		r := Analyze(c, 0)
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		t.Logf("== %s: %d comments, %d distinct phrases, load %s, analyse %v ms, heap %d MB",
			filepath.Base(p), c.Len(), len(c.Keys), load.Round(time.Millisecond), r.Timing, ms.HeapAlloc>>20)
		other := 0
		for _, x := range r.Categories {
			if x.Name == "other" {
				other = x.Count
			}
			t.Logf("  category %-9s %5d  %v", x.Name, x.Count, x.Phrases)
		}
		t.Logf("  coverage %.1f%%, languages %v", 100*(1-float64(other)/float64(c.Len())), r.Languages)
		for _, h := range r.Hotspots {
			t.Logf("  hotspot %4.0f–%4.0f peak %4.0f z %5.1f rate %.1f base %.1f n %d %v", h.Start, h.End, h.Peak, h.Z, h.Rate, h.Baseline, h.Count, h.Phrases)
		}
		for _, x := range r.Terms[:min(30, len(r.Terms))] {
			t.Logf("  term %-16s %5d coh %.2f ent %.2f", x.Text, x.Count, x.Cohesion, x.Entropy)
		}
		for _, x := range r.Families[:min(12, len(r.Families))] {
			t.Logf("  family %s (%d, %d writings) %v", x.Top, x.Count, x.Size, x.Variants)
		}
		for _, tp := range r.Topics {
			t.Logf("  topic %d comments, peak %.0f s: %v", tp.Count, tp.Peak, tp.Terms)
		}
		t.Logf("  users %+v posting first-day %.2f", r.Users, r.Posting.FirstDayShare)
	}
}

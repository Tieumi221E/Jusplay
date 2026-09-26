package subs

import (
	"math"
	"math/rand"
	"testing"
)

// synth: quiet noise with loud 300 Hz bursts at the given [start, end) seconds.
func synth(total float64, bursts ...[2]float64) []int16 {
	r := rand.New(rand.NewSource(1))
	pcm := make([]int16, int(total*Rate))
	for i := range pcm {
		pcm[i] = int16(r.NormFloat64() * 30)
	}
	for _, b := range bursts {
		for i := int(b[0] * Rate); i < int(b[1]*Rate) && i < len(pcm); i++ {
			pcm[i] += int16(8000 * math.Sin(2*math.Pi*300*float64(i)/Rate))
		}
	}
	return pcm
}

func secs(s Segment) [2]float64 { return [2]float64{float64(s.Start) / Rate, float64(s.End) / Rate} }

func near(a, b float64) bool { return math.Abs(a-b) < 0.05 }

func TestSplitLines(t *testing.T) {
	pcm := synth(10, [2]float64{1, 2.5}, [2]float64{2.9, 3.5}, [2]float64{5, 6}, [2]float64{7, 7.1})
	segs, open := Split(pcm, false)
	// 1–2.5 and 2.9–3.5 join (0.4 s pause); 7–7.1 is too short.
	if len(segs) != 2 {
		t.Fatalf("got %d segments %v, want 2", len(segs), segs)
	}
	if a := secs(segs[0]); !near(a[0], 0.85) || !near(a[1], 3.65) {
		t.Errorf("first %v, want [0.85 3.65]", a)
	}
	if a := secs(segs[1]); !near(a[0], 4.85) || !near(a[1], 6.15) {
		t.Errorf("second %v, want [4.85 6.15]", a)
	}
	if open != len(pcm) {
		t.Errorf("open %d, want the end %d", open, len(pcm))
	}
}

func TestSplitOpenLine(t *testing.T) {
	// A line still going when the chunk ends is left for the next chunk.
	pcm := synth(10, [2]float64{1, 2}, [2]float64{8, 10})
	segs, open := Split(pcm, false)
	if len(segs) != 1 {
		t.Fatalf("got %v, want the first line only", segs)
	}
	if o := float64(open) / Rate; !near(o, 7.85) {
		t.Errorf("open at %.2f s, want 7.85", o)
	}
	// The last chunk of the file keeps it.
	if segs, _ := Split(pcm, true); len(segs) != 2 {
		t.Errorf("last chunk: got %v, want both lines", segs)
	}
}

func TestSplitLongRun(t *testing.T) {
	// 20 s without a pause is cut into lines of at most maxLine.
	pcm := synth(22, [2]float64{1, 21})
	segs, _ := Split(pcm, true)
	if len(segs) < 2 {
		t.Fatalf("got %v, want it split", segs)
	}
	for _, s := range segs {
		if d := float64(s.End-s.Start) / Rate; d > maxLine+2*pad+0.05 {
			t.Errorf("segment of %.1f s", d)
		}
	}
}

func TestSplitSilence(t *testing.T) {
	if segs, _ := Split(synth(5), true); len(segs) != 0 {
		t.Errorf("silence gave %v", segs)
	}
}

func TestTrackCover(t *testing.T) {
	var tr Track
	tr.cover(10, 20)
	tr.cover(0, 5)
	tr.cover(19.98, 30)
	if len(tr.Covered) != 2 || tr.Covered[1] != [2]float64{10, 30} {
		t.Errorf("covered %v", tr.Covered)
	}
	tr.add(Cue{Start: 5, Text: "b"})
	tr.add(Cue{Start: 1, Text: "a"})
	tr.add(Cue{Start: 5.2, Text: "b2"})
	if len(tr.Cues) != 2 || tr.Cues[0].Text != "a" || tr.Cues[1].Text != "b2" {
		t.Errorf("cues %+v", tr.Cues)
	}
}

func TestPlausible(t *testing.T) {
	for _, c := range []struct {
		text string
		sec  float64
		want bool
	}{
		{"ありがとうございます。", 1.1, true},
		{"今日はいい天気ですね。", 4.3, true},
		{"Ooooh.", 8.7, false}, // over the opening song
		{"Is.", 11.5, false},
		{"嗯。", 0.6, true}, // short: kept whatever it says
	} {
		if got := Plausible(c.text, c.sec); got != c.want {
			t.Errorf("Plausible(%q, %.1f) = %v", c.text, c.sec, got)
		}
	}
}

func TestSpoken(t *testing.T) {
	tr := Track{}
	for i := 0; i < 7; i++ {
		tr.Cues = append(tr.Cues, Cue{Lang: "Japanese"})
	}
	if l := tr.Spoken(); l != "" {
		t.Errorf("7 lines: %q, want undecided", l)
	}
	tr.Cues = append(tr.Cues, Cue{Lang: "Japanese"}, Cue{Lang: "Chinese"}, Cue{Lang: "English"})
	if l := tr.Spoken(); l != "Japanese" {
		t.Errorf("8 of 10 Japanese: %q", l)
	}
}

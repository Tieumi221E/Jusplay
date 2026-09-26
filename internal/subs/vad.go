package subs

import (
	"math"
	"sort"
)

// Rate is the sample rate of the PCM the page sends: what the recogniser takes.
const Rate = 16000

const (
	frame      = Rate / 50 // 20 ms
	minSpeech  = 0.25      // shorter bursts are noise
	joinGap    = 0.6       // pauses shorter than this stay inside a line (a breath mid-sentence)
	pad        = 0.15      // kept on both sides of a line
	maxLine    = 10.0      // longer stretches are split at their quietest point
	minSplit   = 4.0       // …but not before this
	openMargin = 0.5       // speech this close to the chunk's end may go on
)

// Segment is a stretch of speech in a chunk, in samples.
type Segment struct{ Start, End int }

// Split finds the lines of speech in pcm by loudness: frames well above
// the chunk's quiet level are speech, short pauses are bridged, long runs
// are cut where they are quietest. Under background music the quiet level
// is the music's, so lines still separate at the pauses between them.
//
// A line still going at the end of the chunk is not returned unless last
// is set (the chunk ends the file); open is then where the next chunk
// should start so the line is heard whole.
func Split(pcm []int16, last bool) (segs []Segment, open int) {
	n := len(pcm) / frame
	open = len(pcm)
	if n == 0 {
		return nil, open
	}
	db := make([]float64, n)
	for i := range db {
		var e float64
		for _, s := range pcm[i*frame : (i+1)*frame] {
			e += float64(s) * float64(s)
		}
		db[i] = 10 * math.Log10(e/frame/(32768*32768)+1e-12)
	}
	sorted := append([]float64(nil), db...)
	sort.Float64s(sorted)
	// Speech is well above the quiet level, but never more than 15 dB under
	// the loud level: a chunk that is nearly all speech has no quiet frames
	// to measure, and its "quiet level" is the speech itself.
	floor, loud := sorted[n/10], sorted[n*95/100]
	thr := math.Max(math.Min(floor+9, loud-15), -55)
	// Speech runs in frames, pauses under joinGap bridged.
	type run struct{ a, b int }
	var runs []run
	for i := 0; i < n; {
		if db[i] < thr {
			i++
			continue
		}
		j := i
		for j < n && db[j] >= thr {
			j++
		}
		if len(runs) > 0 && float64(i-runs[len(runs)-1].b)*0.02 < joinGap {
			runs[len(runs)-1].b = j
		} else {
			runs = append(runs, run{i, j})
		}
		i = j
	}
	// Long runs split at the quietest frame between minSplit and maxLine.
	var lines []run
	for _, r := range runs {
		for float64(r.b-r.a)*0.02 > maxLine {
			lo, hi := r.a+int(minSplit/0.02), r.a+int(maxLine/0.02)
			cut := lo
			for k := lo; k < hi; k++ {
				if db[k] < db[cut] {
					cut = k
				}
			}
			lines = append(lines, run{r.a, cut})
			r.a = cut
		}
		lines = append(lines, r)
	}
	p := int(pad * Rate)
	for _, r := range lines {
		if float64(r.b-r.a)*0.02 < minSpeech {
			continue
		}
		s := Segment{max(0, r.a*frame-p), min(len(pcm), r.b*frame+p)}
		if !last && float64(n-r.b)*0.02 < openMargin {
			// Still going at the end: the next chunk starts with it, unless
			// it began at the very start (a chunk that is all one line).
			if r.a > 0 {
				open = s.Start
				break
			}
		}
		segs = append(segs, s)
	}
	return segs, open
}

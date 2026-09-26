package danmaku

import (
	"math"
	"sort"
)

// Timeline counts comments per second of video.
func (c *Corpus) Timeline(seconds int) []int32 {
	if seconds <= 0 {
		for _, v := range c.Vpos {
			seconds = max(seconds, int(v/1000)+1)
		}
	}
	out := make([]int32, seconds)
	for _, v := range c.Vpos {
		if s := int(v / 1000); s < seconds {
			out[s]++
		}
	}
	return out
}

// Hotspot is a moment with far more comments than the minutes around it.
type Hotspot struct {
	Start, Peak, End float64 // seconds
	Count            int     // comments within [Start, End)
	Rate             float64 // comments per second at the peak (smoothed)
	Baseline         float64 // comments per second around it (rolling median)
	Z                float64 // robust score: (rate − baseline) / (1.4826 · MAD)
}

// Hotspot detection parameters.
const (
	smoothSec   = 5  // moving average window
	baseHalfSec = 60 // rolling median ± window
	minZ        = 3.5
	minLift     = 1.6 // rate at least this multiple of the baseline
	minApart    = 15  // seconds between peaks
	maxHotspots = 16
)

// Hotspots finds the moments whose comment rate stands out from their
// surroundings: a 5 s moving average, compared with the rolling median of
// the two minutes around it, in units of the rolling median absolute
// deviation (robust to the peaks themselves, unlike mean and standard
// deviation). Peaks at least 15 s apart, strongest first.
func Hotspots(per []int32) []Hotspot {
	n := len(per)
	if n == 0 {
		return nil
	}
	sm := make([]float64, n)
	var run float64
	for i := 0; i < n; i++ {
		run += float64(per[i])
		if i >= smoothSec {
			run -= float64(per[i-smoothSec])
		}
		// Centred: the average of the window ending here belongs to its middle.
		j := i - smoothSec/2
		if j >= 0 {
			sm[j] = run / smoothSec
		}
	}
	for j := max(0, n-smoothSec/2); j < n; j++ {
		sm[j] = run / smoothSec
	}
	base := make([]float64, n)
	mad := make([]float64, n)
	win := make([]float64, 0, 2*baseHalfSec+1)
	dev := make([]float64, 0, 2*baseHalfSec+1)
	for i := 0; i < n; i++ {
		win = win[:0]
		for j := max(0, i-baseHalfSec); j < min(n, i+baseHalfSec+1); j++ {
			win = append(win, sm[j])
		}
		m := median(win)
		dev = dev[:0]
		for _, x := range win {
			dev = append(dev, math.Abs(x-m))
		}
		base[i], mad[i] = m, median(dev)
	}
	type cand struct {
		i int
		z float64
	}
	var cs []cand
	for i := 0; i < n; i++ {
		// A local maximum of the smoothed rate.
		if (i > 0 && sm[i] < sm[i-1]) || (i+1 < n && sm[i] < sm[i+1]) {
			continue
		}
		scale := 1.4826*mad[i] + 0.5 // +0.5: a flat, quiet stretch is not infinitely surprising
		z := (sm[i] - base[i]) / scale
		if z >= minZ && sm[i] >= minLift*math.Max(base[i], 0.5) {
			cs = append(cs, cand{i, z})
		}
	}
	sort.Slice(cs, func(a, b int) bool { return cs[a].z > cs[b].z })
	var out []Hotspot
	for _, c := range cs {
		near := false
		for _, h := range out {
			if math.Abs(h.Peak-float64(c.i)) < minApart {
				near = true
				break
			}
		}
		if near {
			continue
		}
		// The extent: out to where the rate falls back to halfway to the baseline.
		half := (sm[c.i] + base[c.i]) / 2
		a, b := c.i, c.i
		for a > 0 && sm[a-1] >= half {
			a--
		}
		for b+1 < n && sm[b+1] >= half {
			b++
		}
		cnt := 0
		for s := a; s <= b; s++ {
			cnt += int(per[s])
		}
		out = append(out, Hotspot{Start: float64(a), Peak: float64(c.i), End: float64(b + 1), Count: cnt, Rate: sm[c.i], Baseline: base[c.i], Z: c.z})
		if len(out) == maxHotspots {
			break
		}
	}
	return out
}

func median(x []float64) float64 {
	if len(x) == 0 {
		return 0
	}
	s := append([]float64(nil), x...)
	sort.Float64s(s)
	if len(s)%2 == 1 {
		return s[len(s)/2]
	}
	return (s[len(s)/2-1] + s[len(s)/2]) / 2
}

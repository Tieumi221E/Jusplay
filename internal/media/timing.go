package media

import (
	"io"
	"sort"

	"github.com/Tieumi221E/Jusplay/internal/fmp4"
	"github.com/Tieumi221E/Jusplay/internal/matroska"
)

// ticks converts nanoseconds to units of 1e9/scale ns, rounded to nearest.
func ticks(ns, scale int64) int64 {
	unit := int64(1e9) / scale
	if ns >= 0 {
		return (ns + unit/2) / unit
	}
	return -((-ns + unit/2) / unit)
}

// videoTimeline gives each frame its decode time: Matroska stores only
// presentation times, and with B-frames those are out of decode order. The
// decode times are the presentation times sorted, less the largest amount
// by which a frame would otherwise be decoded after it is shown.
func videoTimeline(fr []matroska.Frame, scale int64, defaultDur uint64) []sample {
	n := len(fr)
	if n == 0 {
		return nil
	}
	pts := make([]int64, n)
	for i, f := range fr {
		pts[i] = ticks(f.PTS, scale)
	}
	sorted := append([]int64(nil), pts...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	for i := 1; i < n; i++ { // decode times must strictly increase
		if sorted[i] <= sorted[i-1] {
			sorted[i] = sorted[i-1] + 1
		}
	}
	var delay int64
	for i := range pts {
		delay = max(delay, sorted[i]-pts[i])
	}
	out := make([]sample, n)
	for i, f := range fr {
		out[i] = sample{pos: f.Pos, size: f.Size, dts: sorted[i] - delay, pts: pts[i], key: f.Key}
		if i > 0 {
			out[i-1].dur = uint32(out[i].dts - out[i-1].dts)
		}
	}
	switch {
	case defaultDur > 0:
		out[n-1].dur = uint32(ticks(int64(defaultDur), scale))
	case n > 1:
		out[n-1].dur = out[n-2].dur
	}
	return out
}

// audioTimeline counts samples at the codec's rate, frame after frame, and
// re-anchors to the container time only where the two part by more than
// 10 ms (a real gap). Laced frames after the first have estimated times and
// are never anchored to.
func audioTimeline(fr []matroska.Frame, heads [][]byte, c *fmp4.Codec) []sample {
	rate := int64(c.Rate)
	out := make([]sample, len(fr))
	var t int64
	tol := rate / 100
	for i, f := range fr {
		cont := f.PTS * rate / 1e9
		d := int64(c.FrameSize)
		if c.VariableFrame && heads != nil {
			switch c.Name {
			case "opus":
				d = int64(fmp4.OpusSamples(heads[i]))
			case "flac":
				d = int64(fmp4.FLACSamples(heads[i]))
			}
		}
		if d <= 0 && i+1 < len(fr) {
			d = max(0, fr[i+1].PTS*rate/1e9-cont)
		}
		if i == 0 || f.Lace == 0 && (cont-t > tol || t-cont > tol) {
			t = cont
		}
		out[i] = sample{pos: f.Pos, size: f.Size, dts: t, pts: t, dur: uint32(d), key: true}
		t += d
	}
	return out
}

// headsOf reads the first bytes of each frame (for codecs whose frame
// length is in each frame's header), going through the file in order with
// a large window instead of one read per frame.
func headsOf(r io.ReaderAt, fr []matroska.Frame, c *fmp4.Codec, prefix []byte) [][]byte {
	if !c.VariableFrame || len(fr) == 0 {
		return nil
	}
	const want = 16
	out := make([][]byte, len(fr))
	win := make([]byte, 1<<20)
	var wpos int64 = -1
	var wlen int
	for i, f := range fr {
		n := min(int(f.Size), want)
		h := make([]byte, 0, len(prefix)+n)
		h = append(h, prefix...)
		if wpos < 0 || f.Pos < wpos || f.Pos+int64(n) > wpos+int64(wlen) {
			wpos = f.Pos
			wlen, _ = r.ReadAt(win, f.Pos)
		}
		if off := f.Pos - wpos; off >= 0 && off+int64(n) <= int64(wlen) {
			h = append(h, win[off:off+int64(n)]...)
		}
		out[i] = h
	}
	return out
}

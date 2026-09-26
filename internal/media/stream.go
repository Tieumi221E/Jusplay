package media

import (
	"bytes"
	"compress/zlib"
	"context"
	"io"
	"math"
	"os"

	"github.com/Tieumi221E/Jusplay/internal/fmp4"
)

// Stream is one track streamed from a sample on: an init segment, then
// movie fragments until the end (or a sample limit).
type Stream struct {
	// Offset (seconds) maps stream time to original time (MSE timestampOffset).
	Offset float64
	// Start is the original presentation time of the first sample in decode
	// order (for video, a keyframe). Open-GOP leading pictures that present
	// earlier cannot be decoded after a seek; players drop anything
	// presented before Start (MSE appendWindowStart).
	Start float64
	// First is the index of that sample in the track.
	First int
	// AlignSamples is kept for the log line; times are exact, nothing is aligned.
	AlignSamples int

	m     *Media
	t     *Track
	ctx   context.Context
	limit int
}

// Fragment lengths: the first short, so the first picture needs little
// data; the rest a second.
const (
	firstFragment = 0.3
	fragment      = 1.0
	maxFragBytes  = 8 << 20
)

// OpenStream streams kind from keyframe k (or the file start when k is at
// or before the first keyframe).
func (m *Media) OpenStream(ctx context.Context, kind string, k float64) (*Stream, error) {
	return m.OpenStreamN(ctx, kind, k, 0)
}

// OpenStreamN is OpenStream with at most n samples (0: all), e.g. one
// keyframe for a thumbnail.
func (m *Media) OpenStreamN(ctx context.Context, kind string, k float64, n int) (*Stream, error) {
	t, err := m.track(kind)
	if err != nil {
		return nil, err
	}
	ts := float64(t.timescale)
	target := int64(math.Round(k * ts))
	j := 0
	for i, s := range t.samples {
		if s.pts > target+1 {
			if t.video {
				continue // B-frames present out of order; keep looking for keyframes
			}
			break
		}
		if !t.video || s.key {
			j = i
		}
	}
	// At or before the first keyframe: from the very start (audio with an
	// encoder delay begins at a negative time).
	if k <= m.Keyframes[0] {
		j = 0
	}
	if t.video && !t.samples[j].key {
		for j < len(t.samples)-1 && !t.samples[j].key {
			j++
		}
	}
	return &Stream{Offset: -float64(t.shift) / ts, Start: float64(t.samples[j].pts) / ts, First: j, m: m, t: t, ctx: ctx, limit: n}, nil
}

// WriteTo writes the stream: the init segment, then the fragments, each
// in one write (the HTTP layer flushes each write).
func (s *Stream) WriteTo(w io.Writer) (int64, error) {
	fh, err := os.Open(s.m.Path)
	if err != nil {
		return 0, err
	}
	defer fh.Close()
	t := s.t
	n, err := w.Write(t.init)
	total := int64(n)
	if err != nil {
		return total, err
	}
	end := len(t.samples)
	if s.limit > 0 {
		end = min(end, s.First+s.limit)
	}
	ts := float64(t.timescale)
	var data, frag []byte
	var smp []fmp4.Sample
	frame := make([]byte, 0, 1<<20)
	seq := uint32(1)
	for i := s.First; i < end; {
		if s.ctx != nil && s.ctx.Err() != nil {
			return total, s.ctx.Err()
		}
		target := int64(fragment * ts)
		if seq == 1 {
			target = int64(firstFragment * ts)
		}
		data, smp = data[:0], smp[:0]
		base := t.samples[i].dts
		var dur int64
		for ; i < end; i++ {
			x := t.samples[i]
			if len(smp) > 0 && (dur >= target || len(data) >= maxFragBytes || t.video && x.key && dur >= target/2) {
				break
			}
			b, err := s.read(fh, x, frame[:0])
			if err != nil {
				return total, err
			}
			frame = b[:0]
			data = append(data, b...)
			smp = append(smp, fmp4.Sample{Dur: x.dur, Size: uint32(len(b)), CTO: int32(x.pts - x.dts), Key: x.key})
			dur += int64(x.dur)
		}
		moof := fmp4.Moof(seq, uint64(base+t.shift), smp)
		frag = append(frag[:0], moof...)
		frag = append(frag, fmp4.MdatHeader(len(data))...)
		frag = append(frag, data...)
		n, err := w.Write(frag)
		total += int64(n)
		if err != nil {
			return total, err
		}
		seq++
	}
	return total, nil
}

// read reads one sample, undoing the track's content encoding.
func (s *Stream) read(fh *os.File, x sample, buf []byte) ([]byte, error) {
	t := s.t
	need := len(t.prefix) + int(x.size)
	if cap(buf) < need {
		buf = make([]byte, 0, need)
	}
	buf = append(buf[:0], t.prefix...)
	buf = buf[:need]
	if _, err := fh.ReadAt(buf[len(t.prefix):], x.pos); err != nil {
		return nil, err
	}
	if t.zlib {
		r, err := zlib.NewReader(bytes.NewReader(buf))
		if err != nil {
			return nil, err
		}
		var out bytes.Buffer
		if _, err := io.Copy(&out, io.LimitReader(r, 64<<20)); err != nil {
			return nil, err
		}
		return out.Bytes(), nil
	}
	return buf, nil
}

// Close ends the stream (WriteTo closes the file itself).
func (s *Stream) Close() {}

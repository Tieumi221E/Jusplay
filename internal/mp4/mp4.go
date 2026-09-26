// Package mp4 reads MP4 (ISO BMFF) files: the tracks, their decoder
// configuration, and every sample's position, size, decode and
// presentation time and sync flag, from the sample tables of a plain file
// or from the movie fragments of a fragmented one. Written for Jusplay.
package mp4

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
)

// Sample is one sample, in decode order.
type Sample struct {
	Pos  int64
	Size uint32
	DTS  int64 // track timescale
	PTS  int64 // track timescale, the edit list applied
	Key  bool
}

// Track is one track.
type Track struct {
	ID         uint32
	Handler    string // "vide", "soun", ...
	Format     string // sample entry: "avc1", "hvc1", "mp4a", "Opus", ...
	Config     map[string][]byte
	Width      int
	Height     int
	Channels   int
	SampleRate int
	Language   string
	Enabled    bool
	Timescale  uint32
	Duration   float64 // seconds
	Samples    []Sample
	Encrypted  bool
	editShift  int64 // media time of the first edit (subtracted)
	editDelay  int64 // empty edits, in the track timescale (added)
	trexDur    uint32
	trexSize   uint32
	trexFlags  uint32
}

// File is a parsed MP4.
type File struct {
	Timescale uint32
	Duration  float64
	Tracks    []*Track
}

type box struct {
	typ        string
	start, end int64 // box bounds in the file
	data       int64 // offset of the payload
}

func readBox(r io.ReaderAt, off, limit int64) (box, error) {
	var h [16]byte
	if _, err := r.ReadAt(h[:8], off); err != nil {
		return box{}, err
	}
	size := int64(binary.BigEndian.Uint32(h[:4]))
	b := box{typ: string(h[4:8]), start: off, data: off + 8}
	switch size {
	case 1:
		if _, err := r.ReadAt(h[8:16], off+8); err != nil {
			return box{}, err
		}
		size = int64(binary.BigEndian.Uint64(h[8:16]))
		b.data = off + 16
	case 0:
		size = limit - off
	}
	b.end = off + size
	if size < b.data-off || b.end > limit {
		return box{}, fmt.Errorf("box %q at %d overruns its parent", b.typ, off)
	}
	return b, nil
}

func children(r io.ReaderAt, start, end int64, visit func(box) error) error {
	for off := start; off+8 <= end; {
		b, err := readBox(r, off, end)
		if err != nil {
			return err
		}
		if err := visit(b); err != nil {
			return err
		}
		off = b.end
	}
	return nil
}

func payload(r io.ReaderAt, b box, max int64) ([]byte, error) {
	n := b.end - b.data
	if n > max {
		return nil, fmt.Errorf("box %q of %d bytes (limit %d)", b.typ, n, max)
	}
	p := make([]byte, n)
	_, err := r.ReadAt(p, b.data)
	return p, err
}

// Open reads path.
func Open(path string) (*File, io.ReaderAt, io.Closer, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, nil, nil, err
	}
	st, err := fh.Stat()
	if err != nil {
		fh.Close()
		return nil, nil, nil, err
	}
	f, err := Read(fh, st.Size())
	if err != nil {
		fh.Close()
		return nil, nil, nil, err
	}
	return f, fh, fh, nil
}

// Read parses r (size bytes).
func Read(r io.ReaderAt, size int64) (*File, error) {
	f := &File{}
	var moov box
	var moofs []box
	if err := children(r, 0, size, func(b box) error {
		switch b.typ {
		case "moov":
			moov = b
		case "moof":
			moofs = append(moofs, b)
		}
		return nil
	}); err != nil && moov.typ == "" {
		return nil, err
	}
	if moov.typ == "" {
		return nil, errors.New("not an MP4 file (no moov)")
	}
	if err := children(r, moov.data, moov.end, func(b box) error {
		switch b.typ {
		case "mvhd":
			p, err := payload(r, b, 256)
			if err != nil || len(p) < 20 {
				return errors.New("bad mvhd")
			}
			if p[0] == 1 {
				f.Timescale = binary.BigEndian.Uint32(p[20:])
				f.Duration = float64(binary.BigEndian.Uint64(p[24:])) / float64(f.Timescale)
			} else {
				f.Timescale = binary.BigEndian.Uint32(p[12:])
				f.Duration = float64(binary.BigEndian.Uint32(p[16:])) / float64(f.Timescale)
			}
		case "trak":
			t, err := readTrak(r, b, f.Timescale)
			if err != nil {
				return err
			}
			f.Tracks = append(f.Tracks, t)
		case "mvex":
			return children(r, b.data, b.end, func(x box) error {
				if x.typ != "trex" {
					return nil
				}
				p, err := payload(r, x, 64)
				if err != nil || len(p) < 24 {
					return errors.New("bad trex")
				}
				id := binary.BigEndian.Uint32(p[4:])
				for _, t := range f.Tracks {
					if t.ID == id {
						t.trexDur, t.trexSize, t.trexFlags = binary.BigEndian.Uint32(p[12:]), binary.BigEndian.Uint32(p[16:]), binary.BigEndian.Uint32(p[20:])
					}
				}
				return nil
			})
		}
		return nil
	}); err != nil {
		return nil, err
	}
	for _, m := range moofs {
		if err := f.readMoof(r, m); err != nil {
			return nil, err
		}
	}
	for _, t := range f.Tracks {
		if f.Timescale > 0 && t.Duration == 0 && len(moofs) == 0 {
			t.Duration = f.Duration
		}
		// The edit list moves the whole timeline (decode times too, so the
		// composition offsets stay what the file says).
		for i := range t.Samples {
			s := &t.Samples[i]
			s.PTS += t.editDelay - t.editShift
			s.DTS += t.editDelay - t.editShift
		}
		if len(moofs) > 0 && len(t.Samples) > 0 {
			last := t.Samples[len(t.Samples)-1]
			if d := float64(last.PTS) / float64(t.Timescale); d > t.Duration {
				t.Duration = d
			}
		}
	}
	return f, nil
}

func readTrak(r io.ReaderAt, trak box, movieScale uint32) (*Track, error) {
	t := &Track{Config: map[string][]byte{}, Enabled: true, Language: "und"}
	var stbl box
	var elst []byte
	err := children(r, trak.data, trak.end, func(b box) error {
		switch b.typ {
		case "tkhd":
			p, err := payload(r, b, 256)
			if err != nil || len(p) < 24 {
				return errors.New("bad tkhd")
			}
			t.Enabled = p[3]&1 != 0
			if p[0] == 1 {
				t.ID = binary.BigEndian.Uint32(p[20:])
			} else {
				t.ID = binary.BigEndian.Uint32(p[12:])
			}
		case "edts":
			return children(r, b.data, b.end, func(x box) error {
				if x.typ == "elst" {
					p, err := payload(r, x, 1<<16)
					elst = p
					return err
				}
				return nil
			})
		case "mdia":
			return children(r, b.data, b.end, func(x box) error {
				switch x.typ {
				case "mdhd":
					p, err := payload(r, x, 256)
					if err != nil || len(p) < 24 {
						return errors.New("bad mdhd")
					}
					var lang uint16
					if p[0] == 1 {
						t.Timescale = binary.BigEndian.Uint32(p[20:])
						t.Duration = float64(binary.BigEndian.Uint64(p[24:])) / float64(t.Timescale)
						lang = binary.BigEndian.Uint16(p[32:])
					} else {
						t.Timescale = binary.BigEndian.Uint32(p[12:])
						t.Duration = float64(binary.BigEndian.Uint32(p[16:])) / float64(t.Timescale)
						lang = binary.BigEndian.Uint16(p[20:])
					}
					if lang != 0 && lang != 0x7fff {
						t.Language = string([]byte{byte(lang>>10&31) + 0x60, byte(lang>>5&31) + 0x60, byte(lang&31) + 0x60})
					}
				case "hdlr":
					p, err := payload(r, x, 1<<12)
					if err != nil || len(p) < 12 {
						return errors.New("bad hdlr")
					}
					t.Handler = string(p[8:12])
				case "minf":
					return children(r, x.data, x.end, func(y box) error {
						if y.typ == "stbl" {
							stbl = y
						}
						return nil
					})
				}
				return nil
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if t.Timescale == 0 {
		return nil, fmt.Errorf("track %d without a timescale", t.ID)
	}
	if elst != nil {
		t.readEdits(elst, movieScale)
	}
	if stbl.typ == "" {
		return t, nil
	}
	return t, t.readStbl(r, stbl)
}

// readEdits takes what an edit list says about the start: empty edits
// delay the track; the first real edit's media time is where it starts.
func (t *Track) readEdits(p []byte, movieScale uint32) {
	if len(p) < 8 {
		return
	}
	v := p[0]
	n := int(binary.BigEndian.Uint32(p[4:]))
	q := p[8:]
	for i := 0; i < n; i++ {
		var dur uint64
		var mt int64
		if v == 1 {
			if len(q) < 20 {
				return
			}
			dur, mt = binary.BigEndian.Uint64(q), int64(binary.BigEndian.Uint64(q[8:]))
			q = q[20:]
		} else {
			if len(q) < 12 {
				return
			}
			dur, mt = uint64(binary.BigEndian.Uint32(q)), int64(int32(binary.BigEndian.Uint32(q[4:])))
			q = q[12:]
		}
		if mt == -1 {
			if movieScale > 0 {
				t.editDelay += int64(dur) * int64(t.Timescale) / int64(movieScale)
			}
			continue
		}
		t.editShift = mt
		return
	}
}

func (t *Track) readStbl(r io.ReaderAt, stbl box) error {
	var stts, ctts, stss, stsz, stz2, stsc, stco, co64 []byte
	err := children(r, stbl.data, stbl.end, func(b box) error {
		var err error
		switch b.typ {
		case "stsd":
			return t.readStsd(r, b)
		case "stts":
			stts, err = payload(r, b, 256<<20)
		case "ctts":
			ctts, err = payload(r, b, 256<<20)
		case "stss":
			stss, err = payload(r, b, 256<<20)
		case "stsz":
			stsz, err = payload(r, b, 256<<20)
		case "stz2":
			stz2, err = payload(r, b, 256<<20)
		case "stsc":
			stsc, err = payload(r, b, 256<<20)
		case "stco":
			stco, err = payload(r, b, 256<<20)
		case "co64":
			co64, err = payload(r, b, 256<<20)
		}
		return err
	})
	if err != nil {
		return err
	}
	u32 := binary.BigEndian.Uint32
	// sizes
	var sizes []uint32
	switch {
	case len(stsz) >= 12:
		fixed, n := u32(stsz[4:]), int(u32(stsz[8:]))
		if fixed != 0 {
			sizes = make([]uint32, n)
			for i := range sizes {
				sizes[i] = fixed
			}
		} else {
			if len(stsz) < 12+4*n {
				return errors.New("short stsz")
			}
			sizes = make([]uint32, n)
			for i := range sizes {
				sizes[i] = u32(stsz[12+4*i:])
			}
		}
	case len(stz2) >= 12:
		bits, n := int(stz2[7]), int(u32(stz2[8:]))
		sizes = make([]uint32, n)
		for i := range sizes {
			switch bits {
			case 4:
				v := stz2[12+i/2]
				if i%2 == 0 {
					sizes[i] = uint32(v >> 4)
				} else {
					sizes[i] = uint32(v & 15)
				}
			case 8:
				sizes[i] = uint32(stz2[12+i])
			case 16:
				sizes[i] = uint32(binary.BigEndian.Uint16(stz2[12+2*i:]))
			}
		}
	}
	n := len(sizes)
	if n == 0 {
		return nil // no samples here (fragmented file)
	}
	// chunk offsets
	var chunks []int64
	if len(stco) >= 8 {
		c := int(u32(stco[4:]))
		for i := 0; i < c && 8+4*i+4 <= len(stco); i++ {
			chunks = append(chunks, int64(u32(stco[8+4*i:])))
		}
	} else if len(co64) >= 8 {
		c := int(u32(co64[4:]))
		for i := 0; i < c && 8+8*i+8 <= len(co64); i++ {
			chunks = append(chunks, int64(binary.BigEndian.Uint64(co64[8+8*i:])))
		}
	}
	if len(stsc) < 8 || len(chunks) == 0 {
		return errors.New("sample table without chunks")
	}
	t.Samples = make([]Sample, n)
	// positions: stsc runs of samples per chunk
	entries := int(u32(stsc[4:]))
	si := 0
	for e := 0; e < entries && si < n; e++ {
		if 8+12*e+12 > len(stsc) {
			break
		}
		first := int(u32(stsc[8+12*e:])) - 1
		per := int(u32(stsc[8+12*e+4:]))
		last := len(chunks)
		if e+1 < entries && 8+12*(e+1)+4 <= len(stsc) {
			last = int(u32(stsc[8+12*(e+1):])) - 1
		}
		for c := first; c < last && c < len(chunks) && si < n; c++ {
			pos := chunks[c]
			for k := 0; k < per && si < n; k++ {
				t.Samples[si].Pos = pos
				t.Samples[si].Size = sizes[si]
				pos += int64(sizes[si])
				si++
			}
		}
	}
	if si < n {
		return fmt.Errorf("track %d: sample table places %d of %d samples", t.ID, si, n)
	}
	// decode times
	var dts int64
	si = 0
	if len(stts) >= 8 {
		for e := 0; e < int(u32(stts[4:])) && 8+8*e+8 <= len(stts); e++ {
			count, delta := int(u32(stts[8+8*e:])), int64(u32(stts[8+8*e+4:]))
			for k := 0; k < count && si < n; k++ {
				t.Samples[si].DTS = dts
				t.Samples[si].PTS = dts
				dts += delta
				si++
			}
		}
	}
	// composition offsets (version 1: signed)
	if len(ctts) >= 8 {
		si = 0
		for e := 0; e < int(u32(ctts[4:])) && 8+8*e+8 <= len(ctts); e++ {
			count := int(u32(ctts[8+8*e:]))
			off := int64(u32(ctts[8+8*e+4:]))
			if ctts[0] == 1 {
				off = int64(int32(uint32(off)))
			}
			for k := 0; k < count && si < n; k++ {
				t.Samples[si].PTS = t.Samples[si].DTS + off
				si++
			}
		}
	}
	// sync samples (none listed: every sample is one)
	if len(stss) >= 8 {
		for e := 0; e < int(u32(stss[4:])) && 8+4*e+4 <= len(stss); e++ {
			if i := int(u32(stss[8+4*e:])) - 1; i >= 0 && i < n {
				t.Samples[i].Key = true
			}
		}
	} else {
		for i := range t.Samples {
			t.Samples[i].Key = true
		}
	}
	return nil
}

// readStsd reads the first sample entry: its format, size or audio
// parameters and its configuration boxes.
func (t *Track) readStsd(r io.ReaderAt, b box) error {
	p, err := payload(r, b, 4<<20)
	if err != nil || len(p) < 16 {
		return errors.New("bad stsd")
	}
	size := int(binary.BigEndian.Uint32(p[8:]))
	if size < 16 || 8+size > len(p) {
		return errors.New("bad sample entry")
	}
	e := p[8 : 8+size]
	t.Format = string(e[4:8])
	body := e[8:]
	var kids []byte
	switch t.Handler {
	case "vide":
		if len(body) < 78 {
			return errors.New("short visual sample entry")
		}
		t.Width, t.Height = int(binary.BigEndian.Uint16(body[24:])), int(binary.BigEndian.Uint16(body[26:]))
		kids = body[78:]
	case "soun":
		if len(body) < 28 {
			return errors.New("short audio sample entry")
		}
		ver := binary.BigEndian.Uint16(body[8:])
		t.Channels = int(binary.BigEndian.Uint16(body[16:]))
		t.SampleRate = int(binary.BigEndian.Uint32(body[24:]) >> 16)
		off := 28
		switch ver {
		case 1:
			off += 16
		case 2:
			off += 36
		}
		if off > len(body) {
			return errors.New("short audio sample entry")
		}
		kids = body[off:]
	default:
		return nil
	}
	if t.Format == "encv" || t.Format == "enca" {
		t.Encrypted = true
	}
	for len(kids) >= 8 {
		n := int(binary.BigEndian.Uint32(kids))
		if n < 8 || n > len(kids) {
			break
		}
		t.Config[string(kids[4:8])] = append([]byte(nil), kids[8:n]...)
		kids = kids[n:]
	}
	return nil
}

// readMoof adds the samples of one movie fragment.
func (f *File) readMoof(r io.ReaderAt, moof box) error {
	return children(r, moof.data, moof.end, func(traf box) error {
		if traf.typ != "traf" {
			return nil
		}
		var t *Track
		var base int64 = moof.start
		var dur, size, flags uint32
		var dts int64 = -1
		return children(r, traf.data, traf.end, func(b box) error {
			p, err := payload(r, b, 64<<20)
			if err != nil || len(p) < 4 {
				return err
			}
			fl := uint32(p[1])<<16 | uint32(p[2])<<8 | uint32(p[3])
			q := p[4:]
			u32 := func() uint32 {
				if len(q) < 4 {
					return 0
				}
				v := binary.BigEndian.Uint32(q)
				q = q[4:]
				return v
			}
			switch b.typ {
			case "tfhd":
				id := u32()
				for _, x := range f.Tracks {
					if x.ID == id {
						t = x
					}
				}
				if t == nil {
					return nil
				}
				dur, size, flags = t.trexDur, t.trexSize, t.trexFlags
				if fl&1 != 0 {
					base = int64(u32())<<32 | int64(u32())
				}
				if fl&2 != 0 {
					u32()
				}
				if fl&8 != 0 {
					dur = u32()
				}
				if fl&0x10 != 0 {
					size = u32()
				}
				if fl&0x20 != 0 {
					flags = u32()
				}
			case "tfdt":
				if p[0] == 1 && len(q) >= 8 {
					dts = int64(binary.BigEndian.Uint64(q))
				} else {
					dts = int64(u32())
				}
			case "trun":
				if t == nil {
					return nil
				}
				if dts < 0 {
					dts = 0
					if n := len(t.Samples); n > 0 {
						dts = t.Samples[n-1].DTS + int64(dur)
					}
				}
				count := int(u32())
				pos := base
				if fl&1 != 0 {
					pos = base + int64(int32(u32()))
				}
				firstFlags, haveFirst := uint32(0), fl&4 != 0
				if haveFirst {
					firstFlags = u32()
				}
				for i := 0; i < count; i++ {
					s := Sample{Pos: pos, DTS: dts}
					d, sz, sf := dur, size, flags
					if fl&0x100 != 0 {
						d = u32()
					}
					if fl&0x200 != 0 {
						sz = u32()
					}
					if fl&0x400 != 0 {
						sf = u32()
					} else if i == 0 && haveFirst {
						sf = firstFlags
					}
					var cto int64
					if fl&0x800 != 0 {
						v := u32()
						if p[0] == 1 {
							cto = int64(int32(v))
						} else {
							cto = int64(v)
						}
					}
					s.Size = sz
					s.PTS = dts + cto
					s.Key = sf&0x00010000 == 0 // not "non-sync"
					t.Samples = append(t.Samples, s)
					pos += int64(sz)
					dts += int64(d)
				}
			}
			return nil
		})
	})
}

// SortedPTS returns a track's presentation times, sorted (for tests).
func (t *Track) SortedPTS() []int64 {
	out := make([]int64, len(t.Samples))
	for i, s := range t.Samples {
		out[i] = s.PTS
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

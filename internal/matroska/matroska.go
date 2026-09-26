// Package matroska reads Matroska and WebM files: the tracks, the
// attachments, and the position, size, time and key flag of every frame,
// without decoding anything. Written for Jusplay (internal/ebml underneath).
//
// Reading the frame index walks every cluster once, reading only element
// and block headers and skipping the frame data (a seek past anything
// larger than the read buffer).
package matroska

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sort"

	"github.com/Tieumi221E/Jusplay/internal/ebml"
)

// Element IDs used here.
const (
	idEBML        = 0x1A45DFA3
	idDocType     = 0x4282
	idSegment     = 0x18538067
	idSeekHead    = 0x114D9B74
	idSeek        = 0x4DBB
	idSeekID      = 0x53AB
	idSeekPos     = 0x53AC
	idInfo        = 0x1549A966
	idTimecodeSc  = 0x2AD7B1
	idDuration    = 0x4489
	idTracks      = 0x1654AE6B
	idTrackEntry  = 0xAE
	idTrackNumber = 0xD7
	idTrackUID    = 0x73C5
	idTrackType   = 0x83
	idFlagDefault = 0x88
	idFlagEnabled = 0xB9
	idDefaultDur  = 0x23E383
	idCodecID     = 0x86
	idCodecPriv   = 0x63A2
	idCodecDelay  = 0x56AA
	idLanguage    = 0x22B59C
	idLangBCP47   = 0x22B59D
	idName        = 0x536E
	idVideo       = 0xE0
	idPixelWidth  = 0xB0
	idPixelHeight = 0xBA
	idAudio       = 0xE1
	idSamplingF   = 0xB5
	idOutSamplF   = 0x78B5
	idChannels    = 0x9F
	idBitDepth    = 0x6264
	idContentEncs = 0x6D80
	idContentEnc  = 0x6240
	idContentComp = 0x5034
	idCompAlgo    = 0x4254
	idCompSetting = 0x4255
	idContentEncr = 0x5035
	idCluster     = 0x1F43B675
	idTimecode    = 0xE7
	idSimpleBlock = 0xA3
	idBlockGroup  = 0xA0
	idBlock       = 0xA1
	idBlockDur    = 0x9B
	idRefBlock    = 0xFB
	idCues        = 0x1C53BB6B
	idAttachments = 0x1941A469
	idAttached    = 0x61A7
	idFileDesc    = 0x467E
	idFileName    = 0x466E
	idFileMedia   = 0x4660
	idFileData    = 0x465C
	idFileUID     = 0x46AE
	idChapters    = 0x1043A770
	idTags        = 0x1254C367
	idVoid        = 0xEC
	idCRC32       = 0xBF
)

// level1 are the elements that can follow a cluster at the segment level
// (they end a cluster of unknown size).
var level1 = map[uint32]bool{idSeekHead: true, idInfo: true, idTracks: true, idCluster: true, idCues: true,
	idAttachments: true, idChapters: true, idTags: true}

// Track is one track entry.
type Track struct {
	Number       uint64
	UID          uint64
	Type         int // 1 video, 2 audio, 17 subtitle
	CodecID      string
	CodecPrivate []byte
	CodecDelay   uint64 // ns
	DefaultDur   uint64 // ns per frame, 0 if not set
	Default      bool
	Enabled      bool
	Language     string
	Name         string
	Width        int
	Height       int
	SampleRate   float64
	OutRate      float64
	Channels     int
	BitDepth     int
	// Frame content encodings: a prefix put back on every frame (header
	// stripping), or zlib; frames that are encrypted cannot be read.
	StripPrefix []byte
	Zlib        bool
	Encrypted   bool
}

// Attachment is an attached file (fonts, Jusplay's comment data, covers).
type Attachment struct {
	Name, MIME, Desc string
	UID              uint64
	Pos              int64 // of the file data
	Size             int64
}

// Frame is one frame of a track, in decode (file) order.
type Frame struct {
	Track uint64
	PTS   int64 // ns, from the block time (and lacing position)
	Dur   int64 // ns, from BlockDuration or DefaultDuration; 0 if unknown
	Pos   int64 // of the frame data in the file
	Size  uint32
	Key   bool
	Laced bool // one of several frames in a block
	Lace  int  // its place in the block: from the second on, the time is estimated
}

// File is what the headers say.
type File struct {
	DocType        string
	TimecodeScale  uint64 // ns per tick
	Duration       float64
	Tracks         []Track
	Attachments    []Attachment
	SegmentStart   int64 // offset of the segment's data
	SegmentEnd     int64 // offset just past the segment (file end for unknown size)
	FirstCluster   int64 // offset of the first cluster (0 if none)
	HasChapters    bool
	segSizeUnknown bool
	f              io.ReaderAt
	size           int64
}

// Open reads the headers of path. The returned File keeps the file open
// only through the returned closer.
func Open(path string) (*File, io.Closer, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	st, err := fh.Stat()
	if err != nil {
		fh.Close()
		return nil, nil, err
	}
	f, err := Read(fh, st.Size())
	if err != nil {
		fh.Close()
		return nil, nil, err
	}
	return f, fh, nil
}

// Read reads the headers from r (size bytes long).
func Read(r io.ReaderAt, size int64) (*File, error) {
	f := &File{f: r, size: size, TimecodeScale: 1000000, Duration: math.NaN()}
	h, err := ebml.ReadHeader(r, 0)
	if err != nil || h.ID != idEBML {
		return nil, errors.New("not a Matroska file (no EBML header)")
	}
	if err := f.children(h, func(e ebml.Element) error {
		if e.ID == idDocType {
			b, err := f.data(e, 64)
			f.DocType = ebml.String(b)
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if f.DocType != "matroska" && f.DocType != "webm" {
		return nil, fmt.Errorf("not a Matroska file (DocType %q)", f.DocType)
	}
	seg, err := ebml.ReadHeader(r, h.End())
	if err != nil || seg.ID != idSegment {
		return nil, errors.New("no Matroska segment")
	}
	f.SegmentStart = seg.DataPos
	f.SegmentEnd = seg.End()
	if seg.Size == ebml.UnknownSize || f.SegmentEnd > size {
		f.segSizeUnknown = seg.Size == ebml.UnknownSize
		f.SegmentEnd = size
	}
	seen := map[int64]bool{}
	var seekTargets []int64
	parse := func(e ebml.Element) error {
		if seen[e.Start] {
			return nil
		}
		seen[e.Start] = true
		switch e.ID {
		case idInfo:
			return f.readInfo(e)
		case idTracks:
			return f.readTracks(e)
		case idAttachments:
			return f.readAttachments(e)
		case idChapters:
			f.HasChapters = true
		case idSeekHead:
			return f.children(e, func(s ebml.Element) error {
				if s.ID != idSeek {
					return nil
				}
				var id uint32
				var pos int64 = -1
				if err := f.children(s, func(c ebml.Element) error {
					b, err := f.data(c, 16)
					switch c.ID {
					case idSeekID:
						id = uint32(ebml.Uint(b))
					case idSeekPos:
						pos = int64(ebml.Uint(b))
					}
					return err
				}); err != nil {
					return err
				}
				if pos >= 0 && (id == idInfo || id == idTracks || id == idAttachments || id == idChapters || id == idSeekHead) {
					seekTargets = append(seekTargets, f.SegmentStart+pos)
				}
				return nil
			})
		}
		return nil
	}
	// The elements before the first cluster, then the ones the seek heads
	// point to (attachments are often written after the clusters).
	for off := f.SegmentStart; off < f.SegmentEnd; {
		e, err := ebml.ReadHeader(r, off)
		if err != nil {
			break
		}
		if e.ID == idCluster {
			f.FirstCluster = e.Start
			break
		}
		if e.Size == ebml.UnknownSize {
			return nil, fmt.Errorf("element %X of unknown size before the clusters", e.ID)
		}
		if err := parse(e); err != nil {
			return nil, err
		}
		off = e.End()
	}
	for i := 0; i < len(seekTargets); i++ {
		e, err := ebml.ReadHeader(r, seekTargets[i])
		if err != nil || e.Size == ebml.UnknownSize || e.End() > size {
			continue // a stale seek entry: not fatal
		}
		if err := parse(e); err != nil {
			return nil, err
		}
	}
	if f.Tracks == nil {
		return nil, errors.New("Matroska file without tracks")
	}
	return f, nil
}

// data reads an element's data, refusing anything larger than max.
func (f *File) data(e ebml.Element, max int64) ([]byte, error) {
	if e.Size == ebml.UnknownSize || e.Size > max {
		return nil, fmt.Errorf("element %X of %d bytes (limit %d)", e.ID, e.Size, max)
	}
	b := make([]byte, e.Size)
	if _, err := f.f.ReadAt(b, e.DataPos); err != nil {
		return nil, err
	}
	return b, nil
}

// children visits the elements inside master m.
func (f *File) children(m ebml.Element, visit func(ebml.Element) error) error {
	end := m.End()
	if m.Size == ebml.UnknownSize {
		return fmt.Errorf("master %X of unknown size", m.ID)
	}
	for off := m.DataPos; off < end; {
		e, err := ebml.ReadHeader(f.f, off)
		if err != nil {
			return err
		}
		if e.Size == ebml.UnknownSize || e.End() > end {
			return fmt.Errorf("element %X at %d overruns its parent", e.ID, e.Start)
		}
		if err := visit(e); err != nil {
			return err
		}
		off = e.End()
	}
	return nil
}

func (f *File) readInfo(m ebml.Element) error {
	return f.children(m, func(e ebml.Element) error {
		switch e.ID {
		case idTimecodeSc:
			b, err := f.data(e, 8)
			if err == nil && ebml.Uint(b) > 0 {
				f.TimecodeScale = ebml.Uint(b)
			}
			return err
		case idDuration:
			b, err := f.data(e, 8)
			f.Duration = ebml.Float(b)
			return err
		}
		return nil
	})
}

func (f *File) readTracks(m ebml.Element) error {
	return f.children(m, func(e ebml.Element) error {
		if e.ID != idTrackEntry {
			return nil
		}
		t := Track{Enabled: true, Default: true, Language: "eng", SampleRate: 8000, Channels: 1}
		err := f.children(e, func(c ebml.Element) error {
			switch c.ID {
			case idVideo:
				return f.children(c, func(v ebml.Element) error {
					if v.ID != idPixelWidth && v.ID != idPixelHeight {
						return nil // colour, projection, ...: not needed
					}
					b, err := f.data(v, 8)
					if v.ID == idPixelWidth {
						t.Width = int(ebml.Uint(b))
					} else {
						t.Height = int(ebml.Uint(b))
					}
					return err
				})
			case idAudio:
				return f.children(c, func(a ebml.Element) error {
					if a.ID != idSamplingF && a.ID != idOutSamplF && a.ID != idChannels && a.ID != idBitDepth {
						return nil
					}
					b, err := f.data(a, 8)
					switch a.ID {
					case idSamplingF:
						t.SampleRate = ebml.Float(b)
					case idOutSamplF:
						t.OutRate = ebml.Float(b)
					case idChannels:
						t.Channels = int(ebml.Uint(b))
					case idBitDepth:
						t.BitDepth = int(ebml.Uint(b))
					}
					return err
				})
			case idContentEncs:
				return f.readEncodings(c, &t)
			case idCodecPriv:
				b, err := f.data(c, 16<<20)
				t.CodecPrivate = b
				return err
			}
			b, err := f.data(c, 1<<16)
			if err != nil {
				return nil // an unknown large child: skip it
			}
			switch c.ID {
			case idTrackNumber:
				t.Number = ebml.Uint(b)
			case idTrackUID:
				t.UID = ebml.Uint(b)
			case idTrackType:
				t.Type = int(ebml.Uint(b))
			case idFlagDefault:
				t.Default = ebml.Uint(b) != 0
			case idFlagEnabled:
				t.Enabled = ebml.Uint(b) != 0
			case idDefaultDur:
				t.DefaultDur = ebml.Uint(b)
			case idCodecID:
				t.CodecID = ebml.String(b)
			case idCodecDelay:
				t.CodecDelay = ebml.Uint(b)
			case idLanguage:
				t.Language = ebml.String(b)
			case idLangBCP47:
				t.Language = ebml.String(b)
			case idName:
				t.Name = ebml.String(b)
			}
			return nil
		})
		if err != nil {
			return err
		}
		if t.Number == 0 {
			return errors.New("track entry without a number")
		}
		f.Tracks = append(f.Tracks, t)
		return nil
	})
}

func (f *File) readEncodings(m ebml.Element, t *Track) error {
	return f.children(m, func(e ebml.Element) error {
		if e.ID != idContentEnc {
			return nil
		}
		return f.children(e, func(c ebml.Element) error {
			switch c.ID {
			case idContentEncr:
				t.Encrypted = true
			case idContentComp:
				algo := uint64(0)
				var settings []byte
				if err := f.children(c, func(x ebml.Element) error {
					b, err := f.data(x, 1<<16)
					switch x.ID {
					case idCompAlgo:
						algo = ebml.Uint(b)
					case idCompSetting:
						settings = b
					}
					return err
				}); err != nil {
					return err
				}
				switch algo {
				case 0:
					t.Zlib = true
				case 3:
					t.StripPrefix = settings
				default:
					return fmt.Errorf("track %d: content compression %d is not supported", t.Number, algo)
				}
			}
			return nil
		})
	})
}

func (f *File) readAttachments(m ebml.Element) error {
	f.Attachments = nil
	return f.children(m, func(e ebml.Element) error {
		if e.ID != idAttached {
			return nil
		}
		var a Attachment
		if err := f.children(e, func(c ebml.Element) error {
			if c.ID == idFileData {
				a.Pos, a.Size = c.DataPos, c.Size
				return nil
			}
			b, err := f.data(c, 1<<16)
			if err != nil {
				return nil
			}
			switch c.ID {
			case idFileName:
				a.Name = ebml.String(b)
			case idFileMedia:
				a.MIME = ebml.String(b)
			case idFileDesc:
				a.Desc = ebml.String(b)
			case idFileUID:
				a.UID = ebml.Uint(b)
			}
			return nil
		}); err != nil {
			return err
		}
		f.Attachments = append(f.Attachments, a)
		return nil
	})
}

// AttachmentData reads an attachment's bytes (at most max).
func (f *File) AttachmentData(a Attachment, max int64) ([]byte, error) {
	if a.Size > max {
		return nil, fmt.Errorf("attachment %s is %d bytes (limit %d)", a.Name, a.Size, max)
	}
	b := make([]byte, a.Size)
	_, err := f.f.ReadAt(b, a.Pos)
	return b, err
}

// Track returns the track with the given number.
func (f *File) Track(n uint64) *Track {
	for i := range f.Tracks {
		if f.Tracks[i].Number == n {
			return &f.Tracks[i]
		}
	}
	return nil
}

// ---- the frame index ----

// scanner reads a file forward with a buffer, skipping by seeking.
type scanner struct {
	r    io.ReaderAt
	size int64
	buf  []byte
	bpos int64 // file offset of buf[0]
	blen int
}

func (s *scanner) at(off int64, n int) ([]byte, error) {
	if off >= s.bpos && off+int64(n) <= s.bpos+int64(s.blen) {
		i := off - s.bpos
		return s.buf[i : i+int64(n)], nil
	}
	s.bpos = off
	m, err := s.r.ReadAt(s.buf, off)
	s.blen = m
	if m < n {
		if err == nil || err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return s.buf[:m], err
	}
	return s.buf[:n], nil
}

func (s *scanner) header(off int64) (ebml.Element, error) {
	n := 12
	if off+int64(n) > s.size {
		n = int(s.size - off)
	}
	if n <= 0 {
		return ebml.Element{}, io.EOF
	}
	b, err := s.at(off, n)
	if err != nil && len(b) == 0 {
		return ebml.Element{}, err
	}
	return ebml.ParseHeader(b, off)
}

// Frames reads every frame of the wanted tracks (all tracks when want is
// nil), in file order.
func (f *File) Frames(want map[uint64]bool) ([]Frame, error) {
	if f.FirstCluster == 0 {
		return nil, nil
	}
	sc := &scanner{r: f.f, size: f.size, buf: make([]byte, 256<<10)}
	var out []Frame
	scale := int64(f.TimecodeScale)
	for off := f.FirstCluster; off < f.SegmentEnd; {
		e, err := sc.header(off)
		if err != nil {
			break // a truncated file: keep what was read
		}
		if e.ID != idCluster {
			if e.Size == ebml.UnknownSize {
				break
			}
			off = e.End()
			continue
		}
		end := e.End()
		unknown := e.Size == ebml.UnknownSize
		if unknown || end > f.SegmentEnd {
			end = f.SegmentEnd
		}
		var clusterTC int64
		p := e.DataPos
		for p < end {
			c, err := sc.header(p)
			if err != nil {
				break
			}
			if unknown && level1[c.ID] {
				break // the next top-level element ends a cluster of unknown size
			}
			if c.Size == ebml.UnknownSize || c.End() > end {
				p = end
				break
			}
			switch c.ID {
			case idTimecode:
				b, err := sc.at(c.DataPos, int(min(c.Size, 8)))
				if err != nil {
					return out, err
				}
				clusterTC = int64(ebml.Uint(b))
			case idSimpleBlock:
				if out, err = f.block(sc, c, clusterTC, scale, want, out, true, -1, false); err != nil {
					return out, err
				}
			case idBlockGroup:
				var blk ebml.Element
				dur, ref := int64(-1), false
				for q := c.DataPos; q < c.End(); {
					g, err := sc.header(q)
					if err != nil || g.Size == ebml.UnknownSize || g.End() > c.End() {
						break
					}
					switch g.ID {
					case idBlock:
						blk = g
					case idBlockDur:
						b, _ := sc.at(g.DataPos, int(min(g.Size, 8)))
						dur = int64(ebml.Uint(b))
					case idRefBlock:
						ref = true
					}
					q = g.End()
				}
				if blk.ID == idBlock {
					if out, err = f.block(sc, blk, clusterTC, scale, want, out, false, dur, !ref); err != nil {
						return out, err
					}
				}
			}
			p = c.End()
		}
		off = p
		if !unknown {
			off = max(e.End(), p)
		}
	}
	return out, nil
}

// block reads one block's header and lacing, appending its frames.
func (f *File) block(sc *scanner, e ebml.Element, clusterTC, scale int64, want map[uint64]bool, out []Frame, simple bool, dur int64, groupKey bool) ([]Frame, error) {
	n := int(min(e.Size, 64))
	b, err := sc.at(e.DataPos, n)
	if err != nil {
		return out, err
	}
	track, tl, err := ebml.Vint(b)
	if err != nil || len(b) < tl+3 {
		return out, fmt.Errorf("bad block at %d", e.Start)
	}
	if want != nil && !want[track] {
		return out, nil
	}
	t := f.Track(track)
	rel := int64(int16(uint16(b[tl])<<8 | uint16(b[tl+1])))
	flags := b[tl+2]
	key := groupKey
	if simple {
		key = flags&0x80 != 0
	}
	// The presentation time: the block's, less the codec delay (the
	// encoder's priming samples, which play before time 0).
	pts := (clusterTC + rel) * scale
	if t != nil {
		pts -= int64(t.CodecDelay)
	}
	var frameDur int64
	if dur >= 0 {
		frameDur = dur * scale
	} else if t != nil {
		frameDur = int64(t.DefaultDur)
	}
	hdr := int64(tl + 3)
	lacing := (flags >> 1) & 3
	if lacing == 0 {
		return append(out, Frame{Track: track, PTS: pts, Dur: frameDur, Pos: e.DataPos + hdr, Size: uint32(e.Size - hdr), Key: key}), nil
	}
	// Laced: several frames in one block.
	full, err := sc.at(e.DataPos, int(min(e.Size, 64<<10)))
	if err != nil && len(full) < int(hdr)+1 {
		return out, err
	}
	count := int(full[hdr]) + 1
	p := int(hdr) + 1
	sizes := make([]int64, count)
	total := e.Size - hdr - 1
	switch lacing {
	case 1: // Xiph
		for i := 0; i < count-1; i++ {
			var s int64
			for {
				if p >= len(full) {
					return out, fmt.Errorf("bad Xiph lacing at %d", e.Start)
				}
				v := full[p]
				p++
				s += int64(v)
				if v != 255 {
					break
				}
			}
			sizes[i] = s
		}
	case 3: // EBML
		v, l, err := ebml.Vint(full[p:])
		if err != nil {
			return out, fmt.Errorf("bad EBML lacing at %d", e.Start)
		}
		sizes[0] = int64(v)
		p += l
		for i := 1; i < count-1; i++ {
			d, l, err := ebml.SignedVint(full[p:])
			if err != nil {
				return out, fmt.Errorf("bad EBML lacing at %d", e.Start)
			}
			sizes[i] = sizes[i-1] + d
			p += l
		}
	case 2: // fixed
		each := total / int64(count)
		for i := range sizes {
			sizes[i] = each
		}
	}
	if lacing != 2 {
		used := int64(p) - hdr - 1
		var sum int64
		for i := 0; i < count-1; i++ {
			sum += sizes[i]
		}
		sizes[count-1] = total - used - sum
	}
	pos := e.DataPos + int64(p)
	if lacing == 2 {
		pos = e.DataPos + hdr + 1
	}
	var each int64
	if frameDur > 0 {
		each = frameDur
		if dur >= 0 {
			each = frameDur / int64(count)
		}
	}
	for i, s := range sizes {
		if s < 0 {
			return out, fmt.Errorf("bad lacing at %d", e.Start)
		}
		out = append(out, Frame{Track: track, PTS: pts + int64(i)*each, Dur: each, Pos: pos, Size: uint32(s), Key: key, Laced: true, Lace: i})
		pos += s
	}
	return out, nil
}

// FrameData reads one frame, undoing the track's content encoding.
func (f *File) FrameData(t *Track, fr Frame, buf []byte) ([]byte, error) {
	if t.Encrypted {
		return nil, errors.New("encrypted track")
	}
	n := len(t.StripPrefix)
	if cap(buf) < n+int(fr.Size) {
		buf = make([]byte, n+int(fr.Size))
	}
	buf = buf[:n+int(fr.Size)]
	copy(buf, t.StripPrefix)
	if _, err := f.f.ReadAt(buf[n:], fr.Pos); err != nil {
		return nil, err
	}
	if t.Zlib {
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

// SortByPTS is a helper for tests: frame PTS values of a track, sorted.
func SortByPTS(fr []Frame) []int64 {
	out := make([]int64, len(fr))
	for i, x := range fr {
		out[i] = x.PTS
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

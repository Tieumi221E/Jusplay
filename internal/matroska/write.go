package matroska

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"

	"github.com/Tieumi221E/Jusplay/internal/ebml"
)

// NewFile is an attachment to write.
type NewFile struct {
	Name, MIME, Desc string
	Data             []byte
}

// The rewrite never moves a cluster. A file is three regions:
//
//	A: from the segment's start to the first cluster (seek head, info,
//	   tracks, tags, ...): rebuilt, exactly as long as before (Void fills
//	   what is left), without the old attachments;
//	B: the clusters: copied byte for byte, so every position the cues and
//	   seek heads hold into it stays right;
//	C: after the last cluster (cues, tags, ...): the elements kept in
//	   order, the old attachments left out, the new attachments last.
//
// The seek head (the first one) is rewritten with every element's new
// position; its CRC-32, if it has one, recomputed.

type element struct {
	e    ebml.Element
	keep bool // copied as it is (false: dropped or rebuilt)
}

type layout struct {
	segHeader []byte // EBML header + segment ID + size field, as in the file
	sizeAt    int    // offset of the segment size field in segHeader
	sizeWidth int
	unknown   bool // the segment's size was not written
	a         []ebml.Element
	bStart    int64
	bEnd      int64
	c         []ebml.Element
}

func (f *File) layout() (*layout, error) {
	if f.FirstCluster == 0 {
		return nil, errors.New("no clusters")
	}
	l := &layout{bStart: f.FirstCluster}
	h, _ := ebml.ReadHeader(f.f, 0)
	seg, err := ebml.ReadHeader(f.f, h.End())
	if err != nil {
		return nil, err
	}
	l.segHeader = make([]byte, seg.DataPos)
	if _, err := f.f.ReadAt(l.segHeader, 0); err != nil {
		return nil, err
	}
	l.sizeAt = int(seg.Start) + 4
	l.sizeWidth = int(seg.DataPos) - l.sizeAt
	l.unknown = f.segSizeUnknown
	for off := f.SegmentStart; off < f.FirstCluster; {
		e, err := ebml.ReadHeader(f.f, off)
		if err != nil {
			return nil, err
		}
		l.a = append(l.a, e)
		off = e.End()
	}
	// The clusters, to the end of the last one (a cluster of unknown size
	// ends at the next top-level element).
	sc := &scanner{r: f.f, size: f.size, buf: make([]byte, 64<<10)}
	off := f.FirstCluster
	l.bEnd = off
	for off < f.SegmentEnd {
		e, err := sc.header(off)
		if err != nil {
			break
		}
		if e.ID != idCluster {
			if e.Size == ebml.UnknownSize {
				return nil, fmt.Errorf("element %X of unknown size after the clusters", e.ID)
			}
			l.c = append(l.c, e)
			off = e.End()
			continue
		}
		if len(l.c) > 0 {
			return nil, errors.New("clusters after other top-level elements: not supported")
		}
		if e.Size != ebml.UnknownSize {
			off = e.End()
			l.bEnd = off
			continue
		}
		p := e.DataPos
		for p < f.SegmentEnd {
			c, err := sc.header(p)
			if err != nil || level1[c.ID] || c.Size == ebml.UnknownSize {
				break
			}
			p = c.End()
		}
		off, l.bEnd = p, p
	}
	if l.bEnd > f.size {
		return nil, errors.New("truncated file")
	}
	return l, nil
}

// WriteAttachments writes the file with its attachments replaced: keep
// decides which existing ones stay, add are appended. The output goes to
// out; it is verified with Verify afterwards by the caller.
func (f *File) WriteAttachments(out io.Writer, keep func(Attachment) bool, add []NewFile) error {
	l, err := f.layout()
	if err != nil {
		return err
	}
	// The new attachments element: the kept files, then the added ones.
	var files []byte
	for _, a := range f.Attachments {
		if !keep(a) {
			continue
		}
		data, err := f.AttachmentData(a, 1<<30)
		if err != nil {
			return err
		}
		files = appendFile(files, a.Name, a.MIME, a.Desc, a.UID, data)
	}
	for _, n := range add {
		files = appendFile(files, n.Name, n.MIME, n.Desc, newUID(), n.Data)
	}
	var atts []byte
	if len(files) > 0 {
		atts = ebml.AppendElement(nil, idAttachments, files)
	}

	// Region C: its elements in order, the old attachments left out.
	newPos := map[int64]int64{} // old element start -> new start, relative to the segment data
	segData := f.SegmentStart
	cPos := l.bEnd
	var cParts []ebml.Element
	for _, e := range l.c {
		// Old attachments go; so do other seek heads (the new first one
		// lists every element, and theirs would point at old places).
		if e.ID == idAttachments || e.ID == idSeekHead {
			continue
		}
		newPos[e.Start] = cPos - segData
		cParts = append(cParts, e)
		cPos += e.End() - e.Start
	}
	attsPos := cPos - segData
	cEnd := cPos + int64(len(atts))

	// Region A: the elements kept, a new seek head first, Void after.
	aLen := l.bStart - f.SegmentStart
	var keptA []ebml.Element
	var oldSeek *ebml.Element
	for i, e := range l.a {
		switch e.ID {
		case idSeekHead:
			if oldSeek == nil {
				oldSeek = &l.a[i]
			}
			continue
		case idVoid, idAttachments:
			continue
		}
		keptA = append(keptA, e)
	}
	crc := false
	if oldSeek != nil {
		if h, err := ebml.ReadHeader(f.f, oldSeek.DataPos); err == nil && h.ID == idCRC32 {
			crc = true
		}
	}
	// Positions in region A depend on the seek head's size, which depends on
	// the positions: repeat until its size no longer changes (then the
	// positions it holds were computed with its own size).
	var seek []byte
	for pass := 0; ; pass++ {
		if pass == 8 {
			return errors.New("the seek head's size does not settle")
		}
		p := int64(len(seek))
		for _, e := range keptA {
			newPos[e.Start] = p
			p += e.End() - e.Start
		}
		var entries []byte
		add := func(id uint32, pos int64) {
			var s []byte
			s = ebml.AppendElement(s, idSeekID, ebml.AppendID(nil, id))
			s = ebml.AppendUint(s, idSeekPos, uint64(pos))
			entries = ebml.AppendElement(entries, idSeek, s)
		}
		for _, e := range keptA {
			if e.ID != idSeekHead {
				add(e.ID, newPos[e.Start])
			}
		}
		for _, e := range cParts {
			add(e.ID, newPos[e.Start])
		}
		if atts != nil {
			add(idAttachments, attsPos)
		}
		body := entries
		if crc {
			sum := crc32.ChecksumIEEE(entries)
			body = append(ebml.AppendElement(nil, idCRC32, binary.LittleEndian.AppendUint32(nil, sum)), entries...)
		}
		next := ebml.AppendElement(nil, idSeekHead, body)
		stable := len(next) == len(seek)
		seek = next
		if stable {
			break
		}
	}
	used := int64(len(seek))
	for _, e := range keptA {
		used += e.End() - e.Start
	}
	pad := aLen - used
	if pad == 1 || pad < 0 {
		return fmt.Errorf("no room before the clusters for the new seek head (%d bytes short)", -pad)
	}

	// The segment's new size.
	head := append([]byte(nil), l.segHeader...)
	if !l.unknown {
		size := uint64(cEnd - segData)
		if size >= uint64(1)<<(7*l.sizeWidth)-1 {
			return errors.New("the segment size field is too short for the new size")
		}
		sz := ebml.AppendSize(nil, size, l.sizeWidth)
		copy(head[l.sizeAt:], sz)
	}

	bw := &countWriter{w: out}
	bw.Write(head)
	bw.Write(seek)
	for _, e := range keptA {
		if err := copyRange(bw, f.f, e.Start, e.End()); err != nil {
			return err
		}
	}
	if pad > 0 {
		v, err := ebml.Void(pad)
		if err != nil {
			return err
		}
		bw.Write(v)
	}
	if err := copyRange(bw, f.f, l.bStart, l.bEnd); err != nil {
		return err
	}
	for _, e := range cParts {
		if err := copyRange(bw, f.f, e.Start, e.End()); err != nil {
			return err
		}
	}
	bw.Write(atts)
	if bw.err != nil {
		return bw.err
	}
	if bw.n != cEnd {
		return fmt.Errorf("wrote %d bytes, expected %d", bw.n, cEnd)
	}
	return nil
}

func appendFile(b []byte, name, mime, desc string, uid uint64, data []byte) []byte {
	var body []byte
	if desc != "" {
		body = ebml.AppendElement(body, idFileDesc, []byte(desc))
	}
	body = ebml.AppendElement(body, idFileName, []byte(name))
	body = ebml.AppendElement(body, idFileMedia, []byte(mime))
	body = ebml.AppendElement(body, idFileData, data)
	body = ebml.AppendUint(body, idFileUID, uid)
	return ebml.AppendElement(b, idAttached, body)
}

func newUID() uint64 {
	var b [8]byte
	for {
		rand.Read(b[:])
		if v := binary.BigEndian.Uint64(b[:]); v != 0 {
			return v
		}
	}
}

type countWriter struct {
	w   io.Writer
	n   int64
	err error
}

func (c *countWriter) Write(p []byte) (int, error) {
	if c.err != nil {
		return 0, c.err
	}
	n, err := c.w.Write(p)
	c.n += int64(n)
	c.err = err
	return n, err
}

func copyRange(w io.Writer, r io.ReaderAt, start, end int64) error {
	_, err := io.CopyBuffer(w, io.NewSectionReader(r, start, end-start), make([]byte, 1<<20))
	return err
}

// ---- verification ----

// Verification records what was compared between a file and its rewrite.
type Verification struct {
	MediaStreams int   `json:"mediaStreams"`
	Packets      int   `json:"packets"`
	Chapters     int   `json:"chapters"`
	Attachments  int   `json:"attachments"`
	ClusterBytes int64 `json:"clusterBytes"`
}

// Verify checks that out is in with the attachments replaced as asked:
// the clusters byte for byte (hashed), every frame's track, time, size and
// key flag, the tracks, the other top-level elements' bytes, the kept
// attachments unchanged and the added ones exactly as given.
func Verify(inPath, outPath string, keep func(Attachment) bool, add []NewFile) (*Verification, error) {
	a, ca, err := Open(inPath)
	if err != nil {
		return nil, err
	}
	defer ca.Close()
	b, cb, err := Open(outPath)
	if err != nil {
		return nil, err
	}
	defer cb.Close()
	v := &Verification{MediaStreams: len(a.Tracks)}
	var errs []error
	// Tracks.
	if len(a.Tracks) != len(b.Tracks) {
		errs = append(errs, fmt.Errorf("tracks: %d in input, %d in output", len(a.Tracks), len(b.Tracks)))
	} else {
		for i := range a.Tracks {
			x, y := a.Tracks[i], b.Tracks[i]
			if x.Number != y.Number || x.Type != y.Type || x.CodecID != y.CodecID || !bytes.Equal(x.CodecPrivate, y.CodecPrivate) ||
				x.Language != y.Language || x.Default != y.Default || x.Enabled != y.Enabled || x.DefaultDur != y.DefaultDur {
				errs = append(errs, fmt.Errorf("track %d differs", x.Number))
			}
		}
	}
	// Clusters: same place, same bytes.
	la, err1 := a.layout()
	lb, err2 := b.layout()
	if err1 != nil || err2 != nil {
		return nil, errors.Join(err1, err2)
	}
	if la.bStart != lb.bStart || la.bEnd-la.bStart != lb.bEnd-lb.bStart {
		errs = append(errs, fmt.Errorf("clusters moved: %d-%d -> %d-%d", la.bStart, la.bEnd, lb.bStart, lb.bEnd))
	} else {
		ha, e1 := hashRange(a.f, la.bStart, la.bEnd)
		hb, e2 := hashRange(b.f, lb.bStart, lb.bEnd)
		if e1 != nil || e2 != nil || ha != hb {
			errs = append(errs, errors.New("cluster bytes differ"))
		}
		v.ClusterBytes = la.bEnd - la.bStart
	}
	// Every seek head entry of the output points at an element of its kind.
	if err := b.checkSeekHeads(); err != nil {
		errs = append(errs, err)
	}
	// Frames.
	fa, e1 := a.Frames(nil)
	fb, e2 := b.Frames(nil)
	if e1 != nil || e2 != nil {
		errs = append(errs, errors.Join(e1, e2))
	}
	v.Packets = len(fa)
	if len(fa) != len(fb) {
		errs = append(errs, fmt.Errorf("frames: %d in input, %d in output", len(fa), len(fb)))
	} else {
		for i := range fa {
			x, y := fa[i], fb[i]
			if x.Track != y.Track || x.PTS != y.PTS || x.Size != y.Size || x.Key != y.Key || x.Pos != y.Pos {
				errs = append(errs, fmt.Errorf("frame %d differs", i))
				break
			}
		}
	}
	// Other top-level elements: the same bytes (seek heads and voids aside).
	other := func(f *File, l *layout) (map[uint32][][]byte, error) {
		m := map[uint32][][]byte{}
		for _, e := range append(append([]ebml.Element{}, l.a...), l.c...) {
			if e.ID == idSeekHead || e.ID == idVoid || e.ID == idAttachments {
				continue
			}
			d := make([]byte, e.End()-e.Start)
			if _, err := f.f.ReadAt(d, e.Start); err != nil {
				return nil, err
			}
			m[e.ID] = append(m[e.ID], d)
		}
		return m, nil
	}
	oa, e1 := other(a, la)
	ob, e2 := other(b, lb)
	if e1 != nil || e2 != nil {
		errs = append(errs, errors.Join(e1, e2))
	}
	for id, xs := range oa {
		ys := ob[id]
		if len(xs) != len(ys) {
			errs = append(errs, fmt.Errorf("element %X: %d in input, %d in output", id, len(xs), len(ys)))
			continue
		}
		for i := range xs {
			if !bytes.Equal(xs[i], ys[i]) {
				errs = append(errs, fmt.Errorf("element %X differs", id))
			}
		}
		if id == idChapters {
			v.Chapters = countChapters(xs[0])
		}
	}
	// Attachments.
	var want []NewFile
	for _, x := range a.Attachments {
		if keep(x) {
			d, err := a.AttachmentData(x, 1<<30)
			if err != nil {
				return nil, err
			}
			want = append(want, NewFile{Name: x.Name, MIME: x.MIME, Desc: x.Desc, Data: d})
		}
	}
	want = append(want, add...)
	if len(want) != len(b.Attachments) {
		errs = append(errs, fmt.Errorf("attachments: %d expected, %d in output", len(want), len(b.Attachments)))
	} else {
		for i, w := range want {
			y := b.Attachments[i]
			d, err := b.AttachmentData(y, 1<<30)
			if err != nil || y.Name != w.Name || y.MIME != w.MIME || !bytes.Equal(d, w.Data) {
				errs = append(errs, fmt.Errorf("attachment %s differs", w.Name))
			}
		}
		v.Attachments = len(add)
	}
	return v, errors.Join(errs...)
}

// checkSeekHeads reads every seek head before the clusters and checks
// that each entry's position holds an element with the entry's ID.
func (f *File) checkSeekHeads() error {
	for off := f.SegmentStart; off < f.FirstCluster; {
		e, err := ebml.ReadHeader(f.f, off)
		if err != nil {
			return err
		}
		if e.ID == idSeekHead {
			if err := f.children(e, func(s ebml.Element) error {
				if s.ID != idSeek {
					return nil
				}
				var id uint32
				var pos int64 = -1
				if err := f.children(s, func(c ebml.Element) error {
					d, err := f.data(c, 16)
					if c.ID == idSeekID {
						id = uint32(ebml.Uint(d))
					} else if c.ID == idSeekPos {
						pos = int64(ebml.Uint(d))
					}
					return err
				}); err != nil {
					return err
				}
				at, err := ebml.ReadHeader(f.f, f.SegmentStart+pos)
				if err != nil || at.ID != id {
					return fmt.Errorf("seek entry for %X at %d finds %X", id, pos, at.ID)
				}
				return nil
			}); err != nil {
				return err
			}
		}
		off = e.End()
	}
	return nil
}

func hashRange(r io.ReaderAt, start, end int64) ([32]byte, error) {
	h := sha256.New()
	if _, err := io.CopyBuffer(h, io.NewSectionReader(r, start, end-start), make([]byte, 1<<20)); err != nil {
		return [32]byte{}, err
	}
	var s [32]byte
	copy(s[:], h.Sum(nil))
	return s, nil
}

// countChapters counts ChapterAtom elements at any depth of a Chapters
// element's bytes.
func countChapters(b []byte) int {
	const idAtom = 0xB6
	n := 0
	var walk func(b []byte, depth int)
	walk = func(b []byte, depth int) {
		for len(b) > 0 && depth < 8 {
			e, err := ebml.ParseHeader(b, 0)
			if err != nil || e.Size == ebml.UnknownSize || e.End() > int64(len(b)) {
				return
			}
			switch e.ID {
			case idChapters, 0x45B9: // Chapters, EditionEntry
				walk(b[e.DataPos:e.End()], depth+1)
			case idAtom:
				n++
				walk(b[e.DataPos:e.End()], depth+1)
			}
			b = b[e.End():]
		}
	}
	walk(b, 0)
	return n
}

// WriteFile writes the rewrite to path (which must not exist).
func (f *File) WriteFile(path string, keep func(Attachment) bool, add []NewFile) error {
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if err := f.WriteAttachments(out, keep, add); err != nil {
		out.Close()
		os.Remove(path)
		return err
	}
	return out.Close()
}

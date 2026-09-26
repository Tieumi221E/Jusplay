// Package fmp4 writes fragmented MP4 for Media Source Extensions: an init
// segment (ftyp + moov with one track) and movie fragments (moof + mdat).
// Written for Jusplay; it only packages samples, it never decodes.
package fmp4

import (
	"encoding/binary"
)

// Track describes the one track of a stream.
type Track struct {
	Video      bool
	Timescale  uint32
	Width      int
	Height     int
	Channels   int
	SampleRate int
	Language   string // ISO 639-2, "und" if unknown
	// Entry is the sample entry box (stsd's child): build it with
	// VisualEntry or AudioEntry.
	Entry []byte
}

// Sample is one sample of a fragment.
type Sample struct {
	Dur  uint32
	Size uint32
	CTO  int32 // presentation time - decode time
	Key  bool
}

type w struct{ b []byte }

func (x *w) u8(v byte)    { x.b = append(x.b, v) }
func (x *w) u16(v uint16) { x.b = binary.BigEndian.AppendUint16(x.b, v) }
func (x *w) u32(v uint32) { x.b = binary.BigEndian.AppendUint32(x.b, v) }
func (x *w) u64(v uint64) { x.b = binary.BigEndian.AppendUint64(x.b, v) }
func (x *w) str(s string) { x.b = append(x.b, s...) }
func (x *w) raw(p []byte) { x.b = append(x.b, p...) }
func (x *w) zeros(n int) {
	for i := 0; i < n; i++ {
		x.b = append(x.b, 0)
	}
}

// open starts a box and returns its offset; close writes its size.
func (x *w) open(typ string) int {
	o := len(x.b)
	x.u32(0)
	x.str(typ)
	return o
}
func (x *w) full(typ string, version byte, flags uint32) int {
	o := x.open(typ)
	x.u32(uint32(version)<<24 | flags)
	return o
}
func (x *w) close(o int) { binary.BigEndian.PutUint32(x.b[o:], uint32(len(x.b)-o)) }

// Box returns a box with the given payload.
func Box(typ string, payload []byte) []byte {
	x := &w{}
	o := x.open(typ)
	x.raw(payload)
	x.close(o)
	return x.b
}

// FullBox returns a full box (version and flags) with the given payload.
func FullBox(typ string, version byte, flags uint32, payload []byte) []byte {
	x := &w{}
	o := x.full(typ, version, flags)
	x.raw(payload)
	x.close(o)
	return x.b
}

// VisualEntry is a visual sample entry ("avc1", "hvc1", "av01", "vp09")
// with its configuration boxes.
func VisualEntry(format string, width, height int, config ...[]byte) []byte {
	x := &w{}
	o := x.open(format)
	x.zeros(6)
	x.u16(1) // data reference index
	x.zeros(16)
	x.u16(uint16(width))
	x.u16(uint16(height))
	x.u32(0x00480000) // 72 dpi
	x.u32(0x00480000)
	x.u32(0)
	x.u16(1) // frame count
	x.zeros(32)
	x.u16(0x0018)
	x.u16(0xffff)
	for _, c := range config {
		x.raw(c)
	}
	x.close(o)
	return x.b
}

// AudioEntry is an audio sample entry ("mp4a", "Opus", "fLaC", "ac-3",
// "ec-3") with its configuration boxes.
func AudioEntry(format string, channels, rate int, config ...[]byte) []byte {
	return AudioEntryBits(format, channels, rate, 16, config...)
}

// AudioEntryBits is AudioEntry with the sample size given (FLAC: Chromium
// requires it to equal STREAMINFO's).
func AudioEntryBits(format string, channels, rate, bits int, config ...[]byte) []byte {
	x := &w{}
	o := x.open(format)
	x.zeros(6)
	x.u16(1)
	x.zeros(8)
	x.u16(uint16(channels))
	x.u16(uint16(bits))
	x.u32(0)
	if rate > 65535 {
		rate = 0 // too large for 16.16; the configuration box carries it
	}
	x.u32(uint32(rate) << 16)
	for _, c := range config {
		x.raw(c)
	}
	x.close(o)
	return x.b
}

// Init returns the init segment: ftyp and a moov with the one track (ID 1).
func Init(t Track) []byte {
	x := &w{}
	f := x.open("ftyp")
	x.str("isom")
	x.u32(0x200)
	x.str("isomiso6mp41")
	x.close(f)

	moov := x.open("moov")
	mvhd := x.full("mvhd", 0, 0)
	x.u32(0)
	x.u32(0)
	x.u32(1000)
	x.u32(0) // duration: fragmented
	x.u32(0x00010000)
	x.u16(0x0100)
	x.zeros(10)
	matrix(x)
	x.zeros(24)
	x.u32(2) // next track ID
	x.close(mvhd)

	trak := x.open("trak")
	tkhd := x.full("tkhd", 0, 3)
	x.u32(0)
	x.u32(0)
	x.u32(1) // track ID
	x.u32(0)
	x.u32(0) // duration
	x.zeros(8)
	x.u16(0)
	x.u16(0)
	if t.Video {
		x.u16(0)
	} else {
		x.u16(0x0100)
	}
	x.u16(0)
	matrix(x)
	if t.Video {
		x.u32(uint32(t.Width) << 16)
		x.u32(uint32(t.Height) << 16)
	} else {
		x.u32(0)
		x.u32(0)
	}
	x.close(tkhd)

	mdia := x.open("mdia")
	mdhd := x.full("mdhd", 0, 0)
	x.u32(0)
	x.u32(0)
	x.u32(t.Timescale)
	x.u32(0)
	x.u16(packLang(t.Language))
	x.u16(0)
	x.close(mdhd)
	hdlr := x.full("hdlr", 0, 0)
	x.u32(0)
	if t.Video {
		x.str("vide")
	} else {
		x.str("soun")
	}
	x.zeros(12)
	x.str("jusplay\x00")
	x.close(hdlr)
	minf := x.open("minf")
	if t.Video {
		v := x.full("vmhd", 0, 1)
		x.zeros(8)
		x.close(v)
	} else {
		s := x.full("smhd", 0, 0)
		x.zeros(4)
		x.close(s)
	}
	dinf := x.open("dinf")
	dref := x.full("dref", 0, 0)
	x.u32(1)
	u := x.full("url ", 0, 1)
	x.close(u)
	x.close(dref)
	x.close(dinf)
	stbl := x.open("stbl")
	stsd := x.full("stsd", 0, 0)
	x.u32(1)
	x.raw(t.Entry)
	x.close(stsd)
	for _, typ := range []string{"stts", "stsc", "stco"} {
		b := x.full(typ, 0, 0)
		x.u32(0)
		x.close(b)
	}
	stsz := x.full("stsz", 0, 0)
	x.u32(0)
	x.u32(0)
	x.close(stsz)
	x.close(stbl)
	x.close(minf)
	x.close(mdia)
	x.close(trak)

	mvex := x.open("mvex")
	trex := x.full("trex", 0, 0)
	x.u32(1) // track ID
	x.u32(1) // sample description index
	x.u32(0)
	x.u32(0)
	x.u32(0)
	x.close(trex)
	x.close(mvex)
	x.close(moov)
	return x.b
}

func matrix(x *w) {
	for _, v := range []uint32{0x00010000, 0, 0, 0, 0x00010000, 0, 0, 0, 0x40000000} {
		x.u32(v)
	}
}

func packLang(l string) uint16 {
	if len(l) != 3 {
		l = "und"
	}
	var v uint16
	for i := 0; i < 3; i++ {
		c := l[i]
		if c < 'a' || c > 'z' {
			return packLang("und")
		}
		v = v<<5 | uint16(c-0x60)
	}
	return v
}

// Sample flags: a sync sample depends on nothing; others depend on earlier
// samples and are not sync samples.
const (
	flagsKey    = 0x02000000
	flagsNonKey = 0x01010000
)

// Moof returns a movie fragment header for samples whose data (the sizes'
// sum) follows in one mdat; baseDTS is the first sample's decode time.
// MdatHeader gives the mdat header to write after it.
func Moof(seq uint32, baseDTS uint64, samples []Sample) []byte {
	x := &w{}
	moof := x.open("moof")
	mfhd := x.full("mfhd", 0, 0)
	x.u32(seq)
	x.close(mfhd)
	traf := x.open("traf")
	tfhd := x.full("tfhd", 0, 0x020000) // default-base-is-moof
	x.u32(1)
	x.close(tfhd)
	tfdt := x.full("tfdt", 1, 0)
	x.u64(baseDTS)
	x.close(tfdt)
	// data offset, duration, size, flags, composition offset per sample;
	// version 1: signed composition offsets.
	trun := x.full("trun", 1, 0x000001|0x000100|0x000200|0x000400|0x000800)
	x.u32(uint32(len(samples)))
	dataOffsetAt := len(x.b)
	x.u32(0)
	for _, s := range samples {
		x.u32(s.Dur)
		x.u32(s.Size)
		if s.Key {
			x.u32(flagsKey)
		} else {
			x.u32(flagsNonKey)
		}
		x.u32(uint32(s.CTO))
	}
	x.close(trun)
	x.close(traf)
	x.close(moof)
	// The data starts after this moof and the 8-byte mdat header.
	binary.BigEndian.PutUint32(x.b[dataOffsetAt:], uint32(len(x.b)+8))
	return x.b
}

// MdatHeader is the header of an mdat holding size bytes.
func MdatHeader(size int) []byte {
	b := binary.BigEndian.AppendUint32(nil, uint32(size+8))
	return append(b, "mdat"...)
}

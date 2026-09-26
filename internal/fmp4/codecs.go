package fmp4

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/bits"
	"strings"
)

// Codec is what a stream needs to be played: the sample entry, the MSE
// type, and the timing facts a demuxer cannot know on its own.
type Codec struct {
	Name    string // ffprobe-style: "hevc", "h264", "av1", "vp9", "aac", "opus", "flac", "mp3", "ac3", "eac3"
	Profile string // e.g. "Main 10", "High", "LC"
	MIME    string // e.g. `video/mp4; codecs="hvc1.2.4.L120.90"`
	Entry   []byte // the sample entry box
	// Audio: the timescale samples are counted in, and the samples per
	// frame when it is fixed (0: see FrameSamples for each frame).
	Rate          int
	FrameSize     int
	VariableFrame bool
}

var ErrUnsupported = errors.New("unsupported for browser playback")

// ---- video ----

// H264 builds the codec from an AVCDecoderConfigurationRecord.
func H264(avcC []byte, width, height int) (*Codec, error) {
	if len(avcC) < 4 || avcC[0] != 1 {
		return nil, fmt.Errorf("%w: bad avcC", ErrUnsupported)
	}
	prof := map[byte]string{66: "Baseline", 77: "Main", 88: "Extended", 100: "High", 110: "High 10", 122: "High 4:2:2", 244: "High 4:4:4 Predictive"}[avcC[1]]
	if avcC[1] == 66 && avcC[2]&0x40 != 0 {
		prof = "Constrained Baseline"
	}
	return &Codec{Name: "h264", Profile: prof,
		MIME:  fmt.Sprintf(`video/mp4; codecs="avc1.%02X%02X%02X"`, avcC[1], avcC[2], avcC[3]),
		Entry: VisualEntry("avc1", width, height, Box("avcC", avcC))}, nil
}

// HEVC builds the codec from an HEVCDecoderConfigurationRecord. The codecs
// string follows ISO/IEC 14496-15 annex E. format is the sample entry:
// "hvc1" (parameter sets in the record) or "hev1" (also in the stream).
func HEVC(hvcC []byte, width, height int, format string) (*Codec, error) {
	if len(hvcC) < 23 {
		return nil, fmt.Errorf("%w: bad hvcC", ErrUnsupported)
	}
	space := hvcC[1] >> 6
	tier := hvcC[1] >> 5 & 1
	idc := hvcC[1] & 31
	compat := bits.Reverse32(binary.BigEndian.Uint32(hvcC[2:6]))
	level := hvcC[12]
	var sb strings.Builder
	sb.WriteString(format + ".")
	if space > 0 {
		sb.WriteByte("ABC"[space-1])
	}
	fmt.Fprintf(&sb, "%d.%X.", idc, compat)
	if tier == 1 {
		sb.WriteByte('H')
	} else {
		sb.WriteByte('L')
	}
	fmt.Fprintf(&sb, "%d", level)
	// Constraint bytes, trailing zero bytes left out.
	cons := hvcC[6:12]
	n := len(cons)
	for n > 0 && cons[n-1] == 0 {
		n--
	}
	for _, c := range cons[:n] {
		fmt.Fprintf(&sb, ".%X", c)
	}
	prof := map[byte]string{1: "Main", 2: "Main 10", 3: "Main Still Picture", 4: "Rext"}[idc]
	return &Codec{Name: "hevc", Profile: prof, MIME: `video/mp4; codecs="` + sb.String() + `"`,
		Entry: VisualEntry(format, width, height, Box("hvcC", hvcC))}, nil
}

// AV1 builds the codec from an AV1CodecConfigurationRecord.
func AV1(av1C []byte, width, height int) (*Codec, error) {
	if len(av1C) < 4 || av1C[0]&0x7f != 1 {
		return nil, fmt.Errorf("%w: bad av1C", ErrUnsupported)
	}
	profile := av1C[1] >> 5
	level := av1C[1] & 31
	tier := "M"
	if av1C[2]&0x80 != 0 {
		tier = "H"
	}
	depth := 8
	if av1C[2]&0x40 != 0 {
		depth = 10
		if profile == 2 && av1C[2]&0x20 != 0 {
			depth = 12
		}
	}
	return &Codec{Name: "av1", Profile: map[byte]string{0: "Main", 1: "High", 2: "Professional"}[profile],
		MIME:  fmt.Sprintf(`video/mp4; codecs="av01.%d.%02d%s.%02d"`, profile, level, tier, depth),
		Entry: VisualEntry("av01", width, height, Box("av1C", av1C))}, nil
}

// VP9 builds the codec from a vpcC payload (a full box's body), or, when
// vpcC is nil, from a keyframe's uncompressed header.
func VP9(vpcC []byte, keyframe []byte, width, height int) (*Codec, error) {
	if vpcC == nil {
		profile, depth, sub, full, err := vp9Header(keyframe)
		if err != nil {
			return nil, err
		}
		level := byte(41)
		if width*height > 2048*1152 {
			level = 51
		}
		p := []byte{profile, level, depth<<4 | sub<<1 | full, 2, 2, 2, 0, 0}
		vpcC = append([]byte{1, 0, 0, 0}, p...)
	}
	if len(vpcC) < 8 {
		return nil, fmt.Errorf("%w: bad vpcC", ErrUnsupported)
	}
	body := vpcC[4:]
	return &Codec{Name: "vp9", Profile: fmt.Sprintf("Profile %d", body[0]),
		MIME:  fmt.Sprintf(`video/mp4; codecs="vp09.%02d.%02d.%02d"`, body[0], body[1], body[2]>>4),
		Entry: VisualEntry("vp09", width, height, Box("vpcC", vpcC))}, nil
}

// vp9Header reads profile, bit depth, chroma subsampling and range from a
// VP9 keyframe's uncompressed header.
func vp9Header(b []byte) (profile, depth, sub, full byte, err error) {
	r := bitReader{b: b}
	if r.bits(2) != 2 {
		return 0, 0, 0, 0, fmt.Errorf("%w: not a VP9 frame", ErrUnsupported)
	}
	lo := r.bits(1)
	hi := r.bits(1)
	profile = byte(hi<<1 | lo)
	if profile == 3 {
		r.bits(1)
	}
	if r.bits(1) == 1 { // show existing frame
		return 0, 0, 0, 0, fmt.Errorf("%w: VP9 frame without a header", ErrUnsupported)
	}
	if r.bits(1) != 0 { // not a keyframe
		return 0, 0, 0, 0, fmt.Errorf("%w: first VP9 frame is not a keyframe", ErrUnsupported)
	}
	r.bits(2) // show frame, error resilient
	if r.bits(24) != 0x498342 {
		return 0, 0, 0, 0, fmt.Errorf("%w: bad VP9 sync code", ErrUnsupported)
	}
	depth = 8
	if profile >= 2 {
		depth = 10
		if r.bits(1) == 1 {
			depth = 12
		}
	}
	cs := r.bits(3)
	sub = 1 // 4:2:0, co-sited-less (vpcC value 1)
	if cs != 7 {
		full = byte(r.bits(1))
		if profile == 1 || profile == 3 {
			sx, sy := r.bits(1), r.bits(1)
			switch {
			case sx == 1 && sy == 0:
				sub = 2
			case sx == 0 && sy == 0:
				sub = 3
			}
		}
	} else {
		full, sub = 1, 3
	}
	if r.err {
		return 0, 0, 0, 0, fmt.Errorf("%w: short VP9 header", ErrUnsupported)
	}
	return
}

type bitReader struct {
	b   []byte
	pos int
	err bool
}

func (r *bitReader) bits(n int) uint32 {
	var v uint32
	for i := 0; i < n; i++ {
		if r.pos >= 8*len(r.b) {
			r.err = true
			return 0
		}
		v = v<<1 | uint32(r.b[r.pos/8]>>(7-r.pos%8)&1)
		r.pos++
	}
	return v
}

// ---- audio ----

// AAC builds the codec from an AudioSpecificConfig.
func AAC(asc []byte, channels int) (*Codec, error) {
	r := bitReader{b: asc}
	aot := r.bits(5)
	if aot == 31 {
		aot = 32 + r.bits(6)
	}
	fi := r.bits(4)
	rate := 0
	rates := []int{96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050, 16000, 12000, 11025, 8000, 7350}
	if fi == 15 {
		rate = int(r.bits(24))
	} else if int(fi) < len(rates) {
		rate = rates[fi]
	}
	ch := int(r.bits(4))
	frame := 1024
	if aot == 5 || aot == 29 { // explicit SBR: the core's rate and frame
		ext := r.bits(4)
		if ext == 15 {
			r.bits(24)
		}
		base := r.bits(5)
		_ = base
	} else if aot == 1 || aot == 2 || aot == 3 || aot == 4 {
		if r.bits(1) == 1 { // frameLengthFlag
			frame = 960
		}
	}
	if r.err || rate == 0 {
		return nil, fmt.Errorf("%w: bad AudioSpecificConfig", ErrUnsupported)
	}
	if ch == 0 {
		ch = channels
	}
	obj := map[uint32]string{2: "LC", 5: "HE-AAC", 29: "HE-AACv2", 1: "Main"}[aot]
	if obj == "" || aot == 1 {
		return nil, fmt.Errorf("%w: AAC object type %d", ErrUnsupported, aot)
	}
	return &Codec{Name: "aac", Profile: obj, MIME: fmt.Sprintf(`audio/mp4; codecs="mp4a.40.%d"`, aot),
		Entry: AudioEntry("mp4a", ch, rate, esds(0x40, asc)), Rate: rate, FrameSize: frame}, nil
}

// esds is an MPEG-4 elementary stream descriptor box for one stream.
func esds(oti byte, dsi []byte) []byte {
	desc := func(tag byte, body []byte) []byte {
		out := []byte{tag}
		n := len(body)
		out = append(out, 0x80|byte(n>>21&0x7f), 0x80|byte(n>>14&0x7f), 0x80|byte(n>>7&0x7f), byte(n&0x7f))
		return append(out, body...)
	}
	dcd := []byte{oti, 0x15, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0} // audio stream, sizes and bitrates unknown
	if dsi != nil {
		dcd = append(dcd, desc(5, dsi)...)
	}
	es := append([]byte{0, 1, 0}, desc(4, dcd)...) // ES_ID 1, no flags
	es = append(es, desc(6, []byte{2})...)
	return FullBox("esds", 0, 0, desc(3, es))
}

// MP3 builds the codec from a first MPEG audio frame header.
func MP3(frame []byte) (*Codec, error) {
	if len(frame) < 4 || frame[0] != 0xff || frame[1]&0xe0 != 0xe0 {
		return nil, fmt.Errorf("%w: bad MP3 frame", ErrUnsupported)
	}
	ver := frame[1] >> 3 & 3 // 3: MPEG-1, 2: MPEG-2, 0: MPEG-2.5
	ri := frame[2] >> 2 & 3
	base := map[byte][]int{3: {44100, 48000, 32000}, 2: {22050, 24000, 16000}, 0: {11025, 12000, 8000}}[ver]
	if base == nil || ri > 2 {
		return nil, fmt.Errorf("%w: bad MP3 header", ErrUnsupported)
	}
	ch := 2
	if frame[3]>>6 == 3 {
		ch = 1
	}
	size := 1152
	if ver != 3 {
		size = 576
	}
	return &Codec{Name: "mp3", MIME: `audio/mp4; codecs="mp4a.6B"`, Entry: AudioEntry("mp4a", ch, base[ri], esds(0x6B, nil)),
		Rate: base[ri], FrameSize: size}, nil
}

// Opus builds the codec from an OpusHead (Matroska's CodecPrivate) or,
// when head is nil, from a dOps payload.
func Opus(head, dOps []byte) (*Codec, error) {
	if head != nil {
		if len(head) < 19 || string(head[:8]) != "OpusHead" {
			return nil, fmt.Errorf("%w: bad OpusHead", ErrUnsupported)
		}
		d := []byte{0, head[9]}
		d = binary.BigEndian.AppendUint16(d, binary.LittleEndian.Uint16(head[10:]))
		d = binary.BigEndian.AppendUint32(d, binary.LittleEndian.Uint32(head[12:]))
		d = binary.BigEndian.AppendUint16(d, binary.LittleEndian.Uint16(head[16:]))
		d = append(d, head[18])
		if head[18] != 0 {
			if len(head) < 21+int(head[9]) {
				return nil, fmt.Errorf("%w: bad OpusHead mapping", ErrUnsupported)
			}
			d = append(d, head[19:21+int(head[9])]...)
		}
		dOps = d
	}
	if len(dOps) < 11 {
		return nil, fmt.Errorf("%w: bad dOps", ErrUnsupported)
	}
	return &Codec{Name: "opus", MIME: `audio/mp4; codecs="opus"`, Entry: AudioEntry("Opus", int(dOps[1]), 48000, Box("dOps", dOps)),
		Rate: 48000, VariableFrame: true}, nil
}

// OpusSamples is how many 48 kHz samples an Opus packet decodes to (its TOC).
func OpusSamples(p []byte) int {
	if len(p) == 0 {
		return 0
	}
	c := p[0] >> 3
	var per int
	switch {
	case c < 12:
		per = []int{480, 960, 1920, 2880}[c&3]
	case c < 16:
		per = []int{480, 960}[c&1]
	default:
		per = []int{120, 240, 480, 960}[c&3]
	}
	switch p[0] & 3 {
	case 0:
		return per
	case 1, 2:
		return 2 * per
	}
	if len(p) < 2 {
		return per
	}
	return int(p[1]&0x3f) * per
}

// FLAC builds the codec from Matroska's CodecPrivate ("fLaC" + metadata
// blocks) or a dfLa payload (a full box's body).
func FLAC(private, dfLa []byte) (*Codec, error) {
	var streaminfo []byte
	blocks := private
	if dfLa != nil {
		blocks = dfLa[4:]
	} else if len(blocks) >= 4 && string(blocks[:4]) == "fLaC" {
		blocks = blocks[4:]
	}
	for len(blocks) >= 4 {
		typ := blocks[0] & 0x7f
		n := int(blocks[1])<<16 | int(blocks[2])<<8 | int(blocks[3])
		if 4+n > len(blocks) {
			break
		}
		if typ == 0 {
			streaminfo = blocks[4 : 4+n]
		}
		blocks = blocks[4+n:]
	}
	if len(streaminfo) < 18 {
		return nil, fmt.Errorf("%w: FLAC without STREAMINFO", ErrUnsupported)
	}
	minBlock := int(binary.BigEndian.Uint16(streaminfo[0:]))
	maxBlock := int(binary.BigEndian.Uint16(streaminfo[2:]))
	rate := int(streaminfo[10])<<12 | int(streaminfo[11])<<4 | int(streaminfo[12])>>4
	ch := int(streaminfo[12]>>1&7) + 1
	bits := int(streaminfo[12]&1)<<4 | int(streaminfo[13]>>4) + 1
	// The box keeps STREAMINFO only, marked as the last block.
	body := append([]byte{0x80, 0, 0, byte(len(streaminfo))}, streaminfo...)
	c := &Codec{Name: "flac", MIME: `audio/mp4; codecs="flac"`, Entry: AudioEntryBits("fLaC", ch, rate, bits, FullBox("dfLa", 0, 0, body)), Rate: rate}
	if minBlock == maxBlock && minBlock > 0 {
		c.FrameSize = minBlock
	} else {
		c.VariableFrame = true
	}
	return c, nil
}

// FLACSamples reads the block size from a FLAC frame header (0 if unknown).
func FLACSamples(p []byte) int {
	if len(p) < 5 || p[0] != 0xff || p[1]&0xfe != 0xf8 {
		return 0
	}
	bs := p[2] >> 4
	switch {
	case bs == 1:
		return 192
	case bs >= 2 && bs <= 5:
		return 576 << (bs - 2)
	case bs >= 8:
		return 256 << (bs - 8)
	}
	// 6 and 7: the size follows the UTF-8 coded frame number.
	i := 4
	for m := p[4]; m&0x80 != 0 && i < len(p); m <<= 1 {
		i++
	}
	if bs == 6 && i+1 <= len(p) {
		return int(p[i]) + 1
	}
	if bs == 7 && i+2 <= len(p) {
		return int(binary.BigEndian.Uint16(p[i:])) + 1
	}
	return 0
}

// AC3 builds the codec from a first AC-3 syncframe.
func AC3(frame []byte) (*Codec, error) {
	if len(frame) < 8 || frame[0] != 0x0b || frame[1] != 0x77 {
		return nil, fmt.Errorf("%w: bad AC-3 frame", ErrUnsupported)
	}
	r := bitReader{b: frame[4:]}
	fscod := r.bits(2)
	frmsizecod := r.bits(6)
	bsid := r.bits(5)
	bsmod := r.bits(3)
	acmod := r.bits(3)
	if acmod&1 != 0 && acmod != 1 {
		r.bits(2)
	}
	if acmod&4 != 0 {
		r.bits(2)
	}
	if acmod == 2 {
		r.bits(2)
	}
	lfe := r.bits(1)
	rate := []int{48000, 44100, 32000, 0}[fscod]
	if rate == 0 || r.err {
		return nil, fmt.Errorf("%w: bad AC-3 header", ErrUnsupported)
	}
	ch := []int{2, 1, 2, 3, 3, 4, 4, 5}[acmod] + int(lfe)
	v := fscod<<22 | bsid<<17 | bsmod<<14 | acmod<<11 | lfe<<10 | (frmsizecod>>1)<<5
	dac3 := []byte{byte(v >> 16), byte(v >> 8), byte(v)}
	return &Codec{Name: "ac3", MIME: `audio/mp4; codecs="ac-3"`, Entry: AudioEntry("ac-3", ch, rate, Box("dac3", dac3)), Rate: rate, FrameSize: 1536}, nil
}

// EAC3 builds the codec from a first E-AC-3 syncframe (one independent
// substream).
func EAC3(frame []byte) (*Codec, error) {
	if len(frame) < 8 || frame[0] != 0x0b || frame[1] != 0x77 {
		return nil, fmt.Errorf("%w: bad E-AC-3 frame", ErrUnsupported)
	}
	r := bitReader{b: frame[2:]}
	r.bits(2) // stream type
	r.bits(3) // substream id
	frmsiz := r.bits(11)
	fscod := r.bits(2)
	rate, blocks := 0, 6
	if fscod == 3 {
		rate = []int{24000, 22050, 16000, 0}[r.bits(2)]
	} else {
		rate = []int{48000, 44100, 32000}[fscod]
		blocks = []int{1, 2, 3, 6}[r.bits(2)]
	}
	acmod := r.bits(3)
	lfe := r.bits(1)
	bsid := r.bits(5)
	if rate == 0 || r.err {
		return nil, fmt.Errorf("%w: bad E-AC-3 header", ErrUnsupported)
	}
	bytesPerFrame := int(frmsiz+1) * 2
	kbps := bytesPerFrame * 8 * rate / (blocks * 256) / 1000
	ch := []int{2, 1, 2, 3, 3, 4, 4, 5}[acmod] + int(lfe)
	fsc := fscod
	if fscod == 3 {
		fsc = 0
	}
	x := &w{}
	x.u16(uint16(kbps<<3 | 0)) // data rate, one independent substream
	x.u8(byte(fsc<<6 | bsid<<1))
	x.u8(byte(0<<5 | acmod<<1 | lfe)) // asvc 0, bsmod 0
	x.u8(0)                           // no dependent substreams
	return &Codec{Name: "eac3", MIME: `audio/mp4; codecs="ec-3"`, Entry: AudioEntry("ec-3", ch, rate, Box("dec3", x.b)), Rate: rate, FrameSize: blocks * 256}, nil
}

// Package media opens a local video (Matroska, WebM or MP4) and streams one
// track at a time as fragmented MP4 for Media Source Extensions, without
// re-encoding and without external programs: the containers are read by
// internal/matroska and internal/mp4 and the fragments written by
// internal/fmp4.
//
// Every sample's decode and presentation time is fixed once, from the whole
// file's index, so a stream started anywhere carries the same times as one
// started at the beginning. Matroska stores presentation times only; decode
// times are derived (the presentation times in decode order, sorted, less
// the reorder delay). Audio times count samples at the codec's rate, frame
// after frame, re-anchored to the container only where it has a real gap:
// Matroska's millisecond times alone would give frames of 21 and 22 ms
// where the decoder produces 21.33.
package media

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sort"

	"github.com/Tieumi221E/Jusplay/internal/fmp4"
	"github.com/Tieumi221E/Jusplay/internal/matroska"
	"github.com/Tieumi221E/Jusplay/internal/mp4"
)

// ErrUnsupported marks media the browser engine cannot play without re-encoding.
var ErrUnsupported = fmp4.ErrUnsupported

// errDolby: the engine (WebView2) says it can play AC-3 and E-AC-3 in MSE,
// then fails to decode them (PIPELINE_ERROR_DECODE, measured), stopping the
// whole video. Such a track is left out; the picture plays with a notice.
// (internal/fmp4 packages both, for a later decoder.)
var errDolby = fmt.Errorf("%w: Dolby Digital (AC-3/E-AC-3) audio", ErrUnsupported)

type sample struct {
	pos  int64
	size uint32
	dts  int64 // track timescale
	pts  int64
	dur  uint32
	key  bool
}

// Track is the video or the audio track being played.
type Track struct {
	Codec    string `json:"codec"`
	Profile  string `json:"profile"`
	MIME     string `json:"mime"`
	Language string `json:"language,omitempty"`

	timescale uint32
	shift     int64 // added to every time written, keeping decode times >= 0
	samples   []sample
	init      []byte
	prefix    []byte // Matroska header stripping: put back on every frame
	zlib      bool   // Matroska zlib-compressed frames
	video     bool
}

// Media is an opened video.
type Media struct {
	Path      string    `json:"-"`
	Duration  float64   `json:"duration"`
	Width     int       `json:"width"`
	Height    int       `json:"height"`
	FrameRate string    `json:"frameRate"`
	Video     Track     `json:"video"`
	Audio     *Track    `json:"audio"`
	Keyframes []float64 `json:"keyframes"`
	// AudioNote says why there is no audio track when the file has one the
	// engine cannot play.
	AudioNote string `json:"audioNote,omitempty"`
	// Subtitles are the file's own subtitle tracks (Matroska), read only
	// when one is chosen (SubtitleEvents).
	Subtitles []SubtitleTrack `json:"subtitles,omitempty"`
}

// Open indexes path; see OpenCached.
func Open(path string) (*Media, error) { return OpenCached(path, "") }

// OpenCached opens path, reading its sample index from cacheDir when the
// cache holds it for this exact file, and writing it there otherwise.
func OpenCached(path, cacheDir string) (*Media, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	st, err := fh.Stat()
	if err != nil {
		return nil, err
	}
	var magic [12]byte
	if _, err := fh.ReadAt(magic[:], 0); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	cache := CacheFile(cacheDir, path)
	var m *Media
	switch {
	case magic[0] == 0x1A && magic[1] == 0x45 && magic[2] == 0xDF && magic[3] == 0xA3:
		m, err = openMatroska(fh, st.Size(), cache)
	case string(magic[4:8]) == "ftyp" || string(magic[4:8]) == "moov" || string(magic[4:8]) == "free" || string(magic[4:8]) == "mdat":
		m, err = openMP4(fh, st.Size())
	default:
		return nil, fmt.Errorf("%w: not a Matroska, WebM or MP4 file", ErrUnsupported)
	}
	if err != nil {
		return nil, err
	}
	m.Path = path
	for _, s := range m.Video.samples {
		if s.key {
			m.Keyframes = append(m.Keyframes, float64(s.pts)/float64(m.Video.timescale))
		}
	}
	sort.Float64s(m.Keyframes)
	if len(m.Keyframes) == 0 {
		return nil, errors.New("no video keyframes")
	}
	return m, nil
}

// ---- Matroska ----

var mkvVideo = map[string]bool{"V_MPEG4/ISO/AVC": true, "V_MPEGH/ISO/HEVC": true, "V_AV1": true, "V_VP9": true}

func openMatroska(r io.ReaderAt, size int64, cache string) (*Media, error) {
	f, err := matroska.Read(r, size)
	if err != nil {
		return nil, err
	}
	m := &Media{}
	if !math.IsNaN(f.Duration) {
		m.Duration = f.Duration * float64(f.TimecodeScale) / 1e9
	}
	// The video: the first one the engine can decode.
	var vt *matroska.Track
	var why string
	for i := range f.Tracks {
		t := &f.Tracks[i]
		if t.Type != 1 {
			continue
		}
		if mkvVideo[t.CodecID] && !t.Encrypted {
			vt = t
			break
		}
		why = t.CodecID
	}
	if vt == nil {
		if why == "" {
			return nil, fmt.Errorf("%w: no video track", ErrUnsupported)
		}
		return nil, fmt.Errorf("%w: video codec %s", ErrUnsupported, why)
	}
	// Audio candidates: the default ones first, then in file order.
	var auds []*matroska.Track
	for i := range f.Tracks {
		if t := &f.Tracks[i]; t.Type == 2 && t.Enabled {
			auds = append(auds, t)
		}
	}
	sort.SliceStable(auds, func(i, j int) bool { return auds[i].Default && !auds[j].Default })

	var idx *index
	if cache != "" {
		idx, _ = readCache(cache)
	}
	fresh := idx == nil
	var frames []matroska.Frame
	if fresh {
		want := map[uint64]bool{vt.Number: true}
		for _, a := range auds {
			want[a.Number] = true
		}
		if frames, err = f.Frames(want); err != nil && len(frames) == 0 {
			return nil, err
		}
		idx = &index{tracks: map[uint64][]sample{}}
	}
	first := func(t *matroska.Track) []byte {
		for _, fr := range frames {
			if fr.Track == t.Number {
				b, _ := f.FrameData(t, fr, nil)
				return b
			}
		}
		if s := idx.tracks[t.Number]; len(s) > 0 {
			b, _ := f.FrameData(t, matroska.Frame{Pos: s[0].pos, Size: s[0].size}, nil)
			return b
		}
		return nil
	}

	vc, err := mkvVideoCodec(vt, first)
	if err != nil {
		return nil, err
	}
	m.Width, m.Height = vt.Width, vt.Height
	if vt.DefaultDur > 0 {
		m.FrameRate = fmt.Sprintf("%d/1000", int64(math.Round(1e12/float64(vt.DefaultDur))))
	}
	scale := int64(1e9) / int64(f.TimecodeScale)
	if scale < 1 || int64(1e9)%int64(f.TimecodeScale) != 0 {
		scale = 1e6 // microseconds
	}
	m.Video = Track{Codec: vc.Name, Profile: vc.Profile, MIME: vc.MIME, Language: vt.Language,
		timescale: uint32(scale), prefix: vt.StripPrefix, zlib: vt.Zlib, video: true}
	if fresh {
		idx.tracks[vt.Number] = videoTimeline(framesOf(frames, vt.Number), int64(scale), vt.DefaultDur)
	}
	m.Video.samples = idx.tracks[vt.Number]
	if len(m.Video.samples) == 0 {
		return nil, errors.New("the video track has no frames")
	}
	m.Video.init = fmp4.Init(fmp4.Track{Video: true, Timescale: m.Video.timescale, Width: vt.Width, Height: vt.Height, Language: vt.Language, Entry: vc.Entry})
	if m.FrameRate == "" {
		m.FrameRate = rateOf(m.Video.samples, m.Video.timescale)
	}

	for _, at := range auds {
		ac, err := mkvAudioCodec(at, first)
		if err != nil {
			if m.AudioNote == "" {
				m.AudioNote = err.Error()
			}
			continue
		}
		if fresh {
			heads := headsOf(r, framesOf(frames, at.Number), ac, at.StripPrefix)
			idx.tracks[at.Number] = audioTimeline(framesOf(frames, at.Number), heads, ac)
		}
		m.Audio = &Track{Codec: ac.Name, Profile: ac.Profile, MIME: ac.MIME, Language: at.Language,
			timescale: uint32(ac.Rate), samples: idx.tracks[at.Number], prefix: at.StripPrefix, zlib: at.Zlib}
		if len(m.Audio.samples) == 0 {
			m.Audio = nil
			continue
		}
		m.Audio.init = fmp4.Init(fmp4.Track{Timescale: m.Audio.timescale, Channels: at.Channels, SampleRate: ac.Rate, Language: at.Language, Entry: ac.Entry})
		m.AudioNote = ""
		break
	}
	for _, t := range f.Tracks {
		if t.Type == 17 && t.Enabled {
			m.Subtitles = append(m.Subtitles, SubtitleTrack{Number: t.Number, Codec: t.CodecID, Language: t.Language, Name: t.Name, Default: t.Default,
				Text: textSubtitle[t.CodecID] && !t.Encrypted})
		}
	}
	if fresh && cache != "" {
		keep := &index{tracks: map[uint64][]sample{vt.Number: idx.tracks[vt.Number]}}
		for _, a := range auds {
			if s, ok := idx.tracks[a.Number]; ok {
				keep.tracks[a.Number] = s
			}
		}
		writeCache(cache, keep)
	}
	m.finish()
	return m, nil
}

func framesOf(all []matroska.Frame, track uint64) []matroska.Frame {
	var out []matroska.Frame
	for _, f := range all {
		if f.Track == track {
			out = append(out, f)
		}
	}
	return out
}

func mkvVideoCodec(t *matroska.Track, first func(*matroska.Track) []byte) (*fmp4.Codec, error) {
	switch t.CodecID {
	case "V_MPEG4/ISO/AVC":
		return fmp4.H264(t.CodecPrivate, t.Width, t.Height)
	case "V_MPEGH/ISO/HEVC":
		return fmp4.HEVC(t.CodecPrivate, t.Width, t.Height, "hvc1")
	case "V_AV1":
		return fmp4.AV1(t.CodecPrivate, t.Width, t.Height)
	case "V_VP9":
		return fmp4.VP9(nil, first(t), t.Width, t.Height)
	}
	return nil, fmt.Errorf("%w: video codec %s", ErrUnsupported, t.CodecID)
}

func mkvAudioCodec(t *matroska.Track, first func(*matroska.Track) []byte) (*fmp4.Codec, error) {
	if t.Encrypted {
		return nil, fmt.Errorf("%w: encrypted audio", ErrUnsupported)
	}
	switch {
	case t.CodecID == "A_AAC" || len(t.CodecID) > 6 && t.CodecID[:6] == "A_AAC/":
		asc := t.CodecPrivate
		if len(asc) < 2 {
			asc = ascFor(t.CodecID, t.SampleRate, t.OutRate, t.Channels)
		}
		return fmp4.AAC(asc, t.Channels)
	case t.CodecID == "A_OPUS":
		return fmp4.Opus(t.CodecPrivate, nil)
	case t.CodecID == "A_FLAC":
		return fmp4.FLAC(t.CodecPrivate, nil)
	case t.CodecID == "A_MPEG/L3":
		return fmp4.MP3(first(t))
	case t.CodecID == "A_AC3", t.CodecID == "A_EAC3":
		return nil, errDolby
	}
	return nil, fmt.Errorf("%w: audio codec %s", ErrUnsupported, t.CodecID)
}

// ascFor makes an AudioSpecificConfig for the old A_AAC/MPEG4/... ids,
// which carry the profile in the name instead of a CodecPrivate.
func ascFor(id string, rate, out float64, ch int) []byte {
	aot := 2
	switch {
	case len(id) >= 4 && id[len(id)-4:] == "/SBR":
		aot = 5
	case len(id) >= 5 && id[len(id)-5:] == "/MAIN":
		aot = 1
	}
	rates := []float64{96000, 88200, 64000, 48000, 44100, 32000, 24000, 22050, 16000, 12000, 11025, 8000, 7350}
	fi := 4
	for i, r := range rates {
		if r == rate {
			fi = i
		}
	}
	v := uint16(aot)<<11 | uint16(fi)<<7 | uint16(ch&15)<<3
	_ = out
	return []byte{byte(v >> 8), byte(v)}
}

// ---- MP4 ----

func openMP4(r io.ReaderAt, size int64) (*Media, error) {
	f, err := mp4.Read(r, size)
	if err != nil {
		return nil, err
	}
	m := &Media{Duration: f.Duration}
	var vt *mp4.Track
	var auds []*mp4.Track
	why := ""
	for _, t := range f.Tracks {
		switch t.Handler {
		case "vide":
			if vt == nil && !t.Encrypted && len(t.Samples) > 0 {
				if _, err := mp4VideoCodec(t); err == nil {
					vt = t
				} else if why == "" {
					why = err.Error()
				}
			}
		case "soun":
			if t.Enabled && len(t.Samples) > 0 {
				auds = append(auds, t)
			}
		}
	}
	if vt == nil {
		if why != "" {
			return nil, errors.New(why)
		}
		return nil, fmt.Errorf("%w: no video track", ErrUnsupported)
	}
	vc, _ := mp4VideoCodec(vt)
	m.Width, m.Height = vt.Width, vt.Height
	m.Video = Track{Codec: vc.Name, Profile: vc.Profile, MIME: vc.MIME, Language: vt.Language, timescale: vt.Timescale, video: true,
		samples: mp4Samples(vt.Samples, 0)}
	m.Video.init = fmp4.Init(fmp4.Track{Video: true, Timescale: vt.Timescale, Width: vt.Width, Height: vt.Height, Language: vt.Language, Entry: vc.Entry})
	m.FrameRate = rateOf(m.Video.samples, vt.Timescale)
	if vt.Duration > m.Duration {
		m.Duration = vt.Duration
	}
	for _, at := range auds {
		ac, err := mp4AudioCodec(r, at)
		if err != nil {
			if m.AudioNote == "" {
				m.AudioNote = err.Error()
			}
			continue
		}
		m.Audio = &Track{Codec: ac.Name, Profile: ac.Profile, MIME: ac.MIME, Language: at.Language, timescale: at.Timescale,
			samples: mp4Samples(at.Samples, ac.FrameSize*int(at.Timescale)/max(ac.Rate, 1))}
		m.Audio.init = fmp4.Init(fmp4.Track{Timescale: at.Timescale, Channels: at.Channels, SampleRate: at.SampleRate, Language: at.Language, Entry: ac.Entry})
		m.AudioNote = ""
		break
	}
	m.finish()
	return m, nil
}

func mp4VideoCodec(t *mp4.Track) (*fmp4.Codec, error) {
	switch t.Format {
	case "avc1", "avc3":
		c, err := fmp4.H264(t.Config["avcC"], t.Width, t.Height)
		if err == nil && t.Format == "avc3" {
			c.Entry = fmp4.VisualEntry("avc3", t.Width, t.Height, fmp4.Box("avcC", t.Config["avcC"]))
			c.MIME = "video/mp4; codecs=\"avc3" + c.MIME[len(`video/mp4; codecs="avc1`):]
		}
		return c, err
	case "hvc1", "hev1":
		return fmp4.HEVC(t.Config["hvcC"], t.Width, t.Height, t.Format)
	case "av01":
		return fmp4.AV1(t.Config["av1C"], t.Width, t.Height)
	case "vp09":
		return fmp4.VP9(t.Config["vpcC"], nil, t.Width, t.Height)
	}
	return nil, fmt.Errorf("%w: video codec %s", ErrUnsupported, t.Format)
}

func mp4AudioCodec(r io.ReaderAt, t *mp4.Track) (*fmp4.Codec, error) {
	firstFrame := func() []byte {
		s := t.Samples[0]
		b := make([]byte, min(s.Size, 64))
		r.ReadAt(b, s.Pos)
		return b
	}
	switch t.Format {
	case "mp4a":
		oti, dsi := parseEsds(t.Config["esds"])
		switch oti {
		case 0x40, 0x66, 0x67, 0x68:
			return fmp4.AAC(dsi, t.Channels)
		case 0x69, 0x6B:
			return fmp4.MP3(firstFrame())
		}
		return nil, fmt.Errorf("%w: MPEG-4 audio object 0x%02X", ErrUnsupported, oti)
	case ".mp3":
		return fmp4.MP3(firstFrame())
	case "Opus":
		return fmp4.Opus(nil, t.Config["dOps"])
	case "fLaC":
		return fmp4.FLAC(nil, t.Config["dfLa"])
	case "ac-3", "ec-3":
		return nil, errDolby
	}
	return nil, fmt.Errorf("%w: audio codec %s", ErrUnsupported, t.Format)
}

// parseEsds returns the object type and decoder-specific info of an esds payload.
func parseEsds(b []byte) (byte, []byte) {
	if len(b) < 4 {
		return 0, nil
	}
	b = b[4:] // version and flags
	var oti byte
	var dsi []byte
	var walk func(b []byte)
	walk = func(b []byte) {
		for len(b) >= 2 {
			tag := b[0]
			n, i := 0, 1
			for ; i < 5 && i < len(b); i++ {
				n = n<<7 | int(b[i]&0x7f)
				if b[i]&0x80 == 0 {
					i++
					break
				}
			}
			if i+n > len(b) {
				return
			}
			body := b[i : i+n]
			switch tag {
			case 3: // ES_Descriptor
				if len(body) < 3 {
					return
				}
				fl := body[2]
				o := 3
				if fl&0x80 != 0 {
					o += 2
				}
				if fl&0x40 != 0 && o < len(body) {
					o += 1 + int(body[o])
				}
				if fl&0x20 != 0 {
					o += 2
				}
				if o <= len(body) {
					walk(body[o:])
				}
			case 4: // DecoderConfigDescriptor
				if len(body) >= 13 {
					oti = body[0]
					walk(body[13:])
				}
			case 5:
				dsi = body
			}
			b = b[i+n:]
		}
	}
	walk(b)
	return oti, dsi
}

// mp4Samples converts sample tables (decode and presentation times as the
// file gives them); durations are the decode-time steps, the last one the
// frame size (audio) or the one before it.
func mp4Samples(in []mp4.Sample, lastDur int) []sample {
	out := make([]sample, len(in))
	for i, s := range in {
		out[i] = sample{pos: s.Pos, size: s.Size, dts: s.DTS, pts: s.PTS, key: s.Key}
		if i > 0 {
			out[i-1].dur = uint32(max(0, s.DTS-in[i-1].DTS))
		}
	}
	if n := len(out); n > 0 {
		switch {
		case lastDur > 0:
			out[n-1].dur = uint32(lastDur)
		case n > 1:
			out[n-1].dur = out[n-2].dur
		}
	}
	return out
}

// finish fixes each track's shift: decode times written must not be
// negative (MSE maps them back with timestampOffset).
func (m *Media) finish() {
	for _, t := range []*Track{&m.Video, m.Audio} {
		if t == nil || len(t.samples) == 0 {
			continue
		}
		lo := t.samples[0].dts
		for _, s := range t.samples {
			lo = min(lo, s.dts, s.pts)
		}
		if lo < 0 {
			t.shift = -lo
		}
	}
	if m.Duration == 0 {
		v := m.Video.samples[len(m.Video.samples)-1]
		m.Duration = float64(v.pts+int64(v.dur)) / float64(m.Video.timescale)
	}
}

// rateOf gives a frame rate "N/1000" from the median frame duration.
func rateOf(s []sample, ts uint32) string {
	if len(s) < 2 {
		return ""
	}
	d := make([]int, 0, len(s))
	for _, x := range s {
		if x.dur > 0 {
			d = append(d, int(x.dur))
		}
	}
	if len(d) == 0 {
		return ""
	}
	sort.Ints(d)
	return fmt.Sprintf("%d/1000", int64(math.Round(float64(ts)*1000/float64(d[len(d)/2]))))
}

// KeyframeAtOrBefore returns the latest keyframe time <= t (or the first).
func (m *Media) KeyframeAtOrBefore(t float64) float64 {
	i := sort.SearchFloat64s(m.Keyframes, t+1e-9)
	if i == 0 {
		return m.Keyframes[0]
	}
	return m.Keyframes[i-1]
}

func (m *Media) track(kind string) (*Track, error) {
	switch kind {
	case "video":
		return &m.Video, nil
	case "audio":
		if m.Audio != nil {
			return m.Audio, nil
		}
	}
	return nil, fmt.Errorf("no %s track", kind)
}

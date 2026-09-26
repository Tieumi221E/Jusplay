// Package mkv reads what the library shows about a video (streams, codecs,
// duration, whether it carries Jusplay's attachments) and adds, replaces
// and reads Jusplay's attachments in Matroska files, with Jusplay's own
// readers and writer (internal/matroska, internal/mp4); no external tools.
//
// A rewrite never moves the media data: the clusters are copied byte for
// byte, and Verify proves it (their hash, every frame's time, size and key
// flag, the tracks, every other element's bytes, the attachments).
package mkv

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Tieumi221E/Jusplay/internal/fmp4"
	"github.com/Tieumi221E/Jusplay/internal/matroska"
	"github.com/Tieumi221E/Jusplay/internal/mp4"
)

type Attachment struct {
	Name string
	MIME string
	Data []byte
}

// Prefix reserved for jusplay attachment file names.
const Prefix = "jusplay."

// LegacyPrefix is the prefix written before the project was renamed
// (kantanplay, until 2026-09-25). Such attachments are still ours: they are
// read under their current names (Canonical) and replaced on re-embedding.
const LegacyPrefix = "kantanplay."

func isOurs(name string) bool {
	return strings.HasPrefix(name, Prefix) || strings.HasPrefix(name, LegacyPrefix)
}

// Canonical is an attachment name under the current prefix.
func Canonical(name string) string {
	if rest, ok := strings.CutPrefix(name, LegacyPrefix); ok {
		return Prefix + rest
	}
	return name
}

// Stream is one media stream (ffprobe's codec names, as the manifest
// records them).
type Stream struct {
	Index     int
	CodecType string // "video", "audio", "subtitle"
	CodecName string
	Profile   string
	Width     int
	Height    int
	Language  string
	Default   bool
}

// Probe is what a file says about itself.
type Probe struct {
	Streams     []Stream
	Duration    float64
	Attachments []matroska.Attachment
}

// Media returns the media streams.
func (p *Probe) Media() []Stream { return p.Streams }

// Ours returns the attachments whose file name has the jusplay prefix.
func (p *Probe) Ours() []matroska.Attachment {
	var out []matroska.Attachment
	for _, a := range p.Attachments {
		if isOurs(a.Name) {
			out = append(out, a)
		}
	}
	return out
}

var mkvCodecs = map[string]string{
	"V_MPEGH/ISO/HEVC": "hevc", "V_MPEG4/ISO/AVC": "h264", "V_AV1": "av1", "V_VP9": "vp9", "V_VP8": "vp8",
	"V_MPEG2": "mpeg2video", "V_MPEG4/ISO/ASP": "mpeg4", "V_MS/VFW/FOURCC": "vfw",
	"A_FLAC": "flac", "A_OPUS": "opus", "A_AC3": "ac3", "A_EAC3": "eac3", "A_DTS": "dts", "A_TRUEHD": "truehd",
	"A_MPEG/L3": "mp3", "A_MPEG/L2": "mp2", "A_VORBIS": "vorbis", "A_PCM/INT/LIT": "pcm_s16le", "A_PCM/FLOAT/IEEE": "pcm_f32le",
	"S_TEXT/ASS": "ass", "S_TEXT/SSA": "ssa", "S_ASS": "ass", "S_SSA": "ssa", "S_TEXT/UTF8": "subrip", "S_TEXT/WEBVTT": "webvtt",
	"S_HDMV/PGS": "hdmv_pgs_subtitle", "S_VOBSUB": "dvd_subtitle",
}

var mp4Codecs = map[string]string{"avc1": "h264", "avc3": "h264", "hvc1": "hevc", "hev1": "hevc", "av01": "av1", "vp09": "vp9",
	"mp4a": "aac", "Opus": "opus", "fLaC": "flac", "ac-3": "ac3", "ec-3": "eac3", ".mp3": "mp3", "tx3g": "mov_text", "wvtt": "webvtt"}

// ProbeFile reads a Matroska, WebM or MP4 file's headers.
func ProbeFile(path string) (*Probe, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	st, err := fh.Stat()
	if err != nil {
		return nil, err
	}
	var magic [8]byte
	if _, err := fh.ReadAt(magic[:], 0); err != nil {
		return nil, err
	}
	p := &Probe{}
	if magic[0] == 0x1A && magic[1] == 0x45 && magic[2] == 0xDF && magic[3] == 0xA3 {
		f, err := matroska.Read(fh, st.Size())
		if err != nil {
			return nil, err
		}
		p.Duration = f.Duration * float64(f.TimecodeScale) / 1e9
		if p.Duration != p.Duration { // NaN: not written
			p.Duration = 0
		}
		for i, t := range f.Tracks {
			s := Stream{Index: i, Language: t.Language, Default: t.Default, Width: t.Width, Height: t.Height}
			s.CodecType = map[int]string{1: "video", 2: "audio", 17: "subtitle"}[t.Type]
			s.CodecName = mkvCodecs[t.CodecID]
			if s.CodecName == "" && strings.HasPrefix(t.CodecID, "A_AAC") {
				s.CodecName = "aac"
			}
			if s.CodecName == "" {
				s.CodecName = strings.ToLower(t.CodecID)
			}
			switch s.CodecName {
			case "hevc":
				if c, err := fmp4.HEVC(t.CodecPrivate, t.Width, t.Height, "hvc1"); err == nil {
					s.Profile = c.Profile
				}
			case "h264":
				if c, err := fmp4.H264(t.CodecPrivate, t.Width, t.Height); err == nil {
					s.Profile = c.Profile
				}
			case "aac":
				if c, err := fmp4.AAC(t.CodecPrivate, t.Channels); err == nil {
					s.Profile = c.Profile
				}
			}
			p.Streams = append(p.Streams, s)
		}
		p.Attachments = f.Attachments
		return p, nil
	}
	f, err := mp4.Read(fh, st.Size())
	if err != nil {
		return nil, err
	}
	p.Duration = f.Duration
	for i, t := range f.Tracks {
		s := Stream{Index: i, Language: t.Language, Default: t.Enabled, Width: t.Width, Height: t.Height, CodecName: mp4Codecs[t.Format]}
		s.CodecType = map[string]string{"vide": "video", "soun": "audio", "text": "subtitle", "subt": "subtitle", "sbtl": "subtitle"}[t.Handler]
		if s.CodecType == "" {
			continue
		}
		if s.CodecName == "" {
			s.CodecName = strings.TrimSpace(strings.ToLower(t.Format))
		}
		switch s.CodecName {
		case "hevc":
			if c, err := fmp4.HEVC(t.Config["hvcC"], t.Width, t.Height, "hvc1"); err == nil {
				s.Profile = c.Profile
			}
		case "h264":
			if c, err := fmp4.H264(t.Config["avcC"], t.Width, t.Height); err == nil {
				s.Profile = c.Profile
			}
		}
		p.Streams = append(p.Streams, s)
	}
	return p, nil
}

// MaxAttachment bounds one attachment read into memory.
const MaxAttachment = 1 << 30

// Extract returns every jusplay attachment in path by file name, legacy
// names under their current ones (Canonical). Duplicate names are an error
// (v0 allows one snapshot per file).
func Extract(path string) (map[string][]byte, error) {
	f, c, err := matroska.Open(path)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	out := map[string][]byte{}
	for _, a := range f.Attachments {
		if !isOurs(a.Name) {
			continue
		}
		if _, dup := out[Canonical(a.Name)]; dup {
			return nil, fmt.Errorf("duplicate attachment %s", a.Name)
		}
		if filepath.Base(a.Name) != a.Name {
			return nil, fmt.Errorf("bad attachment name %q", a.Name)
		}
		b, err := f.AttachmentData(a, MaxAttachment)
		if err != nil {
			return nil, err
		}
		out[Canonical(a.Name)] = b
	}
	return out, nil
}

// Verification records what was compared.
type Verification = matroska.Verification

// Attach writes out = in + attachments without re-encoding. The output is
// written to out+".partial", verified, and only then renamed. in is never
// modified; out must not exist; in must not have jusplay attachments.
func Attach(in, out string, atts []Attachment) (*Verification, error) {
	return attach(in, out, atts, false)
}

// Replace is Attach, except that jusplay attachments already in in are
// dropped and replaced (other attachments, e.g. fonts, are kept).
func Replace(in, out string, atts []Attachment) (*Verification, error) {
	return attach(in, out, atts, true)
}

func attach(in, out string, atts []Attachment, replace bool) (*Verification, error) {
	absIn, _ := filepath.Abs(in)
	absOut, _ := filepath.Abs(out)
	if strings.EqualFold(absIn, absOut) {
		return nil, errors.New("output must differ from input")
	}
	if _, err := os.Stat(out); err == nil {
		return nil, fmt.Errorf("%s already exists", out)
	}
	partial := out + ".partial"
	if _, err := os.Stat(partial); err == nil {
		return nil, fmt.Errorf("%s already exists (left over from a failed run?)", partial)
	}
	f, c, err := matroska.Open(in)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	if f.DocType != "matroska" {
		return nil, fmt.Errorf("%s: attachments need a Matroska file, not %s", filepath.Base(in), f.DocType)
	}
	for _, a := range f.Attachments {
		if isOurs(a.Name) && !replace {
			return nil, fmt.Errorf("input already has %s attachment %q", Prefix, a.Name)
		}
	}
	var add []matroska.NewFile
	for _, a := range atts {
		if !strings.HasPrefix(a.Name, Prefix) || filepath.Base(a.Name) != a.Name {
			return nil, fmt.Errorf("bad attachment name %q", a.Name)
		}
		add = append(add, matroska.NewFile{Name: a.Name, MIME: a.MIME, Data: a.Data})
	}
	keep := func(a matroska.Attachment) bool { return !isOurs(a.Name) }
	if err := f.WriteFile(partial, keep, add); err != nil {
		os.Remove(partial)
		return nil, err
	}
	c.Close()
	v, err := matroska.Verify(in, partial, keep, add)
	if err != nil {
		os.Remove(partial)
		return v, err
	}
	if err := os.Rename(partial, out); err != nil {
		os.Remove(partial)
		return v, err
	}
	return v, nil
}

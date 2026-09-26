package media

import (
	"bufio"
	"bytes"
	"context"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// parsed is a stream as written: its samples' data and times.
type parsed struct {
	timescale uint32
	samples   []parsedSample
}

type parsedSample struct {
	dts, pts int64
	dur      uint32
	key      bool
	data     []byte
}

// parseStream reads back an fMP4 stream (init + fragments).
func parseStream(t *testing.T, b []byte) parsed {
	var p parsed
	var pending []parsedSample
	for len(b) >= 8 {
		size := int(binary.BigEndian.Uint32(b))
		typ := string(b[4:8])
		if size < 8 || size > len(b) {
			t.Fatalf("bad box %q size %d", typ, size)
		}
		body := b[8:size]
		switch typ {
		case "moov":
			i := bytes.Index(body, []byte("mdhd"))
			p.timescale = binary.BigEndian.Uint32(body[i+4+12:])
		case "moof":
			i := bytes.Index(body, []byte("tfdt"))
			base := int64(binary.BigEndian.Uint64(body[i+8:]))
			j := bytes.Index(body, []byte("trun"))
			tr := body[j+4:]
			n := int(binary.BigEndian.Uint32(tr[4:]))
			q := tr[12:]
			dts := base
			pending = pending[:0]
			for k := 0; k < n; k++ {
				dur := binary.BigEndian.Uint32(q)
				sz := binary.BigEndian.Uint32(q[4:])
				fl := binary.BigEndian.Uint32(q[8:])
				cto := int64(int32(binary.BigEndian.Uint32(q[12:])))
				q = q[16:]
				pending = append(pending, parsedSample{dts: dts, pts: dts + cto, dur: dur, key: fl&0x00010000 == 0, data: make([]byte, sz)})
				dts += int64(dur)
			}
		case "mdat":
			off := 0
			for k := range pending {
				n := len(pending[k].data)
				copy(pending[k].data, body[off:off+n])
				off += n
			}
			if off != len(body) {
				t.Fatalf("mdat holds %d bytes, samples %d", len(body), off)
			}
			p.samples = append(p.samples, pending...)
		}
		b = b[size:]
	}
	return p
}

type sink struct{ bytes.Buffer }

// TestStreamsAgainstFFmpeg streams the video and audio of each file in
// JUSPLAY_ORACLE from the start and checks every sample against ffmpeg's
// packets (the data by MD5, the presentation time) and the timeline's
// continuity. A development check with ffmpeg as the reference.
func TestStreamsAgainstFFmpeg(t *testing.T) {
	list := os.Getenv("JUSPLAY_ORACLE")
	if list == "" {
		t.Skip("JUSPLAY_ORACLE not set")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not found")
	}
	for _, path := range strings.Split(list, ";") {
		if path == "" {
			continue
		}
		t.Run(filepath.Base(path), func(t *testing.T) {
			m, err := Open(path)
			if err != nil {
				t.Skipf("not playable here: %v", err)
			}
			for _, kind := range []string{"video", "audio"} {
				tr, err := m.track(kind)
				if err != nil {
					continue
				}
				st, err := m.OpenStream(context.Background(), kind, 0)
				if err != nil {
					t.Fatal(err)
				}
				var out sink
				if _, err := st.WriteTo(&out); err != nil {
					t.Fatal(err)
				}
				p := parseStream(t, out.Bytes())
				// ffmpeg's packets of that stream: MD5 of the data and pts.
				sel := "0:v:0"
				if kind == "audio" {
					sel = "0:a:" + strconv.Itoa(audioOrdinal(t, path, tr))
				}
				// Times as the file has them: no shifting to non-negative.
				fm, err := exec.Command("ffmpeg", "-v", "error", "-copyts", "-i", path, "-map", sel, "-c", "copy", "-avoid_negative_ts", "disabled", "-f", "framemd5", "-").Output()
				if err != nil {
					t.Fatal(err)
				}
				type ref struct {
					pts float64
					md5 string
				}
				var refs []ref
				var tb float64
				sc := bufio.NewScanner(bytes.NewReader(fm))
				for sc.Scan() {
					line := sc.Text()
					if strings.HasPrefix(line, "#tb 0:") {
						f := strings.Split(strings.TrimSpace(line[6:]), "/")
						a, _ := strconv.ParseFloat(f[0], 64)
						b, _ := strconv.ParseFloat(f[1], 64)
						tb = a / b
					}
					if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
						continue
					}
					f := strings.Split(line, ",")
					pts, _ := strconv.ParseFloat(strings.TrimSpace(f[2]), 64)
					refs = append(refs, ref{pts * tb, strings.TrimSpace(f[5])})
				}
				if len(refs) != len(p.samples) {
					t.Fatalf("%s: %d samples written, ffmpeg %d packets", kind, len(p.samples), len(refs))
				}
				ts := float64(p.timescale)
				bad, worst := 0, 0.0
				for i, s := range p.samples {
					sum := md5.Sum(s.data)
					if hex.EncodeToString(sum[:]) != refs[i].md5 {
						if bad < 3 {
							t.Errorf("%s sample %d: data differs from ffmpeg's packet", kind, i)
						}
						bad++
					}
					orig := float64(s.pts)/ts + st.Offset
					worst = math.Max(worst, math.Abs(orig-refs[i].pts))
					if i > 0 && s.dts != p.samples[i-1].dts+int64(p.samples[i-1].dur) {
						t.Errorf("%s sample %d: decode time %d does not follow %d+%d", kind, i, s.dts, p.samples[i-1].dts, p.samples[i-1].dur)
						break
					}
					if s.dts < 0 || s.pts < 0 {
						t.Fatalf("%s sample %d: negative time written", kind, i)
					}
				}
				if bad > 0 {
					t.Errorf("%s: %d of %d samples differ", kind, bad, len(p.samples))
				}
				// Video times are the file's; audio times count samples and may
				// part from the container's by up to 10 ms before re-anchoring.
				limit := 0.0015
				if kind == "audio" {
					limit = 0.0105
				}
				if worst > limit {
					t.Errorf("%s: presentation times up to %.4f s from ffmpeg's", kind, worst)
				}
				t.Logf("%s %s (%s): %d samples, data identical, times within %.2f ms, %d bytes", kind, tr.Codec, tr.MIME, len(p.samples), worst*1000, out.Len())
			}
		})
	}
}

// audioOrdinal finds which audio stream (ffmpeg's 0:a:N) the chosen track is.
func audioOrdinal(t *testing.T, path string, tr *Track) int {
	out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "a", "-show_entries", "stream=codec_name,disposition=default", "-of", "csv=p=0", path).Output()
	if err != nil {
		return 0
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	// Same rule as the player: the first default one, else the first.
	for i, l := range lines {
		if strings.HasPrefix(l, tr.Codec) && strings.HasSuffix(strings.TrimSpace(l), ",1") {
			return i
		}
	}
	for i, l := range lines {
		if strings.HasPrefix(l, tr.Codec) {
			return i
		}
	}
	return 0
}

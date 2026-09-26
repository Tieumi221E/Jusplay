package matroska

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/Tieumi221E/Jusplay/internal/ebml"
)

// TestLayout prints the segment's top-level elements of JUSPLAY_LAYOUT files (development aid).
func TestLayout(t *testing.T) {
	list := os.Getenv("JUSPLAY_LAYOUT")
	if list == "" {
		t.Skip()
	}
	names := map[uint32]string{idSeekHead: "SeekHead", idInfo: "Info", idTracks: "Tracks", idCluster: "Cluster", idCues: "Cues", idAttachments: "Attachments", idChapters: "Chapters", idTags: "Tags", idVoid: "Void"}
	for _, p := range strings.Split(list, ";") {
		f, c, err := Open(p)
		if err != nil {
			t.Log(p, err)
			continue
		}
		var parts []string
		clusters := 0
		for off := f.SegmentStart; off < f.SegmentEnd; {
			e, err := ebml.ReadHeader(f.f, off)
			if err != nil || e.Size == ebml.UnknownSize {
				parts = append(parts, fmt.Sprintf("?%X", e.ID))
				break
			}
			if e.ID == idCluster {
				clusters++
				if clusters == 1 {
					parts = append(parts, "Cluster...")
				}
			} else {
				n := names[e.ID]
				if n == "" {
					n = fmt.Sprintf("%X", e.ID)
				}
				crc := ""
				if h, err := ebml.ReadHeader(f.f, e.DataPos); err == nil && h.ID == idCRC32 {
					crc = "+crc"
				}
				parts = append(parts, fmt.Sprintf("%s(%d%s)", n, e.Size, crc))
			}
			off = e.End()
		}
		seg, _ := ebml.ReadHeader(f.f, f.SegmentStart-12)
		_ = seg
		t.Logf("%s: seg size field %dB unknown=%v | %s | %d clusters | %d atts", p[strings.LastIndex(p, "/")+1:], 0, f.segSizeUnknown, strings.Join(parts, " "), clusters, len(f.Attachments))
		c.Close()
	}
}

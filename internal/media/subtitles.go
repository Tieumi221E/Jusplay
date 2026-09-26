package media

import (
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/Tieumi221E/Jusplay/internal/matroska"
)

// SubtitleTrack is a subtitle track in the file.
type SubtitleTrack struct {
	Number   uint64 `json:"number"`
	Codec    string `json:"codec"` // Matroska codec ID, e.g. "S_TEXT/ASS"
	Language string `json:"language,omitempty"`
	Name     string `json:"name,omitempty"`
	Default  bool   `json:"default,omitempty"`
	// Text is false for picture subtitles (PGS, VobSub), which the page
	// cannot draw yet.
	Text bool `json:"text"`
}

// textSubtitle are the text subtitle codecs the page reads.
var textSubtitle = map[string]bool{"S_TEXT/UTF8": true, "S_TEXT/ASS": true, "S_TEXT/SSA": true, "S_TEXT/WEBVTT": true}

// SubtitleEvent is one subtitle block: its time and its data as stored
// (for ASS/SSA the event line after Start/End: "ReadOrder,Layer,Style,…,Text").
type SubtitleEvent struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Data  string  `json:"data"`
}

// SubtitleEvents reads a text subtitle track of the file: the codec's header
// (ASS/SSA: the script info and styles) and every event in time order.
// It walks the file's clusters once, so it is done when the track is
// chosen, not when the video opens.
func (m *Media) SubtitleEvents(number uint64) (header string, events []SubtitleEvent, err error) {
	var st *SubtitleTrack
	for i := range m.Subtitles {
		if m.Subtitles[i].Number == number {
			st = &m.Subtitles[i]
		}
	}
	if st == nil {
		return "", nil, fmt.Errorf("no subtitle track %d", number)
	}
	if !st.Text {
		return "", nil, fmt.Errorf("%w: picture subtitles (%s)", ErrUnsupported, st.Codec)
	}
	fh, err := os.Open(m.Path)
	if err != nil {
		return "", nil, err
	}
	defer fh.Close()
	fi, err := fh.Stat()
	if err != nil {
		return "", nil, err
	}
	f, err := matroska.Read(fh, fi.Size())
	if err != nil {
		return "", nil, err
	}
	t := f.Track(number)
	if t == nil {
		return "", nil, errors.New("track gone")
	}
	frames, err := f.Frames(map[uint64]bool{number: true})
	if err != nil && len(frames) == 0 {
		return "", nil, err
	}
	var buf []byte
	for _, fr := range frames {
		b, err := f.FrameData(t, fr, buf[:0])
		if err != nil {
			return "", nil, err
		}
		buf = b
		start := float64(fr.PTS) / 1e9
		events = append(events, SubtitleEvent{Start: start, End: start + float64(fr.Dur)/1e9, Data: string(b)})
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].Start < events[j].Start })
	return string(t.CodecPrivate), events, nil
}

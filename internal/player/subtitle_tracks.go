package player

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Tieumi221E/Jus/capreg"
	"github.com/Tieumi221E/Jusplay/internal/library"
	"github.com/Tieumi221E/Jusplay/internal/media"
	"github.com/Tieumi221E/Jusplay/internal/subs"
)

// The subtitle tracks a video has: files beside it, one picked by hand,
// the file's own text tracks, and (with the AI component) the lines made
// from its sound. They are read here (SubtitleLines) and drawn by the page:
//
//	GET  api/subs/tracks?id=            what there is, and what was chosen
//	POST api/cap/subs.show              a track's lines (SubtitleLines), read here for the page and the command line
//	POST api/cap/subs.pick              the choice, kept in the folder records

// maxSubFile bounds a subtitle file read (the longest real ones are a few MB).
const maxSubFile = 32 << 20

type embeddedTrack struct {
	media.SubtitleTrack
	Lang string `json:"lang"` // normalised like the files' ("zh-Hans", "ja", …)
}

func (s *Server) subtitleTracks(w http.ResponseWriter, e library.Entry) {
	writeJSON(w, s.SubtitleTracks(e))
}

// SubtitleTracks is what subtitles e has, and what was chosen.
func (s *Server) SubtitleTracks(e library.Entry) map[string]any {
	out := map[string]any{"files": subs.Beside(e.Path), "pick": e.SubPick, "pick2": e.SubPick2}
	if e.SubFile != "" {
		f := subs.File{Name: filepath.Base(e.SubFile), Format: subs.Formats[strings.ToLower(filepath.Ext(e.SubFile))]}
		f.Lang = subs.LangOfTag(strings.TrimSuffix(f.Name, filepath.Ext(f.Name)))
		out["manual"] = f
	}
	var emb []embeddedTrack
	if sess, err := s.Session(e.ID); err == nil {
		for _, t := range sess.Media.Subtitles {
			emb = append(emb, embeddedTrack{t, subs.LangOfTag(t.Language)})
		}
	}
	out["embedded"] = emb
	out["ai"] = s.aiStatus()
	return out
}

// SubtitleLines is a track of e as timed lines: key is file:<name>,
// manual, mkv:<n>, ai:src (the lines made from the sound) or ai:tr (their
// translation into lang). The player page shows these very lines.
func (s *Server) SubtitleLines(e library.Entry, key, lang string) ([]subs.Line, error) {
	switch {
	case strings.HasPrefix(key, "mkv:"):
		n, err := strconv.ParseUint(strings.TrimPrefix(key, "mkv:"), 10, 64)
		if err != nil {
			return nil, capreg.Usagef("no track %q", key)
		}
		sess, err := s.Session(e.ID)
		if err != nil {
			return nil, err
		}
		_, ev, err := sess.Media.SubtitleEvents(n)
		if err != nil {
			return nil, err
		}
		codec := ""
		for _, t := range sess.Media.Subtitles {
			if t.Number == n {
				codec = t.Codec
			}
		}
		events := make([]subs.Event, len(ev))
		for i, x := range ev {
			events[i] = subs.Event{Start: x.Start, End: x.End, Data: x.Data}
		}
		return subs.FromEmbedded(codec, events), nil
	case key == "ai:src" || key == "ai:tr":
		t, err := subs.Load(s.subsPath(e))
		if err != nil {
			return nil, err
		}
		out := []subs.Line{}
		for _, c := range t.Cues {
			text := c.Text
			if key == "ai:tr" {
				if text = c.Tr[lang]; text == "" {
					continue
				}
			}
			out = append(out, subs.Line{Start: c.Start, End: c.End, Text: text})
		}
		return out, nil
	}
	path, format := s.subtitlePath(e, key)
	if path == "" {
		return nil, capreg.NotFoundf("no subtitle track %q for this video (subs tracks lists them)", key)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxSubFile))
	if err != nil {
		return nil, err
	}
	return subs.ParseFile(b, format), nil
}

// subtitlePath is the file a key names — one beside the video, or the one
// picked by hand, never any other path — and its format.
func (s *Server) subtitlePath(e library.Entry, key string) (path, format string) {
	switch {
	case key == "manual" && e.SubFile != "":
		return e.SubFile, subs.Formats[strings.ToLower(filepath.Ext(e.SubFile))]
	case strings.HasPrefix(key, "file:"):
		name := strings.TrimPrefix(key, "file:")
		for _, f := range subs.Beside(e.Path) {
			if f.Name == name {
				return filepath.Join(filepath.Dir(e.Path), name), f.Format
			}
		}
	}
	return "", ""
}

package player

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Tieumi221E/Jusplay/internal/library"
	"github.com/Tieumi221E/Jusplay/internal/media"
	"github.com/Tieumi221E/Jusplay/internal/subs"
)

// The subtitle tracks a video has: files beside it, one picked by hand,
// the file's own text tracks, and (with the AI component) the lines made
// from its sound. The page parses and draws them all (subformats.ts):
//
//	GET  api/subs/tracks?id=            what there is, and what was chosen
//	GET  api/subs/file?id=&key=         a subtitle file's bytes ("file:<name>" or "manual")
//	GET  api/subs/embedded?id=&track=   a Matroska text track: codec header and events
//	POST api/subs/pick                  {"id", "pick", "pick2", "file"}: the choice, kept in the folder records

// maxSubFile bounds a subtitle file read (the longest real ones are a few MB).
const maxSubFile = 32 << 20

type embeddedTrack struct {
	media.SubtitleTrack
	Lang string `json:"lang"` // normalised like the files' ("zh-Hans", "ja", …)
}

func (s *Server) subtitleTracks(w http.ResponseWriter, e library.Entry) {
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
	writeJSON(w, out)
}

// subtitleFile serves a file beside the video, or the one picked by hand:
// never any other path, whatever the key says.
func (s *Server) subtitleFile(w http.ResponseWriter, e library.Entry, key string) {
	var path string
	switch {
	case key == "manual" && e.SubFile != "":
		path = e.SubFile
	case strings.HasPrefix(key, "file:"):
		name := strings.TrimPrefix(key, "file:")
		for _, f := range subs.Beside(e.Path) {
			if f.Name == name {
				path = filepath.Join(filepath.Dir(e.Path), name)
			}
		}
	}
	if path == "" {
		http.Error(w, "no such subtitle file", http.StatusNotFound)
		return
	}
	fh, err := os.Open(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	defer fh.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	io.Copy(w, io.LimitReader(fh, maxSubFile))
}

func (s *Server) subtitleEmbedded(w http.ResponseWriter, e library.Entry, track string) {
	n, err := strconv.ParseUint(track, 10, 64)
	sess, err2 := s.Session(e.ID)
	if err != nil || err2 != nil {
		http.Error(w, "bad track", http.StatusBadRequest)
		return
	}
	head, ev, err := sess.Media.SubtitleEvents(n)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	codec := ""
	for _, t := range sess.Media.Subtitles {
		if t.Number == n {
			codec = t.Codec
		}
	}
	writeJSON(w, map[string]any{"codec": codec, "header": head, "events": ev})
}

func (s *Server) subtitlePick(w http.ResponseWriter, r *http.Request) {
	var req struct{ ID, Pick, Pick2, File string }
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.File != "" {
		if _, ok := subs.Formats[strings.ToLower(filepath.Ext(req.File))]; !ok || !filepath.IsAbs(req.File) {
			http.Error(w, "not a subtitle file (.srt, .ass, .ssa, .vtt)", http.StatusUnprocessableEntity)
			return
		}
		if _, err := os.Stat(req.File); err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
	}
	err := s.lib.Update(req.ID, func(e *library.Entry) {
		e.SubPick, e.SubPick2 = req.Pick, req.Pick2
		if req.File != "" {
			e.SubFile = req.File
		}
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

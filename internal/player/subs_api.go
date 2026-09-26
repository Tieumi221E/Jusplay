package player

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/Tieumi221E/Jusplay/internal/ai"
	"github.com/Tieumi221E/Jusplay/internal/library"
	"github.com/Tieumi221E/Jusplay/internal/subs"
)

// Subtitles made from the video's sound (package subs), when the optional
// AI component is there (SetAI). The page decodes the audio, since the
// server has no decoder:
//
//	GET  api/subs?id=               status, models and the lines made so far
//	GET  api/subs/audio?id=&t0=&t1= the audio from t0 to t1 as fragmented MP4
//	POST api/subs/warm?id=          start the models (subtitles were turned on)
//	POST api/subs/chunk?id=&start=&target=&lang=&last=  body: 16 kHz mono s16le PCM
//	POST api/subs/translate?id=&from=&to=&target=
//
// A chunk or translation request replaces the one before it for the same
// video (its context is cancelled): after a seek only the new place matters.

type subsState struct {
	mu     sync.Mutex
	e      *ai.Engine
	maker  *subs.Maker
	cancel context.CancelFunc
}

// SetAI gives the server the AI engine (nil: no AI subtitles at all).
func (s *Server) SetAI(e *ai.Engine) {
	s.ai.mu.Lock()
	defer s.ai.mu.Unlock()
	s.ai.e = e
	if e != nil {
		s.ai.maker = &subs.Maker{C: e}
	}
}

func (s *Server) closeAI() {
	s.ai.mu.Lock()
	e := s.ai.e
	if s.ai.cancel != nil {
		s.ai.cancel()
	}
	s.ai.mu.Unlock()
	if e != nil {
		e.Close()
	}
}

// aiStatus is what the page needs to know about the AI backends.
func (s *Server) aiStatus() map[string]any {
	s.ai.mu.Lock()
	e := s.ai.e
	s.ai.mu.Unlock()
	if e == nil {
		return map[string]any{"available": false, "reason": "no AI support in this build"}
	}
	asr, mt := e.Status()
	why := asr.Why
	if why == "" {
		why = mt.Why
	}
	return map[string]any{"available": asr.Ready, "translate": mt.Ready, "reason": why, "asr": asr, "mt": mt,
		"models": map[string]string{"asr": asr.Name, "mt": mt.Name}}
}

// subsPath is where e's subtitles are kept: its folder's records, or the
// temporary folder for a file outside every media folder.
func (s *Server) subsPath(e library.Entry) string {
	d := s.cacheDir(e, "subtitles")
	if d == "" {
		return ""
	}
	return filepath.Join(d, e.ID+".json")
}

// begin cancels the running chunk or translation and returns the new one's context.
func (s *Server) begin(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)
	s.ai.mu.Lock()
	if s.ai.cancel != nil {
		s.ai.cancel()
	}
	s.ai.cancel = cancel
	s.ai.mu.Unlock()
	return ctx, cancel
}

func (s *Server) subsAPI(w http.ResponseWriter, r *http.Request, p string) {
	q := r.URL.Query()
	if p == "api/subs/pick" && r.Method == http.MethodPost {
		s.subtitlePick(w, r)
		return
	}
	e, ok := s.lib.Get(q.Get("id"))
	if !ok {
		http.Error(w, "no such entry", http.StatusNotFound)
		return
	}
	s.ai.mu.Lock()
	eng, maker := s.ai.e, s.ai.maker
	s.ai.mu.Unlock()
	num := func(k string) float64 {
		v, _ := strconv.ParseFloat(q.Get(k), 64)
		return v
	}
	switch {
	case p == "api/subs/tracks" && r.Method == http.MethodGet:
		s.subtitleTracks(w, e)
	case p == "api/subs/file" && r.Method == http.MethodGet:
		s.subtitleFile(w, e, q.Get("key"))
	case p == "api/subs/embedded" && r.Method == http.MethodGet:
		s.subtitleEmbedded(w, e, q.Get("track"))
	case p == "api/subs" && r.Method == http.MethodGet:
		t, err := subs.Load(s.subsPath(e))
		out := s.aiStatus()
		out["track"] = t
		if err != nil {
			out["error"] = err.Error()
		}
		writeJSON(w, out)
	case p == "api/subs/warm" && r.Method == http.MethodPost:
		// The page is about to want lines: load the models now.
		if eng != nil {
			eng.Warm()
		}
		w.WriteHeader(http.StatusNoContent)
	case p == "api/subs/audio" && r.Method == http.MethodGet:
		sess, err := s.Session(e.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		st, err := sess.Media.OpenAudioRange(r.Context(), num("t0"), num("t1"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			return
		}
		w.Header().Set("Content-Type", "audio/mp4")
		w.Header().Set("X-Jusplay-Start", strconv.FormatFloat(st.Start, 'f', 6, 64))
		st.WriteTo(w)
	case p == "api/subs/chunk" && r.Method == http.MethodPost:
		if maker == nil {
			http.Error(w, "no AI support", http.StatusServiceUnavailable)
			return
		}
		// 16 kHz × 2 bytes: at most 2 minutes per chunk.
		b, err := io.ReadAll(io.LimitReader(r.Body, 2*60*subs.Rate*2+1))
		if err != nil || len(b)%2 != 0 || len(b) > 2*60*subs.Rate*2 {
			http.Error(w, "bad audio", http.StatusBadRequest)
			return
		}
		pcm := make([]int16, len(b)/2)
		binary.Decode(b, binary.LittleEndian, pcm)
		ctx, cancel := s.begin(r.Context())
		defer cancel()
		t0 := time.Now()
		res, err := maker.Chunk(ctx, s.subsPath(e), pcm, num("start"), q.Get("target"), q.Get("lang"), q.Get("last") == "1")
		if err != nil {
			s.subsError(w, err)
			return
		}
		s.logf("subs %s: %.1f s from %.1f, %d lines, in %s", e.ID, float64(len(pcm))/subs.Rate, num("start"), len(res.Cues), time.Since(t0).Round(time.Millisecond))
		writeJSON(w, res)
	case p == "api/subs/translate" && r.Method == http.MethodPost:
		if maker == nil {
			http.Error(w, "no AI support", http.StatusServiceUnavailable)
			return
		}
		ctx, cancel := s.begin(r.Context())
		defer cancel()
		cues, err := maker.Translate(ctx, s.subsPath(e), num("from"), num("to"), q.Get("target"))
		if err != nil {
			s.subsError(w, err)
			return
		}
		writeJSON(w, map[string]any{"cues": cues})
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (s *Server) subsError(w http.ResponseWriter, err error) {
	if errors.Is(err, context.Canceled) {
		http.Error(w, "cancelled", http.StatusConflict)
		return
	}
	s.logf("subs: %v", err)
	b, _ := json.Marshal(err.Error())
	http.Error(w, string(b), http.StatusInternalServerError)
}
